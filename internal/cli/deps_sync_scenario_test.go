package cli

import (
	"os"
	"path/filepath"
	"testing"

	"diffusion/internal/config"
)

// depsSyncFixture creates a temp dir with two scenarios (a, b), a lock file
// containing entries for both, and initial requirements.yml/meta/main.yml
// files. It returns the temp dir and file paths for convenience.
func depsSyncFixture(t *testing.T) (tmpDir string, reqA, reqB, metaPath string) {
	t.Helper()
	tmpDir = t.TempDir()

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

	metaDir := filepath.Join(tmpDir, "meta")
	scenarioADir := filepath.Join(tmpDir, "scenarios", "a")
	scenarioBDir := filepath.Join(tmpDir, "scenarios", "b")
	scenarioDefaultDir := filepath.Join(tmpDir, "scenarios", "default")
	for _, d := range []string{metaDir, scenarioADir, scenarioBDir, scenarioDefaultDir} {
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
  - name: b.docker
    namespace: community
    version: ">=1.0.0"
    resolved_version: "5.0.4"
    type: collection
  - name: default.general
    namespace: community
    version: ">=1.0.0"
    resolved_version: "1.5.0"
    type: collection
roles: []
tools: []
`
	if err := os.WriteFile(config.LockFileName, []byte(lockContent), 0644); err != nil {
		t.Fatal(err)
	}

	metaContent := `---
galaxy_info:
  role_name: test_role
  namespace: test_ns
collections: []
`
	metaPath = filepath.Join(metaDir, "main.yml")
	if err := os.WriteFile(metaPath, []byte(metaContent), 0644); err != nil {
		t.Fatal(err)
	}

	reqContent := `---
collections: []
roles: []
`
	reqA = filepath.Join(scenarioADir, "requirements.yml")
	if err := os.WriteFile(reqA, []byte(reqContent), 0644); err != nil {
		t.Fatal(err)
	}
	reqB = filepath.Join(scenarioBDir, "requirements.yml")
	if err := os.WriteFile(reqB, []byte(reqContent), 0644); err != nil {
		t.Fatal(err)
	}
	reqDefault := filepath.Join(scenarioDefaultDir, "requirements.yml")
	if err := os.WriteFile(reqDefault, []byte(reqContent), 0644); err != nil {
		t.Fatal(err)
	}

	return tmpDir, reqA, reqB, metaPath
}

func TestDepsSyncCommand_ScenarioFlagWritesOnlyThatScenario(t *testing.T) {
	_, reqA, reqB, metaPath := depsSyncFixture(t)

	reqABefore, err := os.ReadFile(reqA)
	if err != nil {
		t.Fatal(err)
	}
	metaBefore, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}

	depsCmd := NewDepsCmd(&CLI{})
	depsCmd.SetArgs([]string{"sync", "--scenario", "b"})
	if err := depsCmd.Execute(); err != nil {
		t.Fatalf("deps sync --scenario b failed: %v", err)
	}

	reqAAfter, err := os.ReadFile(reqA)
	if err != nil {
		t.Fatal(err)
	}
	metaAfter, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	reqBAfter, err := os.ReadFile(reqB)
	if err != nil {
		t.Fatal(err)
	}

	if string(reqAAfter) != string(reqABefore) {
		t.Errorf("scenario a requirements.yml was modified by sync --scenario b:\nbefore:\n%s\nafter:\n%s", reqABefore, reqAAfter)
	}
	if string(metaAfter) != string(metaBefore) {
		t.Errorf("meta/main.yml was modified by sync --scenario b:\nbefore:\n%s\nafter:\n%s", metaBefore, metaAfter)
	}
	if string(reqBAfter) == string(reqABefore) {
		t.Errorf("scenario b requirements.yml was not updated by sync --scenario b")
	}
}

func TestDepsSyncCommand_NoFlagUpdatesAllScenariosAndMeta(t *testing.T) {
	_, reqA, reqB, metaPath := depsSyncFixture(t)

	reqABefore, err := os.ReadFile(reqA)
	if err != nil {
		t.Fatal(err)
	}
	reqBBefore, err := os.ReadFile(reqB)
	if err != nil {
		t.Fatal(err)
	}
	metaBefore, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}

	depsCmd := NewDepsCmd(&CLI{})
	depsCmd.SetArgs([]string{"sync"})
	if err := depsCmd.Execute(); err != nil {
		t.Fatalf("deps sync failed: %v", err)
	}

	reqAAfter, err := os.ReadFile(reqA)
	if err != nil {
		t.Fatal(err)
	}
	reqBAfter, err := os.ReadFile(reqB)
	if err != nil {
		t.Fatal(err)
	}
	metaAfter, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(reqAAfter) == string(reqABefore) {
		t.Errorf("scenario a requirements.yml was not updated by sync (all scenarios)")
	}
	if string(reqBAfter) == string(reqBBefore) {
		t.Errorf("scenario b requirements.yml was not updated by sync (all scenarios)")
	}
	if string(metaAfter) == string(metaBefore) {
		t.Errorf("meta/main.yml was not updated by sync (all scenarios)")
	}
}

func TestDepsCheckCommand_UnknownScenarioReturnsError(t *testing.T) {
	depsSyncFixture(t)

	depsCmd := NewDepsCmd(&CLI{})
	depsCmd.SetArgs([]string{"check", "--scenario", "zzz"})
	depsCmd.SilenceErrors = true
	depsCmd.SilenceUsage = true
	err := depsCmd.Execute()
	if err == nil {
		t.Fatalf("expected error from 'deps check --scenario zzz', got nil")
	}
}
