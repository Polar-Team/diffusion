package deploy

import (
	"strings"
	"testing"
)

func TestGeneratePlaybook_SingleRoleDefaultApplyTo(t *testing.T) {
	sources := []RoleSource{
		{SCM: "galaxy", Galaxy: "geerlingguy.docker", Version: ">=6.0.0"},
	}
	out, err := GeneratePlaybook(sources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "hosts: all") {
		t.Errorf("expected default hosts 'all', got:\n%s", s)
	}
	if !strings.Contains(s, "geerlingguy.docker") {
		t.Errorf("expected role name in output, got:\n%s", s)
	}
	if !strings.Contains(s, "Auto-generated") {
		t.Errorf("expected header comment, got:\n%s", s)
	}
}

func TestGeneratePlaybook_MultipleApplyToGroups(t *testing.T) {
	sources := []RoleSource{
		{SCM: "galaxy", Galaxy: "myorg.common", Version: ">=1.0.0"},
		{SCM: "galaxy", Galaxy: "myorg.app", Version: ">=2.0.0", ApplyTo: "webservers"},
		{SCM: "galaxy", Galaxy: "myorg.db", Version: ">=1.0.0", ApplyTo: "dbservers"},
		{SCM: "galaxy", Galaxy: "myorg.monitor", Version: ">=1.0.0"}, // back to "all"
	}
	out, err := GeneratePlaybook(sources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	// All three host patterns should appear
	for _, pat := range []string{"hosts: all", "hosts: webservers", "hosts: dbservers"} {
		if !strings.Contains(s, pat) {
			t.Errorf("expected %q in output, got:\n%s", pat, s)
		}
	}
	// Roles should all appear
	for _, role := range []string{"myorg.common", "myorg.app", "myorg.db", "myorg.monitor"} {
		if !strings.Contains(s, role) {
			t.Errorf("expected role %q in output, got:\n%s", role, s)
		}
	}
}

func TestGeneratePlaybook_GitURLDeriveBaseName(t *testing.T) {
	sources := []RoleSource{
		{SCM: "git", URL: "https://github.com/myorg/ansible-common.git", Version: "main"},
	}
	out, err := GeneratePlaybook(sources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Basename should be "ansible-common" (stripped .git)
	if !strings.Contains(string(out), "ansible-common") {
		t.Errorf("expected basename 'ansible-common', got:\n%s", string(out))
	}
}

func TestGeneratePlaybook_NameOverride(t *testing.T) {
	sources := []RoleSource{
		{SCM: "galaxy", Galaxy: "namespace.long_role_name", Version: ">=1.0.0", Name: "short"},
	}
	out, err := GeneratePlaybook(sources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "short") {
		t.Errorf("expected overridden name 'short', got:\n%s", string(out))
	}
	if strings.Contains(string(out), "long_role_name") {
		t.Errorf("did not expect original name 'long_role_name' when Name override is set, got:\n%s", string(out))
	}
}

func TestGeneratePlaybook_NoSources(t *testing.T) {
	_, err := GeneratePlaybook(nil)
	if err == nil {
		t.Fatal("expected error for empty sources, got nil")
	}
}

func TestGeneratePlaybook_GatherFactsTrue(t *testing.T) {
	sources := []RoleSource{
		{SCM: "galaxy", Galaxy: "ns.role", Version: ">=1.0.0"},
	}
	out, err := GeneratePlaybook(sources)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "gather_facts: true") {
		t.Errorf("expected gather_facts: true, got:\n%s", string(out))
	}
}

func TestRoleBaseName(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"namespace.role", "role"},
		{"bare_role", "bare_role"},
		{"a.b.c", "b.c"},
	}
	for _, c := range cases {
		got := roleBaseName(c.input)
		if got != c.want {
			t.Errorf("roleBaseName(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}

func TestEffectiveName(t *testing.T) {
	cases := []struct {
		rs   RoleSource
		want string
	}{
		{RoleSource{Name: "override"}, "override"},
		{RoleSource{Galaxy: "ns.role"}, "ns.role"},
		{RoleSource{URL: "https://github.com/org/my-role.git"}, "my-role"},
		{RoleSource{URL: "https://github.com/org/my-role"}, "my-role"},
	}
	for _, c := range cases {
		got := c.rs.EffectiveName()
		if got != c.want {
			t.Errorf("EffectiveName(%+v) = %q, want %q", c.rs, got, c.want)
		}
	}
}

func TestEffectiveApplyTo(t *testing.T) {
	cases := []struct {
		rs   RoleSource
		want string
	}{
		{RoleSource{}, "all"},
		{RoleSource{ApplyTo: "webservers"}, "webservers"},
		{RoleSource{ApplyTo: "all"}, "all"},
	}
	for _, c := range cases {
		got := c.rs.EffectiveApplyTo()
		if got != c.want {
			t.Errorf("EffectiveApplyTo(%+v) = %q, want %q", c.rs, got, c.want)
		}
	}
}
