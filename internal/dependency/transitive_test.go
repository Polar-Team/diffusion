package dependency

import (
	"fmt"
	"strings"
	"testing"

	"diffusion/internal/config"
)

// fakeRegistry is an in-memory stand-in for remote git repositories.
type fakeRegistry struct {
	manifests map[string]*RemoteManifest // keyed by normalised identity
	errs      map[string]error
	calls     []string
}

func (f *fakeRegistry) fetch(url, version string, creds []config.ArtifactCredentials) (*RemoteManifest, error) {
	id := normalizeIdentity(url)
	f.calls = append(f.calls, id)
	if err, ok := f.errs[id]; ok {
		return nil, err
	}
	if m, ok := f.manifests[id]; ok {
		return m, nil
	}
	// Unknown repo: a plain Ansible role with no diffusion metadata.
	return &RemoteManifest{}, nil
}

func gitRole(name, url, version string) LockFileEntry {
	return LockFileEntry{
		Name: name, Type: "role", Source: "git", Src: url,
		ResolvedVersion: version,
	}
}

func galaxyCollection(name, namespace, version string) LockFileEntry {
	return LockFileEntry{
		Name: name, Namespace: namespace, Type: "collection",
		Source: "galaxy", ResolvedVersion: version,
	}
}

func warningsContain(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func findEntry(entries []LockFileEntry, name string) *LockFileEntry {
	for i := range entries {
		if entries[i].Name == name {
			return &entries[i]
		}
	}
	return nil
}

// TestResolveTransitive_DuplicatesAndCycles is the core scenario: a direct git
// role A pulls in collection X and git role B; B pulls in X again (duplicate)
// and a role pointing back at us (cycle).
func TestResolveTransitive_DuplicatesAndCycles(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/a": {Lock: &LockFile{
			Collections: []LockFileEntry{galaxyCollection("default.x", "community", "9.0.0")},
			Roles:       []LockFileEntry{gitRole("default.b", "https://github.com/org/b.git", "2.0.0")},
		}},
		"github.com/org/b": {Lock: &LockFile{
			Collections: []LockFileEntry{galaxyCollection("default.x", "community", "9.5.0")},
			Roles:       []LockFileEntry{gitRole("default.self", "https://github.com/me/self.git", "1.0.0")},
		}},
	}}

	direct := &LockFile{
		Roles: []LockFileEntry{gitRole("default.a", "https://github.com/org/a.git", "1.0.0")},
	}

	merged, warnings, err := ResolveTransitive("default", direct, TransitiveOptions{
		Enabled:      true,
		SelfIdentity: []string{"https://github.com/me/self.git"},
		Fetch:        reg.fetch,
	})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}

	// Collection X exactly once, attributed to A (the first one to require it).
	if len(merged.Collections) != 1 {
		t.Fatalf("expected exactly 1 collection, got %d: %+v", len(merged.Collections), merged.Collections)
	}
	x := merged.Collections[0]
	if x.Name != "default.x" {
		t.Errorf("collection name = %q, want %q", x.Name, "default.x")
	}
	if x.RequiredBy != "github.com/org/a" {
		t.Errorf("collection RequiredBy = %q, want %q", x.RequiredBy, "github.com/org/a")
	}

	// Roles: direct A (no RequiredBy) + transitive B (RequiredBy A). Self is skipped.
	if len(merged.Roles) != 2 {
		t.Fatalf("expected 2 roles, got %d: %+v", len(merged.Roles), merged.Roles)
	}
	a := findEntry(merged.Roles, "default.a")
	if a == nil {
		t.Fatal("direct role default.a missing from merged lock")
	}
	if a.RequiredBy != "" {
		t.Errorf("direct role RequiredBy = %q, want empty", a.RequiredBy)
	}
	b := findEntry(merged.Roles, "default.b")
	if b == nil {
		t.Fatal("transitive role default.b missing from merged lock")
	}
	if b.RequiredBy != "github.com/org/a" {
		t.Errorf("role default.b RequiredBy = %q, want %q", b.RequiredBy, "github.com/org/a")
	}
	if findEntry(merged.Roles, "default.self") != nil {
		t.Error("cycle back to self must not be added to the lock file")
	}

	// Both skips must be reported.
	if !warningsContain(warnings, "skipping community.x required by github.com/org/b") {
		t.Errorf("missing duplicate warning, got: %v", warnings)
	}
	if !warningsContain(warnings, "skipping github.com/me/self required by github.com/org/b") {
		t.Errorf("missing cycle warning, got: %v", warnings)
	}

	// Self must never be fetched.
	for _, c := range reg.calls {
		if c == "github.com/me/self" {
			t.Error("the current repository must never be fetched")
		}
	}
}

