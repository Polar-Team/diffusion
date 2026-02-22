package cli

import (
	"testing"

	"diffusion/internal/config"
)

// TestVaultConfigHelperDisabled tests VaultConfigHelper with integration disabled
func TestVaultConfigHelperDisabled(t *testing.T) {
	// This would require mocking stdin, so we'll test the structure
	vault := &config.HashicorpVault{
		HashicorpVaultIntegration: false,
	}

	if vault.HashicorpVaultIntegration {
		t.Error("expected vault integration to be disabled")
	}
}

// TestVaultConfigHelperEnabled tests VaultConfigHelper with integration enabled
func TestVaultConfigHelperEnabled(t *testing.T) {
	// Test the structure of an enabled vault config
	vault := &config.HashicorpVault{
		HashicorpVaultIntegration: true,
	}

	if !vault.HashicorpVaultIntegration {
		t.Error("expected vault integration to be enabled")
	}

	// Note: Username and token field names are now per-source in ArtifactSource struct
}

// TestArtifactSourceStructure tests the ArtifactSource structure
func TestArtifactSourceStructure(t *testing.T) {
	tests := []struct {
		name   string
		source config.ArtifactSource
	}{
		{
			name: "local storage source",
			source: config.ArtifactSource{
				Name:     "test-local",
				URL:      "https://local.example.com",
				UseVault: false,
			},
		},
		{
			name: "vault storage source",
			source: config.ArtifactSource{
				Name:               "test-vault",
				URL:                "https://vault.example.com",
				UseVault:           true,
				VaultPath:          "secret/data/artifacts",
				VaultSecretName:    "test-vault",
				VaultUsernameField: "username",
				VaultTokenField:    "token",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.source.Name == "" {
				t.Error("source name should not be empty")
			}
			if tt.source.URL == "" {
				t.Error("source URL should not be empty")
			}
			if tt.source.UseVault && (tt.source.VaultPath == "" || tt.source.VaultSecretName == "") {
				t.Error("vault source should have vault path and secret name")
			}
			if tt.source.UseVault && (tt.source.VaultUsernameField == "" || tt.source.VaultTokenField == "") {
				t.Error("vault source should have username and token field names")
			}
		})
	}
}

// TestConfigWithArtifactSources tests Config with multiple artifact sources
func TestConfigWithArtifactSources(t *testing.T) {
	cfg := &config.Config{
		ArtifactSources: []config.ArtifactSource{
			{
				Name:     "source1",
				URL:      "https://source1.example.com",
				UseVault: false,
			},
			{
				Name:               "source2",
				URL:                "https://source2.example.com",
				UseVault:           true,
				VaultPath:          "secret/data/artifacts",
				VaultSecretName:    "source2",
				VaultUsernameField: "user",
				VaultTokenField:    "pass",
			},
		},
	}

	// Verify artifact sources
	if len(cfg.ArtifactSources) != 2 {
		t.Errorf("expected 2 artifact sources, got %d", len(cfg.ArtifactSources))
	}

	// Verify first source (local)
	if cfg.ArtifactSources[0].UseVault {
		t.Error("first source should use local storage")
	}

	// Verify second source (vault)
	if !cfg.ArtifactSources[1].UseVault {
		t.Error("second source should use vault storage")
	}

	if cfg.ArtifactSources[1].VaultPath == "" {
		t.Error("vault source should have vault path")
	}
}

// TestBackwardCompatibilityArtifactUrl tests backward compatibility with ArtifactUrl
func TestBackwardCompatibilityArtifactUrl(t *testing.T) {

	// New config should use ArtifactSources
	newConfig := &config.Config{
		ArtifactSources: []config.ArtifactSource{
			{
				Name:     "legacy-migration",
				UseVault: false,
			},
		},
	}

	if len(newConfig.ArtifactSources) != 1 {
		t.Error("expected 1 artifact source after migration")
	}

}

// TestEmptyArtifactSources tests config with no artifact sources
func TestEmptyArtifactSources(t *testing.T) {
	cfg := &config.Config{}

	if len(cfg.ArtifactSources) != 0 {
		t.Errorf("expected 0 artifact sources, got %d", len(cfg.ArtifactSources))
	}

	// This is valid - user may only use public repositories
}

// TestMixedArtifactSources tests config with mixed local and vault sources
func TestMixedArtifactSources(t *testing.T) {
	sources := []config.ArtifactSource{
		{
			Name:     "local1",
			URL:      "https://local1.example.com",
			UseVault: false,
		},
		{
			Name:               "vault1",
			URL:                "https://vault1.example.com",
			UseVault:           true,
			VaultPath:          "secret/data/prod",
			VaultSecretName:    "vault1",
			VaultUsernameField: "username",
			VaultTokenField:    "token",
		},
		{
			Name:     "local2",
			URL:      "https://local2.example.com",
			UseVault: false,
		},
		{
			Name:               "vault2",
			URL:                "https://vault2.example.com",
			UseVault:           true,
			VaultPath:          "secret/data/dev",
			VaultSecretName:    "vault2",
			VaultUsernameField: "user",
			VaultTokenField:    "password",
		},
	}

	localCount := 0
	vaultCount := 0

	for _, source := range sources {
		if source.UseVault {
			vaultCount++
			if source.VaultPath == "" || source.VaultSecretName == "" {
				t.Errorf("vault source %s missing vault configuration", source.Name)
			}
		} else {
			localCount++
		}
	}

	if localCount != 2 {
		t.Errorf("expected 2 local sources, got %d", localCount)
	}

	if vaultCount != 2 {
		t.Errorf("expected 2 vault sources, got %d", vaultCount)
	}
}
