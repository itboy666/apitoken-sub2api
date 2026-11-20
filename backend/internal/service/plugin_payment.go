package service

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/tronpayment"
	api "github.com/Wei-Shaw/sub2api/pkg/pluginapi/v1"
	"time"
)

func maskPaymentPluginConfig(raw json.RawMessage) (json.RawMessage, error) {
	var c map[string]any
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if c == nil {
		return nil, errors.New("支付配置必须是 JSON 对象")
	}
	key, _ := c["tronscan_api_key"].(string)
	c["tronscan_api_key"] = ""
	c["api_key_set"] = key != ""
	return json.Marshal(c)
}
func mergePaymentPluginSecret(raw, previous json.RawMessage) (json.RawMessage, error) {
	if err := tronpayment.ValidateConfigObject(raw); err != nil {
		return nil, err
	}
	var c, old map[string]any
	if json.Unmarshal(raw, &c) != nil || c == nil {
		return nil, errors.New("支付配置必须是 JSON 对象")
	}
	if json.Unmarshal(previous, &old) != nil {
		return nil, errors.New("原支付配置无法读取")
	}
	delete(c, "api_key_set")
	if key, exists := c["tronscan_api_key"]; exists {
		if _, ok := key.(string); !ok {
			return nil, errors.New("tronscan_api_key 必须是字符串")
		}
	}
	if key, _ := c["tronscan_api_key"].(string); key == "" {
		c["tronscan_api_key"] = old["tronscan_api_key"]
	}
	merged, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	if _, err = tronpayment.ParseConfig(merged); err != nil {
		return nil, err
	}
	return merged, nil
}

// Payment runtimes share lifecycle/configuration but never enter the OAuth route.
// Called under operationMu; persistent bindings remain authoritative per request.
func (m *PluginManager) reconcilePaymentPlugins(ctx context.Context, items []*PluginInstallation) error {
	live := map[int64]bool{}
	for _, p := range items {
		if !p.Manifest.IsPaymentPlugin() || !hasEnabledPluginBinding(p.Bindings) {
			continue
		}
		live[p.ID] = true
		m.mu.Lock()
		rt := m.runtimes[p.ID]
		m.mu.Unlock()
		if rt != nil && !rt.client.Exited() && rt.installation.BinarySHA256 == p.BinarySHA256 && rt.installation.ConfigEncrypted == p.ConfigEncrypted {
			continue
		}
		if p.State == PluginStateStarting && !m.startingStateExpired(p) {
			continue
		}
		if !EvaluatePluginCompatibility(p.Manifest, m.hostInfo).Compatible {
			continue
		}
		local, err := m.ensureLocalInstallation(ctx, p)
		if err != nil {
			return err
		}
		rt, err = m.prepareRuntime(ctx, local, true)
		if err != nil {
			return err
		}
		latest, err := m.repo.GetByID(ctx, p.ID)
		if err != nil {
			rt.kill()
			return err
		}
		if !hasEnabledPluginBinding(latest.Bindings) || latest.ConfigEncrypted != p.ConfigEncrypted || latest.BinarySHA256 != p.BinarySHA256 {
			rt.kill()
			continue
		}
		if err = m.repo.MarkRuntimeHealthy(ctx, p.ID, p.BinarySHA256, p.ConfigEncrypted); err != nil {
			rt.kill()
			return err
		}
		m.mu.Lock()
		m.publishRuntimeLocked(local, rt)
		m.mu.Unlock()
	}
	m.mu.Lock()
	var stale []*pluginRuntime
	for id, rt := range m.runtimes {
		if rt.installation.Manifest.IsPaymentPlugin() && !live[id] {
			stale = append(stale, rt)
			delete(m.runtimes, id)
		}
	}
	m.mu.Unlock()
	for _, rt := range stale {
		rt.drain(10 * time.Second)
	}
	return nil
}

func (m *PluginManager) activePaymentPlugin(ctx context.Context) (*PluginInstallation, tronpayment.Config, error) {
	var empty tronpayment.Config
	if m == nil {
		return nil, empty, errors.New("USDT 支付插件尚未启用")
	}
	items, err := m.repo.List(ctx)
	if err != nil {
		return nil, empty, err
	}
	var selected *PluginInstallation
	for _, p := range items {
		if p.Manifest.IsPaymentPlugin() && p.State == PluginStateEnabled && hasEnabledPluginBinding(p.Bindings) {
			if selected != nil {
				return nil, empty, errors.New("多个 USDT 插件同时启用")
			}
			selected = p
		}
	}
	if selected == nil {
		return nil, empty, errors.New("USDT 支付插件尚未启用")
	}
	raw, err := m.decryptConfig(selected)
	if err != nil {
		return nil, empty, err
	}
	cfg, err := tronpayment.ParseConfig(raw)
	if err != nil {
		return nil, empty, err
	}
	m.mu.Lock()
	rt := m.runtimes[selected.ID]
	ready := rt != nil && !rt.client.Exited()
	m.mu.Unlock()
	if !ready {
		return nil, empty, errors.New("USDT 支付插件暂不可用")
	}
	return selected, cfg, nil
}

func (m *PluginManager) verifyPluginPayment(ctx context.Context, id int64, req *api.VerifyPaymentRequest) (*api.VerifyPaymentResponse, error) {
	p, _, err := m.activePaymentPlugin(ctx)
	if err != nil {
		return nil, err
	}
	if p.ID != id {
		return nil, errors.New("订单所属支付插件未启用")
	}
	m.mu.Lock()
	rt := m.runtimes[id]
	ready := rt != nil && rt.beginRequest()
	m.mu.Unlock()
	if !ready {
		return nil, errors.New("支付插件正在停止")
	}
	defer rt.finishRequest()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return rt.api.VerifyPayment(ctx, req)
}

// Older verifiers can only query their configured wallet. Keep their protection
// until a signed verifier supporting immutable invoice recipients is installed.
func (s *PaymentConfigService) guardUSDTWalletChange(ctx context.Context, installation *PluginInstallation, previous, next json.RawMessage) error {
	var before, after map[string]any
	if json.Unmarshal(previous, &before) != nil || json.Unmarshal(next, &after) != nil {
		return errors.New("支付配置无效")
	}
	if before["wallet_address"] == nil || before["wallet_address"] == after["wallet_address"] {
		return nil
	}
	if installation != nil && installation.PluginKey == "local.apitoken.usdt-trc20" &&
		installation.SignatureStatus == PluginSignatureTrusted && matchesSemverRange(installation.Version, ">=1.2.0 <2.0.0-0") {
		return nil
	}
	rows, err := s.entClient.QueryContext(ctx, `SELECT 1 FROM sub2api_usdt_invoices i JOIN payment_orders o ON o.id=i.order_id WHERE i.wallet=$1 AND o.status IN ('PENDING','EXPIRED') AND o.expires_at>=NOW()-INTERVAL '3 hours' LIMIT 1`, before["wallet_address"])
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("地址未保存：当前插件不能同时核验新旧钱包。请先升级 USDT 插件至 1.2.0，或等待旧订单到期满 3 小时后再更换")
	}
	return rows.Err()
}
