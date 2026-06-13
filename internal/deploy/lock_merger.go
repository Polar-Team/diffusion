package deploy

import (
	"fmt"
	"log"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/galaxy"
)

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
	if len(locks) == 0 {
		return &dependency.LockFile{
			Version: dependency.LockFileVersion,
			Python: &config.PythonVersion{
				Min:    config.DefaultMinPythonVersion,
				Max:    config.DefaultMaxPythonVersion,
				Pinned: config.PinnedPythonVersion,
			},
		}, nil
	}

	if len(locks) == 1 {
		l := locks[0]
		return &l, nil
	}

	merged := &dependency.LockFile{
		Version: dependency.LockFileVersion,
	}

	// --- Python version ---
	python, err := mergePythonVersions(locks)
	if err != nil {
		return nil, fmt.Errorf("python version conflict: %w", err)
	}
	merged.Python = python

	// --- Tools ---
	toolEntries, err := mergeEntries(locks, func(lf dependency.LockFile) []dependency.LockFileEntry {
		return lf.Tools
	}, "tool")
	if err != nil {
		return nil, fmt.Errorf("tool version conflict: %w", err)
	}
	merged.Tools = toolEntries

	// --- Collections ---
	colEntries, err := mergeEntries(locks, func(lf dependency.LockFile) []dependency.LockFileEntry {
		return lf.Collections
	}, "collection")
	if err != nil {
		return nil, fmt.Errorf("collection version conflict: %w", err)
	}
	merged.Collections = colEntries

	// --- Roles ---
	roleEntries, err := mergeEntries(locks, func(lf dependency.LockFile) []dependency.LockFileEntry {
		return lf.Roles
	}, "role")
	if err != nil {
		return nil, fmt.Errorf("role version conflict: %w", err)
	}
	merged.Roles = roleEntries

	return merged, nil
}

// mergeEntries merges lock file entries of a given kind across all locks.
// For each unique key (namespace.name) it collects all version constraints,
// intersects them (keeping the strictest lower-bound), then re-resolves via
// Galaxy API to find the highest satisfying version.
func mergeEntries(
	locks []dependency.LockFile,
	extract func(dependency.LockFile) []dependency.LockFileEntry,
	kind string,
) ([]dependency.LockFileEntry, error) {
	type accumulator struct {
		constraints []string
		base        dependency.LockFileEntry
		pythonDeps  map[string]string
	}

	acc := make(map[string]*accumulator)

	for _, lf := range locks {
		for _, entry := range extract(lf) {
			key := entryKey(entry)
			if _, exists := acc[key]; !exists {
				base := entry
				base.PythonDeps = nil
				acc[key] = &accumulator{
					base:       base,
					pythonDeps: make(map[string]string),
				}
			}
			// Collect this lock's constraint.
			if entry.Version != "" && entry.Version != "latest" {
				acc[key].constraints = append(acc[key].constraints, entry.Version)
			}
			// Merge python_deps: keep higher version on conflict.
			for pkg, ver := range entry.PythonDeps {
				if existing, ok := acc[key].pythonDeps[pkg]; !ok || compareSimpleVersions(ver, existing) > 0 {
					acc[key].pythonDeps[pkg] = ver
				}
			}
		}
	}

	galaxyAPI := galaxy.NewGalaxyAPI()
	var result []dependency.LockFileEntry

	for _, a := range acc {
		mergedConstraint := intersectConstraints(a.constraints)
		resolved, err := resolveEntry(galaxyAPI, a.base, mergedConstraint, kind)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve %s %q with constraint %q: %w",
				kind, entryKey(a.base), mergedConstraint, err)
		}

		entry := a.base
		entry.Version = mergedConstraint
		entry.ResolvedVersion = resolved
		if len(a.pythonDeps) > 0 {
			entry.PythonDeps = a.pythonDeps
		}
		result = append(result, entry)
	}

	return result, nil
}

// resolveEntry calls the Galaxy API (or git ls-remote for git roles) to find
// the highest version that satisfies mergedConstraint.
func resolveEntry(api *galaxy.GalaxyAPI, entry dependency.LockFileEntry, constraint, kind string) (string, error) {
	if constraint == "" || constraint == "latest" {
		// Use the already-resolved version from the lock if available.
		if entry.ResolvedVersion != "" {
			return entry.ResolvedVersion, nil
		}
	}

	switch kind {
	case "collection":
		ns, name := splitNamespaceAndName(entry.Namespace, entry.Name)
		resolved, err := api.ResolveVersion(ns, name, "collection", constraint)
		if err != nil {
			// Fall back to the previously resolved version from the lock.
			if entry.ResolvedVersion != "" {
				log.Printf(config.ColorYellow+
					"warning: Galaxy API unavailable for %s.%s; using cached resolved version %s"+config.ColorReset,
					ns, name, entry.ResolvedVersion)
				return entry.ResolvedVersion, nil
			}
			return "", err
		}
		return resolved, nil

	case "role":
		if entry.Source == "git" || entry.Src != "" {
			resolved, err := galaxy.ResolveVersionFromGit(entry.Src, constraint)
			if err != nil {
				if entry.ResolvedVersion != "" {
					return entry.ResolvedVersion, nil
				}
				return "", err
			}
			return resolved, nil
		}
		// Galaxy role
		ns, name := splitNamespaceAndName(entry.Namespace, entry.Name)
		resolved, err := api.ResolveVersion(ns, name, "role", constraint)
		if err != nil {
			if entry.ResolvedVersion != "" {
				return entry.ResolvedVersion, nil
			}
			return "", err
		}
		return resolved, nil

	case "tool":
		// Tools are resolved from PyPI — the constraint itself is the best we
		// can do without a PyPI resolver; return the constraint as resolved.
		if entry.ResolvedVersion != "" && constraint == "" {
			return entry.ResolvedVersion, nil
		}
		return constraint, nil
	}

	return constraint, nil
}

