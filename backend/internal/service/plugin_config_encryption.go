package service

// PluginConfigEncryptor keeps native plugin configuration readable across host
// restarts without changing the shared TOTP or account credential keys.
type PluginConfigEncryptor interface {
	SecretEncryptor
	InitializePluginConfigEncryption() error
	EncryptAfterExplicitRecovery(installationID int64, plaintext string) (string, error)
}

// PluginUIAssetEncryptor separates short-lived UI capability tokens from the
// persistent configuration key, so the configuration recovery UI stays usable.
type PluginUIAssetEncryptor interface {
	EncryptUIAssetToken(plaintext string) (string, error)
	DecryptUIAssetToken(ciphertext string) (string, error)
}
