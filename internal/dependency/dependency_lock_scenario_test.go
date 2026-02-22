package dependency

import (
	"os"
	"testing"
)

func TestLoadDependencyConfigWithScenarioRoles(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Create a test diffusion.toml with scenario-prefixed roles
	configContent := `
[dependencies]
ansible = ">=10.0.0"
molecule = ">=24.0.0"

[[dependencies.roles]]
Name = "default.simple_role"
Src = "https://github.com/geerlingguy/ansible-role-docker.git"
Scm = "git"
Version = "main"

[[dependencies.collections]]
Name = "community.general"
Version = ">=7.4.0"
`

	err := os.WriteFile("diffusion.toml", []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Load the config
	depConfig, err := LoadDependencyConfig()
	if err != nil {
		t.Fatalf("LoadDependencyConfig() error = %v", err)
	}

	// Verify roles are loaded with scenario prefix stripped
	if len(depConfig.Roles) != 1 {
		t.Errorf("Expected 1 role, got %d", len(depConfig.Roles))
	}

	// Check role (simple role with scenario prefix stripped)
	if depConfig.Roles[0].Name != "simple_role" {
		t.Errorf("Expected role name 'simple_role', got '%s'", depConfig.Roles[0].Name)
	}

	// Verify collections are loaded correctly
	if len(depConfig.Collections) != 1 {
		t.Errorf("Expected 1 collection, got %d", len(depConfig.Collections))
	}

	if depConfig.Collections[0].Name != "community.general" {
		t.Errorf("Expected collection name 'community.general', got '%s'", depConfig.Collections[0].Name)
	}
}

func TestUpdateLockFileWithScenarioRoles(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Create a test diffusion.toml with scenario-prefixed roles
	configContent := `
[dependencies]
ansible = ">=10.0.0"

[[dependencies.roles]]
Name = "default.test_role"
Src = "https://github.com/geerlingguy/ansible-role-docker.git"
Scm = "git"
Version = ">=6.0.0"

[[dependencies.collections]]
Name = "community.general"
Version = ">=7.4.0"
`

	err := os.WriteFile("diffusion.toml", []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Update lock file
	err = UpdateLockFile()
	if err != nil {
		t.Fatalf("UpdateLockFile() error = %v", err)
	}

	// Load and verify lock file
	lockFile, err := LoadLockFile()
	if err != nil {
		t.Fatalf("LoadLockFile() error = %v", err)
	}

	// Verify role in lock file has scenario prefix stripped
	if len(lockFile.Roles) != 1 {
		t.Errorf("Expected 1 role in lock file, got %d", len(lockFile.Roles))
	}

	if lockFile.Roles[0].Name != "test_role" {
		t.Errorf("Expected role name 'test_role' in lock file, got '%s'", lockFile.Roles[0].Name)
	}

	// Verify collection in lock file
	if len(lockFile.Collections) != 1 {
		t.Errorf("Expected 1 collection in lock file, got %d", len(lockFile.Collections))
	}

	if lockFile.Collections[0].Name != "community.general" {
		t.Errorf("Expected collection name 'community.general' in lock file, got '%s'", lockFile.Collections[0].Name)
	}
}

func TestRoleNamespacePreservation(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Create a test diffusion.toml with galaxy role (no src)
	configContent := `
[dependencies]
ansible = ">=10.0.0"

[[dependencies.roles]]
Name = "default.geerlingguy.docker"
Version = ">=6.0.0"
`

	err := os.WriteFile("diffusion.toml", []byte(configContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test config: %v", err)
	}

	// Load the config
	depConfig, err := LoadDependencyConfig()
	if err != nil {
		t.Fatalf("LoadDependencyConfig() error = %v", err)
	}

	// For galaxy roles without src, the scenario prefix should be stripped
	// and namespace should be preserved
	if len(depConfig.Roles) != 1 {
		t.Errorf("Expected 1 role, got %d", len(depConfig.Roles))
	}

	// Should strip "default." and keep "geerlingguy.docker"
	if depConfig.Roles[0].Name != "geerlingguy.docker" {
		t.Errorf("Expected role name 'geerlingguy.docker', got '%s'", depConfig.Roles[0].Name)
	}
}
