package patch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixtureRole creates a role with nested includes, a block and an
// opaque (missing) include for analyzer tests.
func writeFixtureRole(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	tasks := filepath.Join(root, "tasks")
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"tasks/main.yml": `---
- name: Setup everything
  include_tasks: setup.yml
  tags:
    - setup
- name: Deploy app config
  ansible.builtin.template:
    src: app.j2
    dest: /etc/app.conf
  notify: restart app
  tags: [deploy]
- name: Harden
  block:
    - name: Copy banner
      copy:
        src: banner
        dest: /etc/banner
    - name: Nested include
      include_tasks: nested.yml
  rescue:
    - name: Fallback file
      file:
        path: /tmp/fallback
        state: touch
- name: Ghost include
  include_tasks: missing.yml
`,
		"tasks/setup.yml": `---
- name: Install pkg
  copy:
    src: foo.conf
    dest: /etc/foo.conf
- name: Render bar
  template:
    src: bar.j2
    dest: /etc/bar.conf
- name: More
  include_tasks: nested.yml
- name: Ensure service
  service:
    name: app
    state: started
`,
		"tasks/nested.yml": `---
- name: Data dir
  file:
    path: /data
    state: directory
`,
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func leafIDs(a *RoleAnalysis) []string {
	ids := make([]string, 0, len(a.Leaves))
	for _, l := range a.Leaves {
		ids = append(ids, l.ID)
	}
	return ids
}

func TestAnalyzeExternalRoleBranchIDs(t *testing.T) {
	a, err := AnalyzeExternalRole(writeFixtureRole(t), "default")
	if err != nil {
		t.Fatalf("AnalyzeExternalRole: %v", err)
	}
	wantLeaves := []string{"1.1", "1.2", "1.3.1", "1.4", "2", "3.1", "3.2.1", "3.3"}
	got := leafIDs(a)
	if strings.Join(got, ",") != strings.Join(wantLeaves, ",") {
		t.Fatalf("leaves = %v, want %v", got, wantLeaves)
	}
	if len(a.Branches) != 5 {
		t.Fatalf("branches = %d, want 5 (1, 1.3, 3 block, 3.2, 4 opaque)", len(a.Branches))
	}
	// Opaque branch for the missing include.
	ghost := a.FindBranchByID("4")
	if ghost == nil || !ghost.Opaque {
		t.Fatalf("branch 4 should be opaque: %+v", ghost)
	}
	if len(a.Warnings) == 0 {
		t.Fatal("expected warnings for missing include")
	}
	// Evidence maps.
	if got := a.Templates["app.j2"]; len(got) != 1 || got[0] != "2" {
		t.Fatalf("templates[app.j2] = %v", got)
	}
	if got := a.Files["foo.conf"]; len(got) != 1 || got[0] != "1.1" {
		t.Fatalf("files[foo.conf] = %v", got)
	}
	if got := a.Handlers["restart app"]; len(got) != 1 || got[0] != "2" {
		t.Fatalf("handlers[restart app] = %v", got)
	}
	// Leaf lookup shared with patch.go.
	leaf := a.FindLeafByID(" 1.3.1 ")
	if leaf == nil || leaf.Name != "Data dir" || leaf.Module != "file" {
		t.Fatalf("FindLeafByID(1.3.1) = %+v", leaf)
	}
	if a.FindLeafByID("1") != nil {
		t.Fatal("FindLeafByID(1) must be nil: branches are not leaves")
	}
	if a.FindLeafByID("bogus") != nil {
		t.Fatal("FindLeafByID(bogus) must be nil")
	}
}

func TestPrintTreeBranchesColorsAndMarkers(t *testing.T) {
	a, err := AnalyzeExternalRole(writeFixtureRole(t), "default")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	a.PrintTree(&sb, TreeOptions{NoColor: true})
	out := sb.String()
	for _, want := range []string{"▶ 1 [branch include_tasks]", "● 1.1 [copy]", "● 1.3.1 [file]", "● 2 [template]", "◆ tags:[setup]", "⇄", "warnings:"} {
		if !strings.Contains(out, want) {
			t.Errorf("tree missing %q\n%s", want, out)
		}
	}
	// Multiple top-level branches get distinct random-picked colors.
	colors := map[string]bool{}
	for _, b := range a.Branches {
		if !strings.Contains(b.ID, ".") {
			if b.Color == "" {
				t.Errorf("top branch %s has no color", b.ID)
			}
			colors[b.Color] = true
		}
	}
	if len(colors) < 2 {
		t.Errorf("expected distinct colors per top branch, got %v", colors)
	}
	// Nested branches inherit the top branch color.
	if c1, c2 := a.FindBranchByID("1").Color, a.FindBranchByID("1.3").Color; c1 == "" || c1 != c2 {
		t.Errorf("branch 1.3 should inherit color of 1: %q vs %q", c1, c2)
	}
	// Stable colors across runs (seeded shuffle).
	root := writeFixtureRole(t)
	first, err := AnalyzeExternalRole(root, "default")
	if err != nil {
		t.Fatal(err)
	}
	second, err := AnalyzeExternalRole(root, "default")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"1", "3", "4"} {
		if first.FindBranchByID(id).Color != second.FindBranchByID(id).Color {
			t.Errorf("branch %s color unstable across runs", id)
		}
	}
}

func TestAnalyzeExternalRoleMissingEntry(t *testing.T) {
	if _, err := AnalyzeExternalRole(t.TempDir(), "default"); err == nil {
		t.Fatal("expected error for role without tasks/main.yml")
	}
}

func TestAnalysisSerialization(t *testing.T) {
	a, err := AnalyzeExternalRole(writeFixtureRole(t), "default")
	if err != nil {
		t.Fatal(err)
	}
	yml, err := a.ToYAML()
	if err != nil || len(yml) == 0 {
		t.Fatalf("ToYAML: %v", err)
	}
	if !strings.Contains(string(yml), "1.3.1") {
		t.Errorf("YAML missing leaf 1.3.1: %s", yml)
	}
}
