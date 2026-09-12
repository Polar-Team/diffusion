package dependency

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"diffusion/internal/role"
)

// setupScenarioFixture creates a role fixture with two scenarios (a, b) plus
// meta/main.yml, and a diffusion.lock file that matches scenario "a" and
// "default" meta, but is stale with respect to scenario "b".
func setupScenarioFixture(t *testing.T) string {
	t.Helper()
	dir := chdirTemp(t)

	for _, name := range []string{"a", "b", "default"} {
		if err := os.MkdirAll(filepath.Join(dir, "scenarios", name), 0o755); err != nil {
			t.Fatalf("mkdir scenarios/%s: %v", name, err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, "meta"), 0o755); err != nil {
		t.Fatalf("mkdir meta: %v", err)
	}

	// requirements.yml for scenario "a" matches the lock file.
	reqA := &role.Requirement{
		Collections: []role.RequirementCollection{{Name: "community.general", Version: "1.0.0"}},
	}
	if err := role.SaveRequirementFile(reqA, "a"); err != nil {
		t.Fatalf("save requirements a: %v", err)
	}

	// requirements.yml for scenario "b" does NOT match the lock file (stale version).
	reqB := &role.Requirement{
		Collections: []role.RequirementCollection{{Name: "community.docker", Version: "STALE"}},
	}
	if err := role.SaveRequirementFile(reqB, "b"); err != nil {
		t.Fatalf("save requirements b: %v", err)
	}

	// requirements.yml for scenario "default" matches the lock file.
	reqDefault := &role.Requirement{
		Collections: []role.RequirementCollection{{Name: "community.general", Version: "1.0.0"}},
	}
	if err := role.SaveRequirementFile(reqDefault, "default"); err != nil {
		t.Fatalf("save requirements default: %v", err)
	}

	// meta/main.yml matches lock file's default-scenario collections.
	meta := &role.Meta{
		GalaxyInfo:  &role.GalaxyInfo{RoleName: "test"},
		Collections: []string{"community.general>=1.0.0"},
	}
	if err := role.SaveMetaFile(meta); err != nil {
		t.Fatalf("save meta: %v", err)
	}

	lock := &LockFile{
		Collections: []LockFileEntry{
			{Name: "a.general", Namespace: "community", ResolvedVersion: "1.0.0"},
			{Name: "b.docker", Namespace: "community", ResolvedVersion: "5.0.0"},
			{Name: "default.general", Namespace: "community", ResolvedVersion: "1.0.0"},
		},
	}
	if err := SaveLockFile(lock); err != nil {
		t.Fatalf("save lock: %v", err)
	}

	return dir
}

func TestCheckLockFileStatus_ScenarioScoping(t *testing.T) {
	setupScenarioFixture(t)

	okA, err := CheckLockFileStatus("a")
	if err != nil {
		t.Fatalf("CheckLockFileStatus(a) error = %v", err)
	}
	if !okA {
		t.Errorf("CheckLockFileStatus(a) = false, want true (scenario a matches lock)")
	}

	okB, err := CheckLockFileStatus("b")
	if err != nil {
		t.Fatalf("CheckLockFileStatus(b) error = %v", err)
	}
	if okB {
		t.Errorf("CheckLockFileStatus(b) = true, want false (scenario b is stale)")
	}

	okAll, err := CheckLockFileStatus("")
	if err != nil {
		t.Fatalf("CheckLockFileStatus('') error = %v", err)
	}
	if okAll {
		t.Errorf("CheckLockFileStatus('') = true, want false (scenario b within all scenarios is stale)")
	}
}

func TestCheckLockFileStatus_NonDefaultScenarioIgnoresMeta(t *testing.T) {
	setupScenarioFixture(t)

	// Corrupt meta/main.yml so it disagrees with the lock file.
	meta := &role.Meta{
		GalaxyInfo:  &role.GalaxyInfo{RoleName: "test"},
		Collections: []string{"community.totally_different>=9.9.9"},
	}
	if err := role.SaveMetaFile(meta); err != nil {
		t.Fatalf("save mismatched meta: %v", err)
	}

	// Scenario "b"'s own requirements.yml is stale independent of meta, so use
	// scenario "a" (whose requirements.yml matches the lock) to isolate the
	// meta.yml effect: a non-default scenario must not consult meta.yml at all.
	okA, err := CheckLockFileStatus("a")
	if err != nil {
		t.Fatalf("CheckLockFileStatus(a) error = %v", err)
	}
	if !okA {
		t.Errorf("CheckLockFileStatus(a) = false, want true (meta mismatch must not affect non-default scenario)")
	}

	// Scenario "default" does consult meta.yml and must now fail.
	okDefault, err := CheckLockFileStatus("default")
	if err != nil {
		t.Fatalf("CheckLockFileStatus(default) error = %v", err)
	}
	if okDefault {
		t.Errorf("CheckLockFileStatus(default) = true, want false (meta.yml mismatch must be detected)")
	}
}

func TestCheckLockFileStatus_UnknownScenarioErrors(t *testing.T) {
	setupScenarioFixture(t)

	_, err := CheckLockFileStatus("nonexistent")
	if err == nil {
		t.Fatalf("CheckLockFileStatus(nonexistent) expected error, got nil")
	}
}

func TestCheckLockFileStatus_NoLockFile(t *testing.T) {
	chdirTemp(t)

	ok, err := CheckLockFileStatus("")
	if err != nil {
		t.Fatalf("CheckLockFileStatus('') error = %v", err)
	}
	if ok {
		t.Errorf("CheckLockFileStatus('') = true, want false when no lock file exists")
	}
}

func TestMergeLockFileForScenario_PrefixCollisionNotRemoved(t *testing.T) {
	existing := &LockFile{
		Hash: "old",
		Collections: []LockFileEntry{
			{Name: "prod.general", ResolvedVersion: "1.0.0"},
			{Name: "prod-eu.general", ResolvedVersion: "2.0.0"},
		},
	}
	fresh := &LockFile{
		Hash:        "new",
		Collections: []LockFileEntry{{Name: "prod.general", ResolvedVersion: "1.5.0"}},
	}

	got := MergeLockFileForScenario(existing, fresh, "prod")

	want := []string{"prod-eu.general", "prod.general"}
	if !reflect.DeepEqual(names(got.Collections), want) {
		t.Fatalf("collections = %v, want %v (prod-eu entries must survive scoping to prod)", names(got.Collections), want)
	}
}

func TestMergeLockFileForScenario_FreshHasZeroEntriesRemovesScenario(t *testing.T) {
	existing := &LockFile{
		Hash: "old",
		Collections: []LockFileEntry{
			{Name: "default.general", ResolvedVersion: "1.0.0"},
			{Name: "extra.posix", ResolvedVersion: "2.0.0"},
		},
		Roles: []LockFileEntry{
			{Name: "default.nginx", ResolvedVersion: "1.1.1"},
		},
	}
	// Fresh generation for "default" produced no entries (e.g. all removed from config).
	fresh := &LockFile{
		Hash:        "new",
		Collections: []LockFileEntry{},
		Roles:       []LockFileEntry{},
	}

	got := MergeLockFileForScenario(existing, fresh, "default")

	if len(got.Collections) != 1 || got.Collections[0].Name != "extra.posix" {
		t.Errorf("collections = %v, want only extra.posix (default entries removed)", names(got.Collections))
	}
	if len(got.Roles) != 0 {
		t.Errorf("roles = %v, want none (default entries removed)", names(got.Roles))
	}
}
