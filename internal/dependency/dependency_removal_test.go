package dependency

import (
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/role"
)

func TestRoleRemovalFromLockFile(t *testing.T) {
	// Simulate a scenario where:
	// - requirements.yml has 2 roles
	// - diffusion.toml has only 1 role (one was removed)
	// - lock file should only contain the 1 role from diffusion.toml

	meta := &role.Meta{
		GalaxyInfo: &role.GalaxyInfo{
			RoleName:  "test_role",
			Namespace: "test_ns",
		},
	}

	// requirements.yml has 2 roles
	req := &role.Requirement{
		Roles: []role.RequirementRole{
			{
				Name:    "geerlingguy.docker",
				Src:     "https://github.com/geerlingguy/ansible-role-docker.git",
				Scm:     "git",
				Version: "v7.9.0",
			},
			{
				Name:    "konstruktoid.docker_rootless",
				Src:     "https://github.com/konstruktoid/ansible-role-docker-rootless.git",
				Scm:     "git",
				Version: "v1.10.0",
			},
		},
	}

	// diffusion.toml has only 1 role (docker_rootless was removed)
	depConfig := &config.DependencyConfig{
		Roles: []config.RoleRequirement{
			{
				Name:    "default.geerlingguy.docker",
				Src:     "https://github.com/geerlingguy/ansible-role-docker.git",
				Scm:     "git",
				Version: ">=6.0.0",
			},
			// docker_rootless is NOT here - it was removed
		},
	}

	resolver := NewDependencyResolver(meta, req, depConfig)
	roles, err := resolver.ResolveRoleDependencies()
	if err != nil {
		t.Fatalf("ResolveRoleDependencies() error = %v", err)
	}

	// Should only have 1 role (the one in diffusion.toml)
	if len(roles) != 1 {
		t.Errorf("Expected 1 role (only from diffusion.toml), got %d", len(roles))
		for i, role := range roles {
			t.Logf("  Role %d: %s", i, role.Name)
		}
	}

	// Verify it's the correct role
	if len(roles) > 0 {
		if roles[0].Name != "geerlingguy.docker" {
			t.Errorf("Expected role 'geerlingguy.docker', got %q", roles[0].Name)
		}

		// Verify it has the constraint from diffusion.toml
		if roles[0].Version != ">=6.0.0" {
			t.Errorf("Expected version constraint '>=6.0.0', got %q", roles[0].Version)
		}
	}

	t.Logf("✅ Role removal test passed: Only roles in diffusion.toml are included")
}

func TestEmptyDiffusionTomlRoles(t *testing.T) {
	// Test that if diffusion.toml has no roles, lock file should have no roles
	// even if requirements.yml has roles

	meta := &role.Meta{
		GalaxyInfo: &role.GalaxyInfo{
			RoleName:  "test_role",
			Namespace: "test_ns",
		},
	}

	// requirements.yml has roles
	req := &role.Requirement{
		Roles: []role.RequirementRole{
			{
				Name:    "geerlingguy.docker",
				Src:     "https://github.com/geerlingguy/ansible-role-docker.git",
				Scm:     "git",
				Version: "v7.9.0",
			},
		},
	}

	// diffusion.toml has NO roles
	depConfig := &config.DependencyConfig{
		Roles: []config.RoleRequirement{},
	}

	resolver := NewDependencyResolver(meta, req, depConfig)
	roles, err := resolver.ResolveRoleDependencies()
	if err != nil {
		t.Fatalf("ResolveRoleDependencies() error = %v", err)
	}

	// Should have 0 roles
	if len(roles) != 0 {
		t.Errorf("Expected 0 roles (diffusion.toml is empty), got %d", len(roles))
		for i, role := range roles {
			t.Logf("  Role %d: %s", i, role.Name)
		}
	}

	t.Logf("✅ Empty diffusion.toml test passed: No roles when diffusion.toml is empty")
}

func TestCollectionConstraintsInLockFile(t *testing.T) {
	// Test that collection version constraints from diffusion.toml
	// are stored in the lock file's version field (not resolved versions)

	meta := &role.Meta{
		GalaxyInfo: &role.GalaxyInfo{
			RoleName:  "test_role",
			Namespace: "test_ns",
		},
		Collections: []string{"community.general"},
	}

	// requirements.yml has resolved versions
	req := &role.Requirement{
		Collections: []role.RequirementCollection{
			{Name: "community.general", Version: "12.2.0"}, // Resolved version
			{Name: "community.docker", Version: "5.0.4"},   // Resolved version
		},
	}

	// diffusion.toml has version constraints
	depConfig := &config.DependencyConfig{
		Collections: []config.CollectionRequirement{
			{Name: "community.general", Version: ">=7.4.0"}, // Constraint
			{Name: "community.docker", Version: ">=5.0.4"},  // Constraint
		},
	}

	resolver := NewDependencyResolver(meta, req, depConfig)
	collections, err := resolver.ResolveCollectionDependencies()
	if err != nil {
		t.Fatalf("ResolveCollectionDependencies() error = %v", err)
	}

	// Verify that constraints from diffusion.toml are used, not resolved versions
	for _, col := range collections {
		switch col.Name {
		case "community.general":
			if col.Version != ">=7.4.0" {
				t.Errorf("Expected constraint '>=7.4.0' for community.general, got %q", col.Version)
			}
		case "community.docker":
			if col.Version != ">=5.0.4" {
				t.Errorf("Expected constraint '>=5.0.4' for community.docker, got %q", col.Version)
			}
		}
	}

	t.Logf("✅ Collection constraints test passed: diffusion.toml constraints are used")
}
