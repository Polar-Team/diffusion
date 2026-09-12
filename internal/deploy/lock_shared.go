package deploy

import (
	"diffusion/internal/config"
	"diffusion/internal/dependency"
)

// The lock merging algorithm lives in internal/dependency so that the
// dependency package (which cannot import deploy — deploy already imports
// dependency) can reuse it for transitive dependency resolution.
//
// The symbols below preserve the historical deploy API.

// MergeLocks takes lock files from N remote role repos and produces a single
// merged lock that satisfies all constraints across all sources.
//
// Merge rules:
//   - Collections / Roles with the same key: intersect constraints, re-resolve
//     via Galaxy API to find the highest version satisfying all of them.
//   - Python version: min = max(all mins), max = min(all maxes).
//   - Tools (ansible, molecule, etc.): same intersection as collections.
//   - python_deps maps: merge; on key conflict keep the higher pinned version.
func MergeLocks(locks []dependency.LockFile) (*dependency.LockFile, error) {
	return dependency.MergeLocks(locks)
}

// readLockFromDir reads and parses diffusion.lock from the given directory.
// Returns (nil, nil) if the file does not exist (role has no lock).
func readLockFromDir(dir string) (*dependency.LockFile, error) {
	return dependency.ReadLockFromDir(dir)
}

// resolveGitRef converts a version constraint or ref name into a usable git ref.
func resolveGitRef(version string) string {
	return dependency.ResolveGitRef(version)
}

// buildGitEnv constructs an os.Environ slice injecting credential env vars for
// the matching artifact credential.
func buildGitEnv(repoURL string, creds []config.ArtifactCredentials) []string {
	return dependency.BuildGitEnv(repoURL, creds)
}

// stripScheme removes a URL scheme / scp-style user prefix.
func stripScheme(u string) string {
	return dependency.StripScheme(u)
}

// sanitizeEnvKey converts an artifact source name into an env-var-safe suffix.
func sanitizeEnvKey(name string) string {
	return dependency.SanitizeEnvKey(name)
}
