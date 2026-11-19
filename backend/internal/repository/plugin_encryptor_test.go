//go:build unit

package repository

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func newTestPluginEncryptor(t *testing.T) *pluginConfigEncryptor {
	t.Helper()
	return &pluginConfigEncryptor{
		fallback:             aesEncryptor(t),
		keyPath:              filepath.Join(t.TempDir(), "secrets", "plugin-config.key"),
		hasStoredCiphertexts: func() (bool, error) { return false, nil },
	}
}

func TestPluginConfigEncryptionSurvivesRestartAndSharedKeyChange(t *testing.T) {
	e := newTestPluginEncryptor(t)
	require.NoError(t, os.MkdirAll(filepath.Dir(e.keyPath), 0o700))
	credentialPath := filepath.Join(filepath.Dir(e.keyPath), "credential-operations.key")
	require.NoError(t, os.WriteFile(credentialPath, []byte("existing credential key"), 0o600))
	const payload = `{"service_contact":"123456789","qq_group_number":"987654321"}`
	ciphertext, err := e.Encrypt(payload)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(ciphertext, pluginConfigCipherPrefix))
	keyBefore, err := os.ReadFile(e.keyPath)
	require.NoError(t, err)
	keyInfo, err := os.Stat(e.keyPath)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), keyInfo.Mode().Perm())
	dirInfo, err := os.Stat(filepath.Dir(e.keyPath))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())

	newShared, err := NewAESEncryptor(aesTestCfg(aesHexKey(32, 0x55)))
	require.NoError(t, err)
	restarted := &pluginConfigEncryptor{fallback: newShared, keyPath: e.keyPath}
	plaintext, err := restarted.Decrypt(ciphertext)
	require.NoError(t, err)
	require.Equal(t, payload, plaintext)
	// The existing key works without consulting the database or legacy key.
	require.NoError(t, restarted.InitializePluginConfigEncryption())
	second, err := restarted.Encrypt(payload)
	require.NoError(t, err)
	require.NotEqual(t, ciphertext, second)
	plaintext, err = e.Decrypt(second)
	require.NoError(t, err)
	require.Equal(t, payload, plaintext)
	keyAfter, err := os.ReadFile(e.keyPath)
	require.NoError(t, err)
	require.Equal(t, keyBefore, keyAfter)
	credential, err := os.ReadFile(credentialPath)
	require.NoError(t, err)
	require.Equal(t, "existing credential key", string(credential))
}

