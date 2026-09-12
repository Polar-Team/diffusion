package utils

import "testing"

func TestIsGitURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{name: "https url", input: "https://github.com/org/foo.git", want: true},
		{name: "ssh scheme", input: "ssh://git@github.com/org/foo.git", want: true},
		{name: "scp syntax", input: "git@github.com:org/foo.git", want: true},
		{name: "galaxy name", input: "community.general", want: false},
		{name: "short name", input: "general", want: false},
		{name: "empty", input: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsGitURL(tt.input); got != tt.want {
				t.Errorf("IsGitURL(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDeriveCollectionShortName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "collection prefix stripped", input: "https://github.com/org/ansible-collection-foo.git", want: "foo"},
		{name: "underscore prefix stripped", input: "https://github.com/org/ansible_collection_foo.git", want: "foo"},
		{name: "no prefix", input: "https://github.com/org/foo.git", want: "foo"},
		{name: "scp syntax", input: "git@github.com:org/ansible-collection-bar.git", want: "bar"},
		{name: "no .git suffix", input: "https://gitlab.com/org/baz", want: "baz"},
		{name: "trailing slash", input: "https://gitlab.com/org/baz/", want: "baz"},
		{name: "dots become underscores", input: "https://github.com/org/my.repo.git", want: "my_repo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DeriveCollectionShortName(tt.input); got != tt.want {
				t.Errorf("DeriveCollectionShortName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
