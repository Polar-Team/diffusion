package cli

import (
	"fmt"
	"os"
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/secrets"
)

// TestIndexedEnvironmentVariables tests setting indexed GIT environment variables
func TestIndexedEnvironmentVariables(t *testing.T) {
	// Clean up any existing variables
	defer func() {
		for i := 1; i <= config.MaxArtifactSources; i++ {
			os.Unsetenv(fmt.Sprintf("%s%d", config.EnvGitUserPrefix, i))
			os.Unsetenv(fmt.Sprintf("%s%d", config.EnvGitPassPrefix, i))
			os.Unsetenv(fmt.Sprintf("%s%d", config.EnvGitURLPrefix, i))
		}
	}()

	// Test setting multiple artifact sources
	testSources := []struct {
		index    int
		username string
		password string
		url      string
	}{
		{1, "user1", "pass1", "https://repo1.example.com"},
		{2, "user2", "pass2", "https://repo2.example.com"},
		{3, "user3", "pass3", "https://repo3.example.com"},
	}

	for _, ts := range testSources {
		os.Setenv(fmt.Sprintf("%s%d", config.EnvGitUserPrefix, ts.index), ts.username)
		os.Setenv(fmt.Sprintf("%s%d", config.EnvGitPassPrefix, ts.index), ts.password)
		os.Setenv(fmt.Sprintf("%s%d", config.EnvGitURLPrefix, ts.index), ts.url)
	}

	// Verify all variables are set correctly
	for _, ts := range testSources {
		gotUser := os.Getenv(fmt.Sprintf("%s%d", config.EnvGitUserPrefix, ts.index))
		if gotUser != ts.username {
			t.Errorf("GIT_USER_%d: got %q, want %q", ts.index, gotUser, ts.username)
		}

		gotPass := os.Getenv(fmt.Sprintf("%s%d", config.EnvGitPassPrefix, ts.index))
		if gotPass != ts.password {
			t.Errorf("GIT_PASSWORD_%d: got %q, want %q", ts.index, gotPass, ts.password)
		}

		gotURL := os.Getenv(fmt.Sprintf("%s%d", config.EnvGitURLPrefix, ts.index))
		if gotURL != ts.url {
			t.Errorf("GIT_URL_%d: got %q, want %q", ts.index, gotURL, ts.url)
		}
	}
}

// TestMaxArtifactSources verifies the maximum number of sources
func TestMaxArtifactSources(t *testing.T) {
	if config.MaxArtifactSources < 1 {
		t.Error("config.MaxArtifactSources should be at least 1")
	}

	if config.MaxArtifactSources > 100 {
		t.Error("config.MaxArtifactSources seems unreasonably high")
	}

	// Verify we can set variables up to the max
	defer func() {
		for i := 1; i <= config.MaxArtifactSources; i++ {
			os.Unsetenv(fmt.Sprintf("%s%d", config.EnvGitUserPrefix, i))
		}
	}()

	for i := 1; i <= config.MaxArtifactSources; i++ {
		varName := fmt.Sprintf("%s%d", config.EnvGitUserPrefix, i)
		testValue := fmt.Sprintf("user%d", i)
		os.Setenv(varName, testValue)

		got := os.Getenv(varName)
		if got != testValue {
			t.Errorf("failed to set %s: got %q, want %q", varName, got, testValue)
		}
	}
}

// TestEnvironmentVariableConstants verifies the constant values
func TestEnvironmentVariableConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"config.EnvGitUserPrefix", config.EnvGitUserPrefix, "GIT_USER_"},
		{"config.EnvGitPassPrefix", config.EnvGitPassPrefix, "GIT_PASSWORD_"},
		{"config.EnvGitURLPrefix", config.EnvGitURLPrefix, "GIT_URL_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s: got %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

// TestGetArtifactCredentialsIntegration tests the full flow
func TestGetArtifactCredentialsIntegration(t *testing.T) {
	// Create test credentials
	creds := &config.ArtifactCredentials{
		Name:     "test-integration",
		URL:      "https://test-integration.example.com",
		Username: "testuser",
		Token:    "testtoken",
	}

	// Save credentials
	err := secrets.SaveArtifactCredentials(creds)
	if err != nil {
		t.Fatalf("SaveArtifactCredentials failed: %v", err)
	}
	defer secrets.DeleteArtifactCredentials(creds.Name)

	// Create artifact source (local storage)
	source := &config.ArtifactSource{
		Name:     creds.Name,
		URL:      creds.URL,
		UseVault: false,
	}

	// Get credentials
	retrieved, err := secrets.GetArtifactCredentials(source, nil)
	if err != nil {
		t.Fatalf("GetArtifactCredentials failed: %v", err)
	}

	// Verify
	if retrieved.Username != creds.Username {
		t.Errorf("username mismatch: got %q, want %q", retrieved.Username, creds.Username)
	}
	if retrieved.Token != creds.Token {
		t.Errorf("token mismatch: got %q, want %q", retrieved.Token, creds.Token)
	}
	if retrieved.URL != creds.URL {
		t.Errorf("URL mismatch: got %q, want %q", retrieved.URL, creds.URL)
	}
}

// TestMultipleArtifactSourcesSimulation simulates loading multiple sources
func TestMultipleArtifactSourcesSimulation(t *testing.T) {
	// Clean up
	defer func() {
		for i := 1; i <= 3; i++ {
			os.Unsetenv(fmt.Sprintf("%s%d", config.EnvGitUserPrefix, i))
			os.Unsetenv(fmt.Sprintf("%s%d", config.EnvGitPassPrefix, i))
			os.Unsetenv(fmt.Sprintf("%s%d", config.EnvGitURLPrefix, i))
		}
	}()

	// Create multiple test sources
	sources := []struct {
		name     string
		url      string
		username string
		token    string
	}{
		{"source1", "https://repo1.example.com", "user1", "token1"},
		{"source2", "https://repo2.example.com", "user2", "token2"},
		{"source3", "https://repo3.example.com", "user3", "token3"},
	}

	// Save all credentials
	for _, s := range sources {
		creds := &config.ArtifactCredentials{
			Name:     s.name,
			URL:      s.url,
			Username: s.username,
			Token:    s.token,
		}
		if err := secrets.SaveArtifactCredentials(creds); err != nil {
			t.Fatalf("failed to save credentials for %s: %v", s.name, err)
		}
		defer secrets.DeleteArtifactCredentials(s.name)
	}

	// Simulate loading and setting environment variables
	for i, s := range sources {
		index := i + 1
		source := &config.ArtifactSource{
			Name:     s.name,
			URL:      s.url,
			UseVault: false,
		}

		creds, err := secrets.GetArtifactCredentials(source, nil)
		if err != nil {
			t.Fatalf("failed to get credentials for %s: %v", s.name, err)
		}

		// Set environment variables
		os.Setenv(fmt.Sprintf("%s%d", config.EnvGitUserPrefix, index), creds.Username)
		os.Setenv(fmt.Sprintf("%s%d", config.EnvGitPassPrefix, index), creds.Token)
		os.Setenv(fmt.Sprintf("%s%d", config.EnvGitURLPrefix, index), creds.URL)
	}

	// Verify all environment variables are set correctly
	for i, s := range sources {
		index := i + 1
		gotUser := os.Getenv(fmt.Sprintf("%s%d", config.EnvGitUserPrefix, index))
		if gotUser != s.username {
			t.Errorf("GIT_USER_%d: got %q, want %q", index, gotUser, s.username)
		}

		gotPass := os.Getenv(fmt.Sprintf("%s%d", config.EnvGitPassPrefix, index))
		if gotPass != s.token {
			t.Errorf("GIT_PASSWORD_%d: got %q, want %q", index, gotPass, s.token)
		}

		gotURL := os.Getenv(fmt.Sprintf("%s%d", config.EnvGitURLPrefix, index))
		if gotURL != s.url {
			t.Errorf("GIT_URL_%d: got %q, want %q", index, gotURL, s.url)
		}
	}
}
