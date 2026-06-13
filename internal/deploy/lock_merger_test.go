package deploy

import (
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
)

// ---------------------------------------------------------------------------
// intersectConstraints
// ---------------------------------------------------------------------------

func TestIntersectConstraints_Empty(t *testing.T) {
	got := intersectConstraints(nil)
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestIntersectConstraints_Single(t *testing.T) {
	got := intersectConstraints([]string{">=9.0.0"})
	if got != ">=9.0.0" {
		t.Errorf("expected '>=9.0.0', got %q", got)
	}
}

func TestIntersectConstraints_KeepsHighestLowerBound(t *testing.T) {
	got := intersectConstraints([]string{">=9.0.0", ">=9.2.0", ">=9.1.0"})
	if got != ">=9.2.0" {
		t.Errorf("expected '>=9.2.0', got %q", got)
	}
}

func TestIntersectConstraints_ExactPin(t *testing.T) {
	got := intersectConstraints([]string{">=9.0.0", "==9.2.0"})
	if got != "==9.2.0" {
		t.Errorf("expected '==9.2.0', got %q", got)
	}
}

func TestIntersectConstraints_ConflictingExactPins(t *testing.T) {
	got := intersectConstraints([]string{"==9.0.0", "==9.1.0"})
	if got == "" || got[:8] != "CONFLICT" {
		t.Errorf("expected CONFLICT marker, got %q", got)
	}
}

func TestIntersectConstraints_UpperAndLower(t *testing.T) {
	got := intersectConstraints([]string{">=9.0.0", "<=10.0.0"})
	if got != ">=9.0.0,<=10.0.0" {
		t.Errorf("expected '>=9.0.0,<=10.0.0', got %q", got)
	}
}

func TestIntersectConstraints_KeepsLowestUpperBound(t *testing.T) {
	got := intersectConstraints([]string{"<=10.0.0", "<=9.5.0"})
	if got != "<=9.5.0" {
		t.Errorf("expected '<=9.5.0', got %q", got)
	}
}

// ---------------------------------------------------------------------------
// mergePythonVersions
// ---------------------------------------------------------------------------

func makeLocksWithPython(specs ...config.PythonVersion) []dependency.LockFile {
	locks := make([]dependency.LockFile, len(specs))
	for i, s := range specs {
		p := s
		locks[i] = dependency.LockFile{Python: &p}
	}
	return locks
}

func TestMergePythonVersions_TakesHighestMin(t *testing.T) {
	// Default min is 3.11; supply a higher value (3.12) — that should win.
	locks := makeLocksWithPython(
		config.PythonVersion{Min: "3.11", Max: "3.13", Pinned: "3.11"},
		config.PythonVersion{Min: "3.12", Max: "3.13", Pinned: "3.12"},
	)
	got, err := mergePythonVersions(locks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Min != "3.12" {
		t.Errorf("expected Min=3.12, got %q", got.Min)
	}
}

func TestMergePythonVersions_TakesLowestMax(t *testing.T) {
	// Default max is 3.13; supply a lower value (3.12) — that should win.
	locks := makeLocksWithPython(
		config.PythonVersion{Min: "3.11", Max: "3.13", Pinned: "3.11"},
		config.PythonVersion{Min: "3.11", Max: "3.12", Pinned: "3.11"},
	)
	got, err := mergePythonVersions(locks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Max != "3.12" {
		t.Errorf("expected Max=3.12, got %q", got.Max)
	}
}

func TestMergePythonVersions_Conflict(t *testing.T) {
	// Force min > max: default min=3.11, supply max=3.10 → 3.11 > 3.10 = conflict.
	locks := makeLocksWithPython(
		config.PythonVersion{Min: "3.11", Max: "3.13", Pinned: "3.11"},
		config.PythonVersion{Min: "3.11", Max: "3.10", Pinned: "3.10"},
	)
	_, err := mergePythonVersions(locks)
	if err == nil {
		t.Fatal("expected error for min > max conflict, got nil")
	}
}

func TestMergePythonVersions_NilPythonSkipped(t *testing.T) {
	// Default min is 3.11; a lock with nil Python should not override it.
	// Supply a non-nil lock with a higher min to verify the non-nil one wins.
	locks := []dependency.LockFile{
		{}, // nil Python — should be skipped
		{Python: &config.PythonVersion{Min: "3.12", Max: "3.13", Pinned: "3.12"}},
	}
	got, err := mergePythonVersions(locks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Min != "3.12" {
		t.Errorf("expected Min=3.12, got %q", got.Min)
	}
}

// ---------------------------------------------------------------------------
// MergeLocks high-level
// ---------------------------------------------------------------------------

func TestMergeLocks_EmptyReturnsDefaults(t *testing.T) {
	result, err := MergeLocks(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.Python == nil {
		t.Fatal("expected Python defaults to be set")
	}
}

func TestMergeLocks_SingleLock(t *testing.T) {
	lock := dependency.LockFile{
		Version: dependency.LockFileVersion,
		Python:  &config.PythonVersion{Min: "3.10", Max: "3.12", Pinned: "3.10"},
		Tools: []dependency.LockFileEntry{
			{Name: "ansible", Version: ">=9.0.0", ResolvedVersion: "9.2.0", Type: "tool"},
		},
	}
	result, err := MergeLocks([]dependency.LockFile{lock})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(result.Tools))
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func TestEntryKey(t *testing.T) {
	e := dependency.LockFileEntry{Namespace: "community", Name: "default.general"}
	got := entryKey(e)
	if got != "community.general" {
		t.Errorf("expected 'community.general', got %q", got)
	}
}

func TestSplitNamespaceAndName_StripScenarioPrefix(t *testing.T) {
	ns, name := splitNamespaceAndName("community", "default.general")
	if ns != "community" || name != "general" {
		t.Errorf("got (%q, %q), want ('community', 'general')", ns, name)
	}
}

func TestSplitNamespaceAndName_NoPrefix(t *testing.T) {
	ns, name := splitNamespaceAndName("geerlingguy", "docker")
	if ns != "geerlingguy" || name != "docker" {
		t.Errorf("got (%q, %q), want ('geerlingguy', 'docker')", ns, name)
	}
}
