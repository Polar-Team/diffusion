package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/role"
)

// chdirTemp switches to a fresh temporary directory for the duration of a test.
func chdirTemp(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	return tempDir
}

// TestResolveCollectionRequirementGit covers the version semantics of
// `role add-collection --src`. None of these cases touch the network: a
// branch name or an explicit constraint is stored verbatim.
func TestResolveCollectionRequirementGit(t *testing.T) {
	tests := []struct {
		name string
		spec collectionSpec
		want config.CollectionRequirement
	}{
		{
			name: "branch ref stored verbatim",
			spec: collectionSpec{Scenario: "default", Name: "foo", Src: "https://example.com/x.git", Scm: "git", Version: "main"},
			want: config.CollectionRequirement{Name: "default.foo", Version: "main", Source: "git", SourceURL: "https://example.com/x.git"},
		},
		{
			name: "constraint stored verbatim",
			spec: collectionSpec{Scenario: "default", Name: "foo", Src: "https://example.com/x.git", Scm: "git", Version: ">=1.2.0"},
			want: config.CollectionRequirement{Name: "default.foo", Version: ">=1.2.0", Source: "git", SourceURL: "https://example.com/x.git"},
		},
		{
			name: "semver stored verbatim",
			spec: collectionSpec{Scenario: "ci", Name: "foo", Src: "git@github.com:org/foo.git", Scm: "git", Version: "1.2.3"},
			want: config.CollectionRequirement{Name: "ci.foo", Version: "1.2.3", Source: "git", SourceURL: "git@github.com:org/foo.git"},
		},
		{
			name: "namespace is optional but preserved",
			spec: collectionSpec{Scenario: "default", Name: "foo", Namespace: "org", Src: "https://example.com/x.git", Version: "develop"},
			want: config.CollectionRequirement{Name: "default.foo", Namespace: "org", Version: "develop", Source: "git", SourceURL: "https://example.com/x.git"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveCollectionRequirement(tt.spec)
			if err != nil {
				t.Fatalf("resolveCollectionRequirement returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("resolveCollectionRequirement =\n got: %+v\nwant: %+v", got, tt.want)
			}
		})
	}
}

// TestResolveCollectionRequirementGalaxyRequiresNamespace keeps the existing
// Galaxy behaviour: without --src, --namespace stays mandatory.
func TestResolveCollectionRequirementGalaxyRequiresNamespace(t *testing.T) {
	_, err := resolveCollectionRequirement(collectionSpec{Scenario: "default", Name: "general", Version: "9.0.0"})
	if err == nil {
		t.Fatal("expected an error when --namespace is missing for a Galaxy collection")
	}
	if !strings.Contains(err.Error(), "--namespace") {
		t.Errorf("error should mention --namespace, got: %v", err)
	}
}

// TestAddGitCollectionWritesToml verifies that the resolved git collection is
// persisted into diffusion.toml with Source/SourceURL set.
func TestAddGitCollectionWritesToml(t *testing.T) {
	chdirTemp(t)

	spec := collectionSpec{
		Scenario: "default",
		Name:     "foo",
		Src:      "https://example.com/x.git",
		Scm:      "git",
		Version:  "main",
	}

	req, err := resolveCollectionRequirement(spec)
	if err != nil {
		t.Fatalf("resolveCollectionRequirement returned error: %v", err)
	}

	cfg := &config.Config{DependencyConfig: &config.DependencyConfig{}}
	upsertCollection(cfg.DependencyConfig, req)
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("failed to save diffusion.toml: %v", err)
	}

	// Re-adding the same collection must update in place, not duplicate.
	upsertCollection(cfg.DependencyConfig, config.CollectionRequirement{
		Name: "default.foo", Version: "develop", Source: "git", SourceURL: "https://example.com/x.git",
	})
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("failed to save diffusion.toml: %v", err)
	}

	loaded, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("failed to load diffusion.toml: %v", err)
	}
	if loaded.DependencyConfig == nil || len(loaded.DependencyConfig.Collections) != 1 {
		t.Fatalf("expected exactly 1 collection in diffusion.toml, got %+v", loaded.DependencyConfig)
	}

	got := loaded.DependencyConfig.Collections[0]
	want := config.CollectionRequirement{
		Name: "default.foo", Version: "develop", Source: "git", SourceURL: "https://example.com/x.git",
	}
	if got != want {
		t.Errorf("diffusion.toml collection =\n got: %+v\nwant: %+v", got, want)
	}
}

// TestDepsSyncWritesGitCollection verifies that `deps sync` emits git
// collections in the ansible-galaxy git format into requirements.yml and
// skips them in meta/main.yml.
func TestDepsSyncWritesGitCollection(t *testing.T) {
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
				Name:            "default.general",
				Namespace:       "community",
				Version:         ">=10.0.0",
				ResolvedVersion: "10.5.0",
				Type:            "collection",
				Source:          "galaxy",
			},
			{
				Name:            "default.foo",
				Version:         "main",
				ResolvedVersion: "main",
				Type:            "collection",
				Source:          "git",
				Src:             "https://github.com/org/ansible-collection-foo.git",
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
		t.Fatalf("expected 2 collections in requirements.yml, got %d (%+v)", len(req.Collections), req.Collections)
	}

	var gitEntry *role.RequirementCollection
	for i := range req.Collections {
		if req.Collections[i].Type == "git" {
			gitEntry = &req.Collections[i]
		}
	}
	if gitEntry == nil {
		t.Fatalf("no git collection found in requirements.yml: %+v", req.Collections)
	}
	if gitEntry.Name != "https://github.com/org/ansible-collection-foo.git" {
		t.Errorf("git collection name = %q, want the repository URL", gitEntry.Name)
	}
	if gitEntry.Version != "main" {
		t.Errorf("git collection version = %q, want %q", gitEntry.Version, "main")
	}

	meta, err := role.ParseMetaFile()
	if err != nil {
		t.Fatalf("failed to parse meta/main.yml: %v", err)
	}
	if len(meta.Collections) != 1 || meta.Collections[0] != "community.general" {
		t.Errorf("meta/main.yml should contain only the Galaxy collection, got %+v", meta.Collections)
	}
}
