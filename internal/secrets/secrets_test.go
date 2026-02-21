package secrets

import (
	"testing"
	"os"
	"strings"

	"diffusion/internal/config"
)

func TestGetEncryptionKey(t *testing.T) {
	key, err := getEncryptionKey()
	if err != nil {
		t.Fatalf("getEncryptionKey failed: %v", err)
	}

	if len(key) != 32 {
		t.Errorf("expected key length 32, got %d", len(key))
	}

	// Key should be deterministic for same machine/user
	key2, err := getEncryptionKey()
	if err != nil {
		t.Fatalf("getEncryptionKey failed on second call: %v", err)
	}

	if string(key) != string(key2) {
		t.Error("encryption key should be deterministic")
	}
}

func TestEncryptDecryptData(t *testing.T) {
	key, err := getEncryptionKey()
	if err != nil {
		t.Fatal(err)
	}

	testData := []byte("sensitive credential data")

	// Encrypt
	encrypted, err := encryptData(testData, key)
	if err != nil {
		t.Fatalf("encryptData failed: %v", err)
	}

	if encrypted == string(testData) {
		t.Error("encrypted data should not match plaintext")
	}

	// Decrypt
	decrypted, err := decryptData(encrypted, key)
	if err != nil {
		t.Fatalf("decryptData failed: %v", err)
	}

	if string(decrypted) != string(testData) {
		t.Errorf("decrypted data mismatch: got %q, want %q", decrypted, testData)
	}
}

func TestEncryptDecryptDifferentKeys(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	key2[0] = 1 // Make it different

	testData := []byte("test data")

	encrypted, err := encryptData(testData, key1)
	if err != nil {
		t.Fatal(err)
	}

	// Try to decrypt with wrong key
	_, err = decryptData(encrypted, key2)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestSaveAndLoadArtifactCredentials(t *testing.T) {
	creds := &config.ArtifactCredentials{
		Name:     "test-source",
		URL:      "https://test.example.com",
		Username: "testuser",
		Token:    "testtoken123",
	}

	// Save credentials
	err := SaveArtifactCredentials(creds)
	if err != nil {
		t.Fatalf("SaveArtifactCredentials failed: %v", err)
	}

	// Clean up after test
	defer DeleteArtifactCredentials(creds.Name)

	// Load credentials
	loaded, err := LoadArtifactCredentials(creds.Name)
	if err != nil {
		t.Fatalf("LoadArtifactCredentials failed: %v", err)
	}

	// Verify
	if loaded.Name != creds.Name {
		t.Errorf("name mismatch: got %q, want %q", loaded.Name, creds.Name)
	}
	if loaded.URL != creds.URL {
		t.Errorf("URL mismatch: got %q, want %q", loaded.URL, creds.URL)
	}
	if loaded.Username != creds.Username {
		t.Errorf("username mismatch: got %q, want %q", loaded.Username, creds.Username)
	}
	if loaded.Token != creds.Token {
		t.Errorf("token mismatch: got %q, want %q", loaded.Token, creds.Token)
	}
}

func TestLoadNonExistentCredentials(t *testing.T) {
	_, err := LoadArtifactCredentials("nonexistent-source")
	if err == nil {
		t.Error("expected error when loading nonexistent credentials")
	}
}

func TestDeleteArtifactCredentials(t *testing.T) {
	creds := &config.ArtifactCredentials{
		Name:     "test-delete",
		URL:      "https://test.example.com",
		Username: "testuser",
		Token:    "testtoken",
	}

	// Save credentials
	err := SaveArtifactCredentials(creds)
	if err != nil {
		t.Fatal(err)
	}

	// Verify they exist
	_, err = LoadArtifactCredentials(creds.Name)
	if err != nil {
		t.Fatalf("credentials should exist: %v", err)
	}

	// Delete credentials
	err = DeleteArtifactCredentials(creds.Name)
	if err != nil {
		t.Fatalf("DeleteArtifactCredentials failed: %v", err)
	}

	// Verify they're gone
	_, err = LoadArtifactCredentials(creds.Name)
	if err == nil {
		t.Error("credentials should not exist after deletion")
	}
}

func TestListStoredCredentials(t *testing.T) {
	// Create test credentials
	creds1 := &config.ArtifactCredentials{
		Name:     "test-list-1",
		URL:      "https://test1.example.com",
		Username: "user1",
		Token:    "token1",
	}
	creds2 := &config.ArtifactCredentials{
		Name:     "test-list-2",
		URL:      "https://test2.example.com",
		Username: "user2",
		Token:    "token2",
	}

	// Save credentials
	if err := SaveArtifactCredentials(creds1); err != nil {
		t.Fatal(err)
	}
	defer DeleteArtifactCredentials(creds1.Name)

	if err := SaveArtifactCredentials(creds2); err != nil {
		t.Fatal(err)
	}
	defer DeleteArtifactCredentials(creds2.Name)

	// List credentials
	sources, err := ListStoredCredentials()
	if err != nil {
		t.Fatalf("ListStoredCredentials failed: %v", err)
	}

	// Verify both are in the list
	found1, found2 := false, false
	for _, source := range sources {
		if source == creds1.Name {
			found1 = true
		}
		if source == creds2.Name {
			found2 = true
		}
	}

	if !found1 {
		t.Errorf("expected to find %q in list", creds1.Name)
	}
	if !found2 {
		t.Errorf("expected to find %q in list", creds2.Name)
	}
}

func TestGetSecretsDir(t *testing.T) {
	dir, err := getSecretsDir()
	if err != nil {
		t.Fatalf("getSecretsDir failed: %v", err)
	}

	// Verify directory exists
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("secrets directory should exist: %v", err)
	}

	if !info.IsDir() {
		t.Error("secrets path should be a directory")
	}

	// Verify permissions (Unix-like systems)
	if info.Mode().Perm() != 0700 {
		t.Logf("warning: secrets directory permissions are %o, expected 0700", info.Mode().Perm())
	}
}

