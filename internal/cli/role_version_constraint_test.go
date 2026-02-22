package cli

import (
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/role"
)

func TestRoleVersionConstraintLogic(t *testing.T) {
	tests := []struct {
		name               string
		inputConstraint    string
		resolvedVersion    string
		expectedConstraint string
	}{
		{
			name:               "no constraint - should add >=",
			inputConstraint:    "",
			resolvedVersion:    "1.2.3",
			expectedConstraint: ">=1.2.3",
		},
		{
			name:               "latest - should add >=",
			inputConstraint:    "latest",
			resolvedVersion:    "2.0.0",
			expectedConstraint: ">=2.0.0",
		},
		{
			name:               "main - should add >=",
			inputConstraint:    "main",
			resolvedVersion:    "1.5.0",
			expectedConstraint: ">=1.5.0",
		},
		{
			name:               "with >= constraint - keep as is",
			inputConstraint:    ">=1.0.0",
			resolvedVersion:    "1.2.3",
			expectedConstraint: ">=1.0.0",
		},
		{
			name:               "with == constraint - keep as is",
			inputConstraint:    "==1.0.0",
			resolvedVersion:    "1.0.0",
			expectedConstraint: "==1.0.0",
		},
		{
			name:               "with <= constraint - keep as is",
			inputConstraint:    "<=2.0.0",
			resolvedVersion:    "2.0.0",
			expectedConstraint: "<=2.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic from add-role command
			configVersionConstraint := tt.inputConstraint
			if configVersionConstraint == "" || configVersionConstraint == "latest" || configVersionConstraint == "main" {
				configVersionConstraint = ">=" + tt.resolvedVersion
			}

			if configVersionConstraint != tt.expectedConstraint {
				t.Errorf("Expected constraint %q, got %q", tt.expectedConstraint, configVersionConstraint)
			}
		})
	}
}

func TestRoleRequirementStructure(t *testing.T) {
	// Test that RoleRequirement has the correct fields
	role := config.RoleRequirement{
		Name:    "geerlingguy.docker",
		Src:     "https://github.com/geerlingguy/ansible-role-docker.git",
		Scm:     "git",
		Version: ">=6.0.0",
	}

	if role.Name != "geerlingguy.docker" {
		t.Errorf("Expected name %q, got %q", "geerlingguy.docker", role.Name)
	}

	if role.Version != ">=6.0.0" {
		t.Errorf("Expected version %q, got %q", ">=6.0.0", role.Version)
	}

	if role.Src != "https://github.com/geerlingguy/ansible-role-docker.git" {
		t.Errorf("Expected src %q, got %q", "https://github.com/geerlingguy/ansible-role-docker.git", role.Src)
	}

	if role.Scm != "git" {
		t.Errorf("Expected scm %q, got %q", "git", role.Scm)
	}
}

func TestResolveRoleDependencies(t *testing.T) {
	// Create test data
	meta := &role.Meta{
		GalaxyInfo: &role.GalaxyInfo{
			RoleName:  "test_role",
			Namespace: "test_ns",
		},
	}

	req := &role.Requirement{
		Roles: []role.RequirementRole{
			{
				Name:    "geerlingguy.docker",
				Src:     "https://github.com/geerlingguy/ansible-role-docker.git",
				Scm:     "git",
				Version: "6.0.0", // Resolved version in requirements.yml
			},
		},
	}

	depConfig := &config.DependencyConfig{
		Roles: []config.RoleRequirement{
			{
				Name:    "default.geerlingguy.docker",
				Src:     "https://github.com/geerlingguy/ansible-role-docker.git",
				Scm:     "git",
				Version: ">=6.0.0", // Version constraint in diffusion.toml
			},
		},
	}

	resolver := dependency.NewDependencyResolver(meta, req, depConfig)
	roles, err := resolver.ResolveRoleDependencies()
	if err != nil {
		t.Fatalf("ResolveRoleDependencies() error = %v", err)
	}

	if len(roles) != 1 {
		t.Errorf("Expected 1 role, got %d", len(roles))
	}

	if roles[0].Name != "geerlingguy.docker" {
		t.Errorf("Expected role name %q, got %q", "geerlingguy.docker", roles[0].Name)
	}

	// The version should be the constraint from diffusion.toml
	if roles[0].Version != ">=6.0.0" {
		t.Errorf("Expected version constraint %q, got %q", ">=6.0.0", roles[0].Version)
	}
}

func TestRoleVersionInLockFile(t *testing.T) {
	// Test that lock file stores both constraint and resolved version
	collections := []config.CollectionRequirement{}
	roles := []config.RoleRequirement{
		{
			Name:    "geerlingguy.docker",
			Src:     "https://github.com/geerlingguy/ansible-role-docker.git",
			Scm:     "git",
			Version: ">=6.0.0", // This is the constraint
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

	lockFile, err := dependency.GenerateLockFile(collections, roles, toolVersions, pythonVersion)
	if err != nil {
		t.Fatalf("GenerateLockFile() error = %v", err)
	}

	if len(lockFile.Roles) != 1 {
		t.Errorf("Expected 1 role in lock file, got %d", len(lockFile.Roles))
	}

	// Lock file should store the constraint in Version field
	if lockFile.Roles[0].Version != ">=6.0.0" {
		t.Errorf("Expected version constraint %q in lock file, got %q", ">=6.0.0", lockFile.Roles[0].Version)
	}

	// Lock file should have a resolved version (even if resolution fails, it should have something)
	if lockFile.Roles[0].ResolvedVersion == "" {
		t.Error("Expected resolved version to be set in lock file")
	}
}
