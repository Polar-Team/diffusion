package patch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFixtureRole creates a role with nested includes, a block, an
// opaque (missing) include and handlers for analyzer tests.
func writeFixtureRole(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	tasks := filepath.Join(root, "tasks")
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "handlers"), 0o755); err != nil {
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
		"handlers/main.yml": `---
- name: restart app
  service:
    name: app
    state: restarted
- name: reload config
  template:
    src: app.j2
    dest: /etc/app.conf
  tags: [deploy]
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
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatalf("analyzeRoleAtPath: %v", err)
	}
	if a.RoleName != "fixture.role" {
		t.Fatalf("RoleName = %q, want fixture.role", a.RoleName)
	}
	wantLeaves := []string{"1.1", "1.2", "1.3.1", "1.4", "2", "3.1", "3.2.1", "3.3", "h1", "h2"}
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
	if got := a.Templates["app.j2"]; strings.Join(got, ",") != "2,h2" {
		t.Fatalf("templates[app.j2] = %v, want [2 h2]", got)
	}
	if got := a.Files["foo.conf"]; len(got) != 1 || got[0] != "1.1" {
		t.Fatalf("files[foo.conf] = %v", got)
	}
	// Handlers map holds notifier leaf IDs plus the definition site.
	if got := a.Handlers["restart app"]; strings.Join(got, ",") != "2,h1" {
		t.Fatalf("handlers[restart app] = %v, want [2 h1]", got)
	}
	// Leaf lookup shared with patch.go.
	leaf := a.FindLeafByID(" 1.3.1 ")
	if leaf == nil || leaf.Name != "Data dir" || leaf.Module != "file" {
		t.Fatalf("FindLeafByID(1.3.1) = %+v", leaf)
	}
	if h := a.FindLeafByID("H2"); h == nil || h.Module != "template" {
		t.Fatalf("FindLeafByID(H2) = %+v", h)
	}
	if a.FindLeafByID("1") != nil {
		t.Fatal("FindLeafByID(1) must be nil: branches are not leaves")
	}
	if a.FindLeafByID("bogus") != nil {
		t.Fatal("FindLeafByID(bogus) must be nil")
	}
}

func TestAnalyzeIndexPaths(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	pathOf := func(id string) []FileIndex {
		l := a.FindLeafByID(id)
		if l == nil {
			t.Fatalf("leaf %s not found", id)
		}
		return l.Path
	}
	want := []FileIndex{
		{File: "tasks/main.yml", Index: 0},
		{File: "tasks/setup.yml", Index: 2},
		{File: "tasks/nested.yml", Index: 0},
	}
	got := pathOf("1.3.1")
	if len(got) != len(want) {
		t.Fatalf("path(1.3.1) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("path(1.3.1) = %v, want %v", got, want)
		}
	}
	if got := pathOf("h2"); len(got) != 1 || got[0] != (FileIndex{File: "handlers/main.yml", Index: 1}) {
		t.Fatalf("path(h2) = %v", got)
	}
	b := a.FindBranchByID("1")
	if len(b.Path) != 1 || b.Path[0] != (FileIndex{File: "tasks/main.yml", Index: 0}) {
		t.Fatalf("path(branch 1) = %v", b.Path)
	}
}

func TestPrintTreeBranchesColorsAndMarkers(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	var sb strings.Builder
	a.PrintTree(&sb, TreeOptions{NoColor: true, Overlays: map[string]string{"1.1": "patch/files/banner.custom"}})
	out := sb.String()
	for _, want := range []string{
		"role fixture.role", "▶ 1 [branch include_tasks]", "● 1.1 [copy]",
		"● 1.3.1 [file]", "● 2 [template]", "◆ tags:[setup]", "⇄",
		"<= patch/files/banner.custom", "handlers:", "● h1 [service]", "● h2 [template]",
		"warnings:",
	} {
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
	first, err := analyzeRoleAtPath(root, "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	second, err := analyzeRoleAtPath(root, "default", "fixture.role")
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
	if _, err := analyzeRoleAtPath(t.TempDir(), "default", "x"); err == nil {
		t.Fatal("expected error for role without tasks/main.yml")
	}
	// Unknown role that is nowhere installed must error, never hang.
	if _, err := AnalyzeExternalRole("default", "no.such.role"); err == nil {
		t.Fatal("expected error for uninstalled role")
	}
}

func TestResolveInstalledRolePath(t *testing.T) {
	root := t.TempDir()
	roleDir := filepath.Join(root, "molecule", "geerlingguy.docker", "tasks")
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roleDir, "main.yml"), []byte("---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()

	got, err := ResolveInstalledRolePath("geerlingguy.docker")
	if err != nil {
		t.Fatalf("full name not resolved: %v", err)
	}
	if filepath.Base(got) != "geerlingguy.docker" {
		t.Fatalf("resolved to %s", got)
	}
	// Short name matches the namespaced directory.
	if _, err := ResolveInstalledRolePath("docker"); err != nil {
		t.Fatalf("short name not resolved: %v", err)
	}
	if _, err := ResolveInstalledRolePath("missing.role"); err == nil {
		t.Fatal("expected error for missing role")
	}
}

func TestAnalysisSerialization(t *testing.T) {
	a, err := analyzeRoleAtPath(writeFixtureRole(t), "default", "fixture.role")
	if err != nil {
		t.Fatal(err)
	}
	yml, err := a.ToYAML()
	if err != nil || len(yml) == 0 {
		t.Fatalf("ToYAML: %v", err)
	}
	for _, want := range []string{"1.3.1", "role_name", "h2", "index: 2"} {
		if !strings.Contains(string(yml), want) {
			t.Errorf("YAML missing %q:\n%s", want, yml)
		}
	}
}
