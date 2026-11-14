package molecule

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/patch"
	"diffusion/internal/utils"
)

// The seams below mirror the package-level function variables used by
// runIdempotence. Stubs must match these signatures exactly.
type (
	ensurePatchedRolesFunc = func(opts *MoleculeOptions, scenario, roleDirName string) ([]patch.AppliedPatch, func(), error)
	execInteractiveFunc    = func(role, command string, ciMode bool, args ...string) error
)

// stubIdempotenceSeams replaces the idempotence seams for the duration of the
// test and restores the real implementations via t.Cleanup.
//
// Tests using this helper must NOT call t.Parallel: the seams are package
// globals, so parallel subtests would race with each other.
func stubIdempotenceSeams(t *testing.T, ensure ensurePatchedRolesFunc, exec execInteractiveFunc) {
	t.Helper()

	origEnsure := ensurePatchedRolesFn
	origExec := dockerExecInteractiveFn
	t.Cleanup(func() {
		ensurePatchedRolesFn = origEnsure
		dockerExecInteractiveFn = origExec
	})

	ensurePatchedRolesFn = ensure
	dockerExecInteractiveFn = exec
}

// TestRunIdempotence_PatchApplyFails locks in that a patch failure aborts the
// action before molecule runs and before any restore func is armed.
func TestRunIdempotence_PatchApplyFails(t *testing.T) {
	applyErr := errors.New("bundle copy exploded")
	var execCalls, restoreCalls int

	stubIdempotenceSeams(t,
		func(*MoleculeOptions, string, string) ([]patch.AppliedPatch, func(), error) {
			// Production returns a no-op restore on failure; a counting
			// restore proves the caller does not invoke it either way.
			return nil, func() { restoreCalls++ }, applyErr
		},
		func(string, string, bool, ...string) error {
			execCalls++
			return nil
		},
	)

	err := runIdempotence(&MoleculeOptions{RoleFlag: "web"}, config.DefaultScenario, "acme.web")

	if err == nil {
		t.Fatal("runIdempotence() = nil, want error when patch apply fails")
	}
	if !errors.Is(err, applyErr) {
		t.Errorf("runIdempotence() error = %v, want it to wrap %v", err, applyErr)
	}
	if !strings.Contains(err.Error(), "patch apply failed") {
		t.Errorf("runIdempotence() error = %q, want it to contain %q", err.Error(), "patch apply failed")
	}
	if execCalls != 0 {
		t.Errorf("molecule exec called %d times, want 0 (must not run unpatched)", execCalls)
	}
	if restoreCalls != 0 {
		t.Errorf("restore called %d times, want 0 on patch failure", restoreCalls)
	}
}

// TestRunIdempotence_NoPatches locks in the happy path when there is no
// patch.yml: molecule runs once, the no-op restore still runs, error is nil.
func TestRunIdempotence_NoPatches(t *testing.T) {
	var execCalls, restoreCalls int

	stubIdempotenceSeams(t,
		func(*MoleculeOptions, string, string) ([]patch.AppliedPatch, func(), error) {
			return nil, func() { restoreCalls++ }, nil
		},
		func(string, string, bool, ...string) error {
			execCalls++
			return nil
		},
	)

	err := runIdempotence(&MoleculeOptions{RoleFlag: "web"}, config.DefaultScenario, "acme.web")
	if err != nil {
		t.Fatalf("runIdempotence() = %v, want nil", err)
	}
	if execCalls != 1 {
		t.Errorf("molecule exec called %d times, want 1", execCalls)
	}
	if restoreCalls != 1 {
		t.Errorf("restore called %d times, want 1 (deferred no-op must still run)", restoreCalls)
	}
}

// TestRunIdempotence_PatchesApplied_IdempotenceSucceeds locks in that the
// overlay is restored exactly once, after molecule idempotence finished.
func TestRunIdempotence_PatchesApplied_IdempotenceSucceeds(t *testing.T) {
	var calls []string

	stubIdempotenceSeams(t,
		func(*MoleculeOptions, string, string) ([]patch.AppliedPatch, func(), error) {
			applied := []patch.AppliedPatch{{Bundle: "local", Role: "common", Scenario: "default"}}
			return applied, func() { calls = append(calls, "restore") }, nil
		},
		func(string, string, bool, ...string) error {
			calls = append(calls, "exec")
			return nil
		},
	)

	err := runIdempotence(&MoleculeOptions{RoleFlag: "web"}, config.DefaultScenario, "acme.web")
	if err != nil {
		t.Fatalf("runIdempotence() = %v, want nil", err)
	}

	want := []string{"exec", "restore"}
	if len(calls) != len(want) {
		t.Fatalf("call order = %v, want %v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("call order = %v, want %v", calls, want)
		}
	}
}