// intersectConstraints merges a list of version constraint strings into a
// single intersected constraint by keeping the highest lower-bound.
//
// Example: [">=9.0.0", ">=9.2.0", ">=9.1.0"] → ">=9.2.0"
// Exact pins ("==x.y.z") take precedence; conflicts between two different
// exact pins produce an error string (caller turns it into an error).
func intersectConstraints(constraints []string) string {
	if len(constraints) == 0 {
		return ""
	}
	if len(constraints) == 1 {
		return constraints[0]
	}

	var lowerBound string   // highest ">=" seen
	var exactPin string     // "==" pin if present
	var upperBound string   // lowest "<=" seen

	for _, c := range constraints {
		c = strings.TrimSpace(c)
		switch {
		case strings.HasPrefix(c, "=="):
			v := strings.TrimPrefix(c, "==")
			if exactPin != "" && exactPin != v {
				// Two different exact pins — irreconcilable; return a marker.
				return fmt.Sprintf("CONFLICT(%s,%s)", exactPin, v)
			}
			exactPin = v
		case strings.HasPrefix(c, ">="):
			v := strings.TrimPrefix(c, ">=")
			if lowerBound == "" || compareSimpleVersions(v, lowerBound) > 0 {
				lowerBound = v
			}
		case strings.HasPrefix(c, ">"):
			v := strings.TrimPrefix(c, ">")
			if lowerBound == "" || compareSimpleVersions(v, lowerBound) >= 0 {
				lowerBound = v
			}
		case strings.HasPrefix(c, "<="):
			v := strings.TrimPrefix(c, "<=")
			if upperBound == "" || compareSimpleVersions(v, upperBound) < 0 {
				upperBound = v
			}
		}
	}

	if exactPin != "" {
		return "==" + exactPin
	}
	if lowerBound != "" && upperBound != "" {
		return fmt.Sprintf(">=%s,<=%s", lowerBound, upperBound)
	}
	if lowerBound != "" {
		return ">=" + lowerBound
	}
	if upperBound != "" {
		return "<=" + upperBound
	}
	return ""
}

// mergePythonVersions intersects Python min/max across all lock files.
func mergePythonVersions(locks []dependency.LockFile) (*config.PythonVersion, error) {
	result := &config.PythonVersion{
		Min:    config.DefaultMinPythonVersion,
		Max:    config.DefaultMaxPythonVersion,
		Pinned: config.PinnedPythonVersion,
	}

	for _, lf := range locks {
		if lf.Python == nil {
			continue
		}
		p := lf.Python
		// min = max(all mins)
		if p.Min != "" && compareSimpleVersions(p.Min, result.Min) > 0 {
			result.Min = p.Min
		}
		// max = min(all maxes)
		if p.Max != "" && compareSimpleVersions(p.Max, result.Max) < 0 {
			result.Max = p.Max
		}
		// pinned: use the highest pinned that is within [min, max]
		if p.Pinned != "" && compareSimpleVersions(p.Pinned, result.Pinned) > 0 {
			result.Pinned = p.Pinned
		}
	}

	if compareSimpleVersions(result.Min, result.Max) > 0 {
		return nil, fmt.Errorf("no Python version satisfies all constraints: min %s > max %s",
			result.Min, result.Max)
	}

	// Clamp pinned within [min, max].
	if compareSimpleVersions(result.Pinned, result.Max) > 0 {
		result.Pinned = result.Max
	}
	if compareSimpleVersions(result.Pinned, result.Min) < 0 {
		result.Pinned = result.Min
	}

	return result, nil
}

// entryKey returns a stable map key for a lock file entry.
func entryKey(e dependency.LockFileEntry) string {
	ns, name := splitNamespaceAndName(e.Namespace, e.Name)
	return ns + "." + name
}

// splitNamespaceAndName resolves namespace and name from a LockFileEntry.
// Entry.Name may carry a scenario prefix ("default.rolename") — we strip it.
func splitNamespaceAndName(namespace, name string) (string, string) {
	// Strip scenario prefix, e.g. "default.general" → "general"
	if parts := strings.SplitN(name, ".", 2); len(parts) == 2 {
		name = parts[1]
	}
	return namespace, name
}

// compareSimpleVersions compares two semver-like version strings.
// Returns >0 if a > b, <0 if a < b, 0 if equal.
// Delegates to the galaxy package which already implements this.
func compareSimpleVersions(a, b string) int {
	return galaxy.CompareVersions(a, b)
}
