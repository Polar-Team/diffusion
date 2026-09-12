package dependency

import (
	"fmt"
	"strings"
	"testing"

	"diffusion/internal/config"
)

// noNetworkResolve is an injected resolver that echoes the constraint back.
// It keeps the ResolveTransitive tests offline.
func noNetworkResolve(entry LockFileEntry, kind string) (string, error) {
	if entry.Version == "" {
		return "", fmt.Errorf("no constraint to resolve")
	}
	// Pretend ">=9.0.0" resolves to "9.0.0".
	return trimConstraintOperators(entry.Version), nil
}

func trimConstraintOperators(v string) string {
	for _, op := range []string{">=", "<=", "==", ">", "<"} {
		if len(v) > len(op) && v[:len(op)] == op {
			return v[len(op):]
		}
	}
	return v
}

// TestEntryKey_GitEntriesKeyedByURL is the regression guard for M3: two
// repositories whose short names collide must not share a lock key.
func TestEntryKey_GitEntriesKeyedByURL(t *testing.T) {
	a := LockFileEntry{Name: "default.foo", Src: "https://github.com/org-a/ansible-collection-foo.git"}
	b := LockFileEntry{Name: "default.foo", Src: "https://github.com/org-b/ansible-collection-foo.git"}

	if entryKey(a) == entryKey(b) {
		t.Errorf("two different repos must not share a key, both = %q", entryKey(a))
	}

	// Equivalent URL spellings must still collapse onto one key.
	c := LockFileEntry{Name: "default.foo", Src: "git@github.com:org-a/ansible-collection-foo"}
	if entryKey(a) != entryKey(c) {
		t.Errorf("equivalent URLs must share a key: %q != %q", entryKey(a), entryKey(c))
	}

	// Galaxy entries keep the historical namespace.name key.
	g := LockFileEntry{Name: "default.general", Namespace: "community"}
	if got := entryKey(g); got != "community.general" {
		t.Errorf("galaxy entryKey = %q, want community.general", got)
	}
}

// TestResolveTransitive_SameShortNameDifferentRepos: two remotes contribute git
// collections that derive the same short name from different URLs. Both must
// survive as distinct entries.
func TestResolveTransitive_SameShortNameDifferentRepos(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/one": {Lock: &LockFile{
			Collections: []LockFileEntry{{
				Name: "default.foo", Type: "collection", Source: "git",
				Src: "https://github.com/org-a/ansible-collection-foo.git", ResolvedVersion: "1.0.0",
			}},
		}},
		"github.com/org/two": {Lock: &LockFile{
			Collections: []LockFileEntry{{
				Name: "default.foo", Type: "collection", Source: "git",
				Src: "https://github.com/org-b/ansible-collection-foo.git", ResolvedVersion: "2.0.0",
			}},
		}},
	}}

	direct := &LockFile{Roles: []LockFileEntry{
		gitRole("default.one", "https://github.com/org/one.git", "1.0.0"),
		gitRole("default.two", "https://github.com/org/two.git", "1.0.0"),
	}}

	merged, warnings, err := ResolveTransitive("default", direct, TransitiveOptions{
		Enabled: true, Fetch: reg.fetch, Resolve: noNetworkResolve,
	})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}

	if len(merged.Collections) != 2 {
		t.Fatalf("both distinct repos must be kept, got %d: %+v", len(merged.Collections), merged.Collections)
	}
	srcs := map[string]bool{}
	for _, c := range merged.Collections {
		srcs[c.Src] = true
	}
	if !srcs["https://github.com/org-a/ansible-collection-foo.git"] || !srcs["https://github.com/org-b/ansible-collection-foo.git"] {
		t.Errorf("expected both repository URLs, got %+v", merged.Collections)
	}
	for _, w := range warnings {
		if strings.Contains(w, "already resolved") {
			t.Errorf("distinct repos must not be reported as duplicates: %s", w)
		}
	}
}

// TestResolveTransitive_IdenticalKeyIsMerged: two remotes contributing the same
// Galaxy collection collapse into a single entry.
func TestResolveTransitive_IdenticalKeyIsMerged(t *testing.T) {
	// Both remotes declare community.general. The visited set catches the
	// second one, so exactly one entry survives.
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/one": {Lock: &LockFile{
			Collections: []LockFileEntry{galaxyCollection("default.general", "community", "9.0.0")},
		}},
		"github.com/org/two": {Lock: &LockFile{
			Collections: []LockFileEntry{galaxyCollection("default.general", "community", "9.5.0")},
		}},
	}}

	direct := &LockFile{Roles: []LockFileEntry{
		gitRole("default.one", "https://github.com/org/one.git", "1.0.0"),
		gitRole("default.two", "https://github.com/org/two.git", "1.0.0"),
	}}

	merged, warnings, err := ResolveTransitive("default", direct, TransitiveOptions{
		Enabled: true, Fetch: reg.fetch, Resolve: noNetworkResolve,
	})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}

	if len(merged.Collections) != 1 {
		t.Fatalf("identical keys must collapse to one entry, got %+v", merged.Collections)
	}
	if !warningsContain(warnings, "skipping community.general") {
		t.Errorf("expected a duplicate warning, got %v", warnings)
	}
}

