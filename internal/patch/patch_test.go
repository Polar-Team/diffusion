package patch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// writePatchProject creates a temp project: diffusion.toml with role
// sc.docker, an installed role, and scenario patch overlays. Returns the
// project root and the installed role path. The caller chdirs into root.
func writePatchProject(t *testing.T) (root, installed string) {
	t.Helper()
	root = t.TempDir()
	installed = filepath.Join(root, "installed", "geerlingguy.docker")
	mkdirs := []string{
		filepath.Join(installed, "tasks"),
		filepath.Join(installed, "handlers"),
		filepath.Join(installed, "templates"),
		filepath.Join(installed, "files"),
		filepath.Join(root, "scenarios", "sc", "patch", "files"),
		filepath.Join(root, "scenarios", "sc", "patch", "templates"),
	}
	for _, d := range mkdirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"diffusion.toml": "[dependencies]\n  [[dependencies.roles]]\n    Name = \"sc.docker\"\n    Namespace = \"geerlingguy\"\n    Version = \">=7.0.0\"\n",
		"installed/geerlingguy.docker/tasks/main.yml": `---
- name: Deploy config
  template:
    src: app.j2
    dest: /etc/app.conf
- name: Banner
  copy:
    src: banner
    dest: /etc/banner
- name: Extra template
  template:
    src: extra.j2
    dest: /etc/extra.conf
- name: More stuff
  include_tasks: more.yml
`,
		"installed/geerlingguy.docker/tasks/more.yml": `---
- name: More file
  copy:
    src: more.txt
    dest: /etc/more.txt
`,
		"installed/geerlingguy.docker/handlers/main.yml": `---
- name: restart app
  service:
    name: app
    state: restarted
`,
		"installed/geerlingguy.docker/templates/app.j2":    "v1\n",
		"installed/geerlingguy.docker/templates/extra.j2":  "e1\n",
		"installed/geerlingguy.docker/files/banner":        "old\n",
		"installed/geerlingguy.docker/files/more.txt":      "m\n",
		"scenarios/sc/patch/files/banner.custom":           "new\n",
		"scenarios/sc/patch/templates/extra.custom.j2":     "custom-e\n",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, installed
}

// chdirRoot chdirs into dir, restoring on cleanup.
func chdirRoot(t *testing.T, dir string) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
}

// readTaskFile parses a role-relative YAML task file from dir.
func readTaskFile(t *testing.T, dir, rel string) []*yaml.Node {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Content[0].Content
}

// taskValue returns the value node for key in a task mapping.
func taskValue(task *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(task.Content); i += 2 {
		if task.Content[i].Kind == yaml.ScalarNode && task.Content[i].Value == key {
			return task.Content[i+1]
		}
	}
	return nil
}

func mainBundle() *PatchBundle {
	return &PatchBundle{
		PatchBundleName: "b", Scenario: "sc", RoleName: "geerlingguy.docker",
		TasksToPatch: []PatchingTask{
			{TaskId: "1",
				NewModuleSetup: []PatchModuleSetup{{Key: "mode", Value: "0644"}},
				NewConditions:  []PatchConditions{{Condition: "ansible_os_family == 'Debian'"}},
				NewBecome:      &PatchBecomeSetup{User: "deploy"}},
			{TaskId: "2", NewFileSrc: "banner.custom"},
			{TaskId: "3", NewTemplateSrc: "extra.custom.j2"},
			{TaskId: "4.1", NewConditions: []PatchConditions{{Condition: "x == 'y'"}}},
			{TaskId: "h1", NewModuleSetup: []PatchModuleSetup{{Key: "state", Value: "reloaded"}}},
		},
	}
}

