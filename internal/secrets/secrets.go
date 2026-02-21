package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"diffusion/internal/config"
	"diffusion/internal/role"
)

// getEncryptionKey generates a unique encryption key based on computer name and username
func getEncryptionKey() ([]byte, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}

	username := os.Getenv("USER")
	if username == "" {
		username = os.Getenv("USERNAME") // Windows fallback
	}
	if username == "" {
		return nil, fmt.Errorf("failed to get username")
	}

	// Create a unique string combining hostname and username
	uniqueString := fmt.Sprintf("%s:%s:diffusion-artifact-secrets", hostname, username)

	// Hash it to get a 32-byte key for AES-256
	hash := sha256.Sum256([]byte(uniqueString))
	return hash[:], nil
}

// encryptData encrypts data using AES-256-GCM
func encryptData(plaintext []byte, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptData decrypts data using AES-256-GCM
func decryptData(ciphertext string, key []byte) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// getSecretsDir returns the directory for storing encrypted secrets
func getSecretsDir() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	// Get current role name
	roleName := GetCurrentRoleName()
	if roleName == "" {
		roleName = "default"
	}

	secretsDir := filepath.Join(homeDir, ".diffusion", "secrets", roleName)
	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		return "", fmt.Errorf("failed to create secrets directory: %w", err)
	}

	return secretsDir, nil
}

// getCurrentRoleName returns the current role name from meta/main.yml or empty string
func GetCurrentRoleName() string {
	meta, _, err := role.LoadRoleConfig("")
	if err != nil {
		return ""
	}
	if meta != nil && meta.GalaxyInfo.RoleName != "" {
		return meta.GalaxyInfo.RoleName
	}
	return ""
}

// getSecretFilePath returns the path to the encrypted secret file for a given artifact source
func getSecretFilePath(sourceName string) (string, error) {
	secretsDir, err := getSecretsDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(secretsDir, sourceName), nil
}

// SaveArtifactCredentials saves credentials to encrypted local storage
func SaveArtifactCredentials(creds *config.ArtifactCredentials) error {
	key, err := getEncryptionKey()
	if err != nil {
		return fmt.Errorf("failed to get encryption key: %w", err)
	}

	// Marshal credentials to JSON
	jsonData, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Encrypt the JSON data
	encrypted, err := encryptData(jsonData, key)
	if err != nil {
		return fmt.Errorf("failed to encrypt credentials: %w", err)
	}

	// Get the file path
	filePath, err := getSecretFilePath(creds.Name)
	if err != nil {
		return err
	}

	// Write to file
	if err := os.WriteFile(filePath, []byte(encrypted), 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	return nil
}

// LoadArtifactCredentials loads credentials from encrypted local storage
func LoadArtifactCredentials(sourceName string) (*config.ArtifactCredentials, error) {
	key, err := getEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}

	// Get the file path
	filePath, err := getSecretFilePath(sourceName)
	if err != nil {
		return nil, err
	}

	// Read the encrypted file
	encrypted, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("credentials not found for source '%s'", sourceName)
		}
		return nil, fmt.Errorf("failed to read credentials file: %w", err)
	}

	// Decrypt the data
	decrypted, err := decryptData(string(encrypted), key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt credentials: %w", err)
	}

	// Unmarshal JSON
	var creds config.ArtifactCredentials
	if err := json.Unmarshal(decrypted, &creds); err != nil {
		return nil, fmt.Errorf("failed to unmarshal credentials: %w", err)
	}

	return &creds, nil
}

// DeleteArtifactCredentials removes stored credentials for a source
func DeleteArtifactCredentials(sourceName string) error {
	filePath, err := getSecretFilePath(sourceName)
	if err != nil {
		return err
	}

	if err := os.Remove(filePath); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("credentials not found for source '%s'", sourceName)
		}
		return fmt.Errorf("failed to delete credentials: %w", err)
	}

	return nil
}

// ListStoredCredentials returns a list of artifact sources with stored credentials
func ListStoredCredentials() ([]string, error) {
	secretsDir, err := getSecretsDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(secretsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read secrets directory: %w", err)
	}

	var sources []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Each file is a source name (no suffix anymore)
		sources = append(sources, entry.Name())
	}

	return sources, nil
}

// GetArtifactCredentialsFromVault retrieves credentials from HashiCorp Vault
func GetArtifactCredentialsFromVault(source *config.ArtifactSource, vaultConfig *config.HashicorpVault) (*config.ArtifactCredentials, error) {
	if !source.UseVault {
		return nil, fmt.Errorf("vault not enabled for source '%s'", source.Name)
	}

	if vaultConfig == nil || !vaultConfig.HashicorpVaultIntegration {
		return nil, fmt.Errorf("vault integration not configured")
	}

	// Use the vault_client function from vault.go
	result := vault_client(context.TODO(), source.VaultPath, source.VaultSecretName)
	if result == nil {
		return nil, fmt.Errorf("failed to retrieve credentials from vault")
	}

	// Use per-source field names
	usernameField := source.VaultUsernameField
	if usernameField == "" {
		usernameField = "username" // default
	}

	tokenField := source.VaultTokenField
	if tokenField == "" {
		tokenField = "token" // default
	}

	username, ok := result.Data.Data[usernameField].(string)
	if !ok {
		return nil, fmt.Errorf("username field '%s' not found in vault secret", usernameField)
	}

	token, ok := result.Data.Data[tokenField].(string)
	if !ok {
		return nil, fmt.Errorf("token field '%s' not found in vault secret", tokenField)
	}

	return &config.ArtifactCredentials{
		Name:     source.Name,
		URL:      source.URL,
		Username: username,
		Token:    token,
	}, nil
}

// GetArtifactCredentials retrieves credentials from either Vault or local storage
func GetArtifactCredentials(source *config.ArtifactSource, vaultConfig *config.HashicorpVault) (*config.ArtifactCredentials, error) {
	if source.UseVault {
		return GetArtifactCredentialsFromVault(source, vaultConfig)
	}
	return LoadArtifactCredentials(source.Name)
}
