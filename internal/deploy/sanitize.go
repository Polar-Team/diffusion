package deploy

import (
	"fmt"
	"regexp"
	"strings"
)

// sshKeyNameAllowlist restricts SSH key/host names to characters that are
// safe to interpolate into shell command strings and env var / file names.
// Hostnames, IPs, "group:<name>" prefixes, and the "*" wildcard are all
// permitted; anything else (spaces, quotes, shell metacharacters, newlines,
// etc.) is rejected.
var sshKeyNameAllowlist = regexp.MustCompile(`^[A-Za-z0-9_.:*-]+$`)

// ValidateSSHKeyName rejects any SSH key/host name that is not composed
// exclusively of allowlisted characters. Exported so CLI flag parsing
// (internal/cli) can surface a fast, clear usage error before the value ever
// reaches the deploy package. The deploy package itself also validates
// defensively via validateSSHKeyNames, since SSHKeys is a public field that
// other callers (e.g. the Terraform provider integration) can set directly.
func ValidateSSHKeyName(name string) error {
	return validateSSHKeyName(name)
}

// validateSSHKeyName rejects any SSH key/host name that is not composed
// exclusively of allowlisted characters. This MUST be called on every key in
// SSHKeys before it is used to build a shell command, env var name, or file
// path, since SSHKeys is a public field that can be set directly by callers
// that bypass CLI flag parsing (e.g. the Terraform provider integration).
func validateSSHKeyName(name string) error {
	if name == "" || !sshKeyNameAllowlist.MatchString(name) {
		return fmt.Errorf("invalid SSH key/host name %q: only letters, digits, '.', '-', '_', ':', '*' are allowed", name)
	}
	if name == "." || name == ".." || strings.HasSuffix(name, ":.") || strings.HasSuffix(name, ":..") {
		return fmt.Errorf("invalid SSH key/host name %q: dot-only path segments are not allowed", name)
	}
	// The "*" wildcard is the one legitimate name whose sanitized forms equal
	// the reserved sentinels; allow it explicitly.
	if name == "*" {
		return nil
	}
	// Reject any other name that sanitizes to the same env var suffix or
	// filename as the "*" wildcard key, which would silently clobber one key
	// with the other. The env var side uses ToUpper, so the collision is
	// case-insensitive there (e.g. "wildcard", "WILDCARD"); the filename side
	// is case-sensitive (e.g. "_wildcard_"). Deriving these from the sanitize
	// helpers keeps the guard correct if those helpers change.
	if strings.EqualFold(sshKeyEnvSanitize(name), sshKeyEnvSanitize("*")) ||
		sshKeyFileName(name) == sshKeyFileName("*") {
		return fmt.Errorf("invalid SSH key/host name %q: collides with the reserved wildcard (%q) key sentinel", name, "*")
	}
	return nil
}

// validateSSHKeyNames validates every key in the given SSH keys map.
func validateSSHKeyNames(sshKeys map[string]string) error {
	for host := range sshKeys {
		if err := validateSSHKeyName(host); err != nil {
			return err
		}
	}
	return nil
}