// TestRunIdempotence_PatchesApplied_IdempotenceFails locks in that a molecule
// failure is wrapped and that the overlay is still removed, after exit.
func TestRunIdempotence_PatchesApplied_IdempotenceFails(t *testing.T) {
	execErr := errors.New("molecule idempotence exited 1")
	var calls []string

	stubIdempotenceSeams(t,
		func(*MoleculeOptions, string, string) ([]patch.AppliedPatch, func(), error) {
			applied := []patch.AppliedPatch{{Bundle: "local", Role: "common", Scenario: "default"}}
			return applied, func() { calls = append(calls, "restore") }, nil
		},
		func(string, string, bool, ...string) error {
			calls = append(calls, "exec")
			return execErr
		},
	)

	err := runIdempotence(&MoleculeOptions{RoleFlag: "web"}, config.DefaultScenario, "acme.web")

	if err == nil {
		t.Fatal("runIdempotence() = nil, want error when molecule fails")
	}
	if !errors.Is(err, execErr) {
		t.Errorf("runIdempotence() error = %v, want it to wrap %v", err, execErr)
	}
	if !strings.Contains(err.Error(), "idempotence failed") {
		t.Errorf("runIdempotence() error = %q, want it to contain %q", err.Error(), "idempotence failed")
	}

	want := []string{"exec", "restore"}
	if len(calls) != len(want) {
		t.Fatalf("call order = %v, want %v (restore must still happen on failure)", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("call order = %v, want %v", calls, want)
		}
	}
}

// TestRunIdempotence_ArgumentsPassedThrough locks in the wiring between the
// caller, the patch helper and docker exec: the scenario/roleDirName reach
// ensurePatchedRolesFn unchanged and the shell command keeps its exact shape.
func TestRunIdempotence_ArgumentsPassedThrough(t *testing.T) {
	const (
		orgFlag     = "acme"
		roleFlag    = "web"
		roleDirName = orgFlag + "." + roleFlag
	)

	tests := []struct {
		name       string
		opts       *MoleculeOptions
		scenario   string // scenario argument as supplied by handleSubcommands
		wantCmd    string
		wantCIExec bool
	}{
		{
			name:     "default scenario without tags",
			opts:     &MoleculeOptions{RoleFlag: roleFlag, OrgFlag: orgFlag},
			scenario: config.DefaultScenario,
			wantCmd:  "cd ./acme.web && molecule idempotence",
		},
		{
			name:     "tags become ANSIBLE_RUN_TAGS prefix",
			opts:     &MoleculeOptions{RoleFlag: roleFlag, OrgFlag: orgFlag, TagFlag: "install,config"},
			scenario: config.DefaultScenario,
			wantCmd:  "cd ./acme.web && ANSIBLE_RUN_TAGS=install,config molecule idempotence",
		},
		{
			name:     "non-default scenario appends -s flag",
			opts:     &MoleculeOptions{RoleFlag: roleFlag, OrgFlag: orgFlag, RoleScenario: "production"},
			scenario: "production",
			wantCmd:  "cd ./acme.web && molecule idempotence -s production",
		},
		{
			name:     "tags and non-default scenario combine",
			opts:     &MoleculeOptions{RoleFlag: roleFlag, OrgFlag: orgFlag, TagFlag: "install", RoleScenario: "production"},
			scenario: "production",
			wantCmd:  "cd ./acme.web && ANSIBLE_RUN_TAGS=install molecule idempotence -s production",
		},
		{
			name:       "CI mode is forwarded to exec",
			opts:       &MoleculeOptions{RoleFlag: roleFlag, OrgFlag: orgFlag, CIMode: true},
			scenario:   config.DefaultScenario,
			wantCmd:    "cd ./acme.web && molecule idempotence",
			wantCIExec: true,
		},
		{
			name:     "explicit default scenario is not forwarded as -s",
			opts:     &MoleculeOptions{RoleFlag: roleFlag, OrgFlag: orgFlag, RoleScenario: config.DefaultScenario},
			scenario: config.DefaultScenario,
			wantCmd:  "cd ./acme.web && molecule idempotence",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				gotScenario string
				gotRoleDir  string
				execCalls   int
				gotRole     string
				gotCommand  string
				gotCIMode   bool
				gotArgs     []string
			)

			stubIdempotenceSeams(t,
				func(_ *MoleculeOptions, scenario, roleDir string) ([]patch.AppliedPatch, func(), error) {
					gotScenario = scenario
					gotRoleDir = roleDir
					return nil, func() {}, nil
				},
				func(role, command string, ciMode bool, args ...string) error {
					execCalls++
					gotRole = role
					gotCommand = command
					gotCIMode = ciMode
					gotArgs = append([]string(nil), args...)
					return nil
				},
			)

			if err := runIdempotence(tt.opts, tt.scenario, roleDirName); err != nil {
				t.Fatalf("runIdempotence() = %v, want nil", err)
			}

			if gotScenario != tt.scenario {
				t.Errorf("ensure scenario = %q, want %q", gotScenario, tt.scenario)
			}
			if gotRoleDir != roleDirName {
				t.Errorf("ensure roleDirName = %q, want %q", gotRoleDir, roleDirName)
			}

			if execCalls != 1 {
				t.Fatalf("molecule exec called %d times, want 1", execCalls)
			}
			if gotRole != roleFlag {
				t.Errorf("exec role = %q, want %q", gotRole, roleFlag)
			}
			if gotCommand != "/bin/sh" {
				t.Errorf("exec command = %q, want %q", gotCommand, "/bin/sh")
			}
			if gotCIMode != tt.wantCIExec {
				t.Errorf("exec CIMode = %v, want %v", gotCIMode, tt.wantCIExec)
			}
			if len(gotArgs) != 2 {
				t.Fatalf("exec args = %v, want 2 args (-c, cmd)", gotArgs)
			}
			if gotArgs[0] != "-c" {
				t.Errorf("exec args[0] = %q, want %q", gotArgs[0], "-c")
			}
			if gotArgs[1] != tt.wantCmd {
				t.Errorf("exec args[1] = %q, want %q", gotArgs[1], tt.wantCmd)
			}
		})
	}
}

