package dependency

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"diffusion/internal/config"
)

// writeGitCollectionFixture lays out a minimal role tree with a meta/main.yml,
// a scenarios/default/requirements.yml containing the given YAML body, and a
// diffusion.lock holding a single git collection.
func writeGitCollectionFixture(t *testing.T, requirementsYAML string, lockVersion string) {
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

	if err := os.MkdirAll(filepath.Join("scenarios", "default"), 0755); err != nil {
		t.Fatalf("failed to create scenario dir: %v", err)
	}
	if err := os.MkdirAll("meta", 0755); err != nil {
		t.Fatalf("failed to create meta dir: %v", err)
	}

	// meta/main.yml stays empty: git collections are never written there.
	if err := os.WriteFile(filepath.Join("meta", "main.yml"), []byte("collections: []\n"), 0644); err != nil {
		t.Fatalf("failed to write meta/main.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join("scenarios", "default", "requirements.yml"), []byte(requirementsYAML), 0644); err != nil {
		t.Fatalf("failed to write requirements.yml: %v", err)
	}

	lock := &LockFile{
		Collections: []LockFileEntry{
			{
				Name:            "default.foo",
				Version:         "main",
				ResolvedVersion: lockVersion,
				Type:            "collection",
				Source:          "git",
				Src:             "https://github.com/org/ansible-collection-foo.git",
			},
		},
	}
	if err := SaveLockFile(lock); err != nil {
		t.Fatalf("failed to save lock file: %v", err)
	}
}

func TestCheckLockFileStatusGitCollection(t *testing.T) {
	const inSyncReq = "---\ncollections:\n    - name: https://github.com/org/ansible-collection-foo.git\n      type: git\n      version: main\nroles: []\n"

	tests := []struct {
		name        string
		requirement string
		lockVersion string
		want        bool
	}{
		{
			name:        "in sync",
			requirement: inSyncReq,
			lockVersion: "main",
			want:        true,
		},
		{
			name:        "version drift",
			requirement: inSyncReq,
			lockVersion: "v1.2.3",
			want:        false,
		},
		{
			name:        "url drift",
			requirement: "---\ncollections:\n    - name: https://github.com/org/ansible-collection-bar.git\n      type: git\n      version: main\nroles: []\n",
			lockVersion: "main",
			want:        false,
		},
		{
			name:        "missing from requirements",
			requirement: "---\ncollections: []\nroles: []\n",
			lockVersion: "main",
			want:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeGitCollectionFixture(t, tt.requirement, tt.lockVersion)

			got, err := CheckLockFileStatus("default")
			if err != nil {
				t.Fatalf("CheckLockFileStatus returned error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CheckLockFileStatus = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestGenerateLockFileGitCollectionMissingURL verifies that a non-Galaxy
// collection without a SourceURL is reported as an error instead of being
// silently dropped from the lock file.
func TestGenerateLockFileGitCollectionMissingURL(t *testing.T) {
	collections := []config.CollectionRequirement{
		{Name: "default.foo", Source: "git", Version: "main"},
	}

	lock, err := GenerateLockFile(collections, nil, nil, nil)
	if err == nil {
		t.Fatalf("expected an error for a git collection without SourceURL, got lock file %+v", lock)
	}
	if !strings.Contains(err.Error(), "SourceURL") {
		t.Errorf("error should mention the missing SourceURL, got: %v", err)
	}
}
