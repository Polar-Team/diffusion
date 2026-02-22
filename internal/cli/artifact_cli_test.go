package cli

import (
	"os"
	"testing"

	"diffusion/internal/config"
)

// TestArtifactAddToConfig tests that artifact add command adds source to config
func TestArtifactAddToConfig(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)

	// Change to temp directory
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Create a minimal config
	cfg := &config.Config{
		ContainerRegistry: &config.ContainerRegistry{
			RegistryServer:        "ghcr.io",
			RegistryProvider:      "Public",
			MoleculeContainerName: "test",
			MoleculeContainerTag:  "latest",
		},
		HashicorpVault: &config.HashicorpVault{
			HashicorpVaultIntegration: false,
		},
		ArtifactSources: []config.ArtifactSource{},
		YamlLintConfig: &config.YamlLint{
			Extends: "default",
			Ignore:  []string{},
			Rules:   &config.YamlLintRules{},
		},
		AnsibleLintConfig: &config.AnsibleLint{
			ExcludedPaths: []string{},
			WarnList:      []string{},
			SkipList:      []string{},
		},
		TestsConfig: &config.TestsSettings{
			Type: "diffusion",
		},
	}

	// Save initial config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	// Simulate adding an artifact source (local storage)
	newSource := config.ArtifactSource{
		Name:     "test-source",
		URL:      "https://test.example.com",
		UseVault: false,
	}

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Add source
	cfg.ArtifactSources = append(cfg.ArtifactSources, newSource)

	// Save config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save config with new source: %v", err)
	}

	// Reload and verify
	recfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	if len(recfg.ArtifactSources) != 1 {
		t.Errorf("Expected 1 artifact source, got %d", len(recfg.ArtifactSources))
	}

	if recfg.ArtifactSources[0].Name != "test-source" {
		t.Errorf("Expected source name 'test-source', got '%s'", recfg.ArtifactSources[0].Name)
	}

	if recfg.ArtifactSources[0].URL != "https://test.example.com" {
		t.Errorf("Expected URL 'https://test.example.com', got '%s'", recfg.ArtifactSources[0].URL)
	}
}

// TestArtifactRemoveFromConfig tests that artifact remove command removes source from config
func TestArtifactRemoveFromConfig(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)

	// Change to temp directory
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Create a config with artifact sources
	cfg := &config.Config{
		ContainerRegistry: &config.ContainerRegistry{
			RegistryServer:        "ghcr.io",
			RegistryProvider:      "Public",
			MoleculeContainerName: "test",
			MoleculeContainerTag:  "latest",
		},
		HashicorpVault: &config.HashicorpVault{
			HashicorpVaultIntegration: false,
		},
		ArtifactSources: []config.ArtifactSource{
			{
				Name:     "source1",
				URL:      "https://source1.example.com",
				UseVault: false,
			},
			{
				Name:     "source2",
				URL:      "https://source2.example.com",
				UseVault: false,
			},
		},
		YamlLintConfig: &config.YamlLint{
			Extends: "default",
			Ignore:  []string{},
			Rules:   &config.YamlLintRules{},
		},
		AnsibleLintConfig: &config.AnsibleLint{
			ExcludedPaths: []string{},
			WarnList:      []string{},
			SkipList:      []string{},
		},
		TestsConfig: &config.TestsSettings{
			Type: "diffusion",
		},
	}

	// Save initial config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Remove source1
	for i, source := range cfg.ArtifactSources {
		if source.Name == "source1" {
			cfg.ArtifactSources = append(cfg.ArtifactSources[:i], cfg.ArtifactSources[i+1:]...)
			break
		}
	}

	// Save config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save config after removal: %v", err)
	}

	// Reload and verify
	recfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	if len(recfg.ArtifactSources) != 1 {
		t.Errorf("Expected 1 artifact source after removal, got %d", len(recfg.ArtifactSources))
	}

	if recfg.ArtifactSources[0].Name != "source2" {
		t.Errorf("Expected remaining source to be 'source2', got '%s'", recfg.ArtifactSources[0].Name)
	}
}

