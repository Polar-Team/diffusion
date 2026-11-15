package utils

import (
	"os"
	"strconv"
	"strings"

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

// IsStdinStdoutTerminal reports whether both stdin and stdout are
// connected to interactive terminals, including Windows ConPTY/Cygwin
// (MSYS/mintty) terminals. This is appropriate for general interactivity
// decisions (colors, prompts, spinners). It is deliberately NOT
// sufficient for deciding whether a raw-input TUI (e.g. Bubble Tea) can
// run: see CanRunInteractiveTUI for that.
func IsStdinStdoutTerminal() bool {
	inFd, outFd := os.Stdin.Fd(), os.Stdout.Fd()
	in := isatty.IsTerminal(inFd) || isatty.IsCygwinTerminal(inFd)
	out := isatty.IsTerminal(outFd) || isatty.IsCygwinTerminal(outFd)
	return in && out
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

// ciEnvVendorVars are environment variables set unconditionally by common
// CI providers, used as a fallback signal when CI itself isn't set.
var ciEnvVendorVars = []string{
	"GITHUB_ACTIONS", "GITLAB_CI", "TF_BUILD", "TEAMCITY_VERSION", "BUILDKITE", "JENKINS_URL",
}

// IsCIEnv reports whether the process appears to be running in a CI
// pipeline.
//
// Deliberate convention:
//   - CI counts as "in CI" when it is non-empty (after trimming
//     whitespace) and not a strconv.ParseBool "false" value (so CI=false
//     and CI=0 mean "not CI", while CI=true/1/yes/anything-else-non-empty
//     means CI).
//   - The vendor variables are an independent OR: if any of them is set,
//     the result is CI even when CI itself is explicitly false — a
//     pipeline that sets CI=false to opt out of some CI-only behavior
//     (uncommon but seen in the wild) is still a CI environment for our
//     purposes, since GITHUB_ACTIONS etc. are set unconditionally by
//     their platform regardless of a job's own CI override.
func IsCIEnv() bool {
	return isCIEnv(os.Getenv)
}

// isCIEnv is the pure, testable core of IsCIEnv.
func isCIEnv(getenv func(string) string) bool {
	if v := strings.TrimSpace(getenv("CI")); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			if b {
				return true
			}
		} else {
			// Non-empty but not a recognized boolean (e.g. "yes"): treat
			// as truthy.
			return true
		}
	}
	for _, k := range ciEnvVendorVars {
		if getenv(k) != "" {
			return true
		}
	}
	return false
}