func TestPluginConfigEncryptionMissingKeyDoesNotReplaceExistingSecrets(t *testing.T) {
	e := newTestPluginEncryptor(t)
	ciphertext, err := e.Encrypt(`{"keep":true}`)
	require.NoError(t, err)
	require.NoError(t, os.Remove(e.keyPath))
	checks := 0
	e.hasStoredCiphertexts = func() (bool, error) { checks++; return true, nil }
	_, err = e.Decrypt(ciphertext)
	require.ErrorContains(t, err, "restore its original key")
	require.Zero(t, checks, "decrypt must never initialize a replacement key")
	_, err = e.Encrypt(`{"replacement":true}`)
	require.ErrorContains(t, err, "restore the original")
	require.Equal(t, 1, checks)
	_, err = os.Lstat(e.keyPath)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestPluginConfigEncryptionRequiresSuccessfulMissingKeyRecoveryCheck(t *testing.T) {
	for _, unavailable := range []bool{false, true} {
		t.Run(map[bool]string{false: "database failure", true: "check unavailable"}[unavailable], func(t *testing.T) {
			e := newTestPluginEncryptor(t)
			failure := errors.New("database temporarily unavailable")
			e.hasStoredCiphertexts = func() (bool, error) { return false, failure }
			if unavailable {
				e.hasStoredCiphertexts = nil
			}
			_, err := e.Encrypt(`{}`)
			if unavailable {
				require.ErrorContains(t, err, "recovery check is unavailable")
			} else {
				require.ErrorIs(t, err, failure)
			}
			_, err = os.Lstat(e.keyPath)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestPluginConfigEncryptionRejectsUnsafeStorageWithoutRepairingIt(t *testing.T) {
	for _, condition := range []string{"public directory", "public file", "invalid key", "oversized key", "key symlink", "directory symlink"} {
		t.Run(condition, func(t *testing.T) {
			e := newTestPluginEncryptor(t)
			dir := filepath.Dir(e.keyPath)
			require.NoError(t, os.MkdirAll(dir, 0o700))
			data := []byte(aesHexKey(32, 0x10) + "\n")
			switch condition {
			case "public directory":
				require.NoError(t, os.Chmod(dir, 0o755))
			case "public file":
				require.NoError(t, os.WriteFile(e.keyPath, data, 0o600))
				require.NoError(t, os.Chmod(e.keyPath, 0o644))
			case "invalid key":
				data = []byte("not a key")
			case "oversized key":
				data = []byte(strings.Repeat("a", 66))
			case "key symlink":
				target := filepath.Join(t.TempDir(), "original.key")
				require.NoError(t, os.WriteFile(target, data, 0o600))
				require.NoError(t, os.Symlink(target, e.keyPath))
			case "directory symlink":
				target := filepath.Join(t.TempDir(), "private")
				require.NoError(t, os.Mkdir(target, 0o700))
				require.NoError(t, os.Remove(dir))
				require.NoError(t, os.Symlink(target, dir))
			}
			if condition != "public file" && condition != "key symlink" {
				require.NoError(t, os.WriteFile(e.keyPath, data, 0o600))
			}
			beforeInfo, err := os.Lstat(e.keyPath)
			require.NoError(t, err)
			checks := 0
			e.hasStoredCiphertexts = func() (bool, error) { checks++; return false, nil }
			require.Error(t, e.InitializePluginConfigEncryption())
			_, err = e.Encrypt(`{}`)
			require.Error(t, err)
			require.Zero(t, checks, "existing unsafe storage must not be replaced")
			after, err := os.ReadFile(e.keyPath)
			require.NoError(t, err)
			require.Equal(t, data, after)
			afterInfo, err := os.Lstat(e.keyPath)
			require.NoError(t, err)
			require.True(t, os.SameFile(beforeInfo, afterInfo))
			require.Equal(t, beforeInfo.Mode(), afterInfo.Mode())
		})
	}
}

func TestPluginConfigEncryptionConcurrentInitializationUsesOneKey(t *testing.T) {
	e := newTestPluginEncryptor(t)
	const workers = 24
	ready := sync.WaitGroup{}
	ready.Add(workers)
	start := make(chan struct{})
	results := make(chan struct {
		ciphertext string
		err        error
	}, workers)
	for range workers {
		go func() {
			instance := &pluginConfigEncryptor{keyPath: e.keyPath, hasStoredCiphertexts: func() (bool, error) {
				ready.Done()
				<-start
				return false, nil
			}}
			ciphertext, err := instance.Encrypt(`{"concurrent":true}`)
			results <- struct {
				ciphertext string
				err        error
			}{ciphertext, err}
		}()
	}
	ready.Wait()
	close(start)
	for range workers {
		result := <-results
		require.NoError(t, result.err)
		plaintext, err := e.Decrypt(result.ciphertext)
		require.NoError(t, err)
		require.Equal(t, `{"concurrent":true}`, plaintext)
	}
	files, err := os.ReadDir(filepath.Dir(e.keyPath))
	require.NoError(t, err)
	require.Len(t, files, 1, "temporary first-use key files must be removed")
	require.Equal(t, "plugin-config.key", files[0].Name())
}

func TestPluginConfigEncryptionLegacyFallbackRequiresFixedKey(t *testing.T) {
	e := newTestPluginEncryptor(t)
	legacy, err := e.fallback.Encrypt(`{"legacy":true}`)
	require.NoError(t, err)
	_, err = e.Decrypt(legacy)
	require.ErrorContains(t, err, "original fixed server encryption key")
	e.fixedLegacyKey = true
	plaintext, err := e.Decrypt(legacy)
	require.NoError(t, err)
	require.Equal(t, `{"legacy":true}`, plaintext)
	// Reading legacy data neither creates a key nor silently rewrites data.
	_, err = os.Lstat(e.keyPath)
	require.ErrorIs(t, err, os.ErrNotExist)
	migrated, err := e.Encrypt(plaintext)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(migrated, pluginConfigCipherPrefix))
	e.fixedLegacyKey = false
	e.fallback = nil
	plaintext, err = e.Decrypt(migrated)
	require.NoError(t, err)
	require.Equal(t, `{"legacy":true}`, plaintext)
}

func TestPluginConfigExplicitRecoveryRequiresExclusiveTarget(t *testing.T) {
	t.Run("target is the only encrypted installation", func(t *testing.T) {
		e := newTestPluginEncryptor(t)
		checks := 0
		e.recoveryIsExclusive = func(id int64) (bool, error) {
			checks++
			require.EqualValues(t, 17, id)
			return true, nil
		}
		ciphertext, err := e.EncryptAfterExplicitRecovery(17, `{"recovered":true}`)
		require.NoError(t, err)
		require.Equal(t, 1, checks)
		require.True(t, strings.HasPrefix(ciphertext, pluginConfigCipherPrefix))
		plaintext, err := e.Decrypt(ciphertext)
		require.NoError(t, err)
		require.Equal(t, `{"recovered":true}`, plaintext)
	})

	t.Run("another encrypted installation blocks key creation", func(t *testing.T) {
		e := newTestPluginEncryptor(t)
		e.recoveryIsExclusive = func(id int64) (bool, error) {
			require.EqualValues(t, 17, id)
			return false, nil
		}
		_, err := e.EncryptAfterExplicitRecovery(17, `{"recovered":true}`)
		require.ErrorContains(t, err, "other plugin configurations")
		_, err = os.Lstat(e.keyPath)
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("existing key is reused without replacement", func(t *testing.T) {
		e := newTestPluginEncryptor(t)
		first, err := e.Encrypt(`{"old":true}`)
		require.NoError(t, err)
		keyBefore, err := os.ReadFile(e.keyPath)
		require.NoError(t, err)
		checks := 0
		e.recoveryIsExclusive = func(int64) (bool, error) {
			checks++
			return false, nil
		}
		second, err := e.EncryptAfterExplicitRecovery(17, `{"recovered":true}`)
		require.NoError(t, err)
		require.Zero(t, checks)
		require.NotEqual(t, first, second)
		keyAfter, err := os.ReadFile(e.keyPath)
		require.NoError(t, err)
		require.Equal(t, keyBefore, keyAfter)
		plaintext, err := e.Decrypt(first)
		require.NoError(t, err)
		require.Equal(t, `{"old":true}`, plaintext)
	})
}

func TestPluginConfigUITokensDoNotDependOnOrInitializeConfigurationKey(t *testing.T) {
	e := newTestPluginEncryptor(t)
	checks := 0
	e.hasStoredCiphertexts = func() (bool, error) { checks++; return true, nil }
	e.recoveryIsExclusive = func(int64) (bool, error) { checks++; return false, nil }
	const payload = "sub2api:plugin-ui:v1:test-short-lived-capability"
	token, err := e.EncryptUIAssetToken(payload)
	require.NoError(t, err)
	require.False(t, strings.HasPrefix(token, pluginConfigCipherPrefix))
	plaintext, err := e.DecryptUIAssetToken(token)
	require.NoError(t, err)
	require.Equal(t, payload, plaintext)
	require.Zero(t, checks)
	_, err = os.Lstat(e.keyPath)
	require.ErrorIs(t, err, os.ErrNotExist)

	// The short-lived token retains the existing shared-key semantics even
	// when the shared key is process-generated. It is not accepted as config.
	plaintext, err = e.fallback.Decrypt(token)
	require.NoError(t, err)
	require.Equal(t, payload, plaintext)
	_, err = e.Decrypt(token)
	require.ErrorContains(t, err, "original fixed server encryption key")

	// UI token creation does not repair an invalid persistent key either.
	require.NoError(t, os.MkdirAll(filepath.Dir(e.keyPath), 0o700))
	require.NoError(t, os.WriteFile(e.keyPath, []byte("broken key retained"), 0o600))
	_, err = e.EncryptUIAssetToken(payload)
	require.NoError(t, err)
	after, err := os.ReadFile(e.keyPath)
	require.NoError(t, err)
	require.Equal(t, "broken key retained", string(after))
	require.Zero(t, checks)
}

func TestPluginConfigExplicitRecoveryFailsClosedWhenIsolationCannotBeChecked(t *testing.T) {
	for _, condition := range []string{"database failure", "missing check", "invalid target"} {
		t.Run(condition, func(t *testing.T) {
			e := newTestPluginEncryptor(t)
			failure := errors.New("database unavailable")
			e.recoveryIsExclusive = func(int64) (bool, error) { return false, failure }
			id := int64(17)
			if condition == "missing check" {
				e.recoveryIsExclusive = nil
			} else if condition == "invalid target" {
				id = 0
			}
			_, err := e.EncryptAfterExplicitRecovery(id, `{}`)
			require.Error(t, err)
			if condition == "database failure" {
				require.ErrorIs(t, err, failure)
			}
			_, err = os.Lstat(e.keyPath)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestPluginConfigExplicitRecoveryChecksOtherStoredConfigurations(t *testing.T) {
	for _, tt := range []struct {
		name         string
		targetCipher string
		otherCipher  string
		missing      bool
		allowed      bool
	}{
		{name: "target is sole prefixed row", targetCipher: "plugin-config-v1:target", allowed: true},
		{name: "legacy target and no prefixed rows", targetCipher: "legacy-target", otherCipher: "legacy-other", allowed: true},
		{name: "other plugin uses ciphertext", targetCipher: "plugin-config-v1:target", otherCipher: "plugin-config-v1:other", allowed: false},
		{name: "legacy target cannot replace another key", targetCipher: "legacy-target", otherCipher: "plugin-config-v1:other", allowed: false},
		{name: "target does not exist", missing: true, allowed: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATA_DIR", t.TempDir())
			db, err := sql.Open("sqlite", ":memory:")
			require.NoError(t, err)
			defer db.Close()
			db.SetMaxOpenConns(1)
			_, err = db.Exec("CREATE TABLE sub2api_plugin_installations (id INTEGER PRIMARY KEY, config_encrypted TEXT NOT NULL DEFAULT '')")
			require.NoError(t, err)
			if !tt.missing {
				_, err = db.Exec("INSERT INTO sub2api_plugin_installations VALUES (17, $1)", tt.targetCipher)
				require.NoError(t, err)
			}
			if tt.otherCipher != "" {
				_, err = db.Exec("INSERT INTO sub2api_plugin_installations VALUES (18, $1)", tt.otherCipher)
				require.NoError(t, err)
			}
			provider := NewPluginConfigEncryptor(&config.Config{}, aesEncryptor(t), db).(*pluginConfigEncryptor)
			_, err = provider.EncryptAfterExplicitRecovery(17, `{"recovered":true}`)
			if tt.allowed {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "other plugin configurations")
				_, err = os.Lstat(provider.keyPath)
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

func TestPluginConfigEncryptionProviderChecksStoredVersionWithoutReadingCiphertext(t *testing.T) {
	for _, exists := range []bool{false, true} {
		t.Run(map[bool]string{false: "first use", true: "lost key"}[exists], func(t *testing.T) {
			t.Setenv("DATA_DIR", t.TempDir())
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer db.Close()
			mock.ExpectQuery("SELECT EXISTS .*config_encrypted LIKE 'plugin-config-v1:%'").WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(exists))
			cfg := &config.Config{Totp: config.TotpConfig{EncryptionKey: aesHexKey(32, 0x42), EncryptionKeyConfigured: true}}
			original := cfg.Totp
			e := NewPluginConfigEncryptor(cfg, aesEncryptor(t), db).(*pluginConfigEncryptor)
			require.Equal(t, filepath.Join(os.Getenv("DATA_DIR"), "secrets", "plugin-config.key"), e.keyPath)
			require.True(t, e.fixedLegacyKey)
			err = e.InitializePluginConfigEncryption()
			if exists {
				require.ErrorContains(t, err, "restore the original")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, original, cfg.Totp)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
