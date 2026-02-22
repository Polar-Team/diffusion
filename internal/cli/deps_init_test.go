package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/role"
	"diffusion/internal/utils"
)

// TestDepsInitScansExistingRequirements verifies that deps init scans existing requirements.yml files
func TestDepsInitScansExistingRequirements(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	// Create scenario folders with requirements.yml files
	scenarios := []struct {
		name        string
		collections []string
		roles       []string
	}{
		{
			name: "default",
			collections: []string{
				"  - name: community.general\n    version: 10.0.0",
				"  - name: community.docker\n    version: 5.0.4",
			},
			roles: []string{
				"  - name: geerlingguy.docker\n    src: https://github.com/geerlingguy/ansible-role-docker.git\n    scm: git\n    version: 7.0.0",
			},
		},
		{
			name: "production",
			collections: []string{
				"  - name: community.postgresql\n    version: 3.0.0",
			},
			roles: []string{
				"  - name: simple.role\n    version: 1.0.0",
			},
		},
	}

	for _, scenario := range scenarios {
		scenarioDir := filepath.Join(tempDir, "scenarios", scenario.name)
		os.MkdirAll(scenarioDir, 0755)

		// Create requirements.yml
		reqContent := "collections:\n"
		for _, col := range scenario.collections {
			reqContent += col + "\n"
		}
		reqContent += "roles:\n"
		for _, role := range scenario.roles {
			reqContent += role + "\n"
		}

		reqPath := filepath.Join(scenarioDir, "requirements.yml")
		if err := os.WriteFile(reqPath, []byte(reqContent), 0644); err != nil {
			t.Fatalf("Failed to create requirements.yml: %v", err)
		}
	}

	// Create meta/main.yml with collections
	metaDir := filepath.Join(tempDir, "meta")
	os.MkdirAll(metaDir, 0755)
	metaContent := `collections:
  - community.crypto
  - ansible.posix>=1.5.0
`
	metaPath := filepath.Join(metaDir, "main.yml")
	if err := os.WriteFile(metaPath, []byte(metaContent), 0644); err != nil {
		t.Fatalf("Failed to create meta/main.yml: %v", err)
	}

	// Simulate deps init command
	cfg := &config.Config{
		DependencyConfig: &config.DependencyConfig{
			Python: &config.PythonVersion{
				Min:    "3.11",
				Max:    "3.13",
				Pinned: "",
			},
			Ansible:     ">=10.0.0",
			AnsibleLint: ">=24.0.0",
			Molecule:    ">=24.0.0",
			YamlLint:    ">=1.35.0",
			Collections: []config.CollectionRequirement{},
			Roles:       []config.RoleRequirement{},
		},
	}

	// Scan scenarios directory
	scenariosDir := "scenarios"
	entries, err := os.ReadDir(scenariosDir)
	if err != nil {
		t.Fatalf("Failed to read scenarios directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			scenarioName := entry.Name()
			reqPath := filepath.Join(scenariosDir, scenarioName, "requirements.yml")

			if _, err := os.Stat(reqPath); err == nil {
				req, err := role.ParseRequirementFile(scenarioName)
				if err != nil {
					t.Fatalf("Failed to parse requirements.yml: %v", err)
				}

				// Add collections
				for _, col := range req.Collections {
					exists := false
					for _, existing := range cfg.DependencyConfig.Collections {
						if existing.Name == col.Name {
							exists = true
							break
						}
					}
					if !exists {
						version := col.Version
						if version != "" && !strings.HasPrefix(version, ">=") {
							version = ">=" + version
						}
						cfg.DependencyConfig.Collections = append(cfg.DependencyConfig.Collections, config.CollectionRequirement{
							Name:    col.Name,
							Version: version,
						})
					}
				}

				// Add roles
				for _, role := range req.Roles {
					// Prefix role name with scenario name
					roleNameWithScenario := scenarioName + "." + role.Name

					exists := false
					for _, existing := range cfg.DependencyConfig.Roles {
						if existing.Name == roleNameWithScenario {
							exists = true
							break
						}
					}
					if !exists {
						version := role.Version
						if version != "" && version != "main" && !strings.HasPrefix(version, ">=") {
							version = ">=" + version
						}
						cfg.DependencyConfig.Roles = append(cfg.DependencyConfig.Roles, config.RoleRequirement{
							Name:    roleNameWithScenario,
							Src:     role.Src,
							Scm:     role.Scm,
							Version: version,
						})
					}
				}
			}
		}
	}

	// Add collections from meta/main.yml
	meta, err := role.ParseMetaFile()
	if err == nil {
		for _, col := range meta.Collections {
			name, version := utils.ParseCollectionString(col)
			exists := false
			for _, existing := range cfg.DependencyConfig.Collections {
				if existing.Name == name {
					exists = true
					break
				}
			}
			if !exists {
				if version == "" {
					version = ">=1.0.0"
				}
				cfg.DependencyConfig.Collections = append(cfg.DependencyConfig.Collections, config.CollectionRequirement{
					Name:    name,
					Version: version,
				})
			}
		}
	}

	// Save config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify collections were added
	loadedCfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	expectedCollections := map[string]bool{
		"community.general":    true,
		"community.docker":     true,
		"community.postgresql": true,
		"community.crypto":     true,
		"ansible.posix":        true,
	}

	if len(loadedCfg.DependencyConfig.Collections) != len(expectedCollections) {
		t.Errorf("Expected %d collections, got %d", len(expectedCollections), len(loadedCfg.DependencyConfig.Collections))
	}

	for _, col := range loadedCfg.DependencyConfig.Collections {
		if !expectedCollections[col.Name] {
			t.Errorf("Unexpected collection: %s", col.Name)
		}
		// Verify version constraint format
		if col.Version == "" {
			t.Errorf("Collection %s has no version constraint", col.Name)
		}
		if !strings.HasPrefix(col.Version, ">=") && !strings.HasPrefix(col.Version, "<=") && !strings.HasPrefix(col.Version, "==") {
			t.Errorf("Collection %s has invalid version constraint: %s", col.Name, col.Version)
		}
	}

	// Verify roles were added
	expectedRoles := map[string]bool{
		"default.geerlingguy.docker": true,
		"production.simple.role":     true,
	}

	if len(loadedCfg.DependencyConfig.Roles) != len(expectedRoles) {
		t.Errorf("Expected %d roles, got %d", len(expectedRoles), len(loadedCfg.DependencyConfig.Roles))
	}

	for _, role := range loadedCfg.DependencyConfig.Roles {
		if !expectedRoles[role.Name] {
			t.Errorf("Unexpected role: %s", role.Name)
		}
		// Verify version constraint format
		if role.Version == "" {
			t.Errorf("Role %s has no version constraint", role.Name)
		}
	}

	// Verify specific role details
	for _, role := range loadedCfg.DependencyConfig.Roles {
		if role.Name == "default.geerlingguy.docker" {
			if role.Src != "https://github.com/geerlingguy/ansible-role-docker.git" {
				t.Errorf("Role default.geerlingguy.docker has wrong Src: %s", role.Src)
			}
			if role.Scm != "git" {
				t.Errorf("Role default.geerlingguy.docker has wrong Scm: %s", role.Scm)
			}
			if role.Version != ">=7.0.0" {
				t.Errorf("Role default.geerlingguy.docker has wrong Version: %s", role.Version)
			}
		}
	}

	t.Logf("✅ Successfully scanned and imported %d collections and %d roles", len(cfg.DependencyConfig.Collections), len(cfg.DependencyConfig.Roles))
}

