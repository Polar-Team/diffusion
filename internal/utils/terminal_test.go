package utils

import (
	"os"
	"testing"
)

func TestIsTerminal(t *testing.T) {
	// In a test environment, stdout is typically not a real terminal (it's a pipe).
	// This validates that IsTerminal correctly reports non-TTY when running under `go test`.
	result := IsTerminal()

	// go test redirects stdout, so we expect false in CI/test environments
	if result {
		// If this is somehow running in a real terminal, that's also valid —
		// just verify it doesn't panic and returns a bool.
		t.Logf("IsTerminal() = true (test running in a real terminal)")
	} else {
		t.Logf("IsTerminal() = false (test running in piped/non-TTY environment)")
	}
}

func TestIsInteractive_CIModeTrue(t *testing.T) {
	// CI mode should always return false, regardless of terminal state
	if IsInteractive(true) {
		t.Error("IsInteractive(true) should always return false in CI mode")
	}
}

func TestIsInteractive_CIModeFalse(t *testing.T) {
	// With ciMode=false, result depends on actual TTY state.
	// Under `go test`, stdout is piped so IsInteractive should be false.
	result := IsInteractive(false)

	if result {
		// Running in a real terminal — valid
		t.Logf("IsInteractive(false) = true (real terminal detected)")
	} else {
		t.Logf("IsInteractive(false) = false (no TTY, as expected under go test)")
	}
}

func TestIsInteractive_ConsistentWithIsTerminal(t *testing.T) {
	// When ciMode is false, IsInteractive should match IsTerminal
	terminal := IsTerminal()
	interactive := IsInteractive(false)

	if terminal != interactive {
		t.Errorf("IsInteractive(false) = %v but IsTerminal() = %v; they should match when ciMode is false", interactive, terminal)
	}
}

func TestIsTerminal_StdoutFd(t *testing.T) {
	// Verify os.Stdout.Fd() doesn't panic — basic sanity check
	fd := os.Stdout.Fd()
	t.Logf("os.Stdout.Fd() = %d", fd)
}

func TestIsStdinStdoutTerminal(t *testing.T) {
	// Under `go test`, stdin/stdout are typically piped, so this should be
	// false. Just assert it doesn't panic and matches reality either way.
	result := IsStdinStdoutTerminal()
	t.Logf("IsStdinStdoutTerminal() = %v", result)
}

func TestIsCIEnvPure(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"nothing set", map[string]string{}, false},
		{"CI=true", map[string]string{"CI": "true"}, true},
		{"CI=1", map[string]string{"CI": "1"}, true},
		{"CI=false", map[string]string{"CI": "false"}, false},
		{"CI=0", map[string]string{"CI": "0"}, false},
		{"CI=yes (unparseable, non-empty)", map[string]string{"CI": "yes"}, true},
		{"CI with surrounding whitespace, false", map[string]string{"CI": "  false  "}, false},
		{"CI with surrounding whitespace, true", map[string]string{"CI": "  true  "}, true},
		{"CI=false but GITHUB_ACTIONS set", map[string]string{"CI": "false", "GITHUB_ACTIONS": "true"}, true},
		{"GITHUB_ACTIONS set", map[string]string{"GITHUB_ACTIONS": "true"}, true},
		{"GITLAB_CI set", map[string]string{"GITLAB_CI": "true"}, true},
		{"TF_BUILD set", map[string]string{"TF_BUILD": "True"}, true},
		{"TEAMCITY_VERSION set", map[string]string{"TEAMCITY_VERSION": "2023.1"}, true},
		{"BUILDKITE set", map[string]string{"BUILDKITE": "true"}, true},
		{"JENKINS_URL set", map[string]string{"JENKINS_URL": "http://jenkins"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(k string) string { return tc.env[k] }
			if got := isCIEnv(lookup); got != tc.want {
				t.Errorf("isCIEnv(%v) = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}

func TestIsCIEnv_RealEnv(t *testing.T) {
	// Exercise the exported wrapper against the real environment, using
	// t.Setenv so it's isolated and restored automatically.
	t.Setenv("CI", "true")
	for _, k := range ciEnvVendorVars {
		t.Setenv(k, "")
	}
	if !IsCIEnv() {
		t.Error("expected IsCIEnv() to be true with CI=true")
	}

	t.Setenv("CI", "false")
	if IsCIEnv() {
		t.Error("expected IsCIEnv() to be false with CI=false and no vendor vars")
	}
}

func TestCanRunInteractiveTUI(t *testing.T) {
	// Under `go test`, stdin/stdout are piped, so this should be false on
	// every platform. Just assert it doesn't panic.
	result := CanRunInteractiveTUI()
	t.Logf("CanRunInteractiveTUI() = %v", result)
}