// TestResolveTransitive_Disabled returns the direct lock untouched.
func TestResolveTransitive_Disabled(t *testing.T) {
	reg := &fakeRegistry{}
	direct := &LockFile{Roles: []LockFileEntry{gitRole("default.a", "https://github.com/org/a.git", "1.0.0")}}

	got, warnings, err := ResolveTransitive("default", direct, TransitiveOptions{Enabled: false, Fetch: reg.fetch})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}
	if got != direct {
		t.Error("disabled resolver must return the direct lock unchanged")
	}
	if len(warnings) != 0 || len(reg.calls) != 0 {
		t.Errorf("disabled resolver must not fetch anything, calls=%v warnings=%v", reg.calls, warnings)
	}
}

// TestResolveTransitive_MaxDepth stops descending and warns.
func TestResolveTransitive_MaxDepth(t *testing.T) {
	// Chain a -> b -> c -> d
	chain := func(from, to string) *RemoteManifest {
		return &RemoteManifest{Lock: &LockFile{
			Roles: []LockFileEntry{gitRole("default."+to, "https://github.com/org/"+to+".git", "1.0.0")},
		}}
	}
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/a": chain("a", "b"),
		"github.com/org/b": chain("b", "c"),
		"github.com/org/c": chain("c", "d"),
	}}

	direct := &LockFile{Roles: []LockFileEntry{gitRole("default.a", "https://github.com/org/a.git", "1.0.0")}}

	merged, warnings, err := ResolveTransitive("default", direct, TransitiveOptions{
		Enabled: true, MaxDepth: 2, Fetch: reg.fetch,
	})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}

	// depth 0 = a (expanded -> b), depth 1 = b (expanded -> c), depth 2 = c (stop).
	if findEntry(merged.Roles, "default.b") == nil || findEntry(merged.Roles, "default.c") == nil {
		t.Errorf("expected roles b and c to be resolved, got %+v", merged.Roles)
	}
	if findEntry(merged.Roles, "default.d") != nil {
		t.Error("role d is beyond MaxDepth and must not be resolved")
	}
	if !warningsContain(warnings, "max transitive depth 2 reached at github.com/org/c") {
		t.Errorf("missing depth warning, got: %v", warnings)
	}
}

// TestResolveTransitive_ConfigOnlyManifest covers a remote project that has a
// diffusion.toml but never ran `deps lock`.
func TestResolveTransitive_ConfigOnlyManifest(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/a": {Config: &config.DependencyConfig{
			Collections: []config.CollectionRequirement{
				{Name: "default.general", Namespace: "community", Version: ">=9.0.0"},
				{Name: "ci.only", Namespace: "community", Version: ">=1.0.0"},
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
		t.Fatalf("expected collection default.general, got %+v", merged.Collections)
	}
	if general.Version != ">=9.0.0" {
		t.Errorf("collection version = %q, want the declared constraint", general.Version)
	}
	// Since M7 the collected constraint is resolved before it lands in the
	// lock file (previously this asserted an empty ResolvedVersion).
	if general.ResolvedVersion != "9.0.0" {
		t.Errorf("config-only entries must be version-resolved, got %q", general.ResolvedVersion)
	}
	if general.RequiredBy != "github.com/org/a" {
		t.Errorf("RequiredBy = %q, want github.com/org/a", general.RequiredBy)
	}

	// A remote's non-default scenarios are its own testing concern.
	if findEntry(merged.Collections, "default.only") != nil || findEntry(merged.Collections, "ci.only") != nil {
		t.Errorf("remote non-default scenario entries must be ignored, got %+v", merged.Collections)
	}

	if findEntry(merged.Roles, "default.docker") == nil {
		t.Errorf("expected role default.docker, got %+v", merged.Roles)
	}
}