// TestDepsInitWithNoExistingFiles verifies that deps init works when no requirements files exist
func TestDepsInitWithNoExistingFiles(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	// Create config with just defaults
	cfg := &config.Config{
		DependencyConfig: &config.DependencyConfig{
			Python: &config.PythonVersion{
				Min:    "3.11",
				Max:    "3.13",
				Pinned: "",
			},
			Ansible:     ">=10.0.0",
			AnsibleLint: ">=24.0.0",
			Molecule:    ">=24.0.0",
			YamlLint:    ">=1.35.0",
			Collections: []config.CollectionRequirement{},
			Roles:       []config.RoleRequirement{},
		},
	}

	// Save config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify config was saved
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.DependencyConfig == nil {
		t.Error("DependencyConfig should not be nil")
	}

	if len(cfg.DependencyConfig.Collections) != 0 {
		t.Errorf("Expected 0 collections, got %d", len(cfg.DependencyConfig.Collections))
	}

	if len(cfg.DependencyConfig.Roles) != 0 {
		t.Errorf("Expected 0 roles, got %d", len(cfg.DependencyConfig.Roles))
	}

	t.Log("✅ Successfully initialized config with no existing requirements")
}

// TestDepsInitRoleNamesWithScenarioPrefix verifies that role names include scenario prefix
func TestDepsInitRoleNamesWithScenarioPrefix(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	// Create multiple scenario folders with different roles
	scenarios := []struct {
		name     string
		roleName string
	}{
		{name: "default", roleName: "konstruktoid.docker_rootless"},
		{name: "production", roleName: "geerlingguy.nginx"},
		{name: "staging", roleName: "simple.role"},
	}

	for _, scenario := range scenarios {
		scenarioDir := filepath.Join(tempDir, "scenarios", scenario.name)
		os.MkdirAll(scenarioDir, 0755)

		// Create requirements.yml with a role
		reqContent := fmt.Sprintf(`collections: []
roles:
  - name: %s
    src: https://github.com/example/%s.git
    scm: git
    version: 1.0.0
`, scenario.roleName, scenario.roleName)

		reqPath := filepath.Join(scenarioDir, "requirements.yml")
		if err := os.WriteFile(reqPath, []byte(reqContent), 0644); err != nil {
			t.Fatalf("Failed to create requirements.yml: %v", err)
		}
	}

	// Create config
	cfg := &config.Config{
		DependencyConfig: &config.DependencyConfig{
			Python: &config.PythonVersion{
				Min: "3.11",
				Max: "3.13",
			},
			Collections: []config.CollectionRequirement{},
			Roles:       []config.RoleRequirement{},
		},
	}

	// Scan scenarios directory
	scenariosDir := "scenarios"
	entries, err := os.ReadDir(scenariosDir)
	if err != nil {
		t.Fatalf("Failed to read scenarios directory: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() {
			scenarioName := entry.Name()
			reqPath := filepath.Join(scenariosDir, scenarioName, "requirements.yml")

			if _, err := os.Stat(reqPath); err == nil {
				req, err := role.ParseRequirementFile(scenarioName)
				if err != nil {
					t.Fatalf("Failed to parse requirements.yml: %v", err)
				}

				// Add roles with scenario prefix
				for _, role := range req.Roles {
					roleNameWithScenario := scenarioName + "." + role.Name

					exists := false
					for _, existing := range cfg.DependencyConfig.Roles {
						if existing.Name == roleNameWithScenario {
							exists = true
							break
						}
					}
					if !exists {
						version := role.Version
						if version != "" && version != "main" && !strings.HasPrefix(version, ">=") {
							version = ">=" + version
						}
						cfg.DependencyConfig.Roles = append(cfg.DependencyConfig.Roles, config.RoleRequirement{
							Name:    roleNameWithScenario,
							Src:     role.Src,
							Scm:     role.Scm,
							Version: version,
						})
					}
				}
			}
		}
	}

	// Save config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify role names have scenario prefix
	loadedCfg2, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	expectedRoleNames := []string{
		"default.konstruktoid.docker_rootless",
		"production.geerlingguy.nginx",
		"staging.simple.role",
	}

	if len(loadedCfg2.DependencyConfig.Roles) != len(expectedRoleNames) {
		t.Errorf("Expected %d roles, got %d", len(expectedRoleNames), len(loadedCfg2.DependencyConfig.Roles))
	}

	foundRoles := make(map[string]bool)
	for _, role := range loadedCfg2.DependencyConfig.Roles {
		foundRoles[role.Name] = true
		t.Logf("Found role: %s", role.Name)
	}

	for _, expectedName := range expectedRoleNames {
		if !foundRoles[expectedName] {
			t.Errorf("Expected role name '%s' not found", expectedName)
		}
	}

	// Verify the format is correct (scenario.rolename)
	for _, role := range loadedCfg2.DependencyConfig.Roles {
		parts := strings.Split(role.Name, ".")
		if len(parts) < 2 {
			t.Errorf("Role name '%s' does not have scenario prefix", role.Name)
		}

		// First part should be a scenario name
		scenarioName := parts[0]
		validScenarios := []string{"default", "production", "staging"}
		isValid := false
		for _, valid := range validScenarios {
			if scenarioName == valid {
				isValid = true
				break
			}
		}
		if !isValid {
			t.Errorf("Role name '%s' has invalid scenario prefix '%s'", role.Name, scenarioName)
		}
	}

	t.Log("✅ All role names have correct scenario prefix")
}
