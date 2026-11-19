package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const websiteHostVersion = "2.23.0-apitoken.3"

func websiteTestManifest() (PluginManifest, map[string][]byte) {
	files := map[string][]byte{
		"ui/index.html":     []byte("<!doctype html><title>Support settings</title>"),
		"public/index.html": []byte("<!doctype html><title>Support widget</title>"),
		"public/widget.js":  []byte("document.body.textContent='support'"),
	}
	manifest := testPluginManifest(files)
	manifest.ID, manifest.Name, manifest.Version = "com.example.website-support", "Website support", "1.0.0"
	manifest.Requires.Sub2API = "=" + websiteHostVersion
	manifest.Requires.TestedSub2APIVersions = []string{websiteHostVersion}
	manifest.Runtimes = nil
	manifest.Capabilities = []PluginCapability{{ID: PluginCapabilitySupportWidget, Platform: "website", AccountType: "none"}}
	manifest.PublicUI = &PluginUIManifest{Entrypoint: "public/index.html"}
	return manifest, files
}

func websiteTestArchive(t *testing.T, manifest PluginManifest, files map[string][]byte, key ed25519.PrivateKey, publisher ...string) []byte {
	t.Helper()
	raw, err := json.Marshal(manifest)
	require.NoError(t, err)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	writeZipEntry(t, writer, pluginManifestFilename, raw)
	for name, body := range files {
		writeZipEntry(t, writer, name, body)
	}
	if key != nil {
		keyID := "website-test"
		if len(publisher) > 0 {
			keyID = publisher[0]
		}
		signature, err := json.Marshal(PluginSignature{Algorithm: "ed25519", KeyID: keyID, Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, raw))})
		require.NoError(t, err)
		writeZipEntry(t, writer, pluginSignatureFilename, signature)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

func websiteTestManager(t *testing.T) (*PluginManager, *websiteMemoryRepo, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cfg := testPluginConfig(t.TempDir(), false)
	cfg.Plugins.TrustedPublishers["website-test"] = base64.StdEncoding.EncodeToString(pub)
	repo := &websiteMemoryRepo{items: make(map[int64]*PluginInstallation)}
	return NewPluginManager(repo, pluginTokenEncryptor{}, cfg, PluginHostInfo{Version: websiteHostVersion}, nil), repo, priv
}

func TestWebsitePluginPackageIsSignedStaticAndBoundToAllAssets(t *testing.T) {
	manager, _, key := websiteTestManager(t)
	manifest, files := websiteTestManifest()
	_, err := manager.Install(t.Context(), bytes.NewReader(websiteTestArchive(t, manifest, files, nil)), nil)
	require.ErrorContains(t, err, "不允许安装未签名插件")
	installed, err := manager.Install(t.Context(), bytes.NewReader(websiteTestArchive(t, manifest, files, key)), nil)
	require.NoError(t, err)
	require.Equal(t, PluginStateDisabled, installed.State)
	require.Empty(t, installed.BinaryPath)
	require.Equal(t, manifest.StaticIdentity(), installed.BinarySHA256)
	require.Equal(t, PluginSignatureTrusted, installed.SignatureStatus)
	info, err := os.Stat(filepath.Join(installed.InstallPath, "public/widget.js"))
	require.NoError(t, err)
	require.Zero(t, info.Mode()&0o111, "no native executable permissions")

	changed, changedFiles := websiteTestManifest()
	changedFiles["public/widget.js"] = []byte("changed public implementation")
	changed.Files = testPluginManifest(changedFiles).Files
	require.NotEqual(t, manifest.StaticIdentity(), changed.StaticIdentity(), "same-version public asset changes invalidate CAS/local caches")
}

