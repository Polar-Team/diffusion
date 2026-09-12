package dependency

import (
	"fmt"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/galaxy"
)

// DefaultMaxTransitiveDepth bounds how deep the transitive resolver descends
// into nested git dependencies.
const DefaultMaxTransitiveDepth = 10

// FetchFunc retrieves the diffusion manifest of a remote git dependency.
// It is injectable so that the resolver can be tested without network access.
type FetchFunc func(url, version string, creds []config.ArtifactCredentials) (*RemoteManifest, error)

// ResolveFunc resolves the concrete version of a collected entry from its
// constraint. kind is "collection" or "role". It is injectable for tests.
type ResolveFunc func(entry LockFileEntry, kind string) (string, error)

// defaultFetch is the production implementation used when
// TransitiveOptions.Fetch is nil. It is a package-level variable so tests can
// assert that it is never called when transitive resolution is disabled.
var defaultFetch FetchFunc = FetchLockFromGit

// defaultResolve resolves an entry against Galaxy (or git ls-remote) using the
// same logic the lock merger uses.
func defaultResolve(entry LockFileEntry, kind string) (string, error) {
	return resolveEntry(galaxy.NewGalaxyAPI(), entry, entry.Version, kind)
}

// TransitiveOptions configures nested dependency resolution.
type TransitiveOptions struct {
	// Enabled turns the whole resolution on or off.
	Enabled bool
	// MaxDepth bounds recursion depth; <= 0 means DefaultMaxTransitiveDepth.
	MaxDepth int
	// Creds are artifact credentials used to clone private repositories.
	Creds []config.ArtifactCredentials
	// SelfIdentity lists identities of the current repo (git origin URL, role
	// name, directory basename, ...) used to seed the visited set so that a
	// remote dependency pointing back at us is treated as a cycle.
	SelfIdentity []string
	// Fetch overrides the manifest fetcher; nil uses defaultFetch.
	Fetch FetchFunc
	// Resolve overrides the version resolver applied to collected entries that
	// carry only a constraint; nil uses defaultResolve.
	Resolve ResolveFunc
}

// maxDepth returns the effective recursion bound.
func (o TransitiveOptions) maxDepth() int {
	if o.MaxDepth <= 0 {
		return DefaultMaxTransitiveDepth
	}
	return o.MaxDepth
}

// fetch returns the effective fetcher.
func (o TransitiveOptions) fetch() FetchFunc {
	if o.Fetch != nil {
		return o.Fetch
	}
	return defaultFetch
}

// resolve returns the effective version resolver.
func (o TransitiveOptions) resolve() ResolveFunc {
	if o.Resolve != nil {
		return o.Resolve
	}
	return defaultResolve
}

// normalizeIdentity reduces a git URL or Galaxy name to a stable comparison
// key. It lowercases, strips the scheme / scp-style user prefix, normalises
// the scp ':' separator to '/', and drops trailing slashes and ".git".
//
//	https://GitHub.com/Org/Repo.git/  -> github.com/org/repo
//	git@github.com:org/repo           -> github.com/org/repo
//	community.general                 -> community.general
func normalizeIdentity(id string) string {
	s := strings.ToLower(strings.TrimSpace(id))
	if s == "" {
		return ""
	}

	// Strip scheme, remembering that scp-style syntax uses ':' as separator.
	scp := false
	for _, pfx := range []string{"https://", "http://", "ssh://", "git+ssh://", "git://"} {
		if strings.HasPrefix(s, pfx) {
			s = strings.TrimPrefix(s, pfx)
			break
		}
	}
	if strings.HasPrefix(s, "git@") {
		s = strings.TrimPrefix(s, "git@")
		scp = true
	}
	// "user@host/path" or "user@host:path" after an ssh:// scheme.
	if idx := strings.Index(s, "@"); idx != -1 && !strings.Contains(s[:idx], "/") {
		s = s[idx+1:]
		scp = true
	}
	if scp {
		s = strings.Replace(s, ":", "/", 1)
	}

	s = strings.TrimSuffix(s, "/")
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimSuffix(s, "/")

	return s
}

// entryIdentity returns the comparison key for a lock file entry: the git URL
// for git-sourced entries, otherwise the Galaxy "namespace.name".
func entryIdentity(e LockFileEntry) string {
	if e.Src != "" {
		return normalizeIdentity(e.Src)
	}
	ns, name := splitNamespaceAndName(e.Namespace, e.Name)
	if ns != "" {
		return strings.ToLower(ns + "." + name)
	}
	return strings.ToLower(name)
}

// queueItem is a git dependency awaiting expansion.
type queueItem struct {
	url     string
	version string
	// identity is the normalised identity of this dependency; it becomes the
	// RequiredBy value of everything it contributes.
	identity string
	depth    int
}

