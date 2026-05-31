package cli

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// parseKVPairs
// ---------------------------------------------------------------------------

func TestParseKVPairs_SimpleKeyValues(t *testing.T) {
	got, err := parseKVPairs("scm=git,version=main")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["scm"] != "git" || got["version"] != "main" {
		t.Errorf("unexpected result: %v", got)
	}
}

func TestParseKVPairs_ValueContainsEquals(t *testing.T) {
	// version constraint ">=1.0.0" contains '=' — Cut splits only on first '='
	got, err := parseKVPairs("version=>=1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["version"] != ">=1.0.0" {
		t.Errorf("expected version '>=1.0.0', got %q", got["version"])
	}
}

func TestParseKVPairs_URLValue(t *testing.T) {
	got, err := parseKVPairs("scm=git,version=main,url=https://github.com/org/role.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["url"] != "https://github.com/org/role.git" {
		t.Errorf("expected full URL, got %q", got["url"])
	}
}

func TestParseKVPairs_EmptyString(t *testing.T) {
	got, err := parseKVPairs("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map for empty input, got %v", got)
	}
}

func TestParseKVPairs_MissingEquals(t *testing.T) {
	_, err := parseKVPairs("scm=git,noequals")
	if err == nil {
		t.Fatal("expected error for token without '=', got nil")
	}
	if !strings.Contains(err.Error(), "noequals") {
		t.Errorf("expected error to mention token, got: %v", err)
	}
}

func TestParseKVPairs_TrailingComma(t *testing.T) {
	// trailing comma produces an empty token which should be ignored
	got, err := parseKVPairs("scm=git,version=main,")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 keys, got %d: %v", len(got), got)
	}
}

// ---------------------------------------------------------------------------
// parseRoleSources
// ---------------------------------------------------------------------------

