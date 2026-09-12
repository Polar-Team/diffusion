package dependency

import (
	"os"
	"path/filepath"
	"testing"

	"diffusion/internal/role"
)

// setupNoDefaultDirFixture creates a repository that has scenarios/a only —
// there is no scenarios/default/ directory — while the lock file still carries
// default-scenario collections that belong in meta/main.yml. meta/main.yml is
// deliberately out of sync with the lock file.
func setupNoDefaultDirFixture(t *testing.T) {
	t.Helper()
	dir := chdirTemp(t)

	if err := os.MkdirAll(filepath.Join(dir, "scenarios", "a"), 0o755); err != nil {
		t.Fatalf("mkdir scenarios/a: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "meta"), 0o755); err != nil {
		t.Fatalf("mkdir meta: %v", err)
	}

	// scenario "a" requirements.yml matches the lock file.
	reqA := &role.Requirement{
		Collections: []role.RequirementCollection{{Name: "community.general", Version: "1.0.0"}},
	}
	if err := role.SaveRequirementFile(reqA, "a"); err != nil {
		t.Fatalf("save requirements a: %v", err)
	}

	// meta/main.yml does NOT match the lock file's default.foo collection.
	meta := &role.Meta{
		GalaxyInfo:  &role.GalaxyInfo{RoleName: "test"},
		Collections: []string{},
	}
	if err := role.SaveMetaFile(meta); err != nil {
		t.Fatalf("save meta: %v", err)
	}

	lock := &LockFile{
		Collections: []LockFileEntry{
			{Name: "a.general", Namespace: "community", ResolvedVersion: "1.0.0"},
			{Name: "default.foo", Namespace: "community", ResolvedVersion: "3.0.0"},
		},
	}
	if err := SaveLockFile(lock); err != nil {
		t.Fatalf("save lock: %v", err)
	}
}

// Regression: with the flag omitted, meta/main.yml must always be checked,
// even when no scenarios/default/ directory exists.
func TestCheckLockFileStatus_AllScenariosChecksMetaWithoutDefaultDir(t *testing.T) {
	setupNoDefaultDirFixture(t)

	okAll, err := CheckLockFileStatus("")
	if err != nil {
		t.Fatalf("CheckLockFileStatus('') error = %v", err)
	}
	if okAll {
		t.Errorf("CheckLockFileStatus('') = true, want false (meta/main.yml is out of sync with default.foo)")
	}

	okA, err := CheckLockFileStatus("a")
	if err != nil {
		t.Fatalf("CheckLockFileStatus(a) error = %v", err)
	}
	if !okA {
		t.Errorf("CheckLockFileStatus(a) = false, want true (scenario a matches; meta must not be consulted)")
	}
}

func TestIncludesDefaultScenario(t *testing.T) {
	tests := []struct {
		scenario string
		want     bool
	}{
		{scenario: "", want: true},
		{scenario: "default", want: true},
		{scenario: "a", want: false},
		{scenario: "default-eu", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.scenario, func(t *testing.T) {
			if got := IncludesDefaultScenario(tt.scenario); got != tt.want {
				t.Errorf("IncludesDefaultScenario(%q) = %v, want %v", tt.scenario, got, tt.want)
			}
		})
	}
}