func TestWebsitePluginRejectsMixedScopeNativeRuntimeAndNonPublicFiles(t *testing.T) {
	for name, mutate := range map[string]func(*PluginManifest){
		"mixed capabilities": func(m *PluginManifest) {
			m.Capabilities = append(m.Capabilities, PluginCapability{ID: PluginCapabilityOpenAIOAuthOutbound, Platform: PlatformOpenAI, AccountType: AccountTypeOAuth})
		},
		"account grant": func(m *PluginManifest) { m.Capabilities[0].AccountType = AccountTypeOAuth },
		"native runtime": func(m *PluginManifest) {
			m.Runtimes = map[string]PluginRuntime{m.RuntimeKey(): {Path: "public/widget.js"}}
		},
		"native file":            func(m *PluginManifest) { m.Files["public/native.exe"] = strings.Repeat("a", 64) },
		"private file":           func(m *PluginManifest) { m.Files["secrets/key.json"] = strings.Repeat("a", 64) },
		"traversal":              func(m *PluginManifest) { m.Files["public/../ui/key.json"] = strings.Repeat("a", 64) },
		"public points to admin": func(m *PluginManifest) { m.PublicUI.Entrypoint = "ui/index.html" },
		"missing public entry":   func(m *PluginManifest) { m.PublicUI = nil },
	} {
		t.Run(name, func(t *testing.T) {
			manifest, _ := websiteTestManifest()
			mutate(&manifest)
			require.Error(t, manifest.Validate())
		})
	}
	manifest := testPluginManifest(nil)
	manifest.PublicUI = &PluginUIManifest{Entrypoint: "public/index.html"}
	require.Error(t, manifest.Validate())
}

func TestWebsitePublisherPinCannotGrantOtherPluginIdentity(t *testing.T) {
	cfg := testPluginConfig(t.TempDir(), false)
	cfg.Plugins.TrustedPublishers[builtInSupportWidgetPublisherKeyID] = "must not override"
	require.Equal(t, builtInSupportWidgetPublisherKeyBase64, trustedPluginPublisherKey(cfg, builtInSupportWidgetPublisherKeyID, builtInSupportWidgetPluginID))
	require.Empty(t, trustedPluginPublisherKey(cfg, builtInSupportWidgetPublisherKeyID, builtInOpenAITransportPluginID))
	pub, err := base64.StdEncoding.DecodeString(builtInSupportWidgetPublisherKeyBase64)
	require.NoError(t, err)
	require.Len(t, pub, ed25519.PublicKeySize)
}

func TestWebsitePinnedPublisherRejectsOtherSignersIdentitiesAndNativeCapability(t *testing.T) {
	manager, _, key := websiteTestManager(t)
	manager.cfg.Plugins.TrustedPublishers[builtInSupportWidgetPublisherKeyID] = base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	manifest, files := websiteTestManifest()
	manifest.ID = builtInSupportWidgetPluginID
	_, err := manager.Install(t.Context(), bytes.NewReader(websiteTestArchive(t, manifest, files, key, builtInSupportWidgetPublisherKeyID)), nil)
	require.ErrorContains(t, err, "签名校验失败", "configuration cannot replace the pinned signing key")
	manifest.ID = "com.example.impostor"
	_, err = manager.Install(t.Context(), bytes.NewReader(websiteTestArchive(t, manifest, files, key, builtInSupportWidgetPublisherKeyID)), nil)
	require.ErrorContains(t, err, "密钥不受信任", "the pinned key grants only its one plugin ID")
	files = map[string][]byte{"bin/plugin": []byte("binary"), "ui/index.html": []byte("admin")}
	manifest = testPluginManifest(files)
	manifest.ID = builtInSupportWidgetPluginID
	_, err = manager.Install(t.Context(), bytes.NewReader(websiteTestArchive(t, manifest, files, key, builtInSupportWidgetPublisherKeyID)), nil)
	require.ErrorContains(t, err, "仅支持静态网站客服能力")
}