// TestResolveTransitive_FetchErrorIsWarning: one unreachable dependency must
// not prevent the others from resolving.
func TestResolveTransitive_FetchErrorIsWarning(t *testing.T) {
	reg := &fakeRegistry{
		manifests: map[string]*RemoteManifest{
			"github.com/org/good": {Lock: &LockFile{
				Collections: []LockFileEntry{galaxyCollection("default.x", "community", "9.0.0")},
			}},
		},
		errs: map[string]error{
			"github.com/org/bad": fmt.Errorf("git clone failed: authentication required"),
		},
	}

	direct := &LockFile{Roles: []LockFileEntry{
		gitRole("default.bad", "https://github.com/org/bad.git", "1.0.0"),
		gitRole("default.good", "https://github.com/org/good.git", "1.0.0"),
	}}

	merged, warnings, err := ResolveTransitive("default", direct, TransitiveOptions{Enabled: true, Fetch: reg.fetch})
	if err != nil {
		t.Fatalf("a fetch failure must not be fatal, got error: %v", err)
	}
	if !warningsContain(warnings, "could not fetch dependencies of github.com/org/bad") {
		t.Errorf("missing fetch warning, got: %v", warnings)
	}
	if findEntry(merged.Collections, "default.x") == nil {
		t.Errorf("the reachable dependency must still be resolved, got %+v", merged.Collections)
	}
}

// TestResolveTransitive_NonDiffusionRepoIsSilent: a plain Ansible role has
// neither lock nor config and must not generate noise.
func TestResolveTransitive_NonDiffusionRepoIsSilent(t *testing.T) {
	reg := &fakeRegistry{}
	direct := &LockFile{Roles: []LockFileEntry{gitRole("default.plain", "https://github.com/org/plain.git", "1.0.0")}}

	merged, warnings, err := ResolveTransitive("default", direct, TransitiveOptions{Enabled: true, Fetch: reg.fetch})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("a non-diffusion repo must not warn, got: %v", warnings)
	}
	if len(merged.Roles) != 1 || len(merged.Collections) != 0 {
		t.Errorf("lock must be unchanged, got %+v / %+v", merged.Roles, merged.Collections)
	}
}

// TestResolveTransitive_KeyCollisionPreservesRequiredBy exercises the
// MergeLocks path: a direct git collection and a remote Galaxy collection that
// share a lock key. The direct entry must win (stay direct) while an
// uncontested transitive entry keeps its RequiredBy.
//
// All entries use an empty version constraint so resolveEntry short-circuits
// on the cached ResolvedVersion — no Galaxy/git traffic in this test.
func TestResolveTransitive_KeyCollisionPreservesRequiredBy(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/a": {Lock: &LockFile{
			Collections: []LockFileEntry{
				// Same lock key ".foo" as the direct git collection below.
				{Name: "default.foo", Type: "collection", Source: "galaxy", ResolvedVersion: "2.0.0"},
				{Name: "default.bar", Type: "collection", Source: "galaxy", ResolvedVersion: "3.0.0"},
			},
		}},
	}}

	direct := &LockFile{
		Collections: []LockFileEntry{
			{Name: "default.foo", Type: "collection", Source: "git",
				Src: "https://github.com/org/foo.git", ResolvedVersion: "1.0.0"},
		},
		Roles: []LockFileEntry{gitRole("default.a", "https://github.com/org/a.git", "1.0.0")},
	}

	merged, _, err := ResolveTransitive("default", direct, TransitiveOptions{Enabled: true, Fetch: reg.fetch})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}

	foo := findEntry(merged.Collections, "default.foo")
	if foo == nil {
		t.Fatalf("collection default.foo missing: %+v", merged.Collections)
	}
	if foo.RequiredBy != "" {
		t.Errorf("on a key collision the direct entry must win, got RequiredBy=%q", foo.RequiredBy)
	}
	if foo.Src != "https://github.com/org/foo.git" {
		t.Errorf("direct entry's source must be preserved, got %q", foo.Src)
	}

	bar := findEntry(merged.Collections, "default.bar")
	if bar == nil {
		t.Fatalf("collection default.bar missing: %+v", merged.Collections)
	}
	if bar.RequiredBy != "github.com/org/a" {
		t.Errorf("transitive-only entry must keep RequiredBy, got %q", bar.RequiredBy)
	}
}

