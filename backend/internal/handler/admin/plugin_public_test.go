package admin

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type publicWebsiteRepo struct {
	service.PluginRepository
	installation *service.PluginInstallation
	listErr      error
}

func (r *publicWebsiteRepo) List(context.Context) ([]*service.PluginInstallation, error) {
	return []*service.PluginInstallation{r.installation}, r.listErr
}

func (r *publicWebsiteRepo) GetByID(_ context.Context, id int64) (*service.PluginInstallation, error) {
	if r.installation == nil || id != r.installation.ID {
		return nil, sql.ErrNoRows
	}
	copy := *r.installation
	copy.Bindings = append([]service.PluginBinding(nil), r.installation.Bindings...)
	return &copy, nil
}

type publicWebsiteEncryptor struct{}

func (publicWebsiteEncryptor) Encrypt(raw string) (string, error) { return "ENC:" + raw, nil }
func (publicWebsiteEncryptor) Decrypt(raw string) (string, error) {
	return strings.TrimPrefix(raw, "ENC:"), nil
}

func publicWebsiteRouter(t *testing.T) (*gin.Engine, *publicWebsiteRepo) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	installPath := filepath.Join(root, "installed", "support")
	files := map[string][]byte{"ui/index.html": []byte("private configuration UI"), "public/index.html": []byte("<!doctype html><title>Support</title>")}
	manifest := service.PluginManifest{
		SchemaVersion: 1, ID: "com.example.public-support", Name: "Support", Version: "1.0.0",
		Requires:     service.PluginRequirements{Sub2API: "=2.23.0-apitoken.3", PluginProtocol: 1, TransportAPI: 1, UIBridge: 1},
		Capabilities: []service.PluginCapability{{ID: service.PluginCapabilitySupportWidget, Platform: "website", AccountType: "none"}},
		UI:           service.PluginUIManifest{Entrypoint: "ui/index.html"}, PublicUI: &service.PluginUIManifest{Entrypoint: "public/index.html"},
		Files: make(map[string]string),
	}
	for name, body := range files {
		digest := sha256.Sum256(body)
		manifest.Files[name] = hex.EncodeToString(digest[:])
		path := filepath.Join(installPath, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, body, 0o600))
	}
	repo := &publicWebsiteRepo{installation: &service.PluginInstallation{
		ID: 7, PluginKey: manifest.ID, Name: manifest.Name, Version: manifest.Version, Manifest: manifest,
		InstallPath: installPath, BinarySHA256: manifest.StaticIdentity(), State: service.PluginStateEnabled,
		SignatureStatus: service.PluginSignatureTrusted, ConfigEncrypted: `ENC:{"service_contact":"154215395","qq_group_number":"154895805"}`,
		Bindings: []service.PluginBinding{{Capability: service.PluginCapabilitySupportWidget, Platform: "website", AccountType: "none", Enabled: true, RolloutPercent: 100}},
	}}
	manager := service.NewPluginManager(repo, publicWebsiteEncryptor{}, &config.Config{Plugins: config.PluginConfig{DataDir: root}}, service.PluginHostInfo{Version: "2.23.0-apitoken.3"}, nil)
	handler := NewPluginHandler(manager)
	router := gin.New()
	router.GET("/api/v1/plugins/public", handler.ListPublic)
	router.GET("/api/v1/plugins/public/:id/assets/:hash/*path", handler.ServePublicAsset)
	return router, repo
}

func TestPublicWebsitePluginDescriptorContainsOnlyPublicConfiguration(t *testing.T) {
	router, repo := publicWebsiteRouter(t)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/plugins/public", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	var body struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
	require.Len(t, body.Data, 1)
	require.Len(t, body.Data[0], 6)
	for _, key := range []string{"id", "plugin_key", "name", "version", "entrypoint_url", "config"} {
		require.Contains(t, body.Data[0], key)
	}
	var cfg service.SupportWidgetConfig
	require.NoError(t, json.Unmarshal(body.Data[0]["config"], &cfg))
	require.True(t, cfg.Enabled)
	require.Equal(t, "154215395", cfg.ServiceContact)
	require.NotContains(t, recorder.Body.String(), "ENC:")
	require.NotContains(t, recorder.Body.String(), repo.installation.InstallPath)
	var entrypoint string
	require.NoError(t, json.Unmarshal(body.Data[0]["entrypoint_url"], &entrypoint))
	require.Equal(t, "/api/v1/plugins/public/7/assets/"+repo.installation.BinarySHA256+"/public/index.html", entrypoint)
	asset := httptest.NewRecorder()
	router.ServeHTTP(asset, httptest.NewRequest(http.MethodGet, entrypoint, nil))
	require.Equal(t, http.StatusOK, asset.Code, "the exact descriptor URL must load its packaged widget")
	require.Contains(t, asset.Body.String(), "<title>Support</title>")

	repo.installation.State = service.PluginStateDisabled
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/plugins/public", nil))
	require.JSONEq(t, `{"code":0,"message":"success","data":[]}`, recorder.Body.String())
}

func TestPublicWebsitePluginAssetsAreSandboxedAndPrivatePathsFailClosed(t *testing.T) {
	router, repo := publicWebsiteRouter(t)
	base := "/api/v1/plugins/public/7/assets/" + repo.installation.BinarySHA256 + "/"
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, base+"public/index.html", nil))
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Contains(t, recorder.Header().Get("Content-Type"), "text/html")
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	csp := recorder.Header().Get("Content-Security-Policy")
	for _, directive := range []string{"sandbox allow-scripts;", "connect-src 'none'", "form-action 'none'", "frame-ancestors 'self'"} {
		require.Contains(t, csp, directive)
	}
	require.NotContains(t, csp, "allow-same-origin")
	require.NotContains(t, csp, "allow-popups")
	for _, path := range []string{"ui/index.html", "../ui/index.html", "public/../ui/index.html", "manifest.json", "signature.json", "native.exe", "index.html"} {
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, base+path, nil))
		require.Equal(t, http.StatusNotFound, recorder.Code, path)
		require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	}
	for _, target := range []string{strings.Replace(base, repo.installation.BinarySHA256, strings.Repeat("a", 64), 1) + "public/index.html", base + "public/index.html"} {
		if target == base+"public/index.html" {
			repo.installation.Bindings[0].Enabled = false
		}
		recorder = httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
		require.Equal(t, http.StatusNotFound, recorder.Code)
	}
}

func TestPublicWebsitePluginReadFailureDoesNotLeakInternals(t *testing.T) {
	router, repo := publicWebsiteRouter(t)
	repo.listErr = errors.New("secret database connection detail")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/plugins/public", nil))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	require.NotContains(t, recorder.Body.String(), "secret database")
}
