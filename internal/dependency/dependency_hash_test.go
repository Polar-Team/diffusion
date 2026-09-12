package dependency

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"testing"

	"diffusion/internal/config"
)

// legacyComputeDependencyHash is a verbatim copy of ComputeDependencyHash as it
// existed before git-sourced collections were introduced. It is the reference
// implementation for the backward-compatibility assertions below.
//
// The Galaxy API lookup is deliberately omitted: every fixture uses
// Namespace == "" so the original code would have skipped it too. That keeps
// this test fully offline and deterministic.
func legacyComputeDependencyHash(collections []config.CollectionRequirement, roles []config.RoleRequirement, toolVersions map[string]string, pythonVersion *config.PythonVersion) string {
	h := sha256.New()

	sort.Slice(collections, func(i, j int) bool {
		return collections[i].Name < collections[j].Name
	})
	for _, col := range collections {
		// Namespace == "" in all fixtures → no Galaxy resolution in the original.
		fmt.Fprintf(h, "collection:%s:%s:%s\n", col.Namespace, col.Name, col.Version)
	}

	sort.Slice(roles, func(i, j int) bool {
		return roles[i].Name < roles[j].Name
	})
	for _, role := range roles {
		parts := strings.SplitN(role.Name, ".", 2)
		var scenario, roleName string
		if len(parts) == 2 {
			scenario = parts[0]
			roleName = parts[1]
		} else {
			scenario = "default"
			roleName = role.Name
		}
		fmt.Fprintf(h, "role:%s:%s:%s:%s:%s\n", scenario, role.Namespace, roleName, role.Version, role.Src)
	}

	var tools []string
	for tool := range toolVersions {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	for _, tool := range tools {
		fmt.Fprintf(h, "tool:%s:%s\n", tool, toolVersions[tool])
	}

	if pythonVersion != nil {
		fmt.Fprintf(h, "python:%s:%s:%s\n", pythonVersion.Min, pythonVersion.Max, pythonVersion.Pinned)
	}

	return hex.EncodeToString(h.Sum(nil))
}

// TestComputeDependencyHash_GalaxyOnlyIsBackwardCompatible asserts that a
// project containing no git-sourced collections hashes exactly as it did
// before the feature landed, so existing diffusion.lock files are not
// invalidated by upgrading diffusion.
//
// All fixtures use Namespace == "" so no Galaxy lookup happens: offline, fast.
func TestComputeDependencyHash_GalaxyOnlyIsBackwardCompatible(t *testing.T) {
	python := &config.PythonVersion{Min: "3.11", Max: "3.13", Pinned: "3.12"}
	tools := map[string]string{"ansible": ">=10.0.0", "molecule": ">=24.0.0"}

	tests := []struct {
		name        string
		collections []config.CollectionRequirement
	}{
		{
			name:        "no collections",
			collections: nil,
		},
		{
			name: "implicit galaxy source",
			collections: []config.CollectionRequirement{
				{Name: "default.general", Version: ">=9.0.0"},
				{Name: "default.docker", Version: ">=5.0.0"},
			},
		},
		{
			name: "explicit galaxy source",
			collections: []config.CollectionRequirement{
				{Name: "default.general", Version: ">=9.0.0", Source: "galaxy"},
			},
		},
		{
			name: "mixed implicit and explicit galaxy",
			collections: []config.CollectionRequirement{
				{Name: "default.general", Version: ">=9.0.0", Source: "galaxy"},
				{Name: "default.docker", Version: ">=5.0.0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Each call sorts its argument in place; give both their own copy.
			want := legacyComputeDependencyHash(append([]config.CollectionRequirement{}, tt.collections...), nil, tools, python)
			got := ComputeDependencyHash(append([]config.CollectionRequirement{}, tt.collections...), nil, tools, python)

			if got != want {
				t.Errorf("hash changed for a Galaxy-only project:\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

// TestComputeDependencyHash_GitCollectionChangesHash asserts the opposite
// direction: a git-sourced collection must contribute its Source/SourceURL, so
// that changing the repository URL invalidates the lock file.
func TestComputeDependencyHash_GitCollectionChangesHash(t *testing.T) {
	python := &config.PythonVersion{Min: "3.11", Max: "3.13", Pinned: "3.12"}
	tools := map[string]string{"ansible": ">=10.0.0"}

	gitA := []config.CollectionRequirement{
		{Name: "default.foo", Version: "main", Source: "git", SourceURL: "https://example.invalid/a.git"},
	}
	gitB := []config.CollectionRequirement{
		{Name: "default.foo", Version: "main", Source: "git", SourceURL: "https://example.invalid/b.git"},
	}
	bare := []config.CollectionRequirement{
		{Name: "default.foo", Version: "main"},
	}

	hashA := ComputeDependencyHash(append([]config.CollectionRequirement{}, gitA...), nil, tools, python)
	hashB := ComputeDependencyHash(append([]config.CollectionRequirement{}, gitB...), nil, tools, python)
	hashBare := ComputeDependencyHash(append([]config.CollectionRequirement{}, bare...), nil, tools, python)

	if hashA == hashB {
		t.Error("changing a git collection's SourceURL must change the hash")
	}
	if hashA == hashBare {
		t.Error("a git collection must not hash the same as a bare Galaxy collection with the same name")
	}

	legacyBare := legacyComputeDependencyHash(append([]config.CollectionRequirement{}, bare...), nil, tools, python)
	if hashBare != legacyBare {
		t.Error("a bare collection must keep the legacy hash line")
	}
}