// ResolveTransitive expands the direct lock file with the dependencies
// declared by git-sourced roles and collections that are themselves diffusion
// projects.
//
// The traversal is breadth-first with a visited set seeded from
// opts.SelfIdentity and every direct entry, which makes it immune to cycles
// (a remote depending back on us) and to diamond duplicates (two deps pulling
// the same third dep). Every skip is reported as a warning.
//
// Tools and the Python version always come from the direct lock: a remote role
// must not narrow our container runtime.
//
// A fetch failure for a single dependency is a warning, not an error — a
// private repo we cannot reach must not break `deps lock` entirely.
func ResolveTransitive(scenario string, direct *LockFile, opts TransitiveOptions) (*LockFile, []string, error) {
	if direct == nil {
		return nil, nil, fmt.Errorf("direct lock file is required")
	}
	if !opts.Enabled {
		return direct, nil, nil
	}

	var warnings []string

	visited := make(map[string]bool)
	for _, id := range opts.SelfIdentity {
		if n := normalizeIdentity(id); n != "" {
			visited[n] = true
		}
	}

	prefix := scenario + "."
	var queue []queueItem

	// Seed: every direct entry is visited; git-sourced ones are expanded.
	seed := func(entries []LockFileEntry) {
		for _, e := range entries {
			id := entryIdentity(e)
			if id != "" {
				visited[id] = true
			}
			if e.Src == "" {
				continue
			}
			version := e.ResolvedVersion
			if version == "" {
				version = e.Version
			}
			queue = append(queue, queueItem{url: e.Src, version: version, identity: id, depth: 0})
		}
	}
	seed(direct.Roles)
	seed(direct.Collections)

	var collectedCols, collectedRoles []LockFileEntry
	fetch := opts.fetch()
	maxDepth := opts.maxDepth()

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if item.depth >= maxDepth {
			warnings = append(warnings, fmt.Sprintf(
				"max transitive depth %d reached at %s: not descending further", maxDepth, item.identity))
			continue
		}

		manifest, err := fetch(item.url, item.version, opts.Creds)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf(
				"could not fetch dependencies of %s: %v", item.identity, err))
			continue
		}
		if manifest.IsEmpty() {
			// Not a diffusion project — perfectly normal for a plain role.
			continue
		}

		remoteCols, remoteRoles := remoteEntries(manifest, prefix)

		handle := func(entries []LockFileEntry, collect *[]LockFileEntry) {
			for _, e := range entries {
				id := entryIdentity(e)
				if id == "" {
					continue
				}
				if visited[id] {
					warnings = append(warnings, fmt.Sprintf(
						"skipping %s required by %s: already resolved (duplicate or cycle)", id, item.identity))
					continue
				}
				visited[id] = true

				e.RequiredBy = item.identity
				*collect = append(*collect, e)

				if e.Src != "" {
					version := e.ResolvedVersion
					if version == "" {
						version = e.Version
					}
					queue = append(queue, queueItem{
						url:      e.Src,
						version:  version,
						identity: id,
						depth:    item.depth + 1,
					})
				}
			}
		}

		handle(remoteCols, &collectedCols)
		handle(remoteRoles, &collectedRoles)
	}

	if len(collectedCols) == 0 && len(collectedRoles) == 0 {
		return direct, warnings, nil
	}

	// Remotes that only ship a diffusion.toml contribute constraints without a
	// resolved version. Resolve them now so the lock file is complete on both
	// the fast path and the merge path.
	resolve := opts.resolve()
	warnings = append(warnings, fillResolvedVersions(collectedCols, "collection", resolve)...)
	warnings = append(warnings, fillResolvedVersions(collectedRoles, "role", resolve)...)

	// Fast path: the visited set already guarantees we never collect an entry
	// whose *identity* is known, so a key collision is only possible when two
	// different sources (e.g. a direct git role "foo" and a remote Galaxy role
	// "ns.foo") map onto the same lock key. Only then do we need MergeLocks,
	// which re-resolves every entry against Galaxy/git. Skipping it in the
	// common case keeps `deps lock` from doubling its network traffic.
	if !hasKeyCollision(direct, collectedCols, collectedRoles) {
		merged := *direct
		merged.Collections = append(append([]LockFileEntry{}, direct.Collections...), collectedCols...)
		merged.Roles = append(append([]LockFileEntry{}, direct.Roles...), collectedRoles...)
		return &merged, warnings, nil
	}

	// Merge for constraint intersection and re-resolution. Tools / Python are
	// deliberately omitted from the transitive side so the direct lock wins.
	transitive := LockFile{Collections: collectedCols, Roles: collectedRoles}
	merged, err := MergeLocks([]LockFile{*direct, transitive})
	if err != nil {
		return nil, warnings, fmt.Errorf("failed to merge transitive dependencies: %w", err)
	}

	// MergeLocks rebuilds Tools and Python from all inputs; restore the direct
	// lock's values verbatim.
	merged.Tools = direct.Tools
	merged.Python = direct.Python
	merged.Hash = direct.Hash
	merged.Generated = direct.Generated

	return merged, warnings, nil
}

