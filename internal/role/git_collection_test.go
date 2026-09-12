package role

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRequirementCollectionGitRoundTrip verifies that a git-sourced collection
// survives a SaveRequirementFile/ParseRequirementFile round trip in the
// ansible-galaxy git collection format, and that Galaxy collections keep their
// plain "name/version" output.
func TestRequirementCollectionGitRoundTrip(t *testing.T) {
	tempDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalDir) })
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	if err := os.MkdirAll(filepath.Join("scenarios", "default"), 0755); err != nil {
		t.Fatalf("failed to create scenario dir: %v", err)
	}

	req := &Requirement{
		Collections: []RequirementCollection{
			{Name: "community.general", Version: "10.5.0"},
			{Name: "https://github.com/org/ansible-collection-foo.git", Type: "git", Version: "main"},
		},
		Roles: []RequirementRole{},
	}

	if err := SaveRequirementFile(req, "default"); err != nil {
		t.Fatalf("failed to save requirements.yml: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join("scenarios", "default", "requirements.yml"))
	if err != nil {
		t.Fatalf("failed to read requirements.yml: %v", err)
	}
	content := string(raw)

	if !strings.Contains(content, "name: https://github.com/org/ansible-collection-foo.git") {
		t.Errorf("git collection name missing from output:\n%s", content)
	}
	if !strings.Contains(content, "type: git") {
		t.Errorf("git collection type missing from output:\n%s", content)
	}
	// Galaxy entries must not gain any extra keys.
	if strings.Contains(content, "source:") || strings.Contains(content, "source_url:") {
		t.Errorf("unexpected source keys in output:\n%s", content)
	}

	loaded, err := ParseRequirementFile("default")
	if err != nil {
		t.Fatalf("failed to parse requirements.yml: %v", err)
	}

	if len(loaded.Collections) != 2 {
		t.Fatalf("expected 2 collections, got %d", len(loaded.Collections))
	}

	tests := []struct {
		name    string
		want    RequirementCollection
		gotIdx  int
		wantErr bool
	}{
		{name: "galaxy", want: RequirementCollection{Name: "community.general", Version: "10.5.0"}, gotIdx: 0},
		{name: "git", want: RequirementCollection{Name: "https://github.com/org/ansible-collection-foo.git", Type: "git", Version: "main"}, gotIdx: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := loaded.Collections[tt.gotIdx]
			if got != tt.want {
				t.Errorf("round trip mismatch:\n got: %+v\nwant: %+v", got, tt.want)
			}
		})
	}
}