func TestGetSecretFilePath(t *testing.T) {
	path, err := getSecretFilePath("test-source")
	if err != nil {
		t.Fatalf("getSecretFilePath failed: %v", err)
	}

	if path == "" {
		t.Error("secret file path should not be empty")
	}

	// Should contain secrets directory and source name
	if !strings.Contains(path, "secrets") {
		t.Errorf("path should contain 'secrets' directory, got %q", path)
	}

	// Should end with the source name
	expectedSuffix := "test-source"
	if len(path) < len(expectedSuffix) || path[len(path)-len(expectedSuffix):] != expectedSuffix {
		t.Errorf("path should end with %q, got %q", expectedSuffix, path)
	}
}

func TestArtifactCredentialsRoundTrip(t *testing.T) {
	testCases := []struct {
		name  string
		creds *config.ArtifactCredentials
	}{
		{
			name: "basic credentials",
			creds: &config.ArtifactCredentials{
				Name:     "basic-test",
				URL:      "https://basic.example.com",
				Username: "basicuser",
				Token:    "basictoken",
			},
		},
		{
			name: "special characters",
			creds: &config.ArtifactCredentials{
				Name:     "special-test",
				URL:      "https://special.example.com/path?query=value",
				Username: "user@example.com",
				Token:    "token!@#$%^&*()",
			},
		},
		{
			name: "long token",
			creds: &config.ArtifactCredentials{
				Name:     "long-test",
				URL:      "https://long.example.com",
				Username: "longuser",
				Token:    "verylongtokenverylongtokenverylongtokenverylongtokenverylongtoken",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Save
			err := SaveArtifactCredentials(tc.creds)
			if err != nil {
				t.Fatalf("SaveArtifactCredentials failed: %v", err)
			}
			defer DeleteArtifactCredentials(tc.creds.Name)

			// Load
			loaded, err := LoadArtifactCredentials(tc.creds.Name)
			if err != nil {
				t.Fatalf("LoadArtifactCredentials failed: %v", err)
			}

			// Compare
			if loaded.Name != tc.creds.Name {
				t.Errorf("name mismatch")
			}
			if loaded.URL != tc.creds.URL {
				t.Errorf("URL mismatch")
			}
			if loaded.Username != tc.creds.Username {
				t.Errorf("username mismatch")
			}
			if loaded.Token != tc.creds.Token {
				t.Errorf("token mismatch")
			}
		})
	}
}