func TestApplyBundleOverlaysAndMutations(t *testing.T) {
	root, installed := writePatchProject(t)
	chdirRoot(t, root)
	stage, cleanup, err := StageRoleForPatch(installed)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	analysis, err := analyzeRoleAtPath(stage, "sc", "geerlingguy.docker")
	if err != nil {
		t.Fatal(err)
	}
	res, err := ApplyBundle(mainBundle(), analysis, ApplyOptions{RolePath: stage})
	if err != nil {
		t.Fatalf("ApplyBundle: %v", err)
	}
	if res.BackupPath == "" {
		t.Error("expected backup path")
	}
	if len(res.Overlaid) != 2 {
		t.Errorf("overlaid = %v, want 2", res.Overlaid)
	}
	if len(res.Patched) != 3 {
		t.Errorf("patched = %v, want 3 (1, 4.1, h1)", res.Patched)
	}

	// Overlays replaced content in the stage only.
	if got := readFile(t, filepath.Join(stage, "files", "banner")); got != "new\n" {
		t.Errorf("staged banner = %q", got)
	}
	if got := readFile(t, filepath.Join(stage, "templates", "extra.j2")); got != "custom-e\n" {
		t.Errorf("staged extra.j2 = %q", got)
	}
	if got := readFile(t, filepath.Join(installed, "files", "banner")); got != "old\n" {
		t.Errorf("installed banner mutated: %q", got)
	}

	// YAML mutations.
	tasks := readTaskFile(t, stage, "tasks/main.yml")
	tpl := taskValue(tasks[0], "template")
	if tpl == nil || taskValue(tpl, "mode") == nil || taskValue(tpl, "mode").Value != "0644" {
		t.Errorf("task 1 template args not merged: %v", tpl)
	}
	if w := taskValue(tasks[0], "when"); w == nil || w.Value != "ansible_os_family == 'Debian'" {
		t.Errorf("task 1 when not set: %v", w)
	}
	if bu := taskValue(tasks[0], "become_user"); bu == nil || bu.Value != "deploy" {
		t.Errorf("task 1 become_user not set: %v", bu)
	}
	more := readTaskFile(t, stage, "tasks/more.yml")
	if w := taskValue(more[0], "when"); w == nil || w.Value != "x == 'y'" {
		t.Errorf("task 4.1 when not set: %v", w)
	}
	handlers := readTaskFile(t, stage, "handlers/main.yml")
	svc := taskValue(handlers[0], "service")
	if svc == nil || taskValue(svc, "state") == nil || taskValue(svc, "state").Value != "reloaded" {
		t.Errorf("handler h1 service state not merged: %v", svc)
	}

	if len(res.Files) == 0 {
		t.Error("expected changed files")
	}
	if s := res.Summary(); !strings.Contains(s, "bundle \"b\"") || !strings.Contains(s, "backup:") {
		t.Errorf("summary missing parts:\n%s", s)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestApplyBundleDryRun(t *testing.T) {
	root, installed := writePatchProject(t)
	chdirRoot(t, root)
	stage, cleanup, err := StageRoleForPatch(installed)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	before, err := os.ReadFile(filepath.Join(stage, "tasks", "main.yml"))
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := analyzeRoleAtPath(stage, "sc", "geerlingguy.docker")
	if err != nil {
		t.Fatal(err)
	}
	res, err := ApplyBundle(mainBundle(), analysis, ApplyOptions{RolePath: stage, DryRun: true})
	if err != nil {
		t.Fatalf("dry-run ApplyBundle: %v", err)
	}
	if !res.DryRun || res.BackupPath != "" {
		t.Errorf("dry-run result wrong: %+v", res)
	}
	after, _ := os.ReadFile(filepath.Join(stage, "tasks", "main.yml"))
	if string(before) != string(after) {
		t.Error("dry-run modified tasks/main.yml")
	}
	if _, err := os.Stat(filepath.Join(stage, "files", "banner")); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(stage, "files", "banner")); got != "old\n" {
		t.Error("dry-run modified overlay target")
	}
	if len(res.Patched) != 3 || len(res.Overlaid) != 2 {
		t.Errorf("dry-run should report intended changes: %+v", res)
	}
}

func TestApplyBundleRejects(t *testing.T) {
	root, installed := writePatchProject(t)
	chdirRoot(t, root)
	stage, cleanup, err := StageRoleForPatch(installed)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	analysis, err := analyzeRoleAtPath(stage, "sc", "geerlingguy.docker")
	if err != nil {
		t.Fatal(err)
	}
	mk := func(id string) *PatchBundle {
		return &PatchBundle{PatchBundleName: "b", Scenario: "sc", RoleName: "docker",
			TasksToPatch: []PatchingTask{{TaskId: id, NewModule: "copy"}}}
	}
	if _, err := ApplyBundle(mk("9"), analysis, ApplyOptions{RolePath: stage}); err == nil ||
		!strings.Contains(err.Error(), "unknown task_id") {
		t.Errorf("unknown ID not rejected: %v", err)
	}
	if _, err := ApplyBundle(mk("4"), analysis, ApplyOptions{RolePath: stage}); err == nil ||
		!strings.Contains(err.Error(), "branch") {
		t.Errorf("branch ID not rejected: %v", err)
	}
	// File overlay on a template task is a kind mismatch.
	kind := &PatchBundle{PatchBundleName: "b", Scenario: "sc", RoleName: "docker",
		TasksToPatch: []PatchingTask{{TaskId: "1", NewFileSrc: "banner.custom"}}}
	if _, err := ApplyBundle(kind, analysis, ApplyOptions{RolePath: stage}); err == nil ||
		!strings.Contains(err.Error(), "new_template_src") {
		t.Errorf("kind mismatch not rejected: %v", err)
	}
}

func TestApplyBundleDriftAndForce(t *testing.T) {
	root, installed := writePatchProject(t)
	chdirRoot(t, root)
	stage, cleanup, err := StageRoleForPatch(installed)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	analysis, err := analyzeRoleAtPath(stage, "sc", "geerlingguy.docker")
	if err != nil {
		t.Fatal(err)
	}
	// Drift the staged copy after analysis.
	drifted := strings.Replace(
		readFile(t, filepath.Join(stage, "tasks", "more.yml")),
		"More file", "Renamed file", 1)
	if err := os.WriteFile(filepath.Join(stage, "tasks", "more.yml"), []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle := &PatchBundle{PatchBundleName: "b", Scenario: "sc", RoleName: "docker",
		TasksToPatch: []PatchingTask{{TaskId: "4.1", NewModuleSetup: []PatchModuleSetup{{Key: "mode", Value: "0600"}}}}}
	if _, err := ApplyBundle(bundle, analysis, ApplyOptions{RolePath: stage}); err == nil ||
		!strings.Contains(err.Error(), "drifted") {
		t.Errorf("drift not detected: %v", err)
	}
	if _, err := ApplyBundle(bundle, analysis, ApplyOptions{RolePath: stage, Force: true}); err != nil {
		t.Errorf("force apply failed: %v", err)
	}
}

func TestRevertPatches(t *testing.T) {
	root, installed := writePatchProject(t)
	chdirRoot(t, root)
	stage, cleanup, err := StageRoleForPatch(installed)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	analysis, err := analyzeRoleAtPath(stage, "sc", "geerlingguy.docker")
	if err != nil {
		t.Fatal(err)
	}
	snap, err := os.ReadFile(filepath.Join(stage, "tasks", "main.yml"))
	if err != nil {
		t.Fatal(err)
	}
	res, err := ApplyBundle(mainBundle(), analysis, ApplyOptions{RolePath: stage})
	if err != nil {
		t.Fatal(err)
	}
	changed, _ := os.ReadFile(filepath.Join(stage, "tasks", "main.yml"))
	if string(snap) == string(changed) {
		t.Fatal("apply changed nothing")
	}
	if err := RevertPatches(res.BackupPath, stage); err != nil {
		t.Fatalf("RevertPatches: %v", err)
	}
	restored, _ := os.ReadFile(filepath.Join(stage, "tasks", "main.yml"))
	if string(snap) != string(restored) {
		t.Error("revert did not restore tasks/main.yml")
	}
	if got := readFile(t, filepath.Join(stage, "files", "banner")); got != "old\n" {
		t.Error("revert did not restore overlay target")
	}
}
