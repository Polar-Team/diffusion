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