func TestNormalizeIdentity(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "https with case and trailing slash", input: "https://GitHub.com/Org/Repo.git/", want: "github.com/org/repo"},
		{name: "scp syntax", input: "git@github.com:org/repo", want: "github.com/org/repo"},
		{name: "scp syntax with .git", input: "git@github.com:org/repo.git", want: "github.com/org/repo"},
		{name: "ssh scheme", input: "ssh://git@github.com/org/repo.git", want: "github.com/org/repo"},
		{name: "http", input: "http://gitlab.local/org/repo.git", want: "gitlab.local/org/repo"},
		{name: "galaxy name", input: "Community.General", want: "community.general"},
		{name: "empty", input: "   ", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeIdentity(tt.input); got != tt.want {
				t.Errorf("normalizeIdentity(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestNormalizeIdentity_EquivalentForms is the specific equality required by
// the cycle detector.
func TestNormalizeIdentity_EquivalentForms(t *testing.T) {
	a := normalizeIdentity("https://GitHub.com/Org/Repo.git/")
	b := normalizeIdentity("git@github.com:org/repo")
	if a != b {
		t.Errorf("equivalent URLs normalised differently: %q != %q", a, b)
	}
}

// TestNormalizeIdentity_MoreEdgeCases covers additional scheme/case/trailing
// slash combinations required by the cycle detector.
func TestNormalizeIdentity_MoreEdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "ssh scheme with user", input: "ssh://git@host/org/repo.git", want: "host/org/repo"},
		{name: "git+ssh scheme", input: "git+ssh://git@host/org/repo.git", want: "host/org/repo"},
		{name: "uppercase host", input: "https://HOST/org/repo", want: "host/org/repo"},
		{name: "trailing slash no .git", input: "https://host/org/repo/", want: "host/org/repo"},
		{name: "http vs https equal form", input: "http://host/org/repo.git", want: "host/org/repo"},
		{name: "scp vs https equal form", input: "git@host:org/repo", want: "host/org/repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeIdentity(tt.input); got != tt.want {
				t.Errorf("normalizeIdentity(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}

	// Equality checks across schemes for the exact same repo.
	forms := []string{
		"ssh://git@host/org/repo.git",
		"git+ssh://git@host/org/repo.git",
		"https://HOST/org/repo",
		"http://host/org/repo.git",
		"git@host:org/repo",
	}
	first := normalizeIdentity(forms[0])
	for _, f := range forms[1:] {
		if got := normalizeIdentity(f); got != first {
			t.Errorf("normalizeIdentity(%q) = %q, want %q (equal to %q)", f, got, first, forms[0])
		}
	}

	// Different repos under the same org must NOT collide.
	if normalizeIdentity("org/repo") == normalizeIdentity("org/repo2") {
		t.Error("org/repo and org/repo2 must normalise differently")
	}

	// Galaxy identifiers are case-insensitive too.
	if normalizeIdentity("NS.Name") != normalizeIdentity("ns.name") {
		t.Error("Galaxy identity NS.Name must equal ns.name")
	}
}

// TestResolveTransitive_CycleMixedSchemeForms verifies that a self-identity
// given in ssh form is still recognised as a cycle when the remote manifest
// refers back to us using an https form (A -> B -> A, self in ssh form).
func TestResolveTransitive_CycleMixedSchemeForms(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/a": {Lock: &LockFile{
			Roles: []LockFileEntry{gitRole("default.b", "https://github.com/org/b.git", "1.0.0")},
		}},
		"github.com/org/b": {Lock: &LockFile{
			// Points back to "us" using an https form, while SelfIdentity is ssh.
			Roles: []LockFileEntry{gitRole("default.self", "https://github.com/me/self.git", "1.0.0")},
		}},
	}}

	direct := &LockFile{Roles: []LockFileEntry{gitRole("default.a", "https://github.com/org/a.git", "1.0.0")}}

	merged, warnings, err := ResolveTransitive("default", direct, TransitiveOptions{
		Enabled:      true,
		SelfIdentity: []string{"ssh://git@github.com/me/self.git"},
		Fetch:        reg.fetch,
	})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}
	if findEntry(merged.Roles, "default.self") != nil {
		t.Error("cycle back to self (mixed scheme forms) must not be added to the lock file")
	}
	if !warningsContain(warnings, "skipping github.com/me/self") {
		t.Errorf("expected a cycle warning, got: %v", warnings)
	}
}

