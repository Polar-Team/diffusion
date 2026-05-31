package tfprovider

import (
	"testing"

	"diffusion/internal/deploy"
)

// ---------------------------------------------------------------------------
// buildArgs
// ---------------------------------------------------------------------------

func cfg(roleSources []RoleSourceEntry) DiffusionRunConfig {
	return DiffusionRunConfig{RoleSources: roleSources}
}

func hasArg(args []string, flag, value string) bool {
	for i, a := range args {
		if a == flag && i+1 < len(args) && args[i+1] == value {
			return true
		}
	}
	return false
}

func hasFlag(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func TestBuildArgs_SubCommand(t *testing.T) {
	args := buildArgs(cfg(nil))
	if len(args) == 0 || args[0] != "deploy" {
		t.Errorf("expected first arg to be 'deploy', got %v", args)
	}
}

func TestBuildArgs_RoleSourceGalaxy(t *testing.T) {
	c := cfg([]RoleSourceEntry{
		{SCM: "galaxy", Version: ">=6.0.0", Galaxy: "geerlingguy.docker"},
	})
	args := buildArgs(c)
	if !hasArg(args, "--role-source", "scm=galaxy,version=>=6.0.0,galaxy=geerlingguy.docker") {
		t.Errorf("expected --role-source with galaxy spec, got %v", args)
	}
}

func TestBuildArgs_RoleSourceGit(t *testing.T) {
	c := cfg([]RoleSourceEntry{
		{SCM: "git", Version: "main", URL: "https://github.com/org/role.git"},
	})
	args := buildArgs(c)
	if !hasArg(args, "--role-source", "scm=git,version=main,url=https://github.com/org/role.git") {
		t.Errorf("expected --role-source with git spec, got %v", args)
	}
}

func TestBuildArgs_RoleSourceWithNameAndApplyTo(t *testing.T) {
	c := cfg([]RoleSourceEntry{
		{SCM: "galaxy", Version: ">=1.0.0", Galaxy: "ns.role", Name: "myrole", ApplyTo: "webservers"},
	})
	args := buildArgs(c)
	spec := "scm=galaxy,version=>=1.0.0,galaxy=ns.role,name=myrole,apply_to=webservers"
	if !hasArg(args, "--role-source", spec) {
		t.Errorf("expected --role-source %q, got %v", spec, args)
	}
}

func TestBuildArgs_PlaybookOmittedWhenEmpty(t *testing.T) {
	c := DiffusionRunConfig{Playbook: ""}
	args := buildArgs(c)
	if hasFlag(args, "--playbook") {
		t.Errorf("expected --playbook to be absent when empty, got %v", args)
	}
}

func TestBuildArgs_PlaybookIncludedWhenSet(t *testing.T) {
	c := DiffusionRunConfig{Playbook: "/path/to/site.yml"}
	args := buildArgs(c)
	if !hasArg(args, "--playbook", "/path/to/site.yml") {
		t.Errorf("expected --playbook /path/to/site.yml, got %v", args)
	}
}

func TestBuildArgs_HostWaitSettings(t *testing.T) {
	c := DiffusionRunConfig{
		HostWaitInitialDelay: "10s",
		HostWaitInterval:     "5s",
		HostWaitTimeout:      "2m",
	}
	args := buildArgs(c)
	if !hasArg(args, "--host-wait-initial-delay", "10s") {
		t.Errorf("expected --host-wait-initial-delay 10s, got %v", args)
	}
	if !hasArg(args, "--host-wait-interval", "5s") {
		t.Errorf("expected --host-wait-interval 5s, got %v", args)
	}
	if !hasArg(args, "--host-wait-timeout", "2m") {
		t.Errorf("expected --host-wait-timeout 2m, got %v", args)
	}
}

func TestBuildArgs_SkipPeriod(t *testing.T) {
	c := DiffusionRunConfig{SkipIfSucceededWithin: "24h"}
	args := buildArgs(c)
	if !hasArg(args, "--skip-period", "24h") {
		t.Errorf("expected --skip-period 24h, got %v", args)
	}
}

func TestBuildArgs_HostsAndGroups(t *testing.T) {
	c := DiffusionRunConfig{
		Hosts: []deploy.InventoryHost{
			{Name: "web01", Variables: map[string]string{"ansible_host": "1.2.3.4"}},
		},
		Groups: []deploy.InventoryGroup{
			{Name: "webservers", Hosts: []string{"web01"}},
		},
	}
	args := buildArgs(c)
	if !hasFlag(args, "--host") {
		t.Errorf("expected --host flag, got %v", args)
	}
	if !hasFlag(args, "--group") {
		t.Errorf("expected --group flag, got %v", args)
	}
}

func TestBuildArgs_GlobalAndExtraVars(t *testing.T) {
	c := DiffusionRunConfig{
		GlobalVars: map[string]string{"env": "prod"},
		ExtraVars:  map[string]string{"debug": "true"},
	}
	args := buildArgs(c)
	if !hasFlag(args, "--var") {
		t.Errorf("expected --var flag, got %v", args)
	}
	if !hasFlag(args, "--extra-var") {
		t.Errorf("expected --extra-var flag, got %v", args)
	}
}

func TestBuildArgs_NoPathFlags(t *testing.T) {
	// Ensure removed flags never appear
	c := DiffusionRunConfig{}
	args := buildArgs(c)
	for _, a := range args {
		if a == "--roles-path" || a == "--collections-path" {
			t.Errorf("unexpected removed flag %q in args %v", a, args)
		}
	}
}

// ---------------------------------------------------------------------------
// computeResultRunID
// ---------------------------------------------------------------------------

func TestComputeResultRunID_Stability(t *testing.T) {
	c := DiffusionRunConfig{
		Playbook: "site.yml",
		RoleSources: []RoleSourceEntry{
			{SCM: "galaxy", Version: ">=6.0.0", Galaxy: "ns.role"},
		},
	}
	id1 := computeResultRunID(c)
	id2 := computeResultRunID(c)
	if id1 != id2 {
		t.Errorf("run ID not stable: %q vs %q", id1, id2)
	}
}

func TestComputeResultRunID_DiffersOnChange(t *testing.T) {
	base := DiffusionRunConfig{Playbook: "site.yml"}
	changed := DiffusionRunConfig{Playbook: "other.yml"}
	if computeResultRunID(base) == computeResultRunID(changed) {
		t.Error("expected different run IDs for different playbooks")
	}
}

func TestComputeResultRunID_Length(t *testing.T) {
	id := computeResultRunID(DiffusionRunConfig{})
	if len(id) != 16 {
		t.Errorf("expected run ID length 16, got %d", len(id))
	}
}
