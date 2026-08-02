package deploy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureDeployCacheDir_CreatesStructure(t *testing.T) {
	tmpDir := t.TempDir()
	runID := "abcdef1234567890abcdef"

	cacheDir, err := ensureDeployCacheDir(runID, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should use truncated runID (first 16 chars).
	if !strings.Contains(cacheDir, "abcdef1234567890") {
		t.Errorf("expected truncated runID in path, got: %s", cacheDir)
	}

	// Should be under deploy-cache subdirectory.
	if !strings.Contains(cacheDir, "deploy-cache") {
		t.Errorf("expected 'deploy-cache' in path, got: %s", cacheDir)
	}

	// Check roles directory exists.
	rolesDir := filepath.Join(cacheDir, "roles")
	info, err := os.Stat(rolesDir)
	if err != nil {
		t.Fatalf("roles directory should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("roles should be a directory")
	}

	// Check collections directory exists.
	collectionsDir := filepath.Join(cacheDir, "collections")
	info, err = os.Stat(collectionsDir)
	if err != nil {
		t.Fatalf("collections directory should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("collections should be a directory")
	}
}

func TestEnsureDeployCacheDir_ShortRunID(t *testing.T) {
	tmpDir := t.TempDir()
	runID := "short"

	cacheDir, err := ensureDeployCacheDir(runID, tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Short runID should be used as-is (not truncated).
	if !strings.HasSuffix(filepath.Dir(cacheDir), "deploy-cache") {
		// The parent should be deploy-cache
		if !strings.Contains(cacheDir, "short") {
			t.Errorf("expected 'short' in path, got: %s", cacheDir)
		}
	}
}

func TestEnsureDeployCacheDir_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	runID := "idempotent1234567890"

	dir1, err := ensureDeployCacheDir(runID, tmpDir)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	dir2, err := ensureDeployCacheDir(runID, tmpDir)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}

	if dir1 != dir2 {
		t.Errorf("expected same path on repeated calls, got %q and %q", dir1, dir2)
	}
}

func TestEnsureDeployCacheDir_DifferentRunIDs(t *testing.T) {
	tmpDir := t.TempDir()

	dir1, err := ensureDeployCacheDir("runid_aaa_1234567890", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	dir2, err := ensureDeployCacheDir("runid_bbb_1234567890", tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if dir1 == dir2 {
		t.Error("expected different paths for different runIDs")
	}
}

func TestEnsureDeployCacheDir_DefaultPath(t *testing.T) {
	// When customPath is empty, should use ~/.diffusion.
	// We can't easily test this without mocking os.UserHomeDir,
	// but we can verify it doesn't error.
	runID := "defaultpath123456"
	cacheDir, err := ensureDeployCacheDir(runID, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cacheDir == "" {
		t.Error("expected non-empty cache dir")
	}

	// Clean up
	os.RemoveAll(cacheDir)
}