func TestWebsiteReleasePackageInstallsWithPinnedProductionSignature(t *testing.T) {
	packagePath := os.Getenv("SUB2API_TEST_WEBSITE_PLUGIN_PACKAGE")
	if packagePath == "" {
		t.Skip("provide SUB2API_TEST_WEBSITE_PLUGIN_PACKAGE to verify the signed release artifact")
	}
	archive, err := os.Open(packagePath)
	require.NoError(t, err)
	defer archive.Close()
	repo := &websiteMemoryRepo{items: make(map[int64]*PluginInstallation)}
	manager := NewPluginManager(repo, pluginTokenEncryptor{}, testPluginConfig(t.TempDir(), false), PluginHostInfo{Version: websiteReleaseHostVersion()}, nil)
	installed, err := manager.Install(t.Context(), archive, nil)
	require.NoError(t, err)
	require.Equal(t, builtInSupportWidgetPluginID, installed.PluginKey)
	require.True(t, EvaluatePluginCompatibility(installed.Manifest, PluginHostInfo{Version: "2.24.0-apitoken.1"}).Compatible)
	require.False(t, EvaluatePluginCompatibility(installed.Manifest, PluginHostInfo{Version: "3.0.0-apitoken.1"}).Compatible)
	require.Equal(t, PluginSignatureTrusted, installed.SignatureStatus)
	require.Empty(t, installed.BinaryPath)
	require.Equal(t, PluginStateDisabled, installed.State)
	require.True(t, installed.Compatibility.Compatible)
	acceptCandidate := websiteReleaseHostVersion() == os.Getenv("SUB2API_TEST_CANDIDATE_HOST_VERSION")
	if !acceptCandidate {
		require.True(t, installed.Compatibility.Tested)
	}
	_, err = manager.SaveConfig(t.Context(), installed.ID, []byte(`{"service_contact":"154215395","qq_group_number":"154895805"}`))
	require.NoError(t, err)
	configBefore := repo.items[installed.ID].ConfigEncrypted
	test, err := manager.Test(t.Context(), installed.ID)
	require.NoError(t, err)
	require.True(t, test.Success)
	_, err = manager.Enable(t.Context(), installed.ID, acceptCandidate, 100)
	require.NoError(t, err)
	public, err := manager.ListPublicWebsitePlugins(t.Context())
	require.NoError(t, err)
	require.Len(t, public, 1)
	require.True(t, strings.HasSuffix(public[0].EntrypointURL, "/public/index.html"))
	for name := range installed.Manifest.Files {
		if !strings.HasPrefix(name, "public/") {
			continue
		}
		body, logicalPath, err := manager.ReadPublicWebsiteAsset(t.Context(), installed.ID, installed.BinarySHA256, name)
		require.NoError(t, err, name)
		require.Equal(t, name, logicalPath)
		require.NotEmpty(t, body)
	}
	require.Empty(t, manager.runtimes)
	status, err := manager.Status(t.Context(), installed.ID)
	require.NoError(t, err)
	require.True(t, status.Healthy)
	require.Error(t, manager.Delete(t.Context(), installed.ID), "enabled release cannot be uninstalled")
	_, err = manager.Disable(t.Context(), installed.ID)
	require.NoError(t, err)
	public, err = manager.ListPublicWebsitePlugins(t.Context())
	require.NoError(t, err)
	require.Empty(t, public)
	_, _, err = manager.ReadPublicWebsiteAsset(t.Context(), installed.ID, installed.BinarySHA256, installed.Manifest.PublicUI.Entrypoint)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = manager.Enable(t.Context(), installed.ID, false, 100)
	require.NoError(t, err)
	require.Equal(t, configBefore, repo.items[installed.ID].ConfigEncrypted)
	_, err = manager.Disable(t.Context(), installed.ID)
	require.NoError(t, err)
	require.NoError(t, manager.Delete(t.Context(), installed.ID))
	require.Empty(t, repo.items)
	require.NoDirExists(t, installed.InstallPath)
	require.NoFileExists(t, installed.ArtifactPath)
	require.Empty(t, manager.runtimes)
}

