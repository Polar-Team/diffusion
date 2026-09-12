package dependency

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"diffusion/internal/config"
)

// stubDefaultFetch replaces the package-level fetcher for the duration of a
// test and returns a counter of how many times it was invoked.
func stubDefaultFetch(t *testing.T, manifest *RemoteManifest) *int32 {
	t.Helper()

	var calls int32
	original := defaultFetch
	defaultFetch = func(url, version string, creds []config.ArtifactCredentials) (*RemoteManifest, error) {
		atomic.AddInt32(&calls, 1)
		return manifest, nil
	}
	t.Cleanup(func() { defaultFetch = original })

	return &calls
}

// setupGitDepProject creates a project whose only dependency is a git
// collection with an explicit constraint. The host is unresolvable so that
// GenerateLockFile's git probe fails fast and falls back to the constraint —
// this keeps the test offline and quick.
func setupGitDepProject(t *testing.T, transitive *bool) {
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

	if err := os.MkdirAll("meta", 0755); err != nil {
		t.Fatalf("failed to create meta dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join("meta", "main.yml"), []byte("collections: []\n"), 0644); err != nil {
		t.Fatalf("failed to write meta/main.yml: %v", err)
	}

	cfg := &config.Config{
		DependencyConfig: &config.DependencyConfig{
			Transitive: transitive,
			Collections: []config.CollectionRequirement{
				{
					Name:      "default.foo",
					Version:   ">=1.0.0",
					Source:    "git",
					SourceURL: "https://diffusion-test.invalid/org/ansible-collection-foo.git",
				},
			},
		},
	}
	if err := config.SaveConfig(cfg); err != nil {
		t.Fatalf("failed to save diffusion.toml: %v", err)
	}
}

// TestUpdateLockFileWithOptions_TransitiveDisabled asserts the fetcher is
// never invoked when transitive resolution is turned off via LockOptions.
func TestUpdateLockFileWithOptions_TransitiveDisabled(t *testing.T) {
	setupGitDepProject(t, nil) // config default = enabled
	calls := stubDefaultFetch(t, &RemoteManifest{})

	disabled := false
	if err := UpdateLockFileWithOptions("", LockOptions{Transitive: &disabled}); err != nil {
		t.Fatalf("UpdateLockFileWithOptions returned error: %v", err)
	}

	if got := atomic.LoadInt32(calls); got != 0 {
		t.Errorf("transitive disabled must not fetch, got %d calls", got)
	}
}

// TestUpdateLockFileWithOptions_TransitiveDisabledInConfig asserts the
// diffusion.toml setting is honoured when LockOptions leaves it unset.
func TestUpdateLockFileWithOptions_TransitiveDisabledInConfig(t *testing.T) {
	disabled := false
	setupGitDepProject(t, &disabled)
	calls := stubDefaultFetch(t, &RemoteManifest{})

	if err := UpdateLockFile(""); err != nil {
		t.Fatalf("UpdateLockFile returned error: %v", err)
	}

	if got := atomic.LoadInt32(calls); got != 0 {
		t.Errorf("transitive disabled in config must not fetch, got %d calls", got)
	}
}

// TestUpdateLockFileWithOptions_TransitiveEnabledByDefault asserts that the
// git dependency is expanded and its contribution lands in diffusion.lock with
// RequiredBy set.
func TestUpdateLockFileWithOptions_TransitiveEnabledByDefault(t *testing.T) {
	setupGitDepProject(t, nil)
	calls := stubDefaultFetch(t, &RemoteManifest{Lock: &LockFile{
		Collections: []LockFileEntry{
			{Name: "default.general", Namespace: "community", Type: "collection",
				Source: "galaxy", ResolvedVersion: "9.0.0"},
		},
	}})

	if err := UpdateLockFile(""); err != nil {
		t.Fatalf("UpdateLockFile returned error: %v", err)
	}

	if got := atomic.LoadInt32(calls); got != 1 {
		t.Fatalf("expected exactly 1 fetch, got %d", got)
	}

	lock, err := LoadLockFile()
	if err != nil {
		t.Fatalf("failed to load lock file: %v", err)
	}
	if lock == nil {
		t.Fatal("lock file was not written")
	}

	var found *LockFileEntry
	for i := range lock.Collections {
		if lock.Collections[i].Name == "default.general" {
			found = &lock.Collections[i]
		}
	}
	if found == nil {
		t.Fatalf("transitive collection missing from lock file: %+v", lock.Collections)
	}
	if found.RequiredBy != "diffusion-test.invalid/org/ansible-collection-foo" {
		t.Errorf("RequiredBy = %q, want the parent dependency identity", found.RequiredBy)
	}

	// The direct dependency must still be present.
	if len(lock.Collections) != 2 {
		t.Errorf("expected direct + transitive collections, got %+v", lock.Collections)
	}
}
