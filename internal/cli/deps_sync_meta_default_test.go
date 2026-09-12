package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"diffusion/internal/config"
)

// Regression: `deps sync` with no --scenario flag must still rewrite
// meta/main.yml even in a repository that has no scenarios/default/ directory.
func TestDepsSyncCommand_NoFlagUpdatesMetaWithoutDefaultScenarioDir(t *testing.T) {
	tmpDir := t.TempDir()

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWd); err != nil {
			t.Fatalf("chdir back: %v", err)
		}
	})

	// Only scenarios/a exists — no scenarios/default.
	for _, d := range []string{filepath.Join(tmpDir, "meta"), filepath.Join(tmpDir, "scenarios", "a")} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	lockContent := `version: "1.0"
generated: "2024-01-01T00:00:00Z"
hash: "test-hash"
collections:
  - name: a.general
    namespace: community
    version: ">=1.0.0"
    resolved_version: "1.5.0"
    type: collection
  - name: default.foo
    namespace: community
    version: ">=1.0.0"
    resolved_version: "3.0.0"
    type: collection
roles: []
tools: []
`
	if err := os.WriteFile(config.LockFileName, []byte(lockContent), 0644); err != nil {
		t.Fatal(err)
	}

	metaPath := filepath.Join(tmpDir, "meta", "main.yml")
	metaContent := `---
galaxy_info:
  role_name: test_role
  namespace: test_ns
collections: []
`
	if err := os.WriteFile(metaPath, []byte(metaContent), 0644); err != nil {
		t.Fatal(err)
	}

	reqA := filepath.Join(tmpDir, "scenarios", "a", "requirements.yml")
	if err := os.WriteFile(reqA, []byte("---\ncollections: []\nroles: []\n"), 0644); err != nil {
		t.Fatal(err)
	}

	depsCmd := NewDepsCmd(&CLI{})
	depsCmd.SetArgs([]string{"sync"})
	if err := depsCmd.Execute(); err != nil {
		t.Fatalf("deps sync failed: %v", err)
	}

	metaAfter, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(metaAfter), "community.foo") {
		t.Errorf("meta/main.yml was not updated with default-scenario collection:\n%s", metaAfter)
	}
}
