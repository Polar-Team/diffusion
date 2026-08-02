package deploy

import (
	"testing"
	"time"
)

func TestDefaultWaitConfig(t *testing.T) {
	cfg := DefaultWaitConfig()

	if cfg.InitialDelay != 10*time.Second {
		t.Errorf("expected InitialDelay 10s, got %v", cfg.InitialDelay)
	}
	if cfg.Interval != 15*time.Second {
		t.Errorf("expected Interval 15s, got %v", cfg.Interval)
	}
	if cfg.Timeout != 10*time.Minute {
		t.Errorf("expected Timeout 10m, got %v", cfg.Timeout)
	}
	if cfg.MaxAttempts != 20 {
		t.Errorf("expected MaxAttempts 20, got %d", cfg.MaxAttempts)
	}
}

func TestParseWaitDuration_ValidInputs(t *testing.T) {
	cases := []struct {
		input string
		want  time.Duration
	}{
		{"10s", 10 * time.Second},
		{"5m", 5 * time.Minute},
		{"1h30m", 90 * time.Minute},
		{"0", 0},
		{"", 0},
	}
	for _, c := range cases {
		got, err := ParseWaitDuration(c.input)
		if err != nil {
			t.Errorf("ParseWaitDuration(%q) unexpected error: %v", c.input, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseWaitDuration(%q) = %v, want %v", c.input, got, c.want)
		}
	}
}

func TestParseWaitDuration_Invalid(t *testing.T) {
	cases := []string{"abc", "10", "minutes"}
	for _, c := range cases {
		_, err := ParseWaitDuration(c)
		if err == nil {
			t.Errorf("ParseWaitDuration(%q) expected error, got nil", c)
		}
	}
}

func TestInjectSSHKeyPaths_WildcardSkipped(t *testing.T) {
	inventory := []byte(`all:
  hosts:
    web01:
      ansible_host: "1.2.3.4"
`)
	keys := map[string]string{"*": "c29tZWtleQ=="}
	out, err := injectSSHKeyPaths(inventory, keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Wildcard should not inject per-host key path.
	s := string(out)
	if contains(s, "ansible_ssh_private_key_file") {
		t.Errorf("expected no ansible_ssh_private_key_file for wildcard-only, got:\n%s", s)
	}
}

func TestInjectSSHKeyPaths_PerHost(t *testing.T) {
	inventory := []byte(`all:
  hosts:
    web01:
      ansible_host: "1.2.3.4"
    web02:
      ansible_host: "5.6.7.8"
`)
	keys := map[string]string{"web01": "a2V5MQ=="}
	out, err := injectSSHKeyPaths(inventory, keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !contains(s, "/tmp/ssh-keys/web01") {
		t.Errorf("expected key path for web01, got:\n%s", s)
	}
	// web02 should not have a key injected
	if contains(s, "/tmp/ssh-keys/web02") {
		t.Errorf("unexpected key path for web02, got:\n%s", s)
	}
}

func TestInjectSSHKeyPaths_HostNotInInventory(t *testing.T) {
	inventory := []byte(`all:
  hosts:
    web01:
      ansible_host: "1.2.3.4"
`)
	keys := map[string]string{"unknown-host": "a2V5MQ=="}
	out, err := injectSSHKeyPaths(inventory, keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Key name doesn't match any host — should be treated as a fallback and
	// injected into ALL hosts.
	s := string(out)
	if !contains(s, "/tmp/ssh-keys/unknown-host") {
		t.Errorf("expected fallback key path for web01, got:\n%s", s)
	}
}

func TestInjectSSHKeyPaths_DefaultKeyFallback(t *testing.T) {
	inventory := []byte(`all:
  hosts:
    waf-01:
      ansible_host: "3.91.150.155"
      ansible_user: "ubuntu"
    waf-02:
      ansible_host: "10.0.0.5"
      ansible_user: "ubuntu"
`)
	// "default" doesn't match any host name — should be applied to all hosts.
	keys := map[string]string{"default": "c29tZWtleQ=="}
	out, err := injectSSHKeyPaths(inventory, keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !contains(s, "/tmp/ssh-keys/default") {
		t.Errorf("expected fallback key '/tmp/ssh-keys/default' injected for all hosts, got:\n%s", s)
	}
	if !contains(s, "ansible_ssh_private_key_file") {
		t.Errorf("expected ansible_ssh_private_key_file in output, got:\n%s", s)
	}
}

func TestInjectSSHKeyPaths_MixedPerHostAndFallback(t *testing.T) {
	inventory := []byte(`all:
  hosts:
    web01:
      ansible_host: "1.2.3.4"
    web02:
      ansible_host: "5.6.7.8"
`)
	// web01 gets its own key, "default" covers web02 as fallback.
	keys := map[string]string{"web01": "a2V5MQ==", "default": "ZmFsbGJhY2s="}
	out, err := injectSSHKeyPaths(inventory, keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !contains(s, "/tmp/ssh-keys/web01") {
		t.Errorf("expected per-host key for web01, got:\n%s", s)
	}
	if !contains(s, "/tmp/ssh-keys/default") {
		t.Errorf("expected fallback key for web02, got:\n%s", s)
	}
}

func TestInjectSSHKeyPaths_GroupKey(t *testing.T) {
	inventory := []byte(`all:
  hosts:
    web01:
      ansible_host: "1.2.3.4"
    web02:
      ansible_host: "5.6.7.8"
    db01:
      ansible_host: "10.0.0.1"
  children:
    webservers:
      hosts:
        web01: {}
        web02: {}
    databases:
      hosts:
        db01: {}
`)
	// Group key applies to all hosts in the "webservers" group.
	keys := map[string]string{"group:webservers": "d2Vic2VydmVyX2tleQ=="}
	out, err := injectSSHKeyPaths(inventory, keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	// web01 and web02 should get the group key.
	if !contains(s, "group_webservers") {
		t.Errorf("expected group key path injected, got:\n%s", s)
	}
	if !contains(s, "ansible_ssh_private_key_file") {
		t.Errorf("expected ansible_ssh_private_key_file in output, got:\n%s", s)
	}
}

func TestInjectSSHKeyPaths_GroupKeyDoesNotOverridePerHost(t *testing.T) {
	inventory := []byte(`all:
  hosts:
    web01:
      ansible_host: "1.2.3.4"
    web02:
      ansible_host: "5.6.7.8"
  children:
    webservers:
      hosts:
        web01: {}
        web02: {}
`)
	// web01 gets a specific key, group key should only apply to web02.
	keys := map[string]string{
		"web01":            "cGVyX2hvc3Rfa2V5",
		"group:webservers": "Z3JvdXBfa2V5",
	}
	out, err := injectSSHKeyPaths(inventory, keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !contains(s, "/tmp/ssh-keys/web01") {
		t.Errorf("expected per-host key for web01, got:\n%s", s)
	}
	if !contains(s, "group_webservers") {
		t.Errorf("expected group key for web02, got:\n%s", s)
	}
}

func TestInjectSSHKeyPaths_MultipleGroupKeys(t *testing.T) {
	inventory := []byte(`all:
  hosts:
    web01:
      ansible_host: "1.2.3.4"
    db01:
      ansible_host: "10.0.0.1"
  children:
    webservers:
      hosts:
        web01: {}
    databases:
      hosts:
        db01: {}
`)
	keys := map[string]string{
		"group:webservers": "d2Vi",
		"group:databases":  "ZGI=",
	}
	out, err := injectSSHKeyPaths(inventory, keys)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !contains(s, "group_webservers") {
		t.Errorf("expected webservers group key, got:\n%s", s)
	}
	if !contains(s, "group_databases") {
		t.Errorf("expected databases group key, got:\n%s", s)
	}
}

func TestInjectSSHKeyPaths_InvalidYAML(t *testing.T) {
	_, err := injectSSHKeyPaths([]byte("not: [valid: yaml: {{"), nil)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestExtractSSHKeyDirsFromContent_NoKeys(t *testing.T) {
	inventory := []byte(`all:
  hosts:
    web01:
      ansible_host: "1.2.3.4"
`)
	dirs := extractSSHKeyDirsFromContent(inventory)
	if len(dirs) != 0 {
		t.Errorf("expected no extra dirs, got %v", dirs)
	}
}

func TestExtractSSHKeyDirsFromContent_Empty(t *testing.T) {
	dirs := extractSSHKeyDirsFromContent(nil)
	if dirs != nil {
		t.Errorf("expected nil for empty content, got %v", dirs)
	}
}

func TestRedactProbeArgs(t *testing.T) {
	args := []string{
		"run", "--rm",
		"-e", "DIFFUSION_INVENTORY_B64=dGhpcyBpcyBhIHZlcnkgbG9uZyBiYXNlNjQgc3RyaW5n",
		"-e", "SSH_KEY_WEB01=c2VjcmV0a2V5",
		"image:latest",
	}
	redacted := redactProbeArgs(args)
	if contains(redacted, "dGhpcyBpcyBhIHZlcnkgbG9uZyBiYXNlNjQgc3RyaW5n") {
		t.Error("expected inventory to be redacted")
	}
	if contains(redacted, "c2VjcmV0a2V5") {
		t.Error("expected SSH key to be redacted")
	}
	if !contains(redacted, "DIFFUSION_INVENTORY_B64=<") {
		t.Error("expected redacted inventory placeholder")
	}
	if !contains(redacted, "SSH_KEY_WEB01=<redacted>") {
		t.Error("expected redacted SSH key placeholder")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && containsStr(s, substr)
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
