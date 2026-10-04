package patch

// container.go implements the "bind-mount overlay" patching model.
//
// Patching never mutates anything on the host: the role is copied inside
// the running molecule container into
//
//	<ContainerPatchDir>/<scenario>/work/<role>    patched content
//	<ContainerPatchDir>/<scenario>/backup/<role>  pristine copy (audit)
//
// the work copy is streamed out to a throwaway host temp dir, patched by
// the path-agnostic ApplyBundle, streamed back in, and finally bind-mounted
// over the original <ContainerRolesCachePath>/<role>. Because
// /root/.ansible/roles may itself be a host cache bind mount, we only ever
// read from it — the overlay is removed again by RestoreContainerPatches.
//
// ContainerPatchDir deliberately lives under /var/lib, not /tmp: molecule
// images commonly mount /tmp as tmpfs, and `docker cp` cannot read files
// out of a tmpfs mount (see config.ContainerPatchDir's doc comment).

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/utils"
)

// ErrRoleNotInContainer is returned when a bundle's role is not installed
// under ContainerRolesCachePath inside the molecule container.
var ErrRoleNotInContainer = errors.New("role not installed in container")

// safeContainerName matches names we are willing to interpolate into a
// container-side shell command (role dirs, scenarios, container names).
var safeContainerName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// dockerExec runs `docker exec <container> <args...>` and returns the
// combined output. It is a variable so tests can stub docker away.
var dockerExec = func(container string, args ...string) ([]byte, error) {
	full := append([]string{"exec", container}, args...)
	return exec.Command("docker", full...).CombinedOutput()
}

