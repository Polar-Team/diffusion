package dependency

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const scenariosDirName = "scenarios"

// DiscoverScenarios returns the list of molecule scenarios found in the
// scenarios/ directory. When the directory does not exist (or is empty),
// it falls back to the implicit ["default"] scenario.
func DiscoverScenarios() []string {
	scenarios := []string{}

	if info, err := os.Stat(scenariosDirName); err == nil && info.IsDir() {
		entries, err := os.ReadDir(scenariosDirName)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() {
					scenarios = append(scenarios, entry.Name())
				}
			}
		}
	}

	if len(scenarios) == 0 {
		scenarios = append(scenarios, "default")
	}

	return scenarios
}

// ResolveScenarios resolves the scenario selector into a concrete list of
// scenarios to operate on. An empty selector means "all scenarios".
// A non-empty selector is validated against the scenarios/ directory and,
// as a fallback, against the dependency configuration in diffusion.toml.
func ResolveScenarios(scenario string) ([]string, error) {
	if scenario == "" {
		return DiscoverScenarios(), nil
	}

	// scenarios/<scenario>/ exists
	if info, err := os.Stat(filepath.Join(scenariosDirName, scenario)); err == nil && info.IsDir() {
		return []string{scenario}, nil
	}

	// "default" is implicit when no scenarios/ directory exists at all
	if scenario == "default" {
		if info, err := os.Stat(scenariosDirName); err != nil || !info.IsDir() {
			return []string{scenario}, nil
		}
	}

	// Fall back to diffusion.toml: the scenario is known if any dependency
	// entry is prefixed with "<scenario>.".
	depConfig, err := LoadDependencyConfig()
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		// A missing diffusion.toml simply means "no configured scenarios";
		// anything else is a real problem the caller must know about.
		return nil, fmt.Errorf("failed to load dependency config: %w", err)
	}
	if depConfig != nil {
		prefix := scenario + "."
		for _, col := range depConfig.Collections {
			if strings.HasPrefix(col.Name, prefix) {
				return []string{scenario}, nil
			}
		}
		for _, r := range depConfig.Roles {
			if strings.HasPrefix(r.Name, prefix) {
				return []string{scenario}, nil
			}
		}
	}

	return nil, fmt.Errorf("scenario %q not found (no %s/%s directory)", scenario, scenariosDirName, scenario)
}

// IncludesDefaultScenario reports whether the given scenario selector covers
// the "default" scenario, and therefore whether meta/main.yml must be
// processed. meta/main.yml only ever holds default-scenario collections, but
// it is always processed for the "all scenarios" selector ("") — even in
// repositories that have no scenarios/default/ directory — to preserve the
// historical behaviour of `deps sync` / `deps check` without a flag.
func IncludesDefaultScenario(scenario string) bool {
	return scenario == "" || scenario == "default"
}

// NeedsFullGeneration reports whether an update must regenerate the lock file
// for every scenario instead of performing a scenario-scoped merge. That is
// the case for the "all scenarios" selector ("") and whenever no diffusion.lock
// exists yet — a scoped merge into a missing lock file would produce a lock
// file containing only the requested scenario.
func NeedsFullGeneration(scenario string, existing *LockFile) bool {
	return scenario == "" || existing == nil
}

// MergeLockFileForScenario merges a freshly generated, scenario-scoped lock
// file into an existing lock file. All entries belonging to other scenarios
// are preserved in their original order, followed by the freshly generated
// entries of the target scenario. Tools and Python metadata are global and
// always taken from the fresh lock file. The existing lock file is never
// mutated.
//
// When existing is nil (or scenario is empty) the fresh lock file is returned
// as-is. Callers must not rely on that fallback to produce a complete lock
// file: UpdateLockFile guards against it via NeedsFullGeneration and performs
// a full regeneration instead, so a scenario-only lock file is never written
// to disk.
func MergeLockFileForScenario(existing *LockFile, fresh *LockFile, scenario string) *LockFile {
	if fresh == nil {
		return existing
	}
	if existing == nil || scenario == "" {
		return fresh
	}

	prefix := scenario + "."

	merged := &LockFile{
		Version:     LockFileVersion,
		Hash:        fresh.Hash,
		Python:      fresh.Python,
		Collections: make([]LockFileEntry, 0, len(existing.Collections)+len(fresh.Collections)),
		Roles:       make([]LockFileEntry, 0, len(existing.Roles)+len(fresh.Roles)),
		Tools:       fresh.Tools,
	}

	for _, col := range existing.Collections {
		if !strings.HasPrefix(col.Name, prefix) {
			merged.Collections = append(merged.Collections, col)
		}
	}
	merged.Collections = append(merged.Collections, fresh.Collections...)

	for _, r := range existing.Roles {
		if !strings.HasPrefix(r.Name, prefix) {
			merged.Roles = append(merged.Roles, r)
		}
	}
	merged.Roles = append(merged.Roles, fresh.Roles...)

	return merged
}