func TestWebsiteReleasePackageUpgradePreservesConfiguration(t *testing.T) {
	previousPath := os.Getenv("SUB2API_TEST_WEBSITE_PREVIOUS_PACKAGE")
	nextPath := os.Getenv("SUB2API_TEST_WEBSITE_PLUGIN_PACKAGE")
	if previousPath == "" || nextPath == "" {
		t.Skip("provide both signed website release package paths to verify an upgrade")
	}
	previousArchive, err := os.ReadFile(previousPath)
	require.NoError(t, err)
	nextArchive, err := os.ReadFile(nextPath)
	require.NoError(t, err)
	repo := &websiteMemoryRepo{items: make(map[int64]*PluginInstallation)}
	previousHostVersion := strings.TrimSpace(os.Getenv("SUB2API_TEST_WEBSITE_PREVIOUS_HOST_VERSION"))
	if previousHostVersion == "" {
		previousHostVersion = websiteHostVersion
	}
	manager := NewPluginManager(repo, pluginTokenEncryptor{}, testPluginConfig(t.TempDir(), false), PluginHostInfo{Version: previousHostVersion}, nil)
	previous, err := manager.Install(t.Context(), bytes.NewReader(previousArchive), nil)
	require.NoError(t, err)
	require.Equal(t, PluginSignatureTrusted, previous.SignatureStatus)
	_, err = manager.SaveConfig(t.Context(), previous.ID, []byte(`{"service_contact":"154215395","qq_group_number":"154895805","title":"升级保留配置"}`))
	require.NoError(t, err)
	saved := repo.items[previous.ID].ConfigEncrypted
	_, err = manager.Enable(t.Context(), previous.ID, false, 100)
	require.NoError(t, err)
	_, err = manager.Install(t.Context(), bytes.NewReader(nextArchive), nil)
	require.ErrorContains(t, err, "请先停用当前插件")
	_, err = manager.Disable(t.Context(), previous.ID)
	require.NoError(t, err)
	upgradeHostVersion := websiteReleaseHostVersion()
	manager.hostInfo.Version = upgradeHostVersion
	manager.installer.hostInfo.Version = upgradeHostVersion
	updated, err := manager.Install(t.Context(), bytes.NewReader(nextArchive), nil)
	require.NoError(t, err)
	require.Equal(t, previous.ID, updated.ID)
	require.Equal(t, builtInSupportWidgetPluginID, updated.PluginKey)
	require.Equal(t, PluginSignatureTrusted, updated.SignatureStatus)
	require.NotEqual(t, previous.Version, updated.Version)
	require.NotEqual(t, previous.BinarySHA256, updated.BinarySHA256)
	require.Equal(t, saved, updated.ConfigEncrypted)
	require.Equal(t, PluginStateDisabled, updated.State)
	require.True(t, updated.Compatibility.Compatible)
	require.True(t, updated.Compatibility.Tested)
	config, err := manager.GetConfig(t.Context(), updated.ID)
	require.NoError(t, err)
	var contacts SupportWidgetConfig
	require.NoError(t, json.Unmarshal(config, &contacts))
	require.Equal(t, "升级保留配置", contacts.Title)
	require.Equal(t, "154215395", contacts.ServiceContact)
	require.Equal(t, "154895805", contacts.QQGroupNumber)
	_, err = manager.Enable(t.Context(), updated.ID, false, 100)
	require.NoError(t, err)
	public, err := manager.ListPublicWebsitePlugins(t.Context())
	require.NoError(t, err)
	require.Len(t, public, 1)
	require.Equal(t, updated.Version, public[0].Version)
	require.Equal(t, contacts, public[0].Config)
	_, _, err = manager.ReadPublicWebsiteAsset(t.Context(), updated.ID, previous.BinarySHA256, updated.Manifest.PublicUI.Entrypoint)
	require.ErrorIs(t, err, os.ErrNotExist, "old package identity cannot load upgraded resources")
	for name := range updated.Manifest.Files {
		if !strings.HasPrefix(name, "public/") {
			continue
		}
		body, logicalPath, err := manager.ReadPublicWebsiteAsset(t.Context(), updated.ID, updated.BinarySHA256, name)
		require.NoError(t, err, name)
		require.Equal(t, name, logicalPath)
		require.NotEmpty(t, body)
	}
	_, err = manager.Disable(t.Context(), updated.ID)
	require.NoError(t, err)
	require.NoError(t, manager.Delete(t.Context(), updated.ID))
	require.Empty(t, repo.items)
	require.Empty(t, manager.runtimes)
	t.Logf("signed release upgrade passed: %s -> %s; contacts retained", previous.Version, updated.Version)
}

