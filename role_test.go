package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMetaFile(t *testing.T) {
	tmpDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create meta directory
	metaDir := filepath.Join(tmpDir, "meta")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a valid meta/main.yml
	metaContent := `---
galaxy_info:
  role_name: test_role
  namespace: test_namespace
  author: Test Author
  description: Test Description
  company: Test Company
  license: MIT
  min_ansible_version: "2.10"
  platforms:
    - name: Ubuntu
      versions:
        - "20.04"
        - "22.04"
  galaxy_tags:
    - test
    - example
collections:
  - community.general
  - ansible.posix
`
	metaPath := filepath.Join(metaDir, "main.yml")
	if err := os.WriteFile(metaPath, []byte(metaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test parsing
	meta, err := ParseMetaFile()
	if err != nil {
		t.Fatalf("ParseMetaFile failed: %v", err)
	}

	// Verify parsed values
	if meta.GalaxyInfo.RoleName != "test_role" {
		t.Errorf("unexpected role name: got %q, want %q", meta.GalaxyInfo.RoleName, "test_role")
	}

	if meta.GalaxyInfo.Namespace != "test_namespace" {
		t.Errorf("unexpected namespace: got %q, want %q", meta.GalaxyInfo.Namespace, "test_namespace")
	}

	if len(meta.GalaxyInfo.Platforms) != 1 {
		t.Errorf("unexpected platforms count: got %d, want 1", len(meta.GalaxyInfo.Platforms))
	}

	if len(meta.Collections) != 2 {
		t.Errorf("unexpected collections count: got %d, want 2", len(meta.Collections))
	}
}

func TestParseRequirementFile(t *testing.T) {
	tmpDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create scenarios directory
	scenariosDir := filepath.Join(tmpDir, "scenarios", "default")
	if err := os.MkdirAll(scenariosDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a valid requirements.yml
	reqContent := `---
collections:
  - community.general
  - ansible.posix
roles:
  - name: geerlingguy.nginx
    src: https://github.com/geerlingguy/ansible-role-nginx.git
    version: main
    scm: git
`
	reqPath := filepath.Join(scenariosDir, "requirements.yml")
	if err := os.WriteFile(reqPath, []byte(reqContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test parsing
	req, err := ParseRequirementFile("default")
	if err != nil {
		t.Fatalf("ParseRequirementFile failed: %v", err)
	}

	// Verify parsed values
	if len(req.Collections) != 2 {
		t.Errorf("unexpected collections count: got %d, want 2", len(req.Collections))
	}

	if len(req.Roles) != 1 {
		t.Errorf("unexpected roles count: got %d, want 1", len(req.Roles))
	}

	if req.Roles[0].Name != "geerlingguy.nginx" {
		t.Errorf("unexpected role name: got %q, want %q", req.Roles[0].Name, "geerlingguy.nginx")
	}
}

func TestSaveMetaFile(t *testing.T) {
	tmpDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create meta directory
	metaDir := filepath.Join(tmpDir, "meta")
	if err := os.MkdirAll(metaDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test meta
	meta := &Meta{
		GalaxyInfo: &GalaxyInfo{
			RoleName:          "test_role",
			Namespace:         "test_ns",
			Author:            "Test Author",
			Description:       "Test Description",
			Company:           "Test Co",
			License:           "MIT",
			MinAnsibleVersion: "2.10",
			Platforms: []Platform{
				{
					OsName:   "Ubuntu",
					Versions: []string{"20.04", "22.04"},
				},
			},
			GalaxyTags: []string{"test", "example"},
		},
		Collections: []string{"community.general"},
	}

	// Save meta file
	if err := SaveMetaFile(meta); err != nil {
		t.Fatalf("SaveMetaFile failed: %v", err)
	}

	// Verify file was created
	metaPath := filepath.Join(metaDir, "main.yml")
	if !exists(metaPath) {
		t.Error("meta file was not created")
	}

	// Load and verify
	loadedMeta, err := ParseMetaFile()
	if err != nil {
		t.Fatalf("failed to load saved meta: %v", err)
	}

	if loadedMeta.GalaxyInfo.RoleName != meta.GalaxyInfo.RoleName {
		t.Errorf("role name mismatch: got %q, want %q",
			loadedMeta.GalaxyInfo.RoleName, meta.GalaxyInfo.RoleName)
	}
}

func TestSaveRequirementFile(t *testing.T) {
	tmpDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create scenarios directory
	scenariosDir := filepath.Join(tmpDir, "scenarios", "default")
	if err := os.MkdirAll(scenariosDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create test requirement
	req := &Requirement{
		Collections: []string{"community.general", "ansible.posix"},
		Roles: []RequirementRole{
			{
				Name:    "test.role",
				Src:     "https://github.com/test/role.git",
				Version: "main",
				Scm:     "git",
			},
		},
	}

	// Save requirement file
	if err := SaveRequirementFile(req, "default"); err != nil {
		t.Fatalf("SaveRequirementFile failed: %v", err)
	}

	// Verify file was created
	reqPath := filepath.Join(scenariosDir, "requirements.yml")
	if !exists(reqPath) {
		t.Error("requirements file was not created")
	}

	// Load and verify
	loadedReq, err := ParseRequirementFile("default")
	if err != nil {
		t.Fatalf("failed to load saved requirements: %v", err)
	}

	if len(loadedReq.Collections) != len(req.Collections) {
		t.Errorf("collections count mismatch: got %d, want %d",
			len(loadedReq.Collections), len(req.Collections))
	}

	if len(loadedReq.Roles) != len(req.Roles) {
		t.Errorf("roles count mismatch: got %d, want %d",
			len(loadedReq.Roles), len(req.Roles))
	}
}

func TestLoadRoleConfig(t *testing.T) {
	tmpDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldWd)

	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}

	// Create directory structure
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
  author: Test
  description: Test
  company: Test
  license: MIT
  min_ansible_version: "2.10"
  platforms: []
  galaxy_tags: []
collections: []
`
	if err := os.WriteFile(filepath.Join(metaDir, "main.yml"), []byte(metaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create requirements file
	reqContent := `---
collections: []
roles: []
`
	if err := os.WriteFile(filepath.Join(scenariosDir, "requirements.yml"), []byte(reqContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Test loading
	meta, req, err := LoadRoleConfig("default")
	if err != nil {
		t.Fatalf("LoadRoleConfig failed: %v", err)
	}

	if meta == nil {
		t.Error("expected meta to be non-nil")
	}

	if req == nil {
		t.Error("expected req to be non-nil")
	}

	if meta.GalaxyInfo.RoleName != "test_role" {
		t.Errorf("unexpected role name: got %q, want %q", meta.GalaxyInfo.RoleName, "test_role")
	}
}
