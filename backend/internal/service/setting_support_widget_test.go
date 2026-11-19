//go:build unit

package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type supportWidgetSettingsRepo struct {
	SettingRepository
	values map[string]string
	setErr error
}

func (r *supportWidgetSettingsRepo) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := r.values[key]; ok {
		return value, nil
	}
	return "", ErrSettingNotFound
}
func (r *supportWidgetSettingsRepo) Set(_ context.Context, key, value string) error {
	if r.setErr != nil {
		return r.setErr
	}
	r.values[key] = value
	return nil
}
func (r *supportWidgetSettingsRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string)
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func TestSupportWidgetRejectsUnsafeURLsAndBounds(t *testing.T) {
	for _, value := range []string{"javascript:alert(1)", "data:text/html,test", "//example.com", "/support-tickets?next=x", "https://user:pass@example.com", "https://user@example.com", "https://example.com\n", "https://example.com/%0d%0a", "https://example.com/\\path", "https:///path", "mailto:support@example.com", "https://example.com/" + strings.Repeat("a", 2048)} {
		t.Run(value, func(t *testing.T) {
			cfg := DefaultSupportWidgetConfig()
			cfg.ServiceURL = value
			_, err := NormalizeSupportWidgetConfig(cfg)
			require.Error(t, err)
			cfg.ServiceURL, cfg.QQGroupURL = "", value
			_, err = NormalizeSupportWidgetConfig(cfg)
			require.Error(t, err)
		})
	}
	for _, value := range []string{"https://support.example.com/help?q=one%20two", "http://support.example.com/help", "/support-tickets"} {
		cfg := DefaultSupportWidgetConfig()
		cfg.ServiceURL = value
		_, err := NormalizeSupportWidgetConfig(cfg)
		require.NoError(t, err)
	}
	cfg := DefaultSupportWidgetConfig()
	cfg.QQGroupURL = "/support-tickets"
	_, err := NormalizeSupportWidgetConfig(cfg)
	require.Error(t, err)
	for _, mutate := range []func(*SupportWidgetConfig){
		func(c *SupportWidgetConfig) { c.Title = strings.Repeat("中", 41) },
		func(c *SupportWidgetConfig) { c.Description = strings.Repeat("中", 161) },
		func(c *SupportWidgetConfig) { c.ServiceLabel = strings.Repeat("中", 41) },
		func(c *SupportWidgetConfig) { c.ServiceContact = strings.Repeat("中", 101) },
		func(c *SupportWidgetConfig) { c.QQGroupNumber = "1234" },
		func(c *SupportWidgetConfig) { c.QQGroupNumber = "1234567890123" },
		func(c *SupportWidgetConfig) { c.QQGroupNumber = "15489580x" },
		func(c *SupportWidgetConfig) { c.Enabled, c.UseTicket = true, false },
	} {
		cfg := DefaultSupportWidgetConfig()
		mutate(&cfg)
		_, err := NormalizeSupportWidgetConfig(cfg)
		require.ErrorContains(t, err, "SUPPORT_WIDGET_INVALID")
	}
}

func TestLegacySupportWidgetIsNoLongerExposedByPublicSettings(t *testing.T) {
	raw := `{"enabled":true,"qq_group_number":"154895805","service_contact":"154215395"}`
	repo := &supportWidgetSettingsRepo{values: map[string]string{SettingKeySupportWidgetConfig: raw}}
	svc := NewSettingService(repo, &config.Config{})
	public, err := svc.GetPublicSettings(t.Context())
	require.NoError(t, err)
	encoded, err := json.Marshal(public)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "support_widget")
	injected, err := svc.GetPublicSettingsForInjection(t.Context())
	require.NoError(t, err)
	encoded, err = json.Marshal(injected)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "support_widget")
	require.Equal(t, raw, repo.values[SettingKeySupportWidgetConfig], "retired data is preserved for migration or rollback")
}