func websiteReleaseHostVersion() string {
	if version := strings.TrimSpace(os.Getenv("SUB2API_TEST_WEBSITE_HOST_VERSION")); version != "" {
		return version
	}
	return websiteHostVersion
}

func TestWebsitePluginLifecycleAndConfigurationDoNotTouchTransport(t *testing.T) {
	manager, repo, key := websiteTestManager(t)
	manifest, files := websiteTestManifest()
	installed, err := manager.Install(t.Context(), bytes.NewReader(websiteTestArchive(t, manifest, files, key)), nil)
	require.NoError(t, err)
	transportRoute := &pluginRoute{pluginID: 900, rolloutPercent: 75}
	manager.route.Store(transportRoute)

	public, err := manager.ListPublicWebsitePlugins(t.Context())
	require.NoError(t, err)
	require.Empty(t, public)
	_, err = manager.SaveConfig(t.Context(), installed.ID, []byte(`{"enabled":false,"service_contact":" 154215395 ","qq_group_number":"154895805"}`))
	require.NoError(t, err)
	require.Contains(t, repo.items[installed.ID].ConfigEncrypted, "ENC:")
	require.Empty(t, manager.runtimes)
	test, err := manager.Test(t.Context(), installed.ID)
	require.NoError(t, err)
	require.True(t, test.Success)

	enabled, err := manager.Enable(t.Context(), installed.ID, false, 100)
	require.NoError(t, err)
	require.Equal(t, PluginStateEnabled, enabled.State)
	require.True(t, enabled.RuntimeHealthy)
	require.Empty(t, manager.runtimes)
	require.Same(t, transportRoute, manager.route.Load())
	require.Empty(t, pluginAccountScopeFromManifest(manifest).entries, "static capability grants no account access")
	public, err = manager.ListPublicWebsitePlugins(t.Context())
	require.NoError(t, err)
	require.Len(t, public, 1)
	require.True(t, public[0].Config.Enabled, "the plugin lifecycle is the only enable switch")
	require.Equal(t, "154215395", public[0].Config.ServiceContact)
	require.Contains(t, public[0].EntrypointURL, installed.BinarySHA256)
	body, logicalPath, err := manager.ReadPublicWebsiteAsset(t.Context(), installed.ID, installed.BinarySHA256, "public/index.html")
	require.NoError(t, err)
	require.Equal(t, "public/index.html", logicalPath)
	require.Equal(t, files[logicalPath], body)
	require.Error(t, manager.Delete(t.Context(), installed.ID))
	_, err = manager.Disable(t.Context(), installed.ID)
	require.NoError(t, err)
	public, err = manager.ListPublicWebsitePlugins(t.Context())
	require.NoError(t, err)
	require.Empty(t, public)
	_, _, err = manager.ReadPublicWebsiteAsset(t.Context(), installed.ID, installed.BinarySHA256, "public/index.html")
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, manager.Delete(t.Context(), installed.ID))
	require.Empty(t, repo.items)
	require.Same(t, transportRoute, manager.route.Load())
	require.NoDirExists(t, installed.InstallPath)
}

func TestWebsitePluginRejectsUnsafeConfigAndDoesNotExposePrivateFields(t *testing.T) {
	manager, repo, key := websiteTestManager(t)
	manifest, files := websiteTestManifest()
	installed, err := manager.Install(t.Context(), bytes.NewReader(websiteTestArchive(t, manifest, files, key)), nil)
	require.NoError(t, err)
	for _, raw := range []string{`null`, `[]`, `{"api_key":"secret"}`, `{"use_ticket":null}`, `{"service_url":"javascript:alert(1)"}`, `{"service_url":"https://user:password@example.com"}`, `{"qq_group_number":"123"}`, `{"service_contact":` + strings.Repeat(" ", 16<<10) + `"x"}`} {
		_, err := manager.SaveConfig(t.Context(), installed.ID, []byte(raw))
		require.Error(t, err, raw)
	}
	_, err = manager.Enable(t.Context(), installed.ID, false, 100)
	require.NoError(t, err)
	repo.items[installed.ID].ConfigEncrypted = `ENC:{"api_key":"do not publish"}`
	public, err := manager.ListPublicWebsitePlugins(t.Context())
	require.NoError(t, err)
	require.Empty(t, public)
	require.Empty(t, manager.runtimes)
}

