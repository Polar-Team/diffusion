package cli

import (
	"os"
	"path/filepath"
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/role"
)

// TestDepsSyncRoundTrip_GalaxyGitAndTransitiveGitRole verifies that `deps
// sync` writes a Galaxy collection, a direct git collection, and a
// transitively-resolved git role (RequiredBy set) into requirements.yml in
// the right shapes, that meta/main.yml only receives the Galaxy collection,
// and that `deps check` (CheckLockFileStatus) reports back in sync
// afterwards.
func TestDepsSyncRoundTrip_GalaxyGitAndTransitiveGitRole(t *testing.T) {
	chdirTemp(t)

	if err := os.MkdirAll(filepath.Join("scenarios", "default"), 0755); err != nil {
		t.Fatalf("failed to create scenario dir: %v", err)
	}
	if err := os.MkdirAll("meta", 0755); err != nil {
		t.Fatalf("failed to create meta dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join("meta", "main.yml"), []byte("collections: []\n"), 0644); err != nil {
		t.Fatalf("failed to write meta/main.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join("scenarios", "default", "requirements.yml"), []byte("collections: []\nroles: []\n"), 0644); err != nil {
		t.Fatalf("failed to write requirements.yml: %v", err)
	}

	lock := &dependency.LockFile{
		Collections: []dependency.LockFileEntry{
			{
				Name: "default.general", Namespace: "community", Version: ">=10.0.0",
				ResolvedVersion: "10.5.0", Type: "collection", Source: "galaxy",
			},
			{
				Name: "default.foo", Version: "main", ResolvedVersion: "main",
				Type: "collection", Source: "git", Src: "https://github.com/org/ansible-collection-foo.git",
			},
		},
		Roles: []dependency.LockFileEntry{
			{
				Name: "default.bar", Version: "main", ResolvedVersion: "v1.2.3",
				Type: "role", Source: "git", Src: "https://github.com/org/bar.git",
				RequiredBy: "github.com/org/foo",
			},
		},
	}
	if err := dependency.SaveLockFile(lock); err != nil {
		t.Fatalf("failed to save lock file: %v", err)
	}

	cmd := newDepsSyncCmd()
	cmd.SetArgs([]string{"--scenario", "default"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("deps sync failed: %v", err)
	}

	req, err := role.ParseRequirementFile("default")
	if err != nil {
		t.Fatalf("failed to parse requirements.yml: %v", err)
	}
	if len(req.Collections) != 2 {
		t.Fatalf("expected 2 collections, got %d: %+v", len(req.Collections), req.Collections)
	}
	if len(req.Roles) != 1 {
		t.Fatalf("expected 1 role, got %d: %+v", len(req.Roles), req.Roles)
	}

	roleEntry := req.Roles[0]
	if roleEntry.Src != "https://github.com/org/bar.git" {
		t.Errorf("role Src = %q, want the repository URL", roleEntry.Src)
	}
	if roleEntry.Version != "v1.2.3" {
		t.Errorf("role Version = %q, want resolved version %q", roleEntry.Version, "v1.2.3")
	}
	if roleEntry.Scm != "git" {
		t.Errorf("role Scm = %q, want %q", roleEntry.Scm, "git")
	}

	meta, err := role.ParseMetaFile()
	if err != nil {
		t.Fatalf("failed to parse meta/main.yml: %v", err)
	}
	if len(meta.Collections) != 1 || meta.Collections[0] != "community.general" {
		t.Errorf("meta/main.yml should contain only the Galaxy collection, got %+v", meta.Collections)
	}

	inSync, err := dependency.CheckLockFileStatus("")
	if err != nil {
		t.Fatalf("CheckLockFileStatus returned error: %v", err)
	}
	if !inSync {
		t.Error("expected CheckLockFileStatus to report in-sync after deps sync round trip")
	}
}

// TestDepsInit_MapsGitRequirementBackToConfig verifies that `deps init` maps
// a "type: git" requirements.yml entry back into diffusion.toml as a
// CollectionRequirement with Source="git", the original SourceURL, a derived
// short name, and the version stored verbatim (never turned into ">=main").
func TestDepsInit_MapsGitRequirementBackToConfig(t *testing.T) {
	chdirTemp(t)

	if err := os.MkdirAll(filepath.Join("scenarios", "default"), 0755); err != nil {
		t.Fatalf("failed to create scenario dir: %v", err)
	}
	reqYAML := "---\ncollections:\n" +
		"    - name: https://github.com/org/ansible-collection-foo.git\n" +
		"      type: git\n" +
		"      version: main\n" +
		"roles: []\n"
	if err := os.WriteFile(filepath.Join("scenarios", "default", "requirements.yml"), []byte(reqYAML), 0644); err != nil {
		t.Fatalf("failed to write requirements.yml: %v", err)
	}

	cmd := newDepsInitCmd()
	if err := cmd.Execute(); err != nil {
		t.Fatalf("deps init failed: %v", err)
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("failed to load diffusion.toml: %v", err)
	}
	if cfg.DependencyConfig == nil {
		t.Fatal("expected dependency config to be created")
	}

	var found *config.CollectionRequirement
	for i := range cfg.DependencyConfig.Collections {
		if cfg.DependencyConfig.Collections[i].Name == "default.foo" {
			found = &cfg.DependencyConfig.Collections[i]
		}
	}
	if found == nil {
		t.Fatalf("expected collection default.foo in diffusion.toml, got %+v", cfg.DependencyConfig.Collections)
	}
	if found.Source != "git" {
		t.Errorf("Source = %q, want %q", found.Source, "git")
	}
	if found.SourceURL != "https://github.com/org/ansible-collection-foo.git" {
		t.Errorf("SourceURL = %q, want the repository URL", found.SourceURL)
	}
	if found.Version != "main" {
		t.Errorf("Version = %q, want verbatim %q (not prefixed with >=)", found.Version, "main")
	}
}
