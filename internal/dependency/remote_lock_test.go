package dependency

import (
	"strings"
	"testing"
)

// TestFetchLockFromGit_RejectsOptionLikeArguments guards against git argument
// injection. URLs and refs at depth > 0 originate from third-party lock files,
// so a value such as "--upload-pack=/bin/sh" must never reach the git command.
//
// These cases must fail before git is invoked; the assertions on the error
// text confirm the rejection happened in the guard rather than in git itself
// (a real clone attempt would report "git clone failed").
func TestFetchLockFromGit_RejectsOptionLikeArguments(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		version string
		wantIn  string
	}{
		{
			name:    "option-like url",
			url:     "--upload-pack=touch /tmp/pwned",
			version: "main",
			wantIn:  `refusing to clone "--upload-pack=touch /tmp/pwned": argument looks like a git option`,
		},
		{
			name:    "short option url",
			url:     "-c",
			version: "main",
			wantIn:  `refusing to clone "-c": argument looks like a git option`,
		},
		{
			name:    "option-like ref",
			url:     "https://ok.invalid/r.git",
			version: "-bad",
			wantIn:  `refusing to clone "-bad": argument looks like a git option`,
		},
		{
			name:    "upload-pack ref",
			url:     "https://ok.invalid/r.git",
			version: "--upload-pack=id",
			wantIn:  `refusing to clone "--upload-pack=id": argument looks like a git option`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manifest, err := FetchLockFromGit(tt.url, tt.version, nil)
			if err == nil {
				t.Fatalf("expected a rejection, got manifest %+v", manifest)
			}
			if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.wantIn)
			}
			// A rejection must happen before git runs.
			if strings.Contains(err.Error(), "git clone failed") {
				t.Errorf("git was invoked despite the guard: %v", err)
			}
		})
	}
}

// TestFetchLockFromGit_EmptyURL keeps the pre-existing contract.
func TestFetchLockFromGit_EmptyURL(t *testing.T) {
	if _, err := FetchLockFromGit("", "main", nil); err == nil {
		t.Fatal("expected an error for an empty URL")
	}
}
