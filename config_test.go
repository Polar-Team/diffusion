package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Change to temp directory
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Test loading non-existent config
	_, err = LoadConfig()
	if err == nil {
		t.Error("expected error when config doesn't exist, got nil")
	}

	// Create a valid config file
	configContent := `url = "https://example.com"

[container_registry]
registry_server = "cr.example.com"
registry_provider = "YC"
molecule_container_name = "molecule"
molecule_container_tag = "latest"

[vault]
enabled = true
secret_kv2_path = "secret/data/test"
username_field = "user"
token_field = "token"

[yaml_lint]
extends = "default"
ignore = [".git/*"]

[ansible_lint]
exclude_paths = ["molecule/**"]
warn_list = ["meta-no-info"]
skip_list = ["meta-incorrect"]

[tests]
type = "diffusion"
`
	configPath := filepath.Join(tmpDir, "diffusion.toml")
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test loading valid config
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	// Verify config values
	if config.ContainerRegistry == nil {
		t.Fatal("ContainerRegistry is nil")
	}

	if config.ContainerRegistry.RegistryServer != "cr.example.com" {
		t.Errorf("unexpected registry server: got %q, want %q",
			config.ContainerRegistry.RegistryServer, "cr.example.com")
	}

	if config.ContainerRegistry.RegistryProvider != "YC" {
		t.Errorf("unexpected registry provider: got %q, want %q",
			config.ContainerRegistry.RegistryProvider, "YC")
	}

	if config.HashicorpVault == nil {
		t.Fatal("HashicorpVault is nil")
	}

	if !config.HashicorpVault.HashicorpVaultIntegration {
		t.Error("expected vault integration to be enabled")
	}

}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create a test config
	config := &Config{
		ContainerRegistry: &ContainerRegistry{
			RegistryServer:        "test.registry.com",
			RegistryProvider:      "Public",
			MoleculeContainerName: "test-molecule",
			MoleculeContainerTag:  "v1.0",
		},
		HashicorpVault: &HashicorpVault{
			HashicorpVaultIntegration: false,
		},
		ArtifactSources: []ArtifactSource{
			{
				Name:     "test-source",
				URL:      "https://test.example.com",
				UseVault: false,
			},
		},
		YamlLintConfig: &YamlLint{
			Extends: "default",
			Ignore:  []string{".git/*"},
			Rules: &YamlLintRules{
				CommentsIdentation: false,
			},
		},
		AnsibleLintConfig: &AnsibleLint{
			ExcludedPaths: []string{"test/*"},
			WarnList:      []string{"test-warn"},
			SkipList:      []string{"test-skip"},
		},
		TestsConfig: &TestsSettings{
			Type: "local",
		},
	}

	// Save config
	if err := SaveConfig(config); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify file was created
	configPath := filepath.Join(tmpDir, "diffusion.toml")
	if !exists(configPath) {
		t.Error("config file was not created")
	}

	// Load and verify
	loadedConfig, err := LoadConfig()
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}

	if loadedConfig.ContainerRegistry.RegistryServer != config.ContainerRegistry.RegistryServer {
		t.Errorf("registry server mismatch: got %q, want %q",
			loadedConfig.ContainerRegistry.RegistryServer,
			config.ContainerRegistry.RegistryServer)
	}
}

func BenchmarkLoadConfig(b *testing.B) {
	tmpDir := b.TempDir()

	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	configContent := `
[container_registry]
registry_server = "cr.example.com"
registry_provider = "YC"
molecule_container_name = "molecule"
molecule_container_tag = "latest"

[vault]
enabled = false

url = "https://example.com"

[yaml_lint]
extends = "default"
ignore = []

[ansible_lint]
exclude_paths = []
warn_list = []
skip_list = []

[tests]
type = "diffusion"
`
	os.WriteFile(filepath.Join(tmpDir, "diffusion.toml"), []byte(configContent), 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = LoadConfig()
	}
}
