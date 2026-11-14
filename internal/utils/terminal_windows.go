//go:build windows

package utils

import (
	"os"

	"github.com/mattn/go-isatty"
)

// CanRunInteractiveTUI reports whether stdin and stdout are both a real
// Windows console (a native console or ConPTY) suitable for a raw-input
// TUI such as Bubble Tea.
//
// This deliberately excludes Cygwin/MSYS pty terminals (Git Bash/mintty):
// isatty.IsCygwinTerminal reports true for those, which is fine for color
// and width decisions, but bubbletea's Windows input driver only puts
// the console into raw input mode via the Windows console APIs when
// isatty.IsTerminal is true — which is never the case over an MSYS named
// pipe. Running the TUI there leaves raw mode off, so keystrokes are only
// delivered line-buffered (on Enter) and the viewer appears frozen.
func CanRunInteractiveTUI() bool {
	return isatty.IsTerminal(os.Stdin.Fd()) && isatty.IsTerminal(os.Stdout.Fd())
}