// TestResolveTransitive_DiamondBothDirectRequireSameDep: direct roles A and B
// both require the same collection C. C must appear exactly once, attributed
// to whichever direct dependency was expanded first, with exactly one
// duplicate warning (nothing for the direct roles themselves).
func TestResolveTransitive_DiamondBothDirectRequireSameDep(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/a": {Lock: &LockFile{
			Collections: []LockFileEntry{galaxyCollection("default.c", "community", "1.0.0")},
		}},
		"github.com/org/b": {Lock: &LockFile{
			Collections: []LockFileEntry{galaxyCollection("default.c", "community", "1.0.0")},
		}},
	}}

	direct := &LockFile{Roles: []LockFileEntry{
		gitRole("default.a", "https://github.com/org/a.git", "1.0.0"),
		gitRole("default.b", "https://github.com/org/b.git", "1.0.0"),
	}}

	merged, warnings, err := ResolveTransitive("default", direct, TransitiveOptions{Enabled: true, Fetch: reg.fetch})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}

	if len(merged.Collections) != 1 {
		t.Fatalf("expected collection C exactly once, got %+v", merged.Collections)
	}
	c := merged.Collections[0]
	if c.RequiredBy != "github.com/org/a" {
		t.Errorf("RequiredBy = %q, want first parent github.com/org/a", c.RequiredBy)
	}

	dupWarnings := 0
	for _, w := range warnings {
		if strings.Contains(w, "community.c") {
			dupWarnings++
		}
	}
	if dupWarnings != 1 {
		t.Errorf("expected exactly 1 duplicate warning for community.c, got %d: %v", dupWarnings, warnings)
	}
	// No warning should mention the direct roles a or b themselves.
	if warningsContain(warnings, "github.com/org/a required") || warningsContain(warnings, "github.com/org/b required") {
		t.Errorf("direct roles must not generate warnings, got: %v", warnings)
	}
}

// TestResolveTransitive_RemoteNonDefaultScenarioIgnored_CustomScenario checks
// that remote entries outside "default." are ignored and that "default."
// entries get re-prefixed with our own non-default scenario name ("prod").
func TestResolveTransitive_RemoteNonDefaultScenarioIgnored_CustomScenario(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/a": {Lock: &LockFile{
			Collections: []LockFileEntry{
				galaxyCollection("default.general", "community", "9.0.0"),
				galaxyCollection("ci.only", "community", "1.0.0"),
			},
		}},
	}}

	direct := &LockFile{Roles: []LockFileEntry{gitRole("prod.a", "https://github.com/org/a.git", "1.0.0")}}

	merged, _, err := ResolveTransitive("prod", direct, TransitiveOptions{Enabled: true, Fetch: reg.fetch})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}

	if findEntry(merged.Collections, "prod.general") == nil {
		t.Errorf("expected remote default.general reprefixed to prod.general, got %+v", merged.Collections)
	}
	if findEntry(merged.Collections, "ci.only") != nil || findEntry(merged.Collections, "prod.only") != nil {
		t.Errorf("remote non-default scenario entries must be ignored, got %+v", merged.Collections)
	}
}

// TestResolveTransitive_ConfigOnlyGitCollectionPreservesSource verifies that a
// remote manifest with only a diffusion.toml (no lock) whose git collection
// is collected with Source/SourceURL preserved.
//
// Since M7 the collected entry is also version-resolved before it lands in the
// lock file, so ResolvedVersion is populated rather than left empty. With the
// default resolver a git entry falls back to its constraint when the remote is
// unreachable, which is what this offline test observes.
func TestResolveTransitive_ConfigOnlyGitCollectionPreservesSource(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/a": {Config: &config.DependencyConfig{
			Collections: []config.CollectionRequirement{
				{Name: "default.bar", Version: ">=2.0.0", Source: "git", SourceURL: "https://github.com/org/bar.git"},
			},
		}},
	}}

	direct := &LockFile{Roles: []LockFileEntry{gitRole("default.a", "https://github.com/org/a.git", "1.0.0")}}

	merged, _, err := ResolveTransitive("default", direct, TransitiveOptions{Enabled: true, Fetch: reg.fetch})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}

	bar := findEntry(merged.Collections, "default.bar")
	if bar == nil {
		t.Fatalf("expected collection default.bar, got %+v", merged.Collections)
	}
	if bar.Source != "git" {
		t.Errorf("Source = %q, want %q", bar.Source, "git")
	}
	if bar.Src != "https://github.com/org/bar.git" {
		t.Errorf("Src = %q, want the repository URL", bar.Src)
	}
	if bar.ResolvedVersion == "" {
		t.Error("config-only entries must be version-resolved before landing in the lock file")
	}
}