type websiteRecoveryEncryptor struct {
	pluginTokenEncryptor
	recoveredID int64
	recoveryErr error
}

func (e *websiteRecoveryEncryptor) InitializePluginConfigEncryption() error { return nil }
func (e *websiteRecoveryEncryptor) EncryptAfterExplicitRecovery(id int64, raw string) (string, error) {
	e.recoveredID = id
	if e.recoveryErr != nil {
		return "", e.recoveryErr
	}
	return e.Encrypt(raw)
}

func TestWebsitePluginUnreadableConfigRequiresDisabledExplicitRecovery(t *testing.T) {
	manager, repo, key := websiteTestManager(t)
	manifest, files := websiteTestManifest()
	installed, err := manager.Install(t.Context(), bytes.NewReader(websiteTestArchive(t, manifest, files, key)), nil)
	require.NoError(t, err)
	encryptor := &websiteRecoveryEncryptor{}
	manager.encryptor = encryptor
	const raw = `{"service_contact":"123456789","qq_group_number":"987654321"}`
	_, err = manager.RecoverWebsiteConfig(t.Context(), installed.ID, []byte(raw))
	require.ErrorContains(t, err, "当前配置可以读取")
	require.Zero(t, encryptor.recoveredID)
	repo.items[installed.ID].ConfigEncrypted = "unreadable-old-ciphertext"
	_, err = manager.SaveConfig(t.Context(), installed.ID, []byte(raw))
	require.ErrorContains(t, err, "重新填写流程")
	require.Equal(t, "unreadable-old-ciphertext", repo.items[installed.ID].ConfigEncrypted)
	repo.items[installed.ID].State = PluginStateEnabled
	_, err = manager.RecoverWebsiteConfig(t.Context(), installed.ID, []byte(raw))
	require.ErrorContains(t, err, "请先停用")
	require.Zero(t, encryptor.recoveredID)
	repo.items[installed.ID].State = PluginStateDisabled
	encryptor.recoveryErr = errors.New("key unavailable")
	_, err = manager.RecoverWebsiteConfig(t.Context(), installed.ID, []byte(raw))
	require.ErrorContains(t, err, "key unavailable")
	require.Equal(t, "unreadable-old-ciphertext", repo.items[installed.ID].ConfigEncrypted)
	encryptor.recoveryErr = nil
	saved, err := manager.RecoverWebsiteConfig(t.Context(), installed.ID, []byte(raw))
	require.NoError(t, err)
	require.Equal(t, installed.ID, encryptor.recoveredID)
	read, err := manager.GetConfig(t.Context(), installed.ID)
	require.NoError(t, err)
	require.JSONEq(t, string(saved), string(read))
	require.Equal(t, PluginStateDisabled, repo.items[installed.ID].State)
	_, err = manager.RecoverWebsiteConfig(t.Context(), installed.ID, []byte(raw))
	require.ErrorContains(t, err, "当前配置可以读取")
	repo.items[installed.ID].Manifest.Capabilities = nil
	_, err = manager.RecoverWebsiteConfig(t.Context(), installed.ID, []byte(raw))
	require.ErrorContains(t, err, "仅网站客服插件")
}

