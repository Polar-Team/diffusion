package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDepsSyncCommand(t *testing.T) {
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

	// Create a mock lock file
	lockContent := `version: "1.0"
generated: "2024-01-01T00:00:00Z"
hash: "test-hash"
python:
  min: "3.11"
  max: "3.13"
  pinned: "3.13"
collections:
  - name: community.general
    version: ">=7.4.0"
    resolved_version: "12.2.0"
    type: collection
  - name: community.docker
    version: ">=3.0.0"
    resolved_version: "5.0.4"
    type: collection
roles:
  - name: geerlingguy.docker
    version: ">=7.0.0"
    resolved_version: "7.4.1"
    type: role
tools:
  - name: ansible
    version: ">=10.0.0"
    resolved_version: "13.1.0"
    type: tool
`
	if err := os.WriteFile(LockFileName, []byte(lockContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create initial meta file
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

	// Create initial requirements file
	reqContent := `---
collections: []
roles: []
`
	if err := os.WriteFile(filepath.Join(scenariosDir, "requirements.yml"), []byte(reqContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load lock file
	lockFile, err := LoadLockFile()
	if err != nil {
		t.Fatalf("LoadLockFile failed: %v", err)
	}

	// Load role config
	meta, req, err := LoadRoleConfig("default")
	if err != nil {
		t.Fatalf("LoadRoleConfig failed: %v", err)
	}

	// Simulate sync logic
	// Sync collections to requirements.yml (resolved versions)
	req.Collections = []RequirementCollection{}
	for _, col := range lockFile.Collections {
		version := col.ResolvedVersion
		if version == "" {
			version = col.Version
		}
		req.Collections = append(req.Collections, RequirementCollection{
			Name:    col.Name,
			Version: version,
		})
	}

	// Sync collections to meta.yml (with operators)
	meta.Collections = []string{}
	for _, col := range lockFile.Collections {
		collectionStr := col.Name
		if col.Version != "" && col.Version != "latest" {
			// Check if version already has an operator
			if strings.ContainsAny(col.Version, ">=<") {
				collectionStr = col.Name + col.Version
			} else {
				// No operator, add >=
				collectionStr = col.Name + ">=" + col.Version
			}
		}
		meta.Collections = append(meta.Collections, collectionStr)
	}

	// Sync roles
	req.Roles = []RequirementRole{}
	for _, role := range lockFile.Roles {
		version := role.ResolvedVersion
		if version == "" {
			version = role.Version
		}
		req.Roles = append(req.Roles, RequirementRole{
			Name:    role.Name,
			Version: version,
		})
	}

	// Save files
	if err := SaveRequirementFile(req, "default"); err != nil {
		t.Fatalf("SaveRequirementFile failed: %v", err)
	}

	if err := SaveMetaFile(meta); err != nil {
		t.Fatalf("SaveMetaFile failed: %v", err)
	}

	// Reload and verify
	meta2, req2, err := LoadRoleConfig("default")
	if err != nil {
		t.Fatalf("LoadRoleConfig failed: %v", err)
	}

	// Verify requirements.yml has resolved versions (no operators)
	if len(req2.Collections) != 2 {
		t.Errorf("Expected 2 collections in requirements, got %d", len(req2.Collections))
	}
	if req2.Collections[0].Version != "12.2.0" {
		t.Errorf("Expected resolved version '12.2.0', got %q", req2.Collections[0].Version)
	}
	if strings.ContainsAny(req2.Collections[0].Version, ">=<") {
		t.Errorf("Requirements version should not contain operators, got %q", req2.Collections[0].Version)
	}

	// Verify meta.yml has constraints with operators
	if len(meta2.Collections) != 2 {
		t.Errorf("Expected 2 collections in meta, got %d", len(meta2.Collections))
	}
	if meta2.Collections[0] != "community.general>=7.4.0" {
		t.Errorf("Expected 'community.general>=7.4.0' in meta, got %q", meta2.Collections[0])
	}
	if !strings.Contains(meta2.Collections[0], ">=") {
		t.Errorf("Meta collection should contain operator, got %q", meta2.Collections[0])
	}

	// Verify roles
	if len(req2.Roles) != 1 {
		t.Errorf("Expected 1 role in requirements, got %d", len(req2.Roles))
	}
	if req2.Roles[0].Version != "7.4.1" {
		t.Errorf("Expected role version '7.4.1', got %q", req2.Roles[0].Version)
	}
}

func TestDepsSyncRoleWithSrcScm(t *testing.T) {
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

	// Create a mock lock file with role that has src and scm
	lockContent := `version: "1.0"
generated: "2024-01-01T00:00:00Z"
hash: "test-hash"
python:
  min: "3.11"
  max: "3.13"
  pinned: "3.13"
collections: []
roles:
  - name: konstruktoid.docker_rootless
    version: ">=1.9.0"
    resolved_version: "v1.9.0"
    type: role
    src: "https://github.com/konstruktoid/ansible-role-docker-rootless.git"
    scm: "git"
tools: []
`
	if err := os.WriteFile(LockFileName, []byte(lockContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Create initial meta file
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

	// Create initial requirements file
	reqContent := `---
collections: []
roles: []
`
	if err := os.WriteFile(filepath.Join(scenariosDir, "requirements.yml"), []byte(reqContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Load lock file
	lockFile, err := LoadLockFile()
	if err != nil {
		t.Fatalf("LoadLockFile failed: %v", err)
	}

	// Load role config
	_, req, err := LoadRoleConfig("default")
	if err != nil {
		t.Fatalf("LoadRoleConfig failed: %v", err)
	}

	// Simulate sync logic for roles
	req.Roles = []RequirementRole{}
	for _, role := range lockFile.Roles {
		version := role.ResolvedVersion
		if version == "" {
			version = role.Version
		}
		if version == "" || version == "latest" {
			version = "main"
		}

		req.Roles = append(req.Roles, RequirementRole{
			Name:    role.Name,
			Version: version,
			Src:     role.Src,
			Scm:     role.Source,
		})
	}

	// Save files
	if err := SaveRequirementFile(req, "default"); err != nil {
		t.Fatalf("SaveRequirementFile failed: %v", err)
	}

	// Reload and verify
	_, req2, err := LoadRoleConfig("default")
	if err != nil {
		t.Fatalf("LoadRoleConfig failed: %v", err)
	}

	// Verify role has src and scm restored
	if len(req2.Roles) != 1 {
		t.Fatalf("Expected 1 role in requirements, got %d", len(req2.Roles))
	}

	role := req2.Roles[0]
	if role.Name != "konstruktoid.docker_rootless" {
		t.Errorf("Expected role name 'konstruktoid.docker_rootless', got %q", role.Name)
	}
	if role.Version != "v1.9.0" {
		t.Errorf("Expected role version 'v1.9.0', got %q", role.Version)
	}
	if role.Src != "https://github.com/konstruktoid/ansible-role-docker-rootless.git" {
		t.Errorf("Expected role src 'https://github.com/konstruktoid/ansible-role-docker-rootless.git', got %q", role.Src)
	}
	if role.Scm != "git" {
		t.Errorf("Expected role scm 'git', got %q", role.Scm)
	}
}