// TestArtifactUpdateInConfig tests that updating an existing source works
func TestArtifactUpdateInConfig(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)

	// Change to temp directory
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Create a config with an artifact source
	cfg := &config.Config{
		ContainerRegistry: &config.ContainerRegistry{
			RegistryServer:        "ghcr.io",
			RegistryProvider:      "Public",
			MoleculeContainerName: "test",
			MoleculeContainerTag:  "latest",
		},
		HashicorpVault: &config.HashicorpVault{
			HashicorpVaultIntegration: false,
		},
		ArtifactSources: []config.ArtifactSource{
			{
				Name:     "test-source",
				URL:      "https://old-url.example.com",
				UseVault: false,
			},
		},
		YamlLintConfig: &config.YamlLint{
			Extends: "default",
			Ignore:  []string{},
			Rules:   &config.YamlLintRules{},
		},
		AnsibleLintConfig: &config.AnsibleLint{
			ExcludedPaths: []string{},
			WarnList:      []string{},
			SkipList:      []string{},
		},
		TestsConfig: &config.TestsSettings{
			Type: "diffusion",
		},
	}

	// Save initial config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Update the source
	for i, source := range cfg.ArtifactSources {
		if source.Name == "test-source" {
			cfg.ArtifactSources[i].URL = "https://new-url.example.com"
			cfg.ArtifactSources[i].UseVault = true
			cfg.ArtifactSources[i].VaultPath = "secret/data/test"
			cfg.ArtifactSources[i].VaultSecretName = "test-secret"
			cfg.ArtifactSources[i].VaultUsernameField = "user"
			cfg.ArtifactSources[i].VaultTokenField = "pass"
			break
		}
	}

	// Save config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save updated config: %v", err)
	}

	// Reload and verify
	recfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	if len(recfg.ArtifactSources) != 1 {
		t.Errorf("Expected 1 artifact source, got %d", len(recfg.ArtifactSources))
	}

	source := recfg.ArtifactSources[0]
	if source.URL != "https://new-url.example.com" {
		t.Errorf("Expected updated URL 'https://new-url.example.com', got '%s'", source.URL)
	}

	if !source.UseVault {
		t.Error("Expected UseVault to be true")
	}

	if source.VaultPath != "secret/data/test" {
		t.Errorf("Expected VaultPath 'secret/data/test', got '%s'", source.VaultPath)
	}

	if source.VaultUsernameField != "user" {
		t.Errorf("Expected VaultUsernameField 'user', got '%s'", source.VaultUsernameField)
	}

	if source.VaultTokenField != "pass" {
		t.Errorf("Expected VaultTokenField 'pass', got '%s'", source.VaultTokenField)
	}
}

// TestArtifactVaultSourceInConfig tests that Vault-based sources are saved correctly
func TestArtifactVaultSourceInConfig(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)

	// Change to temp directory
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}

	// Create a minimal config
	cfg := &config.Config{
		ContainerRegistry: &config.ContainerRegistry{
			RegistryServer:        "ghcr.io",
			RegistryProvider:      "Public",
			MoleculeContainerName: "test",
			MoleculeContainerTag:  "latest",
		},
		HashicorpVault: &config.HashicorpVault{
			HashicorpVaultIntegration: true,
		},
		ArtifactSources: []config.ArtifactSource{},
		YamlLintConfig: &config.YamlLint{
			Extends: "default",
			Ignore:  []string{},
			Rules:   &config.YamlLintRules{},
		},
		AnsibleLintConfig: &config.AnsibleLint{
			ExcludedPaths: []string{},
			WarnList:      []string{},
			SkipList:      []string{},
		},
		TestsConfig: &config.TestsSettings{
			Type: "diffusion",
		},
	}

	// Save initial config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	// Add a Vault-based source
	vaultSource := config.ArtifactSource{
		Name:               "vault-source",
		URL:                "https://vault.example.com",
		UseVault:           true,
		VaultPath:          "secret/data/artifacts",
		VaultSecretName:    "vault-source",
		VaultUsernameField: "git_user",
		VaultTokenField:    "git_token",
	}

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Add source
	cfg.ArtifactSources = append(cfg.ArtifactSources, vaultSource)

	// Save config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save config with Vault source: %v", err)
	}

	// Reload and verify
	recfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	if len(recfg.ArtifactSources) != 1 {
		t.Errorf("Expected 1 artifact source, got %d", len(recfg.ArtifactSources))
	}

	source := recfg.ArtifactSources[0]
	if !source.UseVault {
		t.Error("Expected UseVault to be true")
	}

	if source.VaultPath != "secret/data/artifacts" {
		t.Errorf("Expected VaultPath 'secret/data/artifacts', got '%s'", source.VaultPath)
	}

	if source.VaultSecretName != "vault-source" {
		t.Errorf("Expected VaultSecretName 'vault-source', got '%s'", source.VaultSecretName)
	}

	if source.VaultUsernameField != "git_user" {
		t.Errorf("Expected VaultUsernameField 'git_user', got '%s'", source.VaultUsernameField)
	}

	if source.VaultTokenField != "git_token" {
		t.Errorf("Expected VaultTokenField 'git_token', got '%s'", source.VaultTokenField)
	}
}