// dockerCp runs `docker cp <src> <dst>`. Variable for testability.
var dockerCp = func(src, dst string) error {
	out, err := exec.Command("docker", "cp", src, dst).CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker cp %s %s: %v: %s", src, dst, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// dockerPS lists running container names. Variable for testability.
var dockerPS = func() ([]byte, error) {
	return exec.Command("docker", "ps", "--format", "{{.Names}}").Output()
}

// AppliedPatch describes one bundle applied inside a molecule container:
// which bundle/role was patched and the three container-side paths of the
// overlay. Path fields are container paths, never host paths.
type AppliedPatch struct {
	Bundle   string
	Role     string
	Scenario string
	// Container is the molecule container the overlay lives in.
	Container string
	// ContainerPath is the original role dir the overlay is mounted over
	// (<ContainerRolesCachePath>/<role>).
	ContainerPath string
	// WorkPath holds the patched content that is bind-mounted.
	WorkPath string
	// BackupPath holds the pristine copy kept for audit/fallback.
	BackupPath string
	// Result is the detailed ApplyBundle report (nil on failure).
	Result *ApplyResult
}

// ContainerApplyOptions tunes ApplyScenarioPatchesInContainer.
type ContainerApplyOptions struct {
	// Bundle restricts the run to one bundle name (empty: all bundles).
	Bundle string
	// DryRun analyzes and reports without writing back or bind-mounting.
	DryRun bool
	// Force applies despite stale-analysis drift.
	Force bool
}

// MoleculeContainerName returns the molecule container name for a role.
func MoleculeContainerName(role string) string {
	return config.MoleculeContainerPrefix + strings.TrimSpace(role)
}

// FindMoleculeContainer returns the first running molecule-* container.
func FindMoleculeContainer() (string, error) {
	out, err := dockerPS()
	if err != nil {
		return "", fmt.Errorf("docker is unavailable: %w", err)
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if name := strings.TrimSpace(line); strings.HasPrefix(name, config.MoleculeContainerPrefix) {
			return name, nil
		}
	}
	return "", fmt.Errorf("no running %s* container found (run 'diffusion molecule --converge' first)",
		config.MoleculeContainerPrefix)
}

// normalizeScenario trims, defaults and validates a scenario name.
func normalizeScenario(scenario string) (string, error) {
	scenario = strings.TrimSpace(scenario)
	if scenario == "" {
		scenario = config.DefaultScenario
	}
	if err := utils.ValidateCLIArgument("scenario", scenario); err != nil {
		return "", err
	}
	if !safeContainerName.MatchString(scenario) {
		return "", fmt.Errorf("invalid scenario %q: must be a plain scenario name", scenario)
	}
	return scenario, nil
}

// validateContainer checks a container name before shelling out to docker.
func validateContainer(container string) (string, error) {
	container = strings.TrimSpace(container)
	if container == "" {
		return "", fmt.Errorf("container name is required")
	}
	if err := utils.ValidateCLIArgument("container", container); err != nil {
		return "", err
	}
	if !safeContainerName.MatchString(container) {
		return "", fmt.Errorf("invalid container name %q", container)
	}
	return container, nil
}

// patchPaths returns the original, work and backup container paths of a
// role directory name for one scenario.
func patchPaths(scenario, roleDir string) (orig, work, backup string) {
	base := config.ContainerPatchDir + "/" + scenario
	return config.ContainerRolesCachePath + "/" + roleDir,
		base + "/work/" + roleDir,
		base + "/backup/" + roleDir
}

// FindRoleDirInContainer returns the directory name of roleName under
// ContainerRolesCachePath inside container. It tries the full
// "namespace.name" form, then the short name, and finally a directory scan
// matching "<namespace>.<short>".
//
// The scan collects every candidate rather than taking the first hit of an
// unordered listing: an exact directory match wins outright, but when a
// short name matches several namespaced installs (e.g. "docker" matching
// both "geerlingguy.docker" and "acme.docker") it returns an error asking
// for the fully qualified name instead of silently patching an arbitrary
// role.
func FindRoleDirInContainer(container, roleName string) (string, error) {
	container, err := validateContainer(container)
	if err != nil {
		return "", err
	}
	if err := utils.ValidateCLIArgument("role", roleName); err != nil {
		return "", err
	}
	variants := roleDirVariants(roleName)
	if len(variants) == 0 {
		return "", fmt.Errorf("role name cannot be empty")
	}
	for _, v := range variants {
		if !safeContainerName.MatchString(v) {
			return "", fmt.Errorf("invalid role name %q: %q is not a valid role directory name", roleName, v)
		}
	}
	for _, v := range variants {
		if _, err := dockerExec(container, "test", "-d", config.ContainerRolesCachePath+"/"+v); err == nil {
			return v, nil
		}
	}

	// Short-name scan: "docker" also matches "geerlingguy.docker".
	short := roleName
	if i := strings.LastIndex(roleName, "."); i >= 0 {
		short = roleName[i+1:]
	}
	if !safeContainerName.MatchString(short) {
		return "", fmt.Errorf("invalid role name %q: short name %q is not a valid role directory name", roleName, short)
	}
	out, err := dockerExec(container, "ls", "-1", config.ContainerRolesCachePath)
	if err == nil {
		var matches []string
		for line := range strings.SplitSeq(string(out), "\n") {
			name := strings.TrimSpace(line)
			if name == "" || !safeContainerName.MatchString(name) {
				continue
			}
			if name != short && !strings.HasSuffix(name, "."+short) {
				continue
			}
			if _, err := dockerExec(container, "test", "-d", config.ContainerRolesCachePath+"/"+name+"/tasks"); err != nil {
				continue
			}
			// An exact directory match is unambiguous by construction:
			// at most one entry can carry that name.
			if name == short {
				return name, nil
			}
			matches = append(matches, name)
		}
		sort.Strings(matches)
		if len(matches) > 1 {
			return "", fmt.Errorf("role %q is ambiguous in container %s: matches %v — use the full namespace.role name in patch.yml",
				roleName, container, matches)
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
	}
	return "", fmt.Errorf("%w: role %q is not installed in container %s under %s; run 'diffusion molecule --converge' once or check scenarios/<scenario>/requirements.yml",
		ErrRoleNotInContainer, roleName, container, config.ContainerRolesCachePath)
}

// HealStalePatchMounts removes leftover overlay bind mounts and the patch
// work tree of a scenario inside container. It is safe to call when
// nothing is mounted (interrupted runs, --destroy, --wipe).
//
// A mountpoint that resists both umount and umount -l is reported as
// "FAILED:<path>" by the script and turned into an error here; in that
// case the work tree is deliberately NOT removed, because deleting the
// source of a live bind mount would leave the container with a mount
// pointing at a deleted directory. Callers that are about to stack a new
// overlay (ApplyScenarioPatchesInContainer) must treat this as fatal.
func HealStalePatchMounts(container, scenario string) error {
	container, err := validateContainer(container)
	if err != nil {
		return err
	}
	scenario, err = normalizeScenario(scenario)
	if err != nil {
		return err
	}
	// Only unmount entries *below* the roles dir: the roles dir itself may
	// be the host cache bind mount and must stay in place. Deepest paths
	// are unmounted first (sort -r) so nested overlays come apart cleanly.
	script := fmt.Sprintf(
		`failed=''; `+
			`for m in $(awk '{print $2}' /proc/self/mounts | grep '^%s/' | sort -r); do `+
			`umount "$m" 2>/dev/null || umount -l "$m" 2>/dev/null || { failed="$failed $m"; echo "FAILED:$m"; }; `+
			`done; `+
			`if [ -z "$failed" ]; then rm -rf %s/%s; fi`,
		config.ContainerRolesCachePath, config.ContainerPatchDir, scenario)
	out, err := dockerExec(container, "sh", "-c", script)
	if err != nil {
		return fmt.Errorf("failed to clean stale patch mounts in %s: %v: %s",
			container, err, strings.TrimSpace(string(out)))
	}
	if stuck := parseStuckMounts(out); len(stuck) > 0 {
		return fmt.Errorf("stale patch overlay still mounted in container %s: %s (a process may be holding it; restart the container or run 'diffusion molecule --wipe')",
			container, strings.Join(stuck, ", "))
	}
	return nil
}

// parseStuckMounts extracts the mountpoints the heal script could not
// unmount from its output ("FAILED:<path>" lines).
func parseStuckMounts(out []byte) []string {
	var stuck []string
	for line := range strings.SplitSeq(string(out), "\n") {
		if m := strings.TrimSpace(line); strings.HasPrefix(m, "FAILED:") {
			if p := strings.TrimSpace(strings.TrimPrefix(m, "FAILED:")); p != "" {
				stuck = append(stuck, p)
			}
		}
	}
	return stuck
}

// unmountOverlay removes a single overlay bind mount (best effort).
func unmountOverlay(container, orig string) error {
	script := fmt.Sprintf(
		`if grep -q ' %s ' /proc/self/mounts; then umount %s 2>/dev/null || umount -l %s; fi`,
		orig, orig, orig)
	if out, err := dockerExec(container, "sh", "-c", script); err != nil {
		return fmt.Errorf("failed to unmount %s: %v: %s", orig, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// stagingSteps maps the sentinel emitted after each prepareWorkCopy step
// to a description of the step that comes AFTER it. The staging script
// echoes S1..S4 as it progresses, so the last sentinel seen tells us which
// step was running when `set -e` aborted.
var stagingSteps = []struct{ sentinel, failed string }{
	{"", "removing the previous work/backup copies failed"},
	{"S1", "creating the patch directory tree failed"},
	{"S2", "copying the role into the work dir failed"},
	{"S3", "copying the role into the backup dir failed"},
}

// stagingFailure returns a description of the staging step that failed,
// derived from the last sentinel present in the script output.
func stagingFailure(out []byte) string {
	seen := ""
	for line := range strings.SplitSeq(string(out), "\n") {
		switch strings.TrimSpace(line) {
		case "S1", "S2", "S3", "S4":
			seen = strings.TrimSpace(line)
		}
	}
	if seen == "S4" {
		// All steps reported success: the failure is elsewhere.
		return "staging reported success but the command failed"
	}
	for _, s := range stagingSteps {
		if s.sentinel == seen {
			return s.failed
		}
	}
	return "staging failed"
}

// stripSentinels removes the S1..S4 progress markers so only real
// diagnostics (stderr) are shown to the user.
func stripSentinels(out []byte) string {
	var keep []string
	for line := range strings.SplitSeq(string(out), "\n") {
		switch strings.TrimSpace(line) {
		case "S1", "S2", "S3", "S4", "":
		default:
			keep = append(keep, strings.TrimRight(line, "\r"))
		}
	}
	// Only the tail is useful; shell errors are emitted last.
	if len(keep) > 5 {
		keep = keep[len(keep)-5:]
	}
	return strings.Join(keep, "; ")
}

// prepareWorkCopy unmounts a stale overlay and refreshes the work and
// backup copies of roleDir from the pristine original.
//
// The script emits a sentinel after every step so a `set -e` abort can be
// attributed to the exact failing operation (out-of-space on cp, a
// read-only mount, a missing source dir, ...) instead of a bare exit code.
func prepareWorkCopy(container, scenario, roleDir string) (orig, work, backup string, err error) {
	orig, work, backup = patchPaths(scenario, roleDir)
	if err = unmountOverlay(container, orig); err != nil {
		return "", "", "", err
	}
	script := fmt.Sprintf(
		`set -e; `+
			`rm -rf %s %s; echo S1; `+
			`mkdir -p %s/%s/work %s/%s/backup; echo S2; `+
			`cp -a %s %s; echo S3; `+
			`cp -a %s %s; echo S4`,
		work, backup,
		config.ContainerPatchDir, scenario, config.ContainerPatchDir, scenario,
		orig, work,
		orig, backup)
	out, execErr := dockerExec(container, "sh", "-c", script)
	if execErr != nil {
		msg := fmt.Sprintf("staging role %s in container %s: %s", roleDir, container, stagingFailure(out))
		if detail := stripSentinels(out); detail != "" {
			return "", "", "", fmt.Errorf("%s: %v: %s", msg, execErr, detail)
		}
		return "", "", "", fmt.Errorf("%s: %w", msg, execErr)
	}
	return orig, work, backup, nil
}

// ScenarioHasBundles reports whether scenarios/<scenario>/patch.yml exists
// and declares at least one bundle. Callers use it to skip all docker work
// when a project does not use patches.
func ScenarioHasBundles(scenario string) bool {
	cfg, err := loadScenarioBundles(scenario)
	return err == nil && cfg != nil && len(cfg.Bundles) > 0
}

// loadScenarioBundles loads scenarios/<scenario>/patch.yml. It returns
// (nil, nil) when the file does not exist so callers can no-op.
func loadScenarioBundles(scenario string) (*PatchesConfig, error) {
	scenario, err := normalizeScenario(scenario)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(config.ScenariosDir, scenario, "patch.yml")); os.IsNotExist(err) {
		return nil, nil
	}
	pc := &PatchesConfig{}
	return pc.LoadPatchConfigFrom(scenario)
}

// ApplyScenarioPatchesInContainer applies every bundle of
// scenarios/<scenario>/patch.yml inside the running molecule container.
//
// It is a no-op returning (nil, nil) when no patch.yml exists, so molecule
// runs can call it unconditionally. Nothing on the host is modified: the
// only host artifact is a temp dir that is removed before returning.
// Callers MUST pass the result to RestoreContainerPatches once the
// molecule step finished so the bind-mount overlay is removed again.
func ApplyScenarioPatchesInContainer(container, scenario string, opts ContainerApplyOptions) ([]AppliedPatch, error) {
	container, err := validateContainer(container)
	if err != nil {
		return nil, err
	}
	scenario, err = normalizeScenario(scenario)
	if err != nil {
		return nil, err
	}
	cfg, err := loadScenarioBundles(scenario)
	if err != nil {
		return nil, err
	}
	if cfg == nil || len(cfg.Bundles) == 0 {
		return nil, nil
	}

	wanted := strings.TrimSpace(opts.Bundle)
	found := false
	if !opts.DryRun {
		// Self-heal first: an interrupted previous run may have left an
		// overlay mounted over the pristine roles path.
		if err := HealStalePatchMounts(container, scenario); err != nil {
			return nil, err
		}
	}

	var applied []AppliedPatch
	for i := range cfg.Bundles {
		b := &cfg.Bundles[i]
		if wanted != "" && b.PatchBundleName != wanted {
			continue
		}
		found = true
		ap, err := applyBundleInContainer(container, scenario, b, opts)
		if err != nil {
			if !opts.DryRun {
				_ = RestoreContainerPatches(container, applied)
			}
			return nil, fmt.Errorf("bundle %q: %w", b.PatchBundleName, err)
		}
		applied = append(applied, ap)
	}
	if wanted != "" && !found {
		return nil, fmt.Errorf("bundle %q not found in scenario %s", wanted, scenario)
	}
	return applied, nil
}

// applyBundleInContainer stages, patches and overlays one bundle.
func applyBundleInContainer(container, scenario string, b *PatchBundle, opts ContainerApplyOptions) (AppliedPatch, error) {
	var ap AppliedPatch
	roleDir, err := FindRoleDirInContainer(container, b.RoleName)
	if err != nil {
		return ap, err
	}
	orig, work, backup, err := prepareWorkCopy(container, scenario, roleDir)
	if err != nil {
		return ap, err
	}

	hostTmp, err := os.MkdirTemp("", "diffusion-patch-")
	if err != nil {
		return ap, fmt.Errorf("failed to create temp dir: %w", err)
	}
	// The host temp dir is strictly transient — never the cache tree.
	defer func() { _ = os.RemoveAll(hostTmp) }()

	hostRole := filepath.Join(hostTmp, roleDir)
	if err := os.MkdirAll(hostRole, 0o755); err != nil {
		return ap, fmt.Errorf("failed to create temp role dir: %w", err)
	}
	if err := dockerCp(container+":"+work+"/.", hostRole); err != nil {
		return ap, fmt.Errorf("failed to copy role %s out of container: %w", roleDir, err)
	}

	analysis, err := AnalyzeRoleAtPath(hostRole, scenario, b.RoleName)
	if err != nil {
		return ap, fmt.Errorf("analyze %s: %w", roleDir, err)
	}
	res, err := ApplyBundle(b, analysis, ApplyOptions{
		Scenario:  scenario,
		RolePath:  hostRole,
		DryRun:    opts.DryRun,
		Force:     opts.Force,
		BackupDir: filepath.Join(hostTmp, "backup"),
	})
	if err != nil {
		return ap, err
	}

	ap = AppliedPatch{
		Bundle:        b.PatchBundleName,
		Role:          b.RoleName,
		Container:     container,
		Scenario:      scenario,
		ContainerPath: orig,
		WorkPath:      work,
		BackupPath:    backup,
		Result:        res,
	}
	if opts.DryRun {
		return ap, nil
	}

	if err := dockerCp(hostRole+string(os.PathSeparator)+".", container+":"+work); err != nil {
		return ap, fmt.Errorf("failed to copy patched role %s into container: %w", roleDir, err)
	}
	if out, err := dockerExec(container, "mount", "--bind", work, orig); err != nil {
		return ap, fmt.Errorf("failed to bind-mount patched role over %s: %v: %s",
			orig, err, strings.TrimSpace(string(out)))
	}
	log.Printf(config.ColorGreen+"patch: applied bundle %q to %s:%s"+config.ColorReset,
		b.PatchBundleName, container, orig)
	return ap, nil
}

// RestoreContainerPatches removes the bind-mount overlays created by
// ApplyScenarioPatchesInContainer, exposing the pristine roles again. It
// is safe to call with a nil/empty slice. Every overlay is attempted even
// if an earlier one fails; all failures are joined into the returned
// error so no stuck mount goes unreported.
func RestoreContainerPatches(container string, applied []AppliedPatch) error {
	if len(applied) == 0 {
		return nil
	}
	container, err := validateContainer(container)
	if err != nil {
		return err
	}
	var errs []error
	for _, ap := range applied {
		if strings.TrimSpace(ap.ContainerPath) == "" {
			continue
		}
		if err := unmountOverlay(container, ap.ContainerPath); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// CopyRoleFromContainer copies an installed role out of a running
// container into a temp dir for read-only host-side analysis. It never
// installs anything and never writes into the container. It returns the
// host path and a cleanup func removing it.
func CopyRoleFromContainer(container, roleName string) (string, func(), error) {
	noop := func() {}
	roleDir, err := FindRoleDirInContainer(container, roleName)
	if err != nil {
		return "", noop, err
	}
	tmp, err := os.MkdirTemp("", "diffusion-patch-role-")
	if err != nil {
		return "", noop, fmt.Errorf("failed to create temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }
	dst := filepath.Join(tmp, "role")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		cleanup()
		return "", noop, fmt.Errorf("failed to create temp role dir: %w", err)
	}
	if err := dockerCp(container+":"+config.ContainerRolesCachePath+"/"+roleDir+"/.", dst); err != nil {
		cleanup()
		return "", noop, fmt.Errorf("failed to copy role %q from container %s: %w", roleName, container, err)
	}
	return dst, cleanup, nil
}
