package deploy

import (
	"os"
	"strings"
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
)

// --- GenerateRequirements with RoleSources ---

func TestGenerateRequirements_IncludesRoleSources(t *testing.T) {
	lock := lockWithGalaxyRole("geerlingguy", "docker", ">=6.0.0", "6.3.0")
	sources := []RoleSource{
		{SCM: "git", URL: "https://github.com/org/main-role.git", Version: "main", Name: "main-role"},
	}
	out, err := GenerateRequirements(&lock, sources...)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	// Main role source should be present.
	if !strings.Contains(s, "main-role") {
		t.Errorf("expected main-role in requirements, got:\n%s", s)
	}
	if !strings.Contains(s, "https://github.com/org/main-role.git") {
		t.Errorf("expected git URL in requirements, got:\n%s", s)
	}
	// Dependency should also be present.
	if !strings.Contains(s, "geerlingguy.docker") {
		t.Errorf("expected dependency geerlingguy.docker, got:\n%s", s)
	}
}

func TestGenerateRequirements_GalaxyRoleSource(t *testing.T) {
	sources := []RoleSource{
		{SCM: "galaxy", Galaxy: "geerlingguy.nginx", Version: ">=3.0.0"},
	}
	out, err := GenerateRequirements(nil, sources...)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "geerlingguy.nginx") {
		t.Errorf("expected galaxy role in output, got:\n%s", s)
	}
}

func TestGenerateRequirements_DeduplicatesRoleSources(t *testing.T) {
	// Role source and lock entry with same name should not duplicate.
	lock := dependency.LockFile{
		Version: dependency.LockFileVersion,
		Roles: []dependency.LockFileEntry{
			{
				Name:            "main-role",
				Version:         "main",
				ResolvedVersion: "abc123",
				Source:          "git",
				Src:             "https://github.com/org/main-role.git",
				Type:            "role",
			},
		},
	}
	sources := []RoleSource{
		{SCM: "git", URL: "https://github.com/org/main-role.git", Version: "main", Name: "main-role"},
	}
	out, err := GenerateRequirements(&lock, sources...)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	// Count occurrences of "- name: main-role" — should appear only once.
	count := strings.Count(s, "name: main-role")
	if count > 1 {
		t.Errorf("expected role name to appear once, found %d times in:\n%s", count, s)
	}
}

func TestGenerateRequirements_GitRoleBareNameOnly(t *testing.T) {
	// Git-sourced dependency roles should use bare name (no namespace prefix).
	lock := dependency.LockFile{
		Version: dependency.LockFileVersion,
		Roles: []dependency.LockFileEntry{
			{
				Namespace:       "myorg",
				Name:            "docker_rootless",
				Version:         "main",
				ResolvedVersion: "v1.0.0",
				Source:          "git",
				Src:             "https://github.com/myorg/docker-rootless.git",
				Type:            "role",
			},
		},
	}
	out, err := GenerateRequirements(&lock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	// Should use bare name, not "myorg.docker_rootless".
	if strings.Contains(s, "myorg.docker_rootless") {
		t.Errorf("expected bare name for git role, got namespaced:\n%s", s)
	}
	if !strings.Contains(s, "docker_rootless") {
		t.Errorf("expected bare name 'docker_rootless', got:\n%s", s)
	}
}

// --- Inventory default SSH args ---

func TestBuildInventory_DefaultSSHArgs(t *testing.T) {
	hosts := []InventoryHost{
		{Name: "web01", Variables: map[string]string{"ansible_host": "1.2.3.4"}},
	}
	out, err := BuildInventory(hosts, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	if !strings.Contains(s, "ansible_ssh_common_args") {
		t.Error("expected ansible_ssh_common_args in inventory vars")
	}
	if !strings.Contains(s, "StrictHostKeyChecking=no") {
		t.Error("expected StrictHostKeyChecking=no in inventory vars")
	}
}

func TestBuildInventory_UserOverridesSSHArgs(t *testing.T) {
	hosts := []InventoryHost{{Name: "h1"}}
	globalVars := map[string]string{
		"ansible_ssh_common_args": "-o ProxyJump=bastion",
	}
	out, err := BuildInventory(hosts, nil, globalVars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)

	// User-provided value should override the default.
	if !strings.Contains(s, "ProxyJump=bastion") {
		t.Error("expected user-provided SSH args to override default")
	}
}

// --- isDuplicateRole ---

func TestIsDuplicateRole_ByName(t *testing.T) {
	roles := []requirementsRole{
		{Name: "myrole", Src: "https://example.com/myrole.git"},
	}
	candidate := requirementsRole{Name: "myrole", Src: "https://other.com/myrole.git"}

	if !isDuplicateRole(roles, candidate) {
		t.Error("expected duplicate by name")
	}
}

func TestIsDuplicateRole_BySrc(t *testing.T) {
	roles := []requirementsRole{
		{Name: "role-a", Src: "https://github.com/org/role.git"},
	}
	candidate := requirementsRole{Name: "role-b", Src: "https://github.com/org/role.git"}

	if !isDuplicateRole(roles, candidate) {
		t.Error("expected duplicate by src URL")
	}
}

func TestIsDuplicateRole_NoDuplicate(t *testing.T) {
	roles := []requirementsRole{
		{Name: "role-a", Src: "https://github.com/org/role-a.git"},
	}
	candidate := requirementsRole{Name: "role-b", Src: "https://github.com/org/role-b.git"}

	if isDuplicateRole(roles, candidate) {
		t.Error("expected not duplicate")
	}
}

// --- Config DebugEnabled ---

func TestDebugEnabled(t *testing.T) {
	// Save and restore.
	orig := os.Getenv("DIFFUSION_DEBUG")
	defer os.Setenv("DIFFUSION_DEBUG", orig)

	os.Setenv("DIFFUSION_DEBUG", "1")
	if !config.DebugEnabled() {
		t.Error("expected DebugEnabled() == true when DIFFUSION_DEBUG=1")
	}

	os.Setenv("DIFFUSION_DEBUG", "0")
	if config.DebugEnabled() {
		t.Error("expected DebugEnabled() == false when DIFFUSION_DEBUG=0")
	}

	os.Unsetenv("DIFFUSION_DEBUG")
	if config.DebugEnabled() {
		t.Error("expected DebugEnabled() == false when DIFFUSION_DEBUG unset")
	}
}
