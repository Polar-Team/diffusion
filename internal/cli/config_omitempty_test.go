package cli

import (
	"os"
	"strings"
	"testing"

	"diffusion/internal/config"
)

// TestOmitEmptyFieldsInToml verifies that empty fields are not written to diffusion.toml
func TestOmitEmptyFieldsInToml(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	// Create config with collections that have empty Source and SourceURL
	cfg := &config.Config{
		DependencyConfig: &config.DependencyConfig{
			Python: &config.PythonVersion{
				Min:    "3.11",
				Max:    "3.13",
				Pinned: "", // Empty, should be omitted
			},
			Collections: []config.CollectionRequirement{
				{
					Name:      "community.docker",
					Version:   "<=5.0.4",
					Source:    "", // Empty, should be omitted
					SourceURL: "", // Empty, should be omitted
				},
				{
					Name:      "community.general",
					Version:   "<=12.2.0",
					Source:    "", // Empty, should be omitted
					SourceURL: "", // Empty, should be omitted
				},
			},
		},
	}

	// Save config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Read the file content
	content, err := os.ReadFile("diffusion.toml")
	if err != nil {
		t.Fatalf("Failed to read diffusion.toml: %v", err)
	}

	contentStr := string(content)

	// Verify that empty fields are NOT present
	if strings.Contains(contentStr, `Pinned = ""`) {
		t.Error("Empty Pinned field should not be written to TOML")
	}

	if strings.Contains(contentStr, `Source = ""`) {
		t.Error("Empty Source field should not be written to TOML")
	}

	if strings.Contains(contentStr, `SourceURL = ""`) {
		t.Error("Empty SourceURL field should not be written to TOML")
	}

	// Verify that non-empty fields ARE present
	if !strings.Contains(contentStr, `Min = "3.11"`) {
		t.Error("Non-empty Min field should be written to TOML")
	}

	if !strings.Contains(contentStr, `Max = "3.13"`) {
		t.Error("Non-empty Max field should be written to TOML")
	}

	if !strings.Contains(contentStr, `Name = "community.docker"`) {
		t.Error("Collection name should be written to TOML")
	}

	if !strings.Contains(contentStr, `Version = "<=5.0.4"`) {
		t.Error("Collection version should be written to TOML")
	}

	t.Logf("Generated TOML content:\n%s", contentStr)
}

// TestOmitEmptyRoleFields verifies that empty role fields are not written to diffusion.toml
func TestOmitEmptyRoleFields(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	// Create config with roles that have some empty fields
	cfg := &config.Config{
		DependencyConfig: &config.DependencyConfig{
			Python: &config.PythonVersion{
				Min: "3.11",
				Max: "3.13",
			},
			Roles: []config.RoleRequirement{
				{
					Name:    "geerlingguy.docker",
					Src:     "https://github.com/geerlingguy/ansible-role-docker.git",
					Scm:     "git",
					Version: ">=7.0.0",
				},
				{
					Name:    "simple.role",
					Version: "1.0.0",
					// Src and Scm are empty, should be omitted
				},
			},
		},
	}

	// Save config
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Read the file content
	content, err := os.ReadFile("diffusion.toml")
	if err != nil {
		t.Fatalf("Failed to read diffusion.toml: %v", err)
	}

	contentStr := string(content)

	// Count occurrences of Src and Scm fields
	srcCount := strings.Count(contentStr, "Src = ")
	scmCount := strings.Count(contentStr, "Scm = ")

	// Should only have 1 Src and 1 Scm (from the first role)
	if srcCount != 1 {
		t.Errorf("Expected 1 Src field, found %d", srcCount)
	}

	if scmCount != 1 {
		t.Errorf("Expected 1 Scm field, found %d", scmCount)
	}

	// Verify the role with all fields is present
	if !strings.Contains(contentStr, `Name = "geerlingguy.docker"`) {
		t.Error("First role name should be written to TOML")
	}

	if !strings.Contains(contentStr, `Src = "https://github.com/geerlingguy/ansible-role-docker.git"`) {
		t.Error("First role Src should be written to TOML")
	}

	// Verify the role with only name and version is present
	if !strings.Contains(contentStr, `Name = "simple.role"`) {
		t.Error("Second role name should be written to TOML")
	}

	if !strings.Contains(contentStr, `Version = "1.0.0"`) {
		t.Error("Second role version should be written to TOML")
	}

	t.Logf("Generated TOML content:\n%s", contentStr)
}
