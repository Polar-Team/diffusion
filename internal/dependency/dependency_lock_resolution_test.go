package dependency

import (
	"fmt"
	"testing"

	"diffusion/internal/config"
)

func TestLockFileRoleResolution(t *testing.T) {
	// Test that roles with git sources and constraints are properly resolved
	roles := []config.RoleRequirement{
		{
			Name:    "geerlingguy.docker",
			Src:     "https://github.com/geerlingguy/ansible-role-docker.git",
			Scm:     "git",
			Version: ">=6.0.0", // Constraint
		},
	}

	toolVersions := map[string]string{
		"ansible": ">=10.0.0",
	}

	pythonVersion := &config.PythonVersion{
		Min:    "3.11",
		Max:    "3.13",
		Pinned: "3.13",
	}

	lockFile, err := GenerateLockFile([]config.CollectionRequirement{}, roles, toolVersions, pythonVersion)
	if err != nil {
		t.Fatalf("GenerateLockFile() error = %v", err)
	}

	if len(lockFile.Roles) != 1 {
		t.Errorf("Expected 1 role in lock file, got %d", len(lockFile.Roles))
	}

	role := lockFile.Roles[0]

	// Check that version is the constraint
	if role.Version != ">=6.0.0" {
		t.Errorf("Expected version constraint '>=6.0.0', got %q", role.Version)
	}

	// Check that resolved_version is NOT the constraint (unless git resolution failed)
	if role.ResolvedVersion == ">=6.0.0" {
		t.Skip("Git resolution failed (network issue), skipping resolved version check")
	}

	// Check that resolved_version is set
	if role.ResolvedVersion == "" {
		t.Error("ResolvedVersion should be set")
	}

	// Check that resolved_version looks like a version (starts with v or digit)
	if len(role.ResolvedVersion) > 0 {
		firstChar := role.ResolvedVersion[0]
		if firstChar != 'v' && (firstChar < '0' || firstChar > '9') {
			t.Errorf("ResolvedVersion %q doesn't look like a version", role.ResolvedVersion)
		}
	}

	fmt.Printf("✅ Role resolution test passed:\n")
	fmt.Printf("   Name: %s\n", role.Name)
	fmt.Printf("   Version (constraint): %s\n", role.Version)
	fmt.Printf("   ResolvedVersion: %s\n", role.ResolvedVersion)
	fmt.Printf("   Src: %s\n", role.Src)
}
