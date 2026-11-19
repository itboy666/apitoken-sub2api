package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	pluginv1 "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
)

// PublicWebsitePlugin exposes deliberately public, typed contact configuration.
// It never carries a configuration ciphertext, administrator token or account data.
type PublicWebsitePlugin struct {
	ID            int64               `json:"id"`
	PluginKey     string              `json:"plugin_key"`
	Name          string              `json:"name"`
	Version       string              `json:"version"`
	EntrypointURL string              `json:"entrypoint_url"`
	Config        SupportWidgetConfig `json:"config"`
}

func hasEnabledWebsiteBinding(installation *PluginInstallation) bool {
	if installation == nil || !installation.Manifest.IsSupportWidget() || installation.State != PluginStateEnabled {
		return false
	}
	for _, binding := range installation.Bindings {
		if binding.Enabled && binding.Capability == PluginCapabilitySupportWidget && binding.Platform == "website" && binding.AccountType == "none" {
			return true
		}
	}
	return false
}

func hasEnabledPluginBinding(bindings []PluginBinding) bool {
	for _, binding := range bindings {
		if binding.Enabled {
			return true
		}
	}
	return false
}

func normalizeWebsiteConfig(raw []byte) (SupportWidgetConfig, json.RawMessage, error) {
	cfg := DefaultSupportWidgetConfig()
	if len(raw) > 16<<10 {
		return cfg, nil, errors.New("网站客服插件配置超过大小限制")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return cfg, nil, errors.New("网站客服插件配置必须是 JSON 对象")
	}
	for key := range fields {
		switch key {
		case "enabled", "title", "description", "service_label", "service_contact", "service_url", "qq_group_number", "qq_group_url", "use_ticket":
		default:
			return cfg, nil, fmt.Errorf("网站客服插件不支持配置字段 %s", key)
		}
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, nil, errors.New("网站客服插件配置字段类型无效")
	}
	// Enablement is exclusively controlled by the plugin lifecycle, not by a
	// second switch inside the contact configuration.
	cfg.Enabled = true
	cfg, err := NormalizeSupportWidgetConfig(cfg)
	if err != nil {
		return cfg, nil, err
	}
	canonical, err := json.Marshal(cfg)
	return cfg, canonical, err
}

func (m *PluginManager) websiteConfig(installation *PluginInstallation) (SupportWidgetConfig, json.RawMessage, error) {
	raw, err := m.decryptConfig(installation)
	if err != nil {
		return DefaultSupportWidgetConfig(), nil, err
	}
	return normalizeWebsiteConfig(raw)
}

// Called with operationMu held. No process is started and no outbound route is
// acquired, even when a transport plugin is already active.
func (m *PluginManager) enableWebsitePlugin(ctx context.Context, installation *PluginInstallation, acceptUntested bool) (*PluginInstallation, error) {
	compatibility := EvaluatePluginCompatibility(installation.Manifest, m.hostInfo)
	if !compatibility.Compatible {
		stateErr := m.repo.UpdateState(ctx, installation.ID, PluginStateIncompatible, compatibility.Message, nil, installation.BinarySHA256, installation.State)
		return nil, errors.Join(errors.New(compatibility.Message), stateErr)
	}
	if !compatibility.Tested && !acceptUntested {
		return nil, errors.New("插件未声明已测试当前 Sub2API 版本，需要管理员确认后启用")
	}
	local, err := m.ensureLocalInstallation(ctx, installation)
	if err != nil {
		return nil, err
	}
	if _, _, err := m.websiteConfig(local); err != nil {
		return nil, err
	}
	if hasEnabledWebsiteBinding(installation) {
		return m.Get(ctx, installation.ID)
	}
	if err := m.repo.BeginEnable(ctx, installation.ID, installation.BinarySHA256, installation.State); err != nil {
		return nil, err
	}
	bindings := append([]PluginBinding(nil), installation.Bindings...)
	for i := range bindings {
		bindings[i].Enabled = true
		bindings[i].RolloutPercent = 100
	}
	now := time.Now()
	if err := m.repo.UpdateBindingsAndState(ctx, installation.ID, bindings, PluginStateEnabled, "", &now, PluginStateStarting, installation.BinarySHA256); err != nil {
		stateCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		stateErr := m.repo.UpdateState(stateCtx, installation.ID, PluginStateError, err.Error(), nil, installation.BinarySHA256, PluginStateStarting)
		return nil, errors.Join(err, stateErr)
	}
	return m.Get(ctx, installation.ID)
}

func (m *PluginManager) saveWebsiteConfig(ctx context.Context, installation *PluginInstallation, raw []byte) (json.RawMessage, error) {
	if _, err := m.decryptConfig(installation); err != nil {
		return nil, errors.New("旧配置无法读取，请先使用插件配置界面的重新填写流程")
	}
	_, canonical, err := normalizeWebsiteConfig(raw)
	if err != nil {
		return nil, err
	}
	if _, err := m.ensureLocalInstallation(ctx, installation); err != nil {
		return nil, err
	}
	encrypted, err := m.encryptor.Encrypt(string(canonical))
	if err != nil {
		return nil, fmt.Errorf("加密插件配置: %w", err)
	}
	if err := m.repo.UpdateConfig(ctx, installation.ID, encrypted, installation.BinarySHA256); err != nil {
		return nil, err
	}
	return canonical, nil
}

// RecoverWebsiteConfig is a separate, step-up protected administrator action.
// Ordinary saves must never silently replace an unreadable configuration.
func (m *PluginManager) RecoverWebsiteConfig(ctx context.Context, id int64, raw json.RawMessage) (json.RawMessage, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !installation.Manifest.IsSupportWidget() {
		return nil, errors.New("仅网站客服插件支持重新填写公开联系方式")
	}
	if installation.State != PluginStateDisabled || hasEnabledPluginBinding(installation.Bindings) {
		return nil, errors.New("请先停用插件，再重新填写无法读取的配置")
	}
	if _, _, err := m.websiteConfig(installation); err == nil {
		return nil, errors.New("当前配置可以读取，请重新打开配置界面后保存")
	}
	_, canonical, err := normalizeWebsiteConfig(raw)
	if err != nil {
		return nil, err
	}
	if _, err := m.ensureLocalInstallation(ctx, installation); err != nil {
		return nil, err
	}
	encryptor, ok := m.encryptor.(PluginConfigEncryptor)
	if !ok {
		return nil, errors.New("当前宿主不支持安全恢复插件配置")
	}
	encrypted, err := encryptor.EncryptAfterExplicitRecovery(id, string(canonical))
	if err != nil {
		return nil, fmt.Errorf("恢复插件配置: %w", err)
	}
	if err := m.repo.UpdateConfig(ctx, id, encrypted, installation.BinarySHA256); err != nil {
		return nil, err
	}
	return canonical, nil
}

func (m *PluginManager) testWebsitePlugin(ctx context.Context, installation *PluginInstallation) (*pluginv1.TestConfigResponse, error) {
	if !EvaluatePluginCompatibility(installation.Manifest, m.hostInfo).Compatible {
		return nil, errors.New("网站插件与当前宿主版本不兼容")
	}
	if _, err := m.ensureLocalInstallation(ctx, installation); err != nil {
		return nil, err
	}
	if _, _, err := m.websiteConfig(installation); err != nil {
		return nil, err
	}
	return &pluginv1.TestConfigResponse{Success: true, Message: "网站插件资源和公开联系方式校验通过；无需启动进程"}, nil
}

func decorateWebsitePlugin(installation *PluginInstallation, host PluginHostInfo) {
	installation.Compatibility = EvaluatePluginCompatibility(installation.Manifest, host)
	installation.RuntimeHealthy = hasEnabledWebsiteBinding(installation) && installation.Compatibility.Compatible
	if installation.RuntimeHealthy {
		installation.RuntimeMessage = "静态网站插件已启用，无需进程"
	} else {
		installation.RuntimeMessage = "静态网站插件未启用"
	}
}

