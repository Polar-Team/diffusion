package utils

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsTerminal reports whether stdout is connected to a terminal (TTY).
// This is used to determine whether to use interactive features like
// spinners, ANSI escape codes, and docker -ti flags.
// Non-TTY environments (CI pipelines, agent/MCP execution, piped output)
// will get non-interactive behavior automatically.
func IsTerminal() bool {
	return isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd())
}

// IsInteractive returns true when the process can safely use interactive
// features (spinners, docker -ti, ANSI colors). It returns false in CI mode
// OR when no TTY is available (e.g., running under an agent/MCP tool).
func IsInteractive(ciMode bool) bool {
	if ciMode {
		return false
	}
	return IsTerminal()
}