// fillResolvedVersions resolves every entry that carries only a constraint.
// Entries are updated in place; resolution failures degrade to the declared
// constraint (or "main" for git entries, matching GenerateLockFile) and are
// reported as warnings rather than errors.
func fillResolvedVersions(entries []LockFileEntry, kind string, resolve ResolveFunc) []string {
	var warnings []string

	for i := range entries {
		if entries[i].ResolvedVersion != "" {
			continue
		}

		resolved, err := resolve(entries[i], kind)
		if err == nil && resolved != "" {
			entries[i].ResolvedVersion = resolved
			continue
		}

		fallback := entries[i].Version
		if fallback == "" || fallback == "latest" {
			if entries[i].Src != "" {
				fallback = "main"
			} else {
				fallback = "latest"
			}
		}
		if err != nil {
			warnings = append(warnings, fmt.Sprintf(
				"could not resolve %s: %v; leaving constraint %s", entryIdentity(entries[i]), err, fallback))
		}
		entries[i].ResolvedVersion = fallback
	}

	return warnings
}

// hasKeyCollision reports whether any collected transitive entry shares a lock
// key with a direct entry of the same kind, or with another collected entry.

// The intra-collection check matters because two different remote repos can
// contribute entries that collapse onto the same key; without the merge step
// those would both be appended and produce a duplicate in requirements.yml.
func hasKeyCollision(direct *LockFile, cols, roles []LockFileEntry) bool {
	collides := func(existing, added []LockFileEntry) bool {
		if len(added) == 0 {
			return false
		}
		keys := make(map[string]bool, len(existing)+len(added))
		for _, e := range existing {
			keys[entryKey(e)] = true
		}
		for _, e := range added {
			k := entryKey(e)
			if keys[k] {
				return true
			}
			// Build incrementally so duplicates *within* the collected set are
			// detected too.
			keys[k] = true
		}
		return false
	}
	return collides(direct.Collections, cols) || collides(direct.Roles, roles)
}

// remoteEntries extracts the default-scenario collections and roles from a
// remote manifest and re-prefixes them with our scenario prefix.
//
// The remote lock is preferred (it carries resolved versions); its
// diffusion.toml is the fallback for projects that never ran `deps lock`.
func remoteEntries(manifest *RemoteManifest, prefix string) (cols, roles []LockFileEntry) {
	const remotePrefix = "default."

	if manifest.Lock != nil {
		for _, e := range manifest.Lock.Collections {
			if re, ok := reprefix(e, remotePrefix, prefix); ok {
				cols = append(cols, re)
			}
		}
		for _, e := range manifest.Lock.Roles {
			if re, ok := reprefix(e, remotePrefix, prefix); ok {
				roles = append(roles, re)
			}
		}
		if len(cols) > 0 || len(roles) > 0 {
			return cols, roles
		}
	}

	if manifest.Config == nil {
		return nil, nil
	}

	// Fall back to declared constraints; ResolvedVersion stays empty so the
	// merge step resolves them.
	for _, c := range manifest.Config.Collections {
		if !strings.HasPrefix(c.Name, remotePrefix) {
			continue
		}
		source := c.Source
		if source == "" {
			source = "galaxy"
		}
		cols = append(cols, LockFileEntry{
			Name:      prefix + strings.TrimPrefix(c.Name, remotePrefix),
			Namespace: c.Namespace,
			Version:   c.Version,
			Type:      "collection",
			Source:    source,
			Src:       c.SourceURL,
		})
	}
	for _, r := range manifest.Config.Roles {
		if !strings.HasPrefix(r.Name, remotePrefix) {
			continue
		}
		scm := r.Scm
		if scm == "" && r.Src != "" {
			scm = "git"
		}
		roles = append(roles, LockFileEntry{
			Name:      prefix + strings.TrimPrefix(r.Name, remotePrefix),
			Namespace: r.Namespace,
			Version:   r.Version,
			Type:      "role",
			Source:    scm,
			Src:       r.Src,
		})
	}

	return cols, roles
}

// reprefix rewrites a remote entry's scenario prefix to ours. Entries that do
// not belong to the remote's default scenario are dropped: a remote project's
// extra scenarios are its own testing concern, not our runtime dependency.
func reprefix(e LockFileEntry, from, to string) (LockFileEntry, bool) {
	if !strings.HasPrefix(e.Name, from) {
		return LockFileEntry{}, false
	}
	e.Name = to + strings.TrimPrefix(e.Name, from)
	// The remote's own RequiredBy chain is not meaningful in our lock; the
	// caller overwrites it with the direct parent identity.
	e.RequiredBy = ""
	return e, true
}
