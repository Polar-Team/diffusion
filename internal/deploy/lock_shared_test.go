package deploy

import (
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
)

// The lock merging algorithm now lives in internal/dependency (see
// lock_shared.go); its unit tests moved with it to
// internal/dependency/lock_merge_test.go. These smoke tests only guard the
// delegation wrappers so that the deploy package's public API keeps working.

func TestMergeLocks_DelegatesToDependency(t *testing.T) {
	locks := []dependency.LockFile{
		{
			Version:     dependency.LockFileVersion,
			Python:      &config.PythonVersion{Min: "3.11", Max: "3.13", Pinned: "3.12"},
			Collections: []dependency.LockFileEntry{{Name: "default.general", Namespace: "community", ResolvedVersion: "9.0.0"}},
		},
	}

	got, err := MergeLocks(locks)
	if err != nil {
		t.Fatalf("MergeLocks returned error: %v", err)
	}
	if got == nil {
		t.Fatal("MergeLocks returned nil lock")
	}
	if len(got.Collections) != 1 || got.Collections[0].Name != "default.general" {
		t.Errorf("unexpected merged collections: %+v", got.Collections)
	}
}

func TestMergeLocks_EmptyReturnsDefaults(t *testing.T) {
	got, err := MergeLocks(nil)
	if err != nil {
		t.Fatalf("MergeLocks returned error: %v", err)
	}
	if got.Python == nil || got.Python.Min != config.DefaultMinPythonVersion {
		t.Errorf("expected default python version, got %+v", got.Python)
	}
}

func TestResolveGitRef_Delegation(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "latest", input: "latest", want: ""},
		{name: "constraint clones default branch", input: ">=1.0.0", want: ""},
		{name: "branch", input: "main", want: "main"},
		{name: "tag", input: "v2.3.1", want: "v2.3.1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveGitRef(tt.input); got != tt.want {
				t.Errorf("resolveGitRef(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeEnvKey_Delegation(t *testing.T) {
	if got := sanitizeEnvKey("my-artifact.source name"); got != "MY_ARTIFACT_SOURCE_NAME" {
		t.Errorf("sanitizeEnvKey = %q, want MY_ARTIFACT_SOURCE_NAME", got)
	}
}

func TestStripScheme_Delegation(t *testing.T) {
	if got := stripScheme("https://gitlab.local/org"); got != "gitlab.local/org" {
		t.Errorf("stripScheme = %q, want gitlab.local/org", got)
	}
}