// ListPublicWebsitePlugins reads authoritative plugin bindings on every request.
// There is no settings/HTML cache and no fallback to the former built-in widget.
func (m *PluginManager) ListPublicWebsitePlugins(ctx context.Context) ([]PublicWebsitePlugin, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	installations, err := m.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]PublicWebsitePlugin, 0)
	for _, installation := range installations {
		if !hasEnabledWebsiteBinding(installation) || !EvaluatePluginCompatibility(installation.Manifest, m.hostInfo).Compatible {
			continue
		}
		if _, err := m.ensureLocalInstallation(ctx, installation); err != nil {
			continue // A damaged or unavailable package must never be published.
		}
		cfg, _, err := m.websiteConfig(installation)
		if err != nil {
			continue
		}
		latest, err := m.repo.GetByID(ctx, installation.ID)
		if err != nil || !hasEnabledWebsiteBinding(latest) || !samePluginPackage(latest, installation) || latest.ConfigEncrypted != installation.ConfigEncrypted {
			continue
		}
		result = append(result, PublicWebsitePlugin{
			ID: installation.ID, PluginKey: installation.PluginKey, Name: installation.Name, Version: installation.Version,
			EntrypointURL: fmt.Sprintf("/api/v1/plugins/public/%d/assets/%s/%s", installation.ID, installation.BinarySHA256, installation.Manifest.PublicUI.Entrypoint),
			Config:        cfg,
		})
	}
	return result, nil
}

func (m *PluginManager) ReadPublicWebsiteAsset(ctx context.Context, id int64, identity, relative string) ([]byte, string, error) {
	m.operationMu.Lock()
	defer m.operationMu.Unlock()
	installation, err := m.repo.GetByID(ctx, id)
	if err != nil || !hasEnabledWebsiteBinding(installation) || !EvaluatePluginCompatibility(installation.Manifest, m.hostInfo).Compatible || identity != installation.BinarySHA256 {
		return nil, "", os.ErrNotExist
	}
	clean, err := normalizePluginArchivePath(relative)
	if err != nil || clean != relative || !strings.HasPrefix(clean, "public/") {
		return nil, "", os.ErrNotExist
	}
	logicalPath := clean
	if !safeWebsitePluginFile(logicalPath) {
		return nil, "", os.ErrNotExist
	}
	if _, ok := installation.Manifest.Files[logicalPath]; !ok {
		return nil, "", os.ErrNotExist
	}
	installation, err = m.ensureLocalInstallation(ctx, installation)
	if err != nil {
		return nil, "", os.ErrNotExist
	}
	data, err := readVerifiedWebsiteAsset(installation, logicalPath)
	if err != nil {
		return nil, "", os.ErrNotExist
	}
	latest, err := m.repo.GetByID(ctx, id)
	if err != nil || !hasEnabledWebsiteBinding(latest) || !samePluginPackage(latest, installation) {
		return nil, "", os.ErrNotExist
	}
	return data, logicalPath, nil
}

func readVerifiedWebsiteAsset(installation *PluginInstallation, logicalPath string) ([]byte, error) {
	expected, ok := installation.Manifest.Files[logicalPath]
	if !ok {
		return nil, os.ErrNotExist
	}
	fullPath, err := safePluginJoin(installation.InstallPath, logicalPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(fullPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > pluginUIAssetMaxBytes {
		return nil, errors.New("网站插件资源不可用")
	}
	file, err := os.Open(fullPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, pluginUIAssetMaxBytes+1))
	if err != nil || len(data) > pluginUIAssetMaxBytes {
		return nil, errors.New("网站插件资源超过大小限制")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != expected {
		return nil, errors.New("网站插件资源哈希不匹配")
	}
	return data, nil
}

func verifyLocalWebsitePlugin(installation *PluginInstallation) error {
	if err := installation.Manifest.Validate(); err != nil {
		return err
	}
	if installation.BinarySHA256 != installation.Manifest.StaticIdentity() || installation.BinaryPath != "" {
		return errors.New("静态网站插件清单标识不匹配")
	}
	for name := range installation.Manifest.Files {
		if _, err := readVerifiedWebsiteAsset(installation, name); err != nil {
			return err
		}
	}
	return nil
}