// TestHandleSubcommands_RoutesIdempotence locks in that the idempotence flag
// reaches runIdempotence with the resolved scenario (RoleScenario, defaulting
// to config.DefaultScenario) and the composed role directory name.
//
// CIMode is set so handleSubcommands skips CopyRoleData and the docker shell
// probe; the only reachable side effect is creating molecule/<scenario>/tests
// relative to CWD, isolated with t.Chdir.
func TestHandleSubcommands_RoutesIdempotence(t *testing.T) {
	tests := []struct {
		name         string
		roleScenario string
		tagFlag      string
		wantScenario string
		wantCmd      string
	}{
		{
			name:         "falls back to default scenario",
			roleScenario: "",
			wantScenario: config.DefaultScenario,
			wantCmd:      "cd ./acme.web && molecule idempotence",
		},
		{
			name:         "explicit default scenario",
			roleScenario: config.DefaultScenario,
			wantScenario: config.DefaultScenario,
			wantCmd:      "cd ./acme.web && molecule idempotence",
		},
		{
			name:         "custom scenario without tags",
			roleScenario: "production",
			wantScenario: "production",
			wantCmd:      "cd ./acme.web && molecule idempotence -s production",
		},
		{
			name:         "custom scenario with tags",
			roleScenario: "production",
			tagFlag:      "install",
			wantScenario: "production",
			wantCmd:      "cd ./acme.web && ANSIBLE_RUN_TAGS=install molecule idempotence -s production",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// handleSubcommands scaffolds molecule/<role>/tests in CWD.
			t.Chdir(t.TempDir())

			var (
				ensureCalls    int
				gotScenario    string
				gotRoleDirName string
				gotCmd         string
			)

			stubIdempotenceSeams(t,
				func(_ *MoleculeOptions, scenario, roleDirName string) ([]patch.AppliedPatch, func(), error) {
					ensureCalls++
					gotScenario = scenario
					gotRoleDirName = roleDirName
					return nil, func() {}, nil
				},
				func(_ string, _ string, _ bool, args ...string) error {
					if len(args) == 2 {
						gotCmd = args[1]
					}
					return nil
				},
			)

			opts := &MoleculeOptions{
				RoleFlag:        "web",
				OrgFlag:         "acme",
				RoleScenario:    tt.roleScenario,
				TagFlag:         tt.tagFlag,
				IdempotenceFlag: true,
				CIMode:          true,
			}
			path := t.TempDir()
			roleDirName := utils.GetRoleDirName(opts.OrgFlag, opts.RoleFlag)
			roleMoleculePath := filepath.Join(path, config.MoleculeDir, roleDirName)

			if err := handleSubcommands(opts, &config.Config{}, path, roleDirName, roleMoleculePath); err != nil {
				t.Fatalf("handleSubcommands() = %v, want nil", err)
			}

			if ensureCalls != 1 {
				t.Fatalf("ensurePatchedRolesFn called %d times, want 1 (idempotence not routed)", ensureCalls)
			}
			if gotScenario != tt.wantScenario {
				t.Errorf("scenario = %q, want %q", gotScenario, tt.wantScenario)
			}
			if gotRoleDirName != roleDirName {
				t.Errorf("roleDirName = %q, want %q", gotRoleDirName, roleDirName)
			}
			if gotCmd != tt.wantCmd {
				t.Errorf("molecule command = %q, want %q", gotCmd, tt.wantCmd)
			}
		})
	}
}
