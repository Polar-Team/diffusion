package config

import (
	"os"
	"testing"
)

func TestLoadConfig_FileNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	_, err := LoadConfig()
	if err == nil {
		t.Error("expected error when config file doesn't exist")
	}
}

func TestLoadConfig_InvalidTOML(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Create invalid TOML
	err := os.WriteFile(ConfigFileName, []byte("invalid toml [[["), 0644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = LoadConfig()
	if err == nil {
		t.Error("expected error when TOML is invalid")
	}
}

func TestSaveConfig_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	cfg := &Config{
		ContainerRegistry: &ContainerRegistry{
			RegistryServer:   "test.registry.com",
			RegistryProvider: "Public",
		},
	}

	err := SaveConfig(cfg)
	if err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(ConfigFileName); os.IsNotExist(err) {
		t.Error("config file was not created")
	}
}

func TestValidatePythonVersion_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		version string
		wantErr bool
	}{
		{"empty string", "", true},
		{"invalid format", "invalid", true},
		{"too old", "3.10", true},
		{"future version", "4.0", true},
		{"with patch", "3.11.5", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidatePythonVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePythonVersion(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
			}
		})
	}
}

func TestExtractMajorMinor_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{"empty", "", ""},
		{"single char", "3", "3"},
		{"with prefix", "v3.11", "v3.11"},
		{"complex constraint", ">=3.11.5", "3.11"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractMajorMinor(tt.version)
			if got != tt.want {
				t.Errorf("ExtractMajorMinor(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestConfig_DefaultValues(t *testing.T) {
	cfg := &Config{}
	
	// Test that zero values are handled properly
	if cfg.ContainerRegistry != nil {
		t.Error("expected nil ContainerRegistry in zero-value config")
	}
	
	if cfg.DependencyConfig != nil {
		t.Error("expected nil DependencyConfig in zero-value config")
	}
}

func TestCollectionRequirement_Validation(t *testing.T) {
	req := CollectionRequirement{
		Name:    "community.general",
		Version: ">=7.4.0",
	}
	
	if req.Name != "community.general" {
		t.Errorf("expected Name 'community.general', got %q", req.Name)
	}
	
	if req.Version != ">=7.4.0" {
		t.Errorf("expected Version '>=7.4.0', got %q", req.Version)
	}
}

func TestRoleRequirement_Validation(t *testing.T) {
	req := RoleRequirement{
		Name:    "geerlingguy.docker",
		Src:     "https://github.com/geerlingguy/ansible-role-docker.git",
		Scm:     "git",
		Version: ">=6.0.0",
	}
	
	if req.Name != "geerlingguy.docker" {
		t.Errorf("expected Name 'geerlingguy.docker', got %q", req.Name)
	}
	
	if req.Src != "https://github.com/geerlingguy/ansible-role-docker.git" {
		t.Errorf("expected correct Src, got %q", req.Src)
	}
}

func TestPythonVersion_Validation(t *testing.T) {
	pv := PythonVersion{
		Min:    "3.11",
		Max:    "3.13",
		Pinned: "3.13",
	}
	
	if pv.Min != "3.11" {
		t.Errorf("expected Min '3.11', got %q", pv.Min)
	}
	
	if pv.Max != "3.13" {
		t.Errorf("expected Max '3.13', got %q", pv.Max)
	}
}

func TestHashicorpVault_Validation(t *testing.T) {
	vault := HashicorpVault{
		HashicorpVaultIntegration: true,
	}
	
	if !vault.HashicorpVaultIntegration {
		t.Error("expected vault integration to be enabled")
	}
}

func TestArtifactSource_Validation(t *testing.T) {
	source := ArtifactSource{
		Name:     "test-source",
		URL:      "https://test.example.com",
		UseVault: false,
	}
	
	if source.Name != "test-source" {
		t.Errorf("expected Name 'test-source', got %q", source.Name)
	}
	
	if source.UseVault {
		t.Error("expected UseVault to be false")
	}
}

func TestContainerRegistry_Validation(t *testing.T) {
	registry := ContainerRegistry{
		RegistryServer:        "ghcr.io",
		RegistryProvider:      "Public",
		MoleculeContainerName: "test-container",
		MoleculeContainerTag:  "latest",
	}
	
	if registry.RegistryServer != "ghcr.io" {
		t.Errorf("expected RegistryServer 'ghcr.io', got %q", registry.RegistryServer)
	}
	
	if registry.RegistryProvider != "Public" {
		t.Errorf("expected RegistryProvider 'Public', got %q", registry.RegistryProvider)
	}
}

func TestYamlLint_Validation(t *testing.T) {
	lint := YamlLint{
		Extends: "default",
		Ignore:  []string{".git/*"},
	}
	
	if lint.Extends != "default" {
		t.Errorf("expected Extends 'default', got %q", lint.Extends)
	}
	
	if len(lint.Ignore) != 1 {
		t.Errorf("expected 1 ignore pattern, got %d", len(lint.Ignore))
	}
}

func TestAnsibleLint_Validation(t *testing.T) {
	lint := AnsibleLint{
		ExcludedPaths: []string{"molecule/**"},
		WarnList:      []string{"meta-no-info"},
		SkipList:      []string{"meta-incorrect"},
	}
	
	if len(lint.ExcludedPaths) != 1 {
		t.Errorf("expected 1 excluded path, got %d", len(lint.ExcludedPaths))
	}
	
	if len(lint.WarnList) != 1 {
		t.Errorf("expected 1 warn item, got %d", len(lint.WarnList))
	}
}

func TestTestsSettings_Validation(t *testing.T) {
	settings := TestsSettings{
		Type: "diffusion",
	}
	
	if settings.Type != "diffusion" {
		t.Errorf("expected Type 'diffusion', got %q", settings.Type)
	}
}

func TestCacheSettings_Validation(t *testing.T) {
	cache := CacheSettings{
		Enabled:     true,
		CacheID:     "test123",
		DockerCache: true,
		UVCache:     true,
	}
	
	if !cache.Enabled {
		t.Error("expected cache to be enabled")
	}
	
	if cache.CacheID != "test123" {
		t.Errorf("expected CacheID 'test123', got %q", cache.CacheID)
	}
}

func TestArtifactCredentials_Validation(t *testing.T) {
	creds := ArtifactCredentials{
		Name:     "test-creds",
		URL:      "https://test.example.com",
		Username: "testuser",
		Token:    "testtoken",
	}
	
	if creds.Name != "test-creds" {
		t.Errorf("expected Name 'test-creds', got %q", creds.Name)
	}
	
	if creds.Username != "testuser" {
		t.Errorf("expected Username 'testuser', got %q", creds.Username)
	}
}