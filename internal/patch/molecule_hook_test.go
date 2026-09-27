package patch

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeHookProject creates a temp project rooted at root/default scenario:
// diffusion.toml declaring default.docker, a molecule-installed role with one
// task, requirements manifest and patch.yml setting a when condition.
func writeHookProject(t *testing.T, root string) {
	t.Helper()
	roleTasks := filepath.Join(root, "molecule", "geerlingguy.docker", "tasks")
	for _, d := range []string{
		roleTasks,
		filepath.Join(root, "scenarios", "default"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"diffusion.toml": "[dependencies]\n  [[dependencies.roles]]\n    Name = \"default.docker\"\n    Namespace = \"geerlingguy\"\n    Version = \">=1.0.0\"\n",
		"molecule/geerlingguy.docker/tasks/main.yml": `---
- name: Hello
  debug:
    msg: hi
`,
		"scenarios/default/requirements.yml": "---\n",
		"scenarios/default/patch.yml": `Bundles:
- patch_bundle_name: hook-bundle
  scenario: default
  role_name: geerlingguy.docker
  tasks_to_patch:
  - task_id: "1"
    new_conditions:
    - condition: when
      body: x == 'y'
`,
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(rel)), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestApplyScenarioPatchesNoPatchFile(t *testing.T) {
	root := t.TempDir()
	chdirRoot(t, root)

	applied, err := ApplyScenarioPatches("default")
	if err != nil {
		t.Fatalf("expected no-op, got error: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("expected no patches, got %v", applied)
	}
}

func TestApplyScenarioPatchesApplies(t *testing.T) {
	root := t.TempDir()
	writeHookProject(t, root)
	chdirRoot(t, root)

	// Local molecule install must win: docker must never run.
	origDownloader := dockerRoleDownloader
	dockerRoleDownloader = func(sessionID, patchDir, scenario string) error {
		t.Fatal("downloader must not run when the role is installed locally")
		return nil
	}
	defer func() { dockerRoleDownloader = origDownloader }()

	applied, err := ApplyScenarioPatches("default")
	if err != nil {
		t.Fatalf("ApplyScenarioPatches: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("expected 1 applied patch, got %v", applied)
	}
	if applied[0].Bundle != "hook-bundle" {
		t.Fatalf("bundle = %q, want hook-bundle", applied[0].Bundle)
	}
	data, err := os.ReadFile(filepath.Join(root, "molecule", "geerlingguy.docker", "tasks", "main.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "when:") {
		t.Fatalf("patched file missing when condition:\n%s", data)
	}
}
