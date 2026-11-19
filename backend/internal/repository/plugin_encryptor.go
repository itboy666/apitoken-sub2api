package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

const pluginConfigCipherPrefix = "plugin-config-v1:"

type pluginConfigEncryptor struct {
	fallback             service.SecretEncryptor
	fixedLegacyKey       bool
	keyPath              string
	hasStoredCiphertexts func() (bool, error)
	recoveryIsExclusive  func(int64) (bool, error)
}

var _ service.PluginConfigEncryptor = (*pluginConfigEncryptor)(nil)
var _ service.PluginUIAssetEncryptor = (*pluginConfigEncryptor)(nil)

// NewPluginConfigEncryptor never writes with the process-generated TOTP key.
// Existing unprefixed ciphertexts are readable only with an explicitly fixed
// server key; successfully saved configurations always use the dedicated file.
func NewPluginConfigEncryptor(cfg *config.Config, fallback service.SecretEncryptor, db *sql.DB) service.PluginConfigEncryptor {
	dir := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if dir == "" {
		if info, err := os.Stat("/app/data"); err == nil && info.IsDir() {
			dir = "/app/data"
		} else {
			dir = "./data"
		}
	}
	return &pluginConfigEncryptor{
		fallback:       fallback,
		fixedLegacyKey: cfg != nil && cfg.Totp.EncryptionKeyConfigured,
		keyPath:        filepath.Join(dir, "secrets", "plugin-config.key"),
		hasStoredCiphertexts: func() (bool, error) {
			if db == nil {
				return false, errors.New("plugin configuration database is unavailable")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var exists bool
			err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM sub2api_plugin_installations WHERE config_encrypted LIKE 'plugin-config-v1:%')`).Scan(&exists)
			return exists, err
		},
		recoveryIsExclusive: func(installationID int64) (bool, error) {
			if db == nil {
				return false, errors.New("plugin configuration database is unavailable")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var exclusive bool
			err := db.QueryRowContext(ctx, `SELECT EXISTS (
				SELECT 1 FROM sub2api_plugin_installations WHERE id=$1
			) AND NOT EXISTS (
				SELECT 1 FROM sub2api_plugin_installations
				WHERE id<>$1 AND config_encrypted LIKE 'plugin-config-v1:%'
			) AND (
				EXISTS (
					SELECT 1 FROM sub2api_plugin_installations
					WHERE id=$1 AND config_encrypted LIKE 'plugin-config-v1:%'
				) OR NOT EXISTS (
					SELECT 1 FROM sub2api_plugin_installations
					WHERE config_encrypted LIKE 'plugin-config-v1:%'
				)
			)`, installationID).Scan(&exclusive)
			return exclusive, err
		},
	}
}

func (e *pluginConfigEncryptor) localEncryptor() (*AESEncryptor, error) {
	if err := e.validateKeyDirectory(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(e.keyPath)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || info.Size() > 65 {
		return nil, errors.New("plugin configuration key must be a private regular file")
	}
	f, err := os.Open(e.keyPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	opened, err := f.Stat()
	if err != nil || !os.SameFile(info, opened) || !opened.Mode().IsRegular() || opened.Mode().Perm()&0o077 != 0 || opened.Size() > 65 {
		return nil, errors.New("plugin configuration key changed while opening")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 66))
	if err != nil {
		return nil, err
	}
	key, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil || len(key) != 32 {
		return nil, errors.New("invalid plugin configuration encryption key")
	}
	return &AESEncryptor{key: key}, nil
}

func (e *pluginConfigEncryptor) validateKeyDirectory() error {
	info, err := os.Lstat(filepath.Dir(e.keyPath))
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return errors.New("plugin configuration key directory must be private")
	}
	return nil
}

func (e *pluginConfigEncryptor) InitializePluginConfigEncryption() error {
	if _, err := e.localEncryptor(); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if e.hasStoredCiphertexts == nil {
		return errors.New("plugin configuration key recovery check is unavailable")
	}
	exists, err := e.hasStoredCiphertexts()
	if err != nil {
		return err
	}
	if exists {
		return errors.New("restore the original plugin configuration encryption key")
	}
	return e.createKeyIfMissing()
}

func (e *pluginConfigEncryptor) createKeyIfMissing() error {
	dir := filepath.Dir(e.keyPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := e.validateKeyDirectory(); err != nil {
		return err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".plugin-config-key-*")
	if err != nil {
		return err
	}
	defer func() { _ = f.Close(); _ = os.Remove(f.Name()) }()
	if _, err := f.WriteString(hex.EncodeToString(key) + "\n"); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Publish a complete file without replacing an existing key, including when
	// another host worker wins first-use initialization.
	if err := os.Link(f.Name(), e.keyPath); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	if err := d.Sync(); err != nil {
		return err
	}
	_, err = e.localEncryptor()
	return err
}

func (e *pluginConfigEncryptor) Encrypt(plaintext string) (string, error) {
	if err := e.InitializePluginConfigEncryption(); err != nil {
		return "", err
	}
	local, err := e.localEncryptor()
	if err != nil {
		return "", err
	}
	ciphertext, err := local.Encrypt(plaintext)
	if err != nil {
		return "", err
	}
	return pluginConfigCipherPrefix + ciphertext, nil
}

// EncryptAfterExplicitRecovery is reserved for the separately confirmed
// administrator recovery flow. It can create a missing key only when the
// target installation is the sole row using this key format, or no rows use
// the format yet. It never replaces an existing key.
func (e *pluginConfigEncryptor) EncryptAfterExplicitRecovery(installationID int64, plaintext string) (string, error) {
	if installationID <= 0 {
		return "", errors.New("plugin recovery installation is invalid")
	}
	if _, err := e.localEncryptor(); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if e.recoveryIsExclusive == nil {
			return "", errors.New("plugin recovery isolation check is unavailable")
		}
		exclusive, checkErr := e.recoveryIsExclusive(installationID)
		if checkErr != nil {
			return "", checkErr
		}
		if !exclusive {
			return "", errors.New("restore the original plugin configuration encryption key; other plugin configurations still use it")
		}
		if err := e.createKeyIfMissing(); err != nil {
			return "", err
		}
	}
	local, err := e.localEncryptor()
	if err != nil {
		return "", err
	}
	ciphertext, err := local.Encrypt(plaintext)
	if err != nil {
		return "", err
	}
	return pluginConfigCipherPrefix + ciphertext, nil
}

func (e *pluginConfigEncryptor) Decrypt(ciphertext string) (string, error) {
	if strings.HasPrefix(ciphertext, pluginConfigCipherPrefix) {
		local, err := e.localEncryptor()
		if err != nil {
			return "", errors.New("plugin configuration encryption key is unavailable; restore its original key")
		}
		return local.Decrypt(strings.TrimPrefix(ciphertext, pluginConfigCipherPrefix))
	}
	if !e.fixedLegacyKey || e.fallback == nil {
		return "", errors.New("original fixed server encryption key is required for legacy plugin configuration")
	}
	return e.fallback.Decrypt(ciphertext)
}

func (e *pluginConfigEncryptor) EncryptUIAssetToken(plaintext string) (string, error) {
	if e.fallback == nil {
		return "", errors.New("plugin UI encryption is unavailable")
	}
	return e.fallback.Encrypt(plaintext)
}

func (e *pluginConfigEncryptor) DecryptUIAssetToken(ciphertext string) (string, error) {
	if e.fallback == nil {
		return "", errors.New("plugin UI encryption is unavailable")
	}
	return e.fallback.Decrypt(ciphertext)
}