// TestResolveTransitive_MaxDepthOne verifies that with MaxDepth=1 a chain
// A -> B -> C stops after B: C is never collected, and a depth warning is
// emitted for B (the item at depth 1, the bound).
func TestResolveTransitive_MaxDepthOne(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/a": {Lock: &LockFile{
			Roles: []LockFileEntry{gitRole("default.b", "https://github.com/org/b.git", "1.0.0")},
		}},
		"github.com/org/b": {Lock: &LockFile{
			Roles: []LockFileEntry{gitRole("default.c", "https://github.com/org/c.git", "1.0.0")},
		}},
	}}

	direct := &LockFile{Roles: []LockFileEntry{gitRole("default.a", "https://github.com/org/a.git", "1.0.0")}}

	merged, warnings, err := ResolveTransitive("default", direct, TransitiveOptions{
		Enabled: true, MaxDepth: 1, Fetch: reg.fetch,
	})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}

	if findEntry(merged.Roles, "default.b") == nil {
		t.Errorf("expected role b (depth 0 expansion) to be resolved, got %+v", merged.Roles)
	}
	if findEntry(merged.Roles, "default.c") != nil {
		t.Error("role c is beyond MaxDepth=1 and must not be resolved")
	}
	if !warningsContain(warnings, "max transitive depth 1 reached at github.com/org/b") {
		t.Errorf("missing depth warning, got: %v", warnings)
	}
}

// TestHasKeyCollision_DirectGalaxyVsRemoteGitSameShortName exercises the
// hasKeyCollision merge path directly: a direct Galaxy collection
// "default.general" (namespace "community") and a remote git collection that
// resolves to the same lock key ("community.general") must merge into a
// single entry, with the direct entry staying RequiredBy=="".
func TestHasKeyCollision_DirectGalaxyVsRemoteGitSameShortName(t *testing.T) {
	reg := &fakeRegistry{manifests: map[string]*RemoteManifest{
		"github.com/org/a": {Lock: &LockFile{
			Collections: []LockFileEntry{
				{Name: "default.general", Namespace: "community", Type: "collection",
					Source: "git", Src: "https://github.com/community/general.git", ResolvedVersion: "2.0.0"},
			},
		}},
	}}

	direct := &LockFile{
		Collections: []LockFileEntry{
			galaxyCollection("default.general", "community", "1.0.0"),
		},
		Roles: []LockFileEntry{gitRole("default.a", "https://github.com/org/a.git", "1.0.0")},
	}

	merged, _, err := ResolveTransitive("default", direct, TransitiveOptions{Enabled: true, Fetch: reg.fetch})
	if err != nil {
		t.Fatalf("ResolveTransitive returned error: %v", err)
	}

	matches := 0
	for _, c := range merged.Collections {
		if strings.HasSuffix(c.Name, "general") {
			matches++
		}
	}
	if matches != 1 {
		t.Fatalf("expected exactly 1 'general' collection after collision merge, got %d: %+v", matches, merged.Collections)
	}
	general := findEntry(merged.Collections, "default.general")
	if general == nil {
		t.Fatalf("collection default.general missing: %+v", merged.Collections)
	}
	if general.RequiredBy != "" {
		t.Errorf("direct entry must stay RequiredBy empty on collision, got %q", general.RequiredBy)
	}
}

func TestEntryIdentity(t *testing.T) {
	tests := []struct {
		name  string
		entry LockFileEntry
		want  string
	}{
		{name: "git entry uses url", entry: gitRole("default.a", "https://github.com/Org/A.git", "1"), want: "github.com/org/a"},
		{name: "galaxy entry uses ns.name", entry: galaxyCollection("default.general", "community", "9"), want: "community.general"},
		{name: "no namespace", entry: LockFileEntry{Name: "default.solo"}, want: "solo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := entryIdentity(tt.entry); got != tt.want {
				t.Errorf("entryIdentity = %q, want %q", got, tt.want)
			}
		})
	}
}
