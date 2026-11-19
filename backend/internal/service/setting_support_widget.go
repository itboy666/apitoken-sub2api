package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

// Retained only to identify the former settings row during explicit migration.
// Public settings never read it; the plugin owns its encrypted configuration.
const SettingKeySupportWidgetConfig = "support_widget_config"

// SupportWidgetConfig contains only deliberately public contact information.
// It is the allowlisted configuration for website.support_widget.v1 plugins.
type SupportWidgetConfig struct {
	Enabled        bool   `json:"enabled"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	ServiceLabel   string `json:"service_label"`
	ServiceContact string `json:"service_contact"`
	ServiceURL     string `json:"service_url"`
	QQGroupNumber  string `json:"qq_group_number"`
	QQGroupURL     string `json:"qq_group_url"`
	UseTicket      bool   `json:"use_ticket"`
}

// Reject null for typed fields instead of silently retaining defaults. The
// receiver may already contain defaults for fields omitted by older clients.
func (c *SupportWidgetConfig) UnmarshalJSON(data []byte) error {
	type configJSON SupportWidgetConfig
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		return errors.New("support widget configuration must be a JSON object")
	}
	for _, key := range []string{"enabled", "title", "description", "service_label", "service_contact", "service_url", "qq_group_number", "qq_group_url", "use_ticket"} {
		if raw, exists := fields[key]; exists && strings.TrimSpace(string(raw)) == "null" {
			return fmt.Errorf("support widget %s must not be null", key)
		}
	}
	return json.Unmarshal(data, (*configJSON)(c))
}

func DefaultSupportWidgetConfig() SupportWidgetConfig {
	return SupportWidgetConfig{Title: "联系与支持", ServiceLabel: "在线客服", UseTicket: true}
}

var supportWidgetQQGroupPattern = regexp.MustCompile(`^[0-9]{5,12}$`)

func NormalizeSupportWidgetConfig(cfg SupportWidgetConfig) (SupportWidgetConfig, error) {
	bad := func(message string) (SupportWidgetConfig, error) {
		return cfg, infraerrors.BadRequest("SUPPORT_WIDGET_INVALID", message)
	}
	cfg.Title = cleanSupportTicketLine(cfg.Title)
	cfg.Description = cleanSupportTicketText(cfg.Description)
	cfg.ServiceLabel = cleanSupportTicketLine(cfg.ServiceLabel)
	cfg.ServiceContact = cleanSupportTicketLine(cfg.ServiceContact)
	cfg.QQGroupNumber = strings.TrimSpace(cfg.QQGroupNumber)
	if cfg.Title == "" {
		cfg.Title = DefaultSupportWidgetConfig().Title
	}
	if cfg.ServiceLabel == "" {
		cfg.ServiceLabel = DefaultSupportWidgetConfig().ServiceLabel
	}
	for _, field := range []struct {
		name, value string
		max         int
	}{{"title", cfg.Title, 40}, {"description", cfg.Description, 160}, {"service_label", cfg.ServiceLabel, 40}, {"service_contact", cfg.ServiceContact, 100}} {
		if utf8.RuneCountInString(field.value) > field.max {
			return bad(fmt.Sprintf("%s must be at most %d characters", field.name, field.max))
		}
	}
	if cfg.QQGroupNumber != "" && !supportWidgetQQGroupPattern.MatchString(cfg.QQGroupNumber) {
		return bad("qq_group_number must contain 5–12 digits")
	}
	var err error
	if cfg.ServiceURL, err = normalizeSupportWidgetURL(cfg.ServiceURL, true); err != nil {
		return bad("service_url " + err.Error())
	}
	if cfg.QQGroupURL, err = normalizeSupportWidgetURL(cfg.QQGroupURL, false); err != nil {
		return bad("qq_group_url " + err.Error())
	}
	if cfg.Enabled && !cfg.UseTicket && cfg.ServiceContact == "" && cfg.ServiceURL == "" && cfg.QQGroupNumber == "" && cfg.QQGroupURL == "" {
		return bad("enable a ticket entry or provide a customer service contact or group")
	}
	return cfg, nil
}

func normalizeSupportWidgetURL(raw string, allowTicket bool) (string, error) {
	invalid := errors.New("must be an absolute HTTP(S) URL without credentials or control characters")
	if strings.ContainsFunc(raw, unicode.IsControl) {
		return "", invalid
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || (allowTicket && raw == "/support-tickets") {
		return raw, nil
	}
	if len(raw) > 2048 || strings.Contains(raw, `\`) {
		return "", invalid
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil || strings.ContainsFunc(decoded, unicode.IsControl) {
		return "", invalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return "", invalid
	}
	return raw, nil
}
