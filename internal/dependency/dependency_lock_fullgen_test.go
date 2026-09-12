package dependency

import (
	"os"
	"strings"
	"testing"
)

func TestNeedsFullGeneration(t *testing.T) {
	lock := &LockFile{}

	tests := []struct {
		name     string
		scenario string
		existing *LockFile
		want     bool
	}{
		{name: "all scenarios with existing lock", scenario: "", existing: lock, want: true},
		{name: "all scenarios without lock", scenario: "", existing: nil, want: true},
		{name: "scoped without lock", scenario: "default", existing: nil, want: true},
		{name: "scoped with existing lock", scenario: "default", existing: lock, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NeedsFullGeneration(tt.scenario, tt.existing); got != tt.want {
				t.Errorf("NeedsFullGeneration(%q, %v) = %v, want %v", tt.scenario, tt.existing != nil, got, tt.want)
			}
		})
	}
}

// A scenario-scoped lock run must not produce a lock file that contains only
// the requested scenario when no diffusion.lock exists yet: it has to fall back
// to a full generation covering every scenario.
func TestUpdateLockFile_NoExistingLockFallsBackToFullGeneration(t *testing.T) {
	chdirTemp(t)

	// Roles without Src and without Namespace never reach the network: the git
	// branch needs Src, the Galaxy branch needs a Namespace with a non-git SCM.
	tomlContent := `[dependencies]
ansible = "==11.1.0"
molecule = "==25.4.0"
ansible-lint = "==25.1.3"
yamllint = "==1.35.1"

[[dependencies.roles]]
Name = "default.x"
Version = "main"
Scm = "git"

[[dependencies.roles]]
Name = "other.y"
Version = "main"
Scm = "git"
`
	if err := os.WriteFile("diffusion.toml", []byte(tomlContent), 0o644); err != nil {
		t.Fatalf("write diffusion.toml: %v", err)
	}
	if err := os.MkdirAll("scenarios/default", 0o755); err != nil {
		t.Fatalf("mkdir scenarios/default: %v", err)
	}
	if err := os.MkdirAll("scenarios/other", 0o755); err != nil {
		t.Fatalf("mkdir scenarios/other: %v", err)
	}

	if _, err := os.Stat("diffusion.lock"); !os.IsNotExist(err) {
		t.Fatalf("precondition failed: diffusion.lock must not exist")
	}

	if err := UpdateLockFile("default"); err != nil {
		t.Fatalf("UpdateLockFile(default) error = %v", err)
	}

	lockFile, err := LoadLockFile()
	if err != nil {
		t.Fatalf("LoadLockFile() error = %v", err)
	}
	if lockFile == nil {
		t.Fatalf("no lock file was written")
	}

	got := names(lockFile.Roles)
	if !contains(got, "default.x") {
		t.Errorf("roles = %v, want to contain default.x", got)
	}
	if !contains(got, "other.y") {
		t.Errorf("roles = %v, want to contain other.y (full generation must cover all scenarios)", got)
	}
}

// Once a lock file exists, a scoped run must merge rather than regenerate.
func TestUpdateLockFile_ExistingLockKeepsOtherScenarios(t *testing.T) {
	chdirTemp(t)

	tomlContent := `[dependencies]
ansible = "==11.1.0"
molecule = "==25.4.0"
ansible-lint = "==25.1.3"
yamllint = "==1.35.1"

[[dependencies.roles]]
Name = "default.x"
Version = "main"
Scm = "git"
`
	if err := os.WriteFile("diffusion.toml", []byte(tomlContent), 0o644); err != nil {
		t.Fatalf("write diffusion.toml: %v", err)
	}
	if err := os.MkdirAll("scenarios/default", 0o755); err != nil {
		t.Fatalf("mkdir scenarios/default: %v", err)
	}

	// Pre-existing lock file carrying an entry from another scenario that is
	// no longer present in diffusion.toml. A scoped run must preserve it.
	seed := &LockFile{
		Roles: []LockFileEntry{
			{Name: "other.y", ResolvedVersion: "9.9.9", Type: "role"},
		},
	}
	if err := SaveLockFile(seed); err != nil {
		t.Fatalf("SaveLockFile() error = %v", err)
	}

	if err := UpdateLockFile("default"); err != nil {
		t.Fatalf("UpdateLockFile(default) error = %v", err)
	}

	lockFile, err := LoadLockFile()
	if err != nil {
		t.Fatalf("LoadLockFile() error = %v", err)
	}

	got := names(lockFile.Roles)
	if !contains(got, "other.y") {
		t.Errorf("roles = %v, want to contain preserved other.y", got)
	}
	if !contains(got, "default.x") {
		t.Errorf("roles = %v, want to contain regenerated default.x", got)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.EqualFold(s, needle) {
			return true
		}
	}
	return false
}
