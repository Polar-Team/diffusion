package molecule

import (
	"context"
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

// TestRunMoleculeWipeRouting verifies that RunMolecule with WipeFlag set
// routes to the wipe handler. Since Docker is not available in unit tests,
// we expect it to not panic and to return nil (wipe is best-effort).
func TestRunMoleculeWipeRouting(t *testing.T) {
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

// TestDockerMockInterface tests the Docker mock interface
func TestDockerMockInterface(t *testing.T) {
	mock := NewMockDocker()
	ctx := context.Background()

	// Test Run
	err := mock.Run(ctx, "run", "-d", "test-image")
	if err != nil {
		t.Errorf("Mock Run should not fail: %v", err)
	}
	if len(mock.RunCalls) != 1 {
		t.Errorf("Expected 1 Run call, got %d", len(mock.RunCalls))
	}

	// Test Exec
	err = mock.Exec(ctx, "container", "bash", "-c", "echo test")
	if err != nil {
		t.Errorf("Mock Exec should not fail: %v", err)
	}
	if len(mock.ExecCalls) != 1 {
		t.Errorf("Expected 1 Exec call, got %d", len(mock.ExecCalls))
	}

	// Test Pull
	err = mock.Pull(ctx, "test-image:latest")
	if err != nil {
		t.Errorf("Mock Pull should not fail: %v", err)
	}
	if len(mock.PullCalls) != 1 {
		t.Errorf("Expected 1 Pull call, got %d", len(mock.PullCalls))
	}

	// Test ImageExists
	mock.ImageExistsMap["test-image"] = true
	if !mock.ImageExists(ctx, "test-image") {
		t.Error("ImageExists should return true for mapped image")
	}
	if mock.ImageExists(ctx, "nonexistent") {
		t.Error("ImageExists should return false for unmapped image")
	}

	// Test ContainerExists
	mock.ContainerMap["test-container"] = true
	if !mock.ContainerExists(ctx, "test-container") {
		t.Error("ContainerExists should return true for mapped container")
	}
	if mock.ContainerExists(ctx, "nonexistent") {
		t.Error("ContainerExists should return false for unmapped container")
	}
}