package patch

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/utils"
)

// fakeContainer emulates the container side of the overlay flow on the
// host filesystem: fsRoot mirrors the container root so `cp -a`, `test -d`,
// `ls`, `mount --bind` and `umount` can be simulated without docker.
type fakeContainer struct {
	t      *testing.T
	fsRoot string
	cmds   []string
	mounts map[string]string
	// stuckMounts marks mountpoints that refuse both umount and umount -l.
	stuckMounts map[string]bool
	// failStagingAt aborts the staging script just before this sentinel
	// ("S1".."S4") is echoed, emulating a `set -e` abort mid-way.
	failStagingAt string
}

// path maps a container path onto the fake root.
func (f *fakeContainer) path(p string) string {
	return filepath.Join(f.fsRoot, filepath.FromSlash(strings.TrimPrefix(p, "/")))
}

// exec implements the dockerExec contract for the few commands used by
// container.go: test -d, ls -1, mount --bind and sh -c scripts.
func (f *fakeContainer) exec(container string, args ...string) ([]byte, error) {
	f.cmds = append(f.cmds, strings.Join(args, " "))
	switch {
	case len(args) == 3 && args[0] == "test" && args[1] == "-d":
		if st, err := os.Stat(f.path(args[2])); err == nil && st.IsDir() {
			return nil, nil
		}
		return nil, fmt.Errorf("no such dir")
	case len(args) == 3 && args[0] == "ls":
		entries, err := os.ReadDir(f.path(args[2]))
		if err != nil {
			return nil, err
		}
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		return []byte(strings.Join(names, "\n")), nil
	case len(args) == 4 && args[0] == "mount" && args[1] == "--bind":
		f.mounts[args[3]] = args[2]
		return nil, nil
	case len(args) == 3 && args[0] == "sh" && args[1] == "-c":
		return f.shell(args[2])
	}
	return nil, fmt.Errorf("unexpected command %v", args)
}

// shell interprets the fixed script templates container.go emits.
func (f *fakeContainer) shell(script string) ([]byte, error) {
	switch {
	case strings.Contains(script, `failed=''`):
		return f.heal(script)
	case strings.Contains(script, "if grep -q"):
		return f.unmount(script)
	case strings.Contains(script, "echo S1"):
		return f.stage(script)
	}
	return nil, fmt.Errorf("unexpected script: %s", script)
}

// heal emulates HealStalePatchMounts: unmount everything below the roles
// path (deepest first), emitting FAILED:<m> for mounts marked stuck, and
// only remove the work tree when nothing was stuck.
func (f *fakeContainer) heal(script string) ([]byte, error) {
	var targets []string
	for m := range f.mounts {
		if strings.HasPrefix(m, config.ContainerRolesCachePath+"/") {
			targets = append(targets, m)
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(targets)))
	var out []string
	stuck := false
	for _, m := range targets {
		if f.stuckMounts[m] {
			out = append(out, "FAILED:"+m)
			stuck = true
			continue
		}
		delete(f.mounts, m)
	}
	if !stuck {
		// rm -rf <ContainerPatchDir>/<scenario>
		for _, stmt := range strings.Split(script, ";") {
			if fields := strings.Fields(stmt); len(fields) >= 3 && fields[len(fields)-3] == "rm" {
				_ = os.RemoveAll(f.path(fields[len(fields)-1]))
			}
		}
	}
	return []byte(strings.Join(out, "\n")), nil
}

// unmount emulates unmountOverlay for a single mountpoint.
func (f *fakeContainer) unmount(script string) ([]byte, error) {
	for m := range f.mounts {
		if strings.Contains(script, " "+m+" ") {
			if f.stuckMounts[m] {
				return []byte("umount: target is busy"), fmt.Errorf("exit status 32")
			}
			delete(f.mounts, m)
		}
	}
	return nil, nil
}

// stage emulates prepareWorkCopy with `set -e` semantics: each statement
// runs in order, S1..S4 sentinels are echoed, and failStagingAt aborts the
// script right before the named sentinel would be printed.
func (f *fakeContainer) stage(script string) ([]byte, error) {
	var out []string
	for _, stmt := range strings.Split(script, ";") {
		fields := strings.Fields(stmt)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "echo":
			if f.failStagingAt == fields[1] {
				out = append(out, "cp: cannot create directory: No space left on device")
				return []byte(strings.Join(out, "\n")), fmt.Errorf("exit status 1")
			}
			out = append(out, fields[1])
		case "rm":
			for _, p := range fields[2:] {
				_ = os.RemoveAll(f.path(p))
			}
		case "mkdir":
			for _, p := range fields[2:] {
				if err := os.MkdirAll(f.path(p), 0o755); err != nil {
					return []byte(strings.Join(out, "\n")), err
				}
			}
		case "cp":
			if err := utils.CopyDir(f.path(fields[2]), f.path(fields[3])); err != nil {
				return []byte(strings.Join(out, "\n")), err
			}
		}
	}
	return []byte(strings.Join(out, "\n")), nil
}

// cp implements the dockerCp contract for "<container>:<path>/." sources
// and destinations.
func (f *fakeContainer) cp(src, dst string) error {
	strip := func(p string) (string, bool) {
		if i := strings.Index(p, ":"); i > 1 {
			return strings.TrimSuffix(p[i+1:], "/."), true
		}
		return strings.TrimSuffix(strings.TrimSuffix(p, string(os.PathSeparator)+"."), "/."), false
	}
	s, sIn := strip(src)
	d, dIn := strip(dst)
	if sIn {
		s = f.path(s)
	}
	if dIn {
		d = f.path(d)
	}
	return utils.CopyDir(s, d)
}

// newFakeContainer installs the fake docker stubs and a container-side role
// under /root/.ansible/roles/geerlingguy.docker.
func newFakeContainer(t *testing.T) *fakeContainer {
	t.Helper()
	f := &fakeContainer{t: t, fsRoot: t.TempDir(), mounts: map[string]string{}, stuckMounts: map[string]bool{}}
	roleTasks := f.path(config.ContainerRolesCachePath + "/geerlingguy.docker/tasks")
	if err := os.MkdirAll(roleTasks, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roleTasks, "main.yml"), []byte("---\n- name: Hello\n  debug:\n    msg: hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	origExec, origCp, origPS := dockerExec, dockerCp, dockerPS
	dockerExec = f.exec
	dockerCp = f.cp
	dockerPS = func() ([]byte, error) { return []byte("molecule-test\n"), nil }
	t.Cleanup(func() { dockerExec, dockerCp, dockerPS = origExec, origCp, origPS })
	return f
}

// writeScenarioPatch creates scenarios/default/patch.yml plus the
// diffusion.toml declaring the targeted role.
func writeScenarioPatch(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "scenarios", "default"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"diffusion.toml": "[dependencies]\n  [[dependencies.roles]]\n    Name = \"default.docker\"\n    Namespace = \"geerlingguy\"\n    Version = \">=1.0.0\"\n",
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

func TestApplyScenarioPatchesInContainerNoPatchFile(t *testing.T) {
	chdirRoot(t, t.TempDir())
	newFakeContainer(t)

	applied, err := ApplyScenarioPatchesInContainer("molecule-test", "default", ContainerApplyOptions{})
	if err != nil {
		t.Fatalf("expected no-op, got error: %v", err)
	}
	if len(applied) != 0 {
		t.Fatalf("expected no patches, got %v", applied)
	}
}

func TestApplyScenarioPatchesInContainerAppliesAndRestores(t *testing.T) {
	root := t.TempDir()
	writeScenarioPatch(t, root)
	chdirRoot(t, root)
	f := newFakeContainer(t)

	applied, err := ApplyScenarioPatchesInContainer("molecule-test", "default", ContainerApplyOptions{})
	if err != nil {
		t.Fatalf("ApplyScenarioPatchesInContainer: %v", err)
	}
	if len(applied) != 1 {
		t.Fatalf("expected 1 applied patch, got %v", applied)
	}
	ap := applied[0]
	if ap.Bundle != "hook-bundle" || ap.Container != "molecule-test" {
		t.Fatalf("unexpected applied patch %+v", ap)
	}
	wantWork := config.ContainerPatchDir + "/default/work/geerlingguy.docker"
	if ap.WorkPath != wantWork {
		t.Fatalf("WorkPath = %q, want %q", ap.WorkPath, wantWork)
	}
	if ap.ContainerPath != config.ContainerRolesCachePath+"/geerlingguy.docker" {
		t.Fatalf("ContainerPath = %q", ap.ContainerPath)
	}
	if src := f.mounts[ap.ContainerPath]; src != wantWork {
		t.Fatalf("overlay not mounted: mounts = %v", f.mounts)
	}

	// Patched content lands in the work copy...
	data, err := os.ReadFile(filepath.Join(f.path(wantWork), "tasks", "main.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "when:") {
		t.Fatalf("work copy missing when condition:\n%s", data)
	}
	// ...and the pristine original is untouched.
	orig, err := os.ReadFile(filepath.Join(f.path(ap.ContainerPath), "tasks", "main.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(orig), "when:") {
		t.Fatalf("original role was mutated:\n%s", orig)
	}

	if err := RestoreContainerPatches("molecule-test", applied); err != nil {
		t.Fatalf("RestoreContainerPatches: %v", err)
	}
	if len(f.mounts) != 0 {
		t.Fatalf("overlay still mounted after restore: %v", f.mounts)
	}
}

func TestApplyScenarioPatchesInContainerDryRun(t *testing.T) {
	root := t.TempDir()
	writeScenarioPatch(t, root)
	chdirRoot(t, root)
	f := newFakeContainer(t)

	applied, err := ApplyScenarioPatchesInContainer("molecule-test", "default", ContainerApplyOptions{DryRun: true})
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if len(applied) != 1 || applied[0].Result == nil || !applied[0].Result.DryRun {
		t.Fatalf("expected one dry-run result, got %v", applied)
	}
	if len(f.mounts) != 0 {
		t.Fatalf("dry run must not mount anything: %v", f.mounts)
	}
}

func TestApplyScenarioPatchesInContainerUnknownBundle(t *testing.T) {
	root := t.TempDir()
	writeScenarioPatch(t, root)
	chdirRoot(t, root)
	newFakeContainer(t)

	if _, err := ApplyScenarioPatchesInContainer("molecule-test", "default", ContainerApplyOptions{Bundle: "nope"}); err == nil {
		t.Fatal("expected error for unknown bundle name")
	}
}

func TestFindRoleDirInContainer(t *testing.T) {
	newFakeContainer(t)

	for _, ref := range []string{"geerlingguy.docker", "docker"} {
		got, err := FindRoleDirInContainer("molecule-test", ref)
		if err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		if got != "geerlingguy.docker" {
			t.Fatalf("%s resolved to %q", ref, got)
		}
	}
	if _, err := FindRoleDirInContainer("molecule-test", "no.such.role"); err == nil {
		t.Fatal("expected error for a role missing from the container")
	}
}

// addContainerRole creates another installed role in the fake container.
func addContainerRole(t *testing.T, f *fakeContainer, dir string) {
	t.Helper()
	p := f.path(config.ContainerRolesCachePath + "/" + dir + "/tasks")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "main.yml"), []byte("---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestFindRoleDirInContainerShortName covers the short-name scan: a single
// namespaced match resolves, several matches are a hard error, and an
// exact directory name wins over competing suffix matches.
func TestFindRoleDirInContainerShortName(t *testing.T) {
	t.Run("ambiguous short name errors", func(t *testing.T) {
		f := newFakeContainer(t) // has geerlingguy.docker
		addContainerRole(t, f, "acme.docker")

		_, err := FindRoleDirInContainer("molecule-test", "docker")
		if err == nil {
			t.Fatal("expected an ambiguity error for a short name matching two roles")
		}
		for _, want := range []string{"ambiguous", "acme.docker", "geerlingguy.docker", "full namespace.role name"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q missing %q", err, want)
			}
		}
		// Ambiguity must not be reported as "not installed".
		if errors.Is(err, ErrRoleNotInContainer) {
			t.Error("ambiguity must not be wrapped as ErrRoleNotInContainer")
		}
		// The fully qualified name still resolves deterministically.
		for _, ref := range []string{"geerlingguy.docker", "acme.docker"} {
			got, err := FindRoleDirInContainer("molecule-test", ref)
			if err != nil || got != ref {
				t.Errorf("%s resolved to %q, %v", ref, got, err)
			}
		}
	})

	t.Run("exact match wins over suffix matches", func(t *testing.T) {
		f := newFakeContainer(t) // geerlingguy.docker
		addContainerRole(t, f, "docker")
		addContainerRole(t, f, "acme.docker")

		got, err := FindRoleDirInContainer("molecule-test", "docker")
		if err != nil {
			t.Fatalf("exact match must not be ambiguous: %v", err)
		}
		if got != "docker" {
			t.Fatalf("resolved to %q, want the exact dir %q", got, "docker")
		}
	})

	t.Run("single namespaced match resolves", func(t *testing.T) {
		newFakeContainer(t)
		got, err := FindRoleDirInContainer("molecule-test", "docker")
		if err != nil {
			t.Fatalf("single match: %v", err)
		}
		if got != "geerlingguy.docker" {
			t.Fatalf("resolved to %q", got)
		}
	})

	t.Run("invalid role name is rejected explicitly", func(t *testing.T) {
		newFakeContainer(t)
		for _, bad := range []string{"ns./etc/passwd", "ns.ro le", "ns.a$b"} {
			_, err := FindRoleDirInContainer("molecule-test", bad)
			if err == nil {
				t.Fatalf("%q: expected rejection", bad)
			}
			if !strings.Contains(err.Error(), "invalid role name") {
				t.Errorf("%q: error %q should say 'invalid role name', not 'not installed'", bad, err)
			}
		}
	})
}

// TestPrepareWorkCopyStepDiagnostics verifies the staging sentinels map a
// `set -e` abort onto the exact failing step.
func TestPrepareWorkCopyStepDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		failAt  string
		wantMsg string
	}{
		{"rm fails", "S1", "removing the previous work/backup copies failed"},
		{"mkdir fails", "S2", "creating the patch directory tree failed"},
		{"copy to work fails", "S3", "copying the role into the work dir failed"},
		{"copy to backup fails", "S4", "copying the role into the backup dir failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeContainer(t)
			f.failStagingAt = tt.failAt

			_, _, _, err := prepareWorkCopy("molecule-test", "default", "geerlingguy.docker")
			if err == nil {
				t.Fatal("expected a staging error")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("error %q does not identify the failing step %q", err, tt.wantMsg)
			}
			if !strings.Contains(err.Error(), "geerlingguy.docker") {
				t.Errorf("error %q should name the role", err)
			}
			// The stderr tail is surfaced, the sentinels are not.
			if !strings.Contains(err.Error(), "No space left on device") {
				t.Errorf("error %q should carry the stderr tail", err)
			}
			for _, s := range []string{"S1", "S2", "S3", "S4"} {
				if strings.Contains(err.Error(), ": "+s) {
					t.Errorf("error %q leaks sentinel %s", err, s)
				}
			}
		})
	}
}

// TestHealStalePatchMountsStuck verifies a mountpoint that resists umount
// is reported as an error and that the work tree is NOT deleted under it.
func TestHealStalePatchMountsStuck(t *testing.T) {
	f := newFakeContainer(t)
	orig := config.ContainerRolesCachePath + "/geerlingguy.docker"
	work := config.ContainerPatchDir + "/default/work/geerlingguy.docker"
	f.mounts[orig] = work
	f.stuckMounts[orig] = true
	if err := os.MkdirAll(f.path(work), 0o755); err != nil {
		t.Fatal(err)
	}

	err := HealStalePatchMounts("molecule-test", "default")
	if err == nil {
		t.Fatal("expected an error for a stuck mountpoint")
	}
	if !strings.Contains(err.Error(), orig) {
		t.Errorf("error %q should name the stuck mountpoint", err)
	}
	// The work tree must survive: removing the source of a live bind
	// mount would leave a mount pointing at a deleted directory.
	if _, statErr := os.Stat(f.path(work)); statErr != nil {
		t.Error("work tree was removed despite the stuck mount")
	}

	// A stale mount must abort the apply path rather than stacking a new
	// overlay on top of the old one.
	root := t.TempDir()
	writeScenarioPatch(t, root)
	chdirRoot(t, root)
	if _, err := ApplyScenarioPatchesInContainer("molecule-test", "default", ContainerApplyOptions{}); err == nil {
		t.Fatal("apply must fail when heal reports a stuck mount")
	}
}

// TestHealStalePatchMountsClean verifies the happy path still unmounts
// everything and clears the work tree.
func TestHealStalePatchMountsClean(t *testing.T) {
	f := newFakeContainer(t)
	orig := config.ContainerRolesCachePath + "/geerlingguy.docker"
	work := config.ContainerPatchDir + "/default/work/geerlingguy.docker"
	f.mounts[orig] = work
	if err := os.MkdirAll(f.path(work), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := HealStalePatchMounts("molecule-test", "default"); err != nil {
		t.Fatalf("clean heal: %v", err)
	}
	if len(f.mounts) != 0 {
		t.Errorf("mounts not cleared: %v", f.mounts)
	}
	if _, err := os.Stat(f.path(config.ContainerPatchDir + "/default")); !os.IsNotExist(err) {
		t.Error("work tree should be removed when nothing is stuck")
	}
	// The roles dir itself (possibly the host cache bind mount) is never
	// a target: only paths strictly below it are unmounted.
	if _, err := os.Stat(f.path(config.ContainerRolesCachePath + "/geerlingguy.docker")); err != nil {
		t.Error("pristine role dir must remain")
	}
}

// TestRestoreContainerPatchesJoinsErrors verifies every overlay is
// attempted and all failures are reported, not just the first.
func TestRestoreContainerPatchesJoinsErrors(t *testing.T) {
	f := newFakeContainer(t)
	a := config.ContainerRolesCachePath + "/a.role"
	b := config.ContainerRolesCachePath + "/b.role"
	c := config.ContainerRolesCachePath + "/c.role"
	for _, m := range []string{a, b, c} {
		f.mounts[m] = "work" + m
	}
	f.stuckMounts[a] = true
	f.stuckMounts[c] = true

	applied := []AppliedPatch{
		{ContainerPath: a}, {ContainerPath: b}, {ContainerPath: c},
	}
	err := RestoreContainerPatches("molecule-test", applied)
	if err == nil {
		t.Fatal("expected errors for the two stuck mounts")
	}
	for _, want := range []string{a, c} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("joined error %q missing %q", err, want)
		}
	}
	// The healthy mount in between must still have been unmounted.
	if _, ok := f.mounts[b]; ok {
		t.Error("restore stopped early: healthy mount b was not unmounted")
	}
}

func TestValidateContainerAndScenario(t *testing.T) {
	if _, err := validateContainer("--rm"); err == nil {
		t.Fatal("expected rejection of option-like container name")
	}
	if _, err := normalizeScenario("../evil"); err == nil {
		t.Fatal("expected rejection of path-like scenario")
	}
	got, err := normalizeScenario("")
	if err != nil || got != config.DefaultScenario {
		t.Fatalf("normalizeScenario(\"\") = %q, %v", got, err)
	}
}

// TestContainerPatchDirNotUnderTmp is a regression test: molecule images
// commonly mount /tmp as tmpfs (baked into the image, outside our control
// over run args), and `docker cp` cannot read files out of a tmpfs mount
// inside a container — it fails with "Could not find the file ... in
// container" even though `cp -a`/`ls`/`docker exec cat` all see the
// files fine. ContainerPatchDir must stay off /tmp (and off any other
// tmpfs mount) so docker cp can stream the staged role out to the host
// for patching and back in again.
func TestContainerPatchDirNotUnderTmp(t *testing.T) {
	if config.ContainerPatchDir == "/tmp" || strings.HasPrefix(config.ContainerPatchDir, "/tmp/") {
		t.Fatalf("ContainerPatchDir = %q must not live under /tmp: docker cp cannot read from a tmpfs mount",
			config.ContainerPatchDir)
	}
}
