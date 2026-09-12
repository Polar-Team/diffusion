package galaxy

import (
	"strings"
	"testing"
)

// TestResolveVersionFromGit_RejectsOptionLikeArguments guards against git
// argument injection through URLs/refs that originate from third-party
// diffusion.lock files during transitive dependency resolution.
func TestResolveVersionFromGit_RejectsOptionLikeArguments(t *testing.T) {
	tests := []struct {
		name       string
		url        string
		constraint string
		wantIn     string
	}{
		{
			name:       "option-like url",
			url:        "--upload-pack=touch /tmp/pwned",
			constraint: "main",
			wantIn:     "looks like a command line option",
		},
		{
			name:       "option-like constraint",
			url:        "https://ok.invalid/r.git",
			constraint: "-bad",
			wantIn:     "looks like a command line option",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveVersionFromGit(tt.url, tt.constraint)
			if err == nil {
				t.Fatalf("expected a rejection, got %q", got)
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantIn)
			}
		})
	}
}

func TestGetLatestGitTag_RejectsOptionLikeURL(t *testing.T) {
	got, err := GetLatestGitTag("--upload-pack=id")
	if err == nil {
		t.Fatalf("expected a rejection, got %q", got)
	}
	if !strings.Contains(err.Error(), "looks like a command line option") {
		t.Errorf("unexpected error: %v", err)
	}
}
