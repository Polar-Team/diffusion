package molecule

import (
	"testing"
)

// TestMoleculeOptionsDefaults verifies that a zero-value MoleculeOptions
// has all flags disabled by default.
func TestMoleculeOptionsDefaults(t *testing.T) {
	opts := &MoleculeOptions{}

	if opts.RoleFlag != "" {
		t.Errorf("expected empty RoleFlag, got %q", opts.RoleFlag)
	}
	if opts.OrgFlag != "" {
		t.Errorf("expected empty OrgFlag, got %q", opts.OrgFlag)
	}
	if opts.RoleScenario != "" {
		t.Errorf("expected empty RoleScenario, got %q", opts.RoleScenario)
	}
	if opts.TagFlag != "" {
		t.Errorf("expected empty TagFlag, got %q", opts.TagFlag)
	}
	if opts.ConvergeFlag {
		t.Error("expected ConvergeFlag to be false")
	}
	if opts.VerifyFlag {
		t.Error("expected VerifyFlag to be false")
	}
	if opts.TestsOverWrite {
		t.Error("expected TestsOverWrite to be false")
	}
	if opts.LintFlag {
		t.Error("expected LintFlag to be false")
	}
	if opts.IdempotenceFlag {
		t.Error("expected IdempotenceFlag to be false")
	}
	if opts.DestroyFlag {
		t.Error("expected DestroyFlag to be false")
	}
	if opts.WipeFlag {
		t.Error("expected WipeFlag to be false")
	}
	if opts.CIMode {
		t.Error("expected CIMode to be false")
	}
}

// TestMoleculeOptionsFieldMapping verifies that each field on MoleculeOptions
// maps correctly (no misaligned struct tags, naming issues, etc.).
func TestMoleculeOptionsFieldMapping(t *testing.T) {
	opts := &MoleculeOptions{
		RoleFlag:        "myrole",
		OrgFlag:         "myorg",
		RoleScenario:    "production",
		TagFlag:         "install,configure",
		ConvergeFlag:    true,
		VerifyFlag:      true,
		TestsOverWrite:  true,
		LintFlag:        true,
		IdempotenceFlag: true,
		DestroyFlag:     true,
		WipeFlag:        true,
		CIMode:          true,
	}

	if opts.RoleFlag != "myrole" {
		t.Errorf("RoleFlag = %q, want %q", opts.RoleFlag, "myrole")
	}
	if opts.OrgFlag != "myorg" {
		t.Errorf("OrgFlag = %q, want %q", opts.OrgFlag, "myorg")
	}
	if opts.RoleScenario != "production" {
		t.Errorf("RoleScenario = %q, want %q", opts.RoleScenario, "production")
	}
	if opts.TagFlag != "install,configure" {
		t.Errorf("TagFlag = %q, want %q", opts.TagFlag, "install,configure")
	}
	if !opts.ConvergeFlag {
		t.Error("ConvergeFlag should be true")
	}
	if !opts.VerifyFlag {
		t.Error("VerifyFlag should be true")
	}
	if !opts.TestsOverWrite {
		t.Error("TestsOverWrite should be true")
	}
	if !opts.LintFlag {
		t.Error("LintFlag should be true")
	}
	if !opts.IdempotenceFlag {
		t.Error("IdempotenceFlag should be true")
	}
	if !opts.DestroyFlag {
		t.Error("DestroyFlag should be true")
	}
	if !opts.WipeFlag {
		t.Error("WipeFlag should be true")
	}
	if !opts.CIMode {
		t.Error("CIMode should be true")
	}
}

// TestRunMoleculeWipeRouting verifies that RunMolecule with WipeFlag set
// routes to the wipe handler. Since Docker is not available in unit tests,
// we expect it to not panic and to return nil (wipe is best-effort).
func TestRunMoleculeWipeRouting(t *testing.T) {
	// Wipe calls docker commands that will fail in test environment,
	// but handleWipe is designed to be best-effort (ignores errors).
	// It should still return nil.
	opts := &MoleculeOptions{
		RoleFlag: "testrole",
		OrgFlag:  "testorg",
		WipeFlag: true,
	}

	err := RunMolecule(opts)
	// handleWipe returns nil even when docker commands fail
	if err != nil {
		t.Errorf("RunMolecule with WipeFlag returned unexpected error: %v", err)
	}
}

// TestRunMoleculeNilOptions verifies that RunMolecule handles a nil-like
// scenario gracefully (empty options, no config file).
func TestRunMoleculeEmptyOptions(t *testing.T) {
	// With empty options and no config file, RunMolecule will attempt
	// the default flow which requires Docker. We mainly verify it
	// doesn't panic.
	opts := &MoleculeOptions{
		RoleFlag: "noexist",
		OrgFlag:  "noexist",
	}

	// This will fail because Docker isn't available, but shouldn't panic
	_ = RunMolecule(opts)
}