func TestWebsitePluginPublicAssetsRejectPrivatePathsWrongHashAndDisabledState(t *testing.T) {
	manager, repo, key := websiteTestManager(t)
	manifest, files := websiteTestManifest()
	installed, err := manager.Install(t.Context(), bytes.NewReader(websiteTestArchive(t, manifest, files, key)), nil)
	require.NoError(t, err)
	_, err = manager.Enable(t.Context(), installed.ID, false, 100)
	require.NoError(t, err)
	for _, name := range []string{"../ui/index.html", "ui/index.html", "../manifest.json", "/index.html", "index.html/../widget.js", `..\ui\index.html`, "public/not-declared.js", "index.html", "public/../ui/index.html"} {
		_, _, err := manager.ReadPublicWebsiteAsset(t.Context(), installed.ID, installed.BinarySHA256, name)
		require.ErrorIs(t, err, os.ErrNotExist, name)
	}
	_, _, err = manager.ReadPublicWebsiteAsset(t.Context(), installed.ID, strings.Repeat("f", 64), "public/index.html")
	require.ErrorIs(t, err, os.ErrNotExist)
	repo.items[installed.ID].Manifest.Requires.Sub2API = "=9.9.9"
	_, _, err = manager.ReadPublicWebsiteAsset(t.Context(), installed.ID, installed.BinarySHA256, "public/index.html")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestWebsitePluginRestoresVerifiedPackageAcrossInstancesAndRejectsTampering(t *testing.T) {
	manager, repo, key := websiteTestManager(t)
	manifest, files := websiteTestManifest()
	installed, err := manager.Install(t.Context(), bytes.NewReader(websiteTestArchive(t, manifest, files, key)), nil)
	require.NoError(t, err)
	_, err = manager.Enable(t.Context(), installed.ID, false, 100)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(installed.InstallPath))
	second := NewPluginManager(repo, pluginTokenEncryptor{}, manager.cfg, manager.hostInfo, nil)
	public, err := second.ListPublicWebsitePlugins(t.Context())
	require.NoError(t, err)
	require.Len(t, public, 1)
	require.Empty(t, second.runtimes)
	local := second.localInstallations[installed.ID]
	require.NoError(t, os.WriteFile(filepath.Join(local.InstallPath, "public/widget.js"), []byte("tampered"), 0o600))
	repo.items[installed.ID].ArtifactData = []byte("corrupt archive")
	public, err = second.ListPublicWebsitePlugins(t.Context())
	require.NoError(t, err)
	require.Empty(t, public)
	_, _, err = second.ReadPublicWebsiteAsset(t.Context(), installed.ID, installed.BinarySHA256, "public/widget.js")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestWebsitePluginUpgradePreservesConfigAndRejectsStaleCAS(t *testing.T) {
	manager, repo, key := websiteTestManager(t)
	manifest, files := websiteTestManifest()
	old, err := manager.Install(t.Context(), bytes.NewReader(websiteTestArchive(t, manifest, files, key)), nil)
	require.NoError(t, err)
	_, err = manager.SaveConfig(t.Context(), old.ID, []byte(`{"service_contact":"154215395"}`))
	require.NoError(t, err)
	previousConfig := repo.items[old.ID].ConfigEncrypted
	files["public/widget.js"] = []byte("new same-version widget")
	manifest.Files = testPluginManifest(files).Files
	updated, err := manager.Install(t.Context(), bytes.NewReader(websiteTestArchive(t, manifest, files, key)), nil)
	require.NoError(t, err)
	require.Equal(t, old.ID, updated.ID)
	require.NotEqual(t, old.BinarySHA256, updated.BinarySHA256)
	require.Equal(t, previousConfig, updated.ConfigEncrypted)
	require.ErrorIs(t, repo.UpdateConfig(t.Context(), updated.ID, "stale", old.BinarySHA256), ErrPluginStateChanged)
	require.ErrorIs(t, repo.BeginEnable(t.Context(), updated.ID, old.BinarySHA256, PluginStateDisabled), ErrPluginStateChanged)
	_, err = manager.Enable(t.Context(), updated.ID, false, 100)
	require.NoError(t, err)
	body, _, err := manager.ReadPublicWebsiteAsset(t.Context(), updated.ID, updated.BinarySHA256, "public/widget.js")
	require.NoError(t, err)
	require.Equal(t, files["public/widget.js"], body)
}

// The fake models the repository's state/hash CAS and enabled-scope uniqueness;
// lifecycle tests otherwise exercise the real signed installer and asset reader.
type websiteMemoryRepo struct {
	PluginRepository
	items map[int64]*PluginInstallation
	next  int64
}

func (r *websiteMemoryRepo) GetByID(_ context.Context, id int64) (*PluginInstallation, error) {
	p, ok := r.items[id]
	if !ok {
		return nil, sql.ErrNoRows
	}
	copy := *p
	copy.Bindings = append([]PluginBinding(nil), p.Bindings...)
	return &copy, nil
}
func (r *websiteMemoryRepo) GetByKey(ctx context.Context, key string) (*PluginInstallation, error) {
	for id, p := range r.items {
		if p.PluginKey == key {
			return r.GetByID(ctx, id)
		}
	}
	return nil, sql.ErrNoRows
}
func (r *websiteMemoryRepo) List(ctx context.Context) ([]*PluginInstallation, error) {
	out := make([]*PluginInstallation, 0, len(r.items))
	for id := range r.items {
		p, _ := r.GetByID(ctx, id)
		out = append(out, p)
	}
	return out, nil
}
func (r *websiteMemoryRepo) Install(ctx context.Context, p *PluginInstallation, bindings []PluginBinding) (*PluginInstallation, error) {
	copy := *p
	previous, err := r.GetByKey(ctx, p.PluginKey)
	if err == nil {
		if previous.State == PluginStateEnabled || hasEnabledPluginBinding(previous.Bindings) {
			return nil, ErrPluginStateChanged
		}
		copy.ID = previous.ID
		copy.ConfigEncrypted = previous.ConfigEncrypted
	} else {
		r.next++
		copy.ID = r.next
	}
	copy.Bindings = append([]PluginBinding(nil), bindings...)
	r.items[copy.ID] = &copy
	return r.GetByID(ctx, copy.ID)
}
func (r *websiteMemoryRepo) GetArtifact(_ context.Context, id int64) ([]byte, error) {
	p := r.items[id]
	if p == nil {
		return nil, sql.ErrNoRows
	}
	return p.ArtifactData, nil
}
func (r *websiteMemoryRepo) BeginEnable(_ context.Context, id int64, hash, state string) error {
	p := r.items[id]
	if p == nil || p.BinarySHA256 != hash || p.State != state || state == PluginStateStarting {
		return ErrPluginStateChanged
	}
	p.State = PluginStateStarting
	return nil
}
func (r *websiteMemoryRepo) UpdateState(_ context.Context, id int64, state, lastError string, enabledAt *time.Time, hash, expectedState string) error {
	p := r.items[id]
	if p == nil || p.BinarySHA256 != hash || p.State != expectedState {
		return ErrPluginStateChanged
	}
	p.State, p.LastError, p.EnabledAt = state, lastError, enabledAt
	return nil
}
func (r *websiteMemoryRepo) UpdateConfig(_ context.Context, id int64, encrypted, hash string) error {
	p := r.items[id]
	if p == nil || p.BinarySHA256 != hash {
		return ErrPluginStateChanged
	}
	p.ConfigEncrypted = encrypted
	return nil
}
func (r *websiteMemoryRepo) UpdateBindingsAndState(_ context.Context, id int64, bindings []PluginBinding, state, lastError string, enabledAt *time.Time, expectedState, hash string) error {
	p := r.items[id]
	if p == nil || p.BinarySHA256 != hash || (expectedState != "" && p.State != expectedState) {
		return ErrPluginStateChanged
	}
	for _, b := range bindings {
		if !b.Enabled {
			continue
		}
		for otherID, other := range r.items {
			if otherID == id {
				continue
			}
			for _, o := range other.Bindings {
				if o.Enabled && o.Capability == b.Capability && o.Platform == b.Platform && o.AccountType == b.AccountType {
					return errors.New("duplicate enabled scope")
				}
			}
		}
	}
	p.State, p.LastError, p.EnabledAt = state, lastError, enabledAt
	p.Bindings = append([]PluginBinding(nil), bindings...)
	return nil
}
func (r *websiteMemoryRepo) Delete(_ context.Context, id int64, hash string) error {
	p := r.items[id]
	if p == nil || p.BinarySHA256 != hash || p.State == PluginStateEnabled || p.State == PluginStateStarting || hasEnabledPluginBinding(p.Bindings) {
		return ErrPluginStateChanged
	}
	delete(r.items, id)
	return nil
}
