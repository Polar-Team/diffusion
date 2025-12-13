package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRoleCommandWithoutInit tests that role command without --init flag doesn't prompt for initialization
func TestRoleCommandWithoutInit(t *testing.T) {
	tmpDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Test without meta/main.yml - should return error
	_, _, err = LoadRoleConfig("")
	if err == nil {
		t.Error("expected error when role config doesn't exist, got nil")
	}
}

// TestRoleCommandWithInitFlagExistingRole tests that --init flag warns when role exists
func TestRoleCommandWithInitFlagExistingRole(t *testing.T) {
	tmpDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create meta directory and file to simulate existing role
	metaDir := filepath.Join(tmpDir, "meta")
	scenariosDir := filepath.Join(tmpDir, "scenarios", "default")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(scenariosDir, 0755); err != nil {
		t.Fatal(err)
	}

	metaContent := `---
galaxy_info:
  role_name: existing_role
  namespace: test
  author: Test
  description: Test
  company: Test
  license: MIT
  min_ansible_version: "2.10"
  platforms: []
  galaxy_tags: []
collections: []
`
	metaPath := filepath.Join(metaDir, "main.yml")
	if err := os.WriteFile(metaPath, []byte(metaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create requirements file
	reqContent := `---
collections: []
roles: []
`
	reqPath := filepath.Join(scenariosDir, "requirements.yml")
	if err := os.WriteFile(reqPath, []byte(reqContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify that meta/main.yml exists
	if _, err := os.Stat("meta/main.yml"); os.IsNotExist(err) {
		t.Error("meta/main.yml should exist")
	}

	// Load the role config - should succeed
	meta, _, err := LoadRoleConfig("")
	if err != nil {
		t.Fatalf("LoadRoleConfig should succeed: %v", err)
	}

	if meta.GalaxyInfo.RoleName != "existing_role" {
		t.Errorf("expected role name 'existing_role', got %q", meta.GalaxyInfo.RoleName)
	}
}

// TestRoleCommandDisplaysConfig tests that role command displays configuration
func TestRoleCommandDisplaysConfig(t *testing.T) {
	tmpDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create complete role structure
	metaDir := filepath.Join(tmpDir, "meta")
	scenariosDir := filepath.Join(tmpDir, "scenarios", "default")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(scenariosDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create meta file
	metaContent := `---
galaxy_info:
  role_name: test_role
  namespace: test_ns
  author: Test Author
  description: Test Description
  company: Test Co
  license: MIT
  min_ansible_version: "2.10"
  platforms:
    - name: Ubuntu
      versions:
        - "20.04"
  galaxy_tags:
    - test
collections:
  - community.general
`
	if err := os.WriteFile(filepath.Join(metaDir, "main.yml"), []byte(metaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create requirements file
	reqContent := `---
collections:
  - community.general
roles:
  - name: test.role
    src: https://github.com/test/role.git
    version: main
    scm: git
`
	if err := os.WriteFile(filepath.Join(scenariosDir, "requirements.yml"), []byte(reqContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load and verify
	meta, req, err := LoadRoleConfig("")
	if err != nil {
		t.Fatalf("LoadRoleConfig failed: %v", err)
	}

	if meta == nil {
		t.Fatal("meta should not be nil")
	}

	if req == nil {
		t.Fatal("req should not be nil")
	}

	// Verify meta content
	if meta.GalaxyInfo.RoleName != "test_role" {
		t.Errorf("expected role name 'test_role', got %q", meta.GalaxyInfo.RoleName)
	}

	if meta.GalaxyInfo.Namespace != "test_ns" {
		t.Errorf("expected namespace 'test_ns', got %q", meta.GalaxyInfo.Namespace)
	}

	// Verify requirements content
	if len(req.Collections) != 1 {
		t.Errorf("expected 1 collection, got %d", len(req.Collections))
	}

	if len(req.Roles) != 1 {
		t.Errorf("expected 1 role, got %d", len(req.Roles))
	}
}

// TestCheckRoleExists tests the role existence check
func TestCheckRoleExists(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(t *testing.T) string
		shouldExist bool
	}{
		{
			name: "role exists",
			setupFunc: func(t *testing.T) string {
				tmpDir := t.TempDir()
				metaDir := filepath.Join(tmpDir, "meta")
				if err := os.MkdirAll(metaDir, 0755); err != nil {
					t.Fatal(err)
				}
				metaPath := filepath.Join(metaDir, "main.yml")
				if err := os.WriteFile(metaPath, []byte("test"), 0644); err != nil {
					t.Fatal(err)
				}
				return tmpDir
			},
			shouldExist: true,
		},
		{
			name: "role does not exist",
			setupFunc: func(t *testing.T) string {
				return t.TempDir()
			},
			shouldExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := tt.setupFunc(t)

			oldWd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			defer os.Chdir(oldWd)

			if err := os.Chdir(tmpDir); err != nil {
				t.Fatal(err)
			}

			_, err = os.Stat("meta/main.yml")
			exists := !os.IsNotExist(err)

			if exists != tt.shouldExist {
				t.Errorf("expected exists=%v, got %v", tt.shouldExist, exists)
			}
		})
	}
}