func TestParseRoleSources_GalaxyHappyPath(t *testing.T) {
	sources, err := parseRoleSources([]string{
		"scm=galaxy,version=>=6.0.0,galaxy=geerlingguy.docker",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 1 {
		t.Fatalf("expected 1 source, got %d", len(sources))
	}
	s := sources[0]
	if s.SCM != "galaxy" || s.Version != ">=6.0.0" || s.Galaxy != "geerlingguy.docker" {
		t.Errorf("unexpected source: %+v", s)
	}
}

func TestParseRoleSources_GitHappyPath(t *testing.T) {
	sources, err := parseRoleSources([]string{
		"scm=git,version=main,url=https://github.com/org/role.git",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sources[0].URL != "https://github.com/org/role.git" {
		t.Errorf("expected URL preserved, got %q", sources[0].URL)
	}
}

func TestParseRoleSources_WithNameAndApplyTo(t *testing.T) {
	sources, err := parseRoleSources([]string{
		"scm=galaxy,version=>=1.0.0,galaxy=ns.role,name=myrole,apply_to=webservers",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := sources[0]
	if s.Name != "myrole" || s.ApplyTo != "webservers" {
		t.Errorf("expected name=myrole apply_to=webservers, got %+v", s)
	}
}

func TestParseRoleSources_MultipleEntries(t *testing.T) {
	sources, err := parseRoleSources([]string{
		"scm=galaxy,version=>=1.0.0,galaxy=ns.a",
		"scm=galaxy,version=>=2.0.0,galaxy=ns.b",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sources) != 2 {
		t.Errorf("expected 2 sources, got %d", len(sources))
	}
}

func TestParseRoleSources_MissingSCM(t *testing.T) {
	_, err := parseRoleSources([]string{"version=main,url=https://github.com/org/role.git"})
	if err == nil {
		t.Fatal("expected error for missing scm")
	}
	if !strings.Contains(err.Error(), "scm") {
		t.Errorf("expected error to mention 'scm', got: %v", err)
	}
}

func TestParseRoleSources_MissingVersion(t *testing.T) {
	_, err := parseRoleSources([]string{"scm=galaxy,galaxy=ns.role"})
	if err == nil {
		t.Fatal("expected error for missing version")
	}
	if !strings.Contains(err.Error(), "version") {
		t.Errorf("expected error to mention 'version', got: %v", err)
	}
}

func TestParseRoleSources_GitMissingURL(t *testing.T) {
	_, err := parseRoleSources([]string{"scm=git,version=main"})
	if err == nil {
		t.Fatal("expected error for git without url")
	}
	if !strings.Contains(err.Error(), "url") {
		t.Errorf("expected error to mention 'url', got: %v", err)
	}
}

func TestParseRoleSources_GalaxyMissingGalaxy(t *testing.T) {
	_, err := parseRoleSources([]string{"scm=galaxy,version=>=1.0.0"})
	if err == nil {
		t.Fatal("expected error for galaxy without galaxy field")
	}
	if !strings.Contains(err.Error(), "galaxy") {
		t.Errorf("expected error to mention 'galaxy', got: %v", err)
	}
}

func TestParseRoleSources_BadSCM(t *testing.T) {
	_, err := parseRoleSources([]string{"scm=svn,version=main,url=https://x"})
	if err == nil {
		t.Fatal("expected error for unsupported scm")
	}
	if !strings.Contains(err.Error(), "svn") {
		t.Errorf("expected error to mention scm value, got: %v", err)
	}
}

func TestParseRoleSources_Empty(t *testing.T) {
	sources, err := parseRoleSources(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sources != nil {
		t.Errorf("expected nil for nil input, got %v", sources)
	}
}

// ---------------------------------------------------------------------------
// parseHosts
// ---------------------------------------------------------------------------

func TestParseHosts_HappyPath(t *testing.T) {
	hosts, err := parseHosts([]string{"web01=ansible_host=1.2.3.4,ansible_user=ubuntu"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("expected 1 host, got %d", len(hosts))
	}
	h := hosts[0]
	if h.Name != "web01" {
		t.Errorf("expected name 'web01', got %q", h.Name)
	}
	if h.Variables["ansible_host"] != "1.2.3.4" || h.Variables["ansible_user"] != "ubuntu" {
		t.Errorf("unexpected variables: %v", h.Variables)
	}
}

func TestParseHosts_NoVariables(t *testing.T) {
	// "hostname=" with empty vars is technically valid
	hosts, err := parseHosts([]string{"web01="})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hosts[0].Name != "web01" {
		t.Errorf("expected name 'web01', got %q", hosts[0].Name)
	}
}

func TestParseHosts_LocalConnection(t *testing.T) {
	hosts, err := parseHosts([]string{"testhost=ansible_connection=local"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hosts[0].Variables["ansible_connection"] != "local" {
		t.Errorf("expected ansible_connection=local, got %v", hosts[0].Variables)
	}
}

func TestParseHosts_NoEquals(t *testing.T) {
	_, err := parseHosts([]string{"web01"})
	if err == nil {
		t.Fatal("expected error for host without '='")
	}
}

func TestParseHosts_InvalidVarPair(t *testing.T) {
	_, err := parseHosts([]string{"web01=noequals"})
	if err == nil {
		t.Fatal("expected error for var without '='")
	}
}

func TestParseHosts_Multiple(t *testing.T) {
	hosts, err := parseHosts([]string{
		"web01=ansible_host=1.1.1.1",
		"web02=ansible_host=2.2.2.2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(hosts) != 2 {
		t.Errorf("expected 2 hosts, got %d", len(hosts))
	}
}

// ---------------------------------------------------------------------------
// parseGroups
// ---------------------------------------------------------------------------

func TestParseGroups_HappyPath(t *testing.T) {
	groups, err := parseGroups([]string{"webservers=web01,web02"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	g := groups[0]
	if g.Name != "webservers" {
		t.Errorf("expected name 'webservers', got %q", g.Name)
	}
	if len(g.Hosts) != 2 || g.Hosts[0] != "web01" || g.Hosts[1] != "web02" {
		t.Errorf("unexpected hosts: %v", g.Hosts)
	}
}

func TestParseGroups_SingleHost(t *testing.T) {
	groups, err := parseGroups([]string{"dbs=db01"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groups[0].Hosts[0] != "db01" {
		t.Errorf("expected 'db01', got %v", groups[0].Hosts)
	}
}

func TestParseGroups_NoEquals(t *testing.T) {
	_, err := parseGroups([]string{"webservers"})
	if err == nil {
		t.Fatal("expected error for group without '='")
	}
}

func TestParseGroups_TrimsSpaces(t *testing.T) {
	groups, err := parseGroups([]string{"g= h1 , h2 "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if groups[0].Hosts[0] != "h1" || groups[0].Hosts[1] != "h2" {
		t.Errorf("expected trimmed host names, got %v", groups[0].Hosts)
	}
}

// ---------------------------------------------------------------------------
// parseKeyValues
// ---------------------------------------------------------------------------

func TestParseKeyValues_HappyPath(t *testing.T) {
	m, err := parseKeyValues([]string{"env=production", "debug=false"}, "var")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["env"] != "production" || m["debug"] != "false" {
		t.Errorf("unexpected map: %v", m)
	}
}

func TestParseKeyValues_Nil(t *testing.T) {
	m, err := parseKeyValues(nil, "var")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Errorf("expected nil for nil input, got %v", m)
	}
}

func TestParseKeyValues_MissingEquals(t *testing.T) {
	_, err := parseKeyValues([]string{"noequals"}, "var")
	if err == nil {
		t.Fatal("expected error for missing '='")
	}
	if !strings.Contains(err.Error(), "var") {
		t.Errorf("expected error to mention flag name 'var', got: %v", err)
	}
}

func TestParseKeyValues_ValueWithEquals(t *testing.T) {
	// Cut uses only first '=' so value can contain '='
	m, err := parseKeyValues([]string{"constraint=>=1.0.0"}, "extra-var")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m["constraint"] != ">=1.0.0" {
		t.Errorf("expected value '>=1.0.0', got %q", m["constraint"])
	}
}

// ---------------------------------------------------------------------------
// parseWaitConfig
// ---------------------------------------------------------------------------

func TestParseWaitConfig_ValidDurations(t *testing.T) {
	f := &deployFlags{
		hostWaitInitialDelay: "5s",
		hostWaitInterval:     "10s",
		hostWaitTimeout:      "2m",
	}
	cfg, err := parseWaitConfig(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.InitialDelay != 5*time.Second {
		t.Errorf("expected InitialDelay=5s, got %v", cfg.InitialDelay)
	}
	if cfg.Interval != 10*time.Second {
		t.Errorf("expected Interval=10s, got %v", cfg.Interval)
	}
	if cfg.Timeout != 2*time.Minute {
		t.Errorf("expected Timeout=2m, got %v", cfg.Timeout)
	}
}

func TestParseWaitConfig_EmptyUsesDefaults(t *testing.T) {
	f := &deployFlags{}
	cfg, err := parseWaitConfig(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.InitialDelay == 0 || cfg.Interval == 0 || cfg.Timeout == 0 {
		t.Errorf("expected non-zero defaults, got %+v", cfg)
	}
}

func TestParseWaitConfig_InvalidDuration(t *testing.T) {
	f := &deployFlags{hostWaitInitialDelay: "notaduration"}
	_, err := parseWaitConfig(f)
	if err == nil {
		t.Fatal("expected error for invalid duration")
	}
	if !strings.Contains(err.Error(), "host-wait-initial-delay") {
		t.Errorf("expected error to mention flag, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NewDeployCmd — command structure
// ---------------------------------------------------------------------------

func TestNewDeployCmd_Use(t *testing.T) {
	cmd := NewDeployCmd(&CLI{})
	if cmd.Use != "deploy" {
		t.Errorf("expected Use='deploy', got %q", cmd.Use)
	}
}

func TestNewDeployCmd_AllFlagsRegistered(t *testing.T) {
	cmd := NewDeployCmd(&CLI{})
	required := []string{
		"role-source",
		"playbook",
		"host",
		"group",
		"var",
		"extra-var",
		"skip-period",
		"host-wait-initial-delay",
		"host-wait-interval",
		"host-wait-timeout",
	}
	for _, name := range required {
		if f := cmd.Flags().Lookup(name); f == nil {
			t.Errorf("flag --%s not registered", name)
		}
	}
}

func TestNewDeployCmd_RoleSourceIsRequired(t *testing.T) {
	cmd := NewDeployCmd(&CLI{})
	// Execute without --role-source; Cobra should return an error about the required flag.
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --role-source is missing")
	}
	if !strings.Contains(err.Error(), "role-source") {
		t.Errorf("expected error to mention 'role-source', got: %v", err)
	}
}

func TestNewDeployCmd_PlaybookOptional(t *testing.T) {
	cmd := NewDeployCmd(&CLI{})
	f := cmd.Flags().Lookup("playbook")
	if f == nil {
		t.Fatal("--playbook flag not registered")
	}
	// Playbook should NOT be in required flags annotation
	ann := f.Annotations["cobra_annotation_bash_completion_one_required_flag"]
	if len(ann) > 0 {
		t.Errorf("--playbook should be optional, but has required annotation")
	}
}

func TestNewDeployCmd_HostWaitDefaultValues(t *testing.T) {
	cmd := NewDeployCmd(&CLI{})
	cases := map[string]string{
		"host-wait-initial-delay": "10s",
		"host-wait-interval":      "15s",
		"host-wait-timeout":       "10m",
	}
	for flag, want := range cases {
		f := cmd.Flags().Lookup(flag)
		if f == nil {
			t.Errorf("flag --%s not registered", flag)
			continue
		}
		if f.DefValue != want {
			t.Errorf("--%s default = %q, want %q", flag, f.DefValue, want)
		}
	}
}

// ---------------------------------------------------------------------------
// parseDurationWithDefault
// ---------------------------------------------------------------------------

func TestParseDurationWithDefault_EmptyReturnsDefault(t *testing.T) {
	d, err := parseDurationWithDefault("", 30*time.Second, "--flag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 30*time.Second {
		t.Errorf("expected 30s default, got %v", d)
	}
}

func TestParseDurationWithDefault_ParsesValue(t *testing.T) {
	d, err := parseDurationWithDefault("2m30s", 0, "--flag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d != 2*time.Minute+30*time.Second {
		t.Errorf("expected 2m30s, got %v", d)
	}
}

func TestParseDurationWithDefault_InvalidErrors(t *testing.T) {
	_, err := parseDurationWithDefault("invalid", 0, "--myflag")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "myflag") {
		t.Errorf("expected flag name in error, got: %v", err)
	}
}
