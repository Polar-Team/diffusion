//go:build !windows

package utils

// CanRunInteractiveTUI reports whether stdin and stdout are both
// interactive terminals suitable for running a raw-input TUI (e.g.
// Bubble Tea). On non-Windows platforms this is the same check as
// IsStdinStdoutTerminal: there is no Cygwin/ConPTY raw-mode caveat here.
func CanRunInteractiveTUI() bool {
	return IsStdinStdoutTerminal()
}