// TestHasKeyCollision_WithinCollectedSet is the direct unit guard for M4.
func TestHasKeyCollision_WithinCollectedSet(t *testing.T) {
	direct := &LockFile{}

	// Two collected entries that map onto the same key.
	dup := []LockFileEntry{
		{Name: "default.general", Namespace: "community"},
		{Name: "default.general", Namespace: "community"},
	}
	if !hasKeyCollision(direct, dup, nil) {
		t.Error("duplicates within the collected set must be detected")
	}

	// Distinct git URLs with the same short name must NOT collide.
	distinct := []LockFileEntry{
		{Name: "default.foo", Src: "https://github.com/org-a/foo.git"},
		{Name: "default.foo", Src: "https://github.com/org-b/foo.git"},
	}
	if hasKeyCollision(direct, distinct, nil) {
		t.Error("distinct repositories must not be reported as colliding")
	}
}

// TestResolveTransitive_ConfigOnlyGetsResolvedVersion is the M7 guard: a remote
// that ships only a diffusion.toml contributes constraints; those must be
// resolved before landing in our lock file.
func TestResolveTransitive_ConfigOnlyGetsResolvedVersion(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/a": {Config: &config.DependencyConfig{
			Collections: []config.CollectionRequirement{
				{Name: "default.general", Namespace: "community", Version: ">=9.0.0"},
			},
			Roles: []config.RoleRequirement{
				{Name: "default.docker", Namespace: "geerlingguy", Version: ">=7.0.0"},
			},
		}},
	}}

	direct := &LockFile{Roles: []LockFileEntry{gitRole("default.a", "https://github.com/org/a.git", "1.0.0")}}

	merged, _, err := ResolveTransitive("default", direct, TransitiveOptions{
		Enabled: true, Fetch: reg.fetch, Resolve: noNetworkResolve,
	})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}

	general := findEntry(merged.Collections, "default.general")
	if general == nil {
		t.Fatalf("collection missing: %+v", merged.Collections)
	}
	if general.ResolvedVersion != "9.0.0" {
		t.Errorf("ResolvedVersion = %q, want 9.0.0 (resolved from the constraint)", general.ResolvedVersion)
	}
	if general.Version != ">=9.0.0" {
		t.Errorf("the original constraint must be preserved, got %q", general.Version)
	}

	docker := findEntry(merged.Roles, "default.docker")
	if docker == nil {
		t.Fatalf("role missing: %+v", merged.Roles)
	}
	if docker.ResolvedVersion != "7.0.0" {
		t.Errorf("role ResolvedVersion = %q, want 7.0.0", docker.ResolvedVersion)
	}
}

// TestResolveTransitive_ResolveFailureDegradesToConstraint asserts a failing
// resolver produces a warning and a usable fallback rather than an error.
func TestResolveTransitive_ResolveFailureDegradesToConstraint(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/a": {Config: &config.DependencyConfig{
			Collections: []config.CollectionRequirement{
				{Name: "default.general", Namespace: "community", Version: ">=9.0.0"},
			},
		}},
	}}

	failing := func(entry LockFileEntry, kind string) (string, error) {
		return "", fmt.Errorf("galaxy unreachable")
	}

	direct := &LockFile{Roles: []LockFileEntry{gitRole("default.a", "https://github.com/org/a.git", "1.0.0")}}

	merged, warnings, err := ResolveTransitive("default", direct, TransitiveOptions{
		Enabled: true, Fetch: reg.fetch, Resolve: failing,
	})
	if err != nil {
		t.Fatalf("a resolution failure must not be fatal, got: %v", err)
	}
	if !warningsContain(warnings, "could not resolve community.general") {
		t.Errorf("expected a resolution warning, got %v", warnings)
	}

	general := findEntry(merged.Collections, "default.general")
	if general == nil || general.ResolvedVersion != ">=9.0.0" {
		t.Errorf("expected the constraint as fallback, got %+v", general)
	}
}

// TestResolveTransitive_RemoteGitCollectionWithoutSrcDegrades is the M8
// regression: GenerateLockFile hard-errors on a non-Galaxy collection with no
// source_url, but that rule must NOT propagate to remote manifests — a broken
// third-party repo may only degrade to a warning here.
//
// Such an entry arrives via remoteEntries (not GenerateLockFile), so it is
// treated as an ordinary entry whose version simply cannot be resolved.
func TestResolveTransitive_RemoteGitCollectionWithoutSrcDegrades(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		// A remote whose diffusion.toml declares a git collection but forgot
		// the source URL.
		"github.com/org/a": {Config: &config.DependencyConfig{
			Collections: []config.CollectionRequirement{
				{Name: "default.broken", Source: "git", Version: "main"},
			},
		}},
	}}

	direct := &LockFile{Roles: []LockFileEntry{gitRole("default.a", "https://github.com/org/a.git", "1.0.0")}}

	merged, warnings, err := ResolveTransitive("default", direct, TransitiveOptions{
		Enabled: true, Fetch: reg.fetch, Resolve: noNetworkResolve,
	})
	if err != nil {
		t.Fatalf("a malformed remote entry must not be fatal, got error: %v", err)
	}

	broken := findEntry(merged.Collections, "default.broken")
	if broken == nil {
		t.Fatalf("the entry should still be recorded, got %+v", merged.Collections)
	}
	if broken.Src != "" {
		t.Errorf("Src should remain empty, got %q", broken.Src)
	}
	if broken.ResolvedVersion == "" {
		t.Error("a fallback ResolvedVersion must be set")
	}
	// It must not have been enqueued for expansion (no Src to clone).
	for _, c := range reg.calls {
		if c == "" {
			t.Error("an entry without Src must never be enqueued for fetching")
		}
	}
	_ = warnings
}
