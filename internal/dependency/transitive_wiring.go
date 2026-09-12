package dependency

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/role"
	"diffusion/internal/secrets"
)

// expandTransitiveAllScenarios runs the transitive resolver once per scenario
// present in the lock file, then reassembles a single lock file.
//
// Entries are grouped by their "<scenario>." name prefix so that a nested
// dependency is attributed to the scenario that pulled it in. Tools and Python
// are carried over from the input lock untouched.
func expandTransitiveAllScenarios(lockFile *LockFile, opts TransitiveOptions) (*LockFile, error) {
	scenarios := lockScenarios(lockFile)
	if len(scenarios) == 0 {
		return lockFile, nil
	}

	result := *lockFile
	result.Collections = nil
	result.Roles = nil

	for _, scenario := range scenarios {
		prefix := scenario + "."

		scoped := &LockFile{
			Version: lockFile.Version,
			Python:  lockFile.Python,
			Tools:   lockFile.Tools,
		}
		for _, e := range lockFile.Collections {
			if strings.HasPrefix(e.Name, prefix) {
				scoped.Collections = append(scoped.Collections, e)
			}
		}
		for _, e := range lockFile.Roles {
			if strings.HasPrefix(e.Name, prefix) {
				scoped.Roles = append(scoped.Roles, e)
			}
		}

		expanded, warnings, err := ResolveTransitive(scenario, scoped, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve transitive dependencies for scenario %s: %w", scenario, err)
		}
		printTransitiveWarnings(warnings)

		result.Collections = append(result.Collections, expanded.Collections...)
		result.Roles = append(result.Roles, expanded.Roles...)
	}

	return &result, nil
}

// lockScenarios returns the distinct scenario prefixes present in a lock file,
// in first-seen order so the regenerated file keeps a stable layout.
func lockScenarios(lockFile *LockFile) []string {
	seen := map[string]bool{}
	var scenarios []string

	collect := func(entries []LockFileEntry) {
		for _, e := range entries {
			parts := strings.SplitN(e.Name, ".", 2)
			if len(parts) != 2 {
				continue
			}
			if !seen[parts[0]] {
				seen[parts[0]] = true
				scenarios = append(scenarios, parts[0])
			}
		}
	}
	collect(lockFile.Collections)
	collect(lockFile.Roles)

	return scenarios
}

// printTransitiveWarnings reports skipped / unreachable dependencies in yellow.
func printTransitiveWarnings(warnings []string) {
	for _, w := range warnings {
		fmt.Printf("%s%s%s\n", config.ColorYellow, w, config.ColorReset)
	}
}

// warnOnWeakSelfIdentity reports when the only thing we know about ourselves
// is the working directory name. In that case a remote dependency that depends
// back on this repository cannot be recognised by URL or Galaxy name, and the
// cycle is only broken by the dependency graph itself (the visited set), which
// still terminates but may fetch our own repo once.
func warnOnWeakSelfIdentity(identities []string) {
	if len(identities) > 1 {
		return
	}
	fmt.Printf("%stransitive: could not determine repository identity "+
		"(no git origin / meta role_name); cycle detection relies on dependency graph only%s\n",
		config.ColorYellow, config.ColorReset)
}

// DetectSelfIdentity collects identities for the repository we are currently
// locking. They seed the transitive resolver's visited set so a remote
// dependency that depends back on us is detected as a cycle rather than being
// fetched (and, worse, merged into its own lock file).
//
// Sources, all best-effort: the git "origin" remote URL, the role's
// "namespace.role_name" from meta/main.yml, and the working directory name.
func DetectSelfIdentity() []string {
	var identities []string

	if out, err := exec.Command("git", "-C", ".", "remote", "get-url", "origin").Output(); err == nil {
		if url := strings.TrimSpace(string(out)); url != "" {
			identities = append(identities, url)
		}
	}

	if meta, err := role.ParseMetaFile(); err == nil && meta != nil && meta.GalaxyInfo != nil {
		if meta.GalaxyInfo.RoleName != "" {
			identities = append(identities, meta.GalaxyInfo.RoleName)
			if meta.GalaxyInfo.Namespace != "" {
				identities = append(identities, meta.GalaxyInfo.Namespace+"."+meta.GalaxyInfo.RoleName)
			}
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		if base := filepath.Base(cwd); base != "" && base != "." && base != string(filepath.Separator) {
			identities = append(identities, base)
		}
	}

	return identities
}

// loadArtifactCredentials resolves credentials for every configured artifact
// source so that private git dependencies can be cloned. It reuses the shared
// secrets loader (Vault or local encrypted store) rather than duplicating it.
//
// Failures are non-fatal: a dependency we cannot authenticate to simply
// produces a fetch warning later.
func loadArtifactCredentials() []config.ArtifactCredentials {
	cfg, err := config.LoadConfig()
	if err != nil || cfg == nil || len(cfg.ArtifactSources) == 0 {
		return nil
	}

	var creds []config.ArtifactCredentials
	for _, src := range cfg.ArtifactSources {
		cred, err := secrets.GetArtifactCredentials(&src, cfg.HashicorpVault)
		if err != nil {
			fmt.Printf("%swarning: could not load credentials for artifact source %q: %v%s\n",
				config.ColorYellow, src.Name, err, config.ColorReset)
			continue
		}
		creds = append(creds, *cred)
	}

	return creds
}
