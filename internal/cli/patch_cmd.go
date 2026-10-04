package cli

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/patch"

	"github.com/spf13/cobra"
)

// NewPatchCmd creates the patch command with subcommands
func NewPatchCmd(_ *CLI) *cobra.Command {
	patchCmd := &cobra.Command{
		Use:   "patch",
		Short: "Inspect and patch external Ansible roles (ephemeral, per scenario)",
		Long: `Inspect external role task trees and apply patch bundles.

Patch bundles live in scenarios/<scenario>/patch.yml with overlay sources
under scenarios/<scenario>/patch/files|templates/. Patching happens inside
the running molecule container: the role is copied to a container-local
work dir and bind-mounted over the original roles path, so neither the
installed source of truth nor the host cache is ever mutated.`,
	}

	patchCmd.AddCommand(newPatchAnalyzeCmd())
	patchCmd.AddCommand(newPatchListCmd())
	patchCmd.AddCommand(newPatchCheckCmd())
	patchCmd.AddCommand(newPatchDiffCmd())
	patchCmd.AddCommand(newPatchApplyCmd())

	return patchCmd
}

// loadPatchBundles loads scenarios/<scenario>/patch.yml.
func loadPatchBundles(scenario string) (*patch.PatchesConfig, error) {
	pc := &patch.PatchesConfig{}
	return pc.LoadPatchConfigFrom(scenario)
}

// overlayAnnotations builds leaf-ID -> overlay display path entries from
// bundles targeting role (best-effort, for tree rendering).
func overlayAnnotations(cfg *patch.PatchesConfig, role string) map[string]string {
	out := map[string]string{}
	if cfg == nil {
		return out
	}
	for i := range cfg.Bundles {
		b := &cfg.Bundles[i]
		if !b.MatchesRole(role) {
			continue
		}
		for j := range b.TasksToPatch {
			pt := &b.TasksToPatch[j]
			if v := strings.TrimSpace(pt.NewFileSrc); v != "" {
				out[strings.TrimSpace(pt.TaskId)] = "patch/files/" + v
			}
			if v := strings.TrimSpace(pt.NewTemplateSrc); v != "" {
				out[strings.TrimSpace(pt.TaskId)] = "patch/templates/" + v
			}
		}
	}
	return out
}

func newPatchAnalyzeCmd() *cobra.Command {
	var scenario, format string
	var noColor bool
	var tags []string

	cmd := &cobra.Command{
		Use:   "analyze [role]",
		Short: "Print the task/branch tree of an installed external role",
		Long: `Analyze an installed external role and print its task tree with
dotted leaf IDs (branches are visual-only, leaves are patchable).

The role is resolved on this system (diffusion cache, molecule working
copies, Galaxy roles path). When missing, a read-only copy is pulled out
of the running molecule container and analyzed from a temporary copy.

EXAMPLES
  diffusion patch analyze geerlingguy.docker
  diffusion patch analyze geerlingguy.docker -s production --format yaml
  diffusion patch analyze docker --tag install --no-color`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			role := args[0]
			analysis, err := patch.AnalyzeExternalRole(scenario, role)
			if err != nil {
				return err
			}
			if format == "yaml" {
				data, err := analysis.ToYAML()
				if err != nil {
					return err
				}
				fmt.Print(string(data))
				return nil
			}
			if format != "tree" {
				return fmt.Errorf("unknown format %q (want tree|yaml)", format)
			}
			overlays := map[string]string{}
			if cfg, err := loadPatchBundles(scenario); err == nil {
				overlays = overlayAnnotations(cfg, role)
			}
			analysis.PrintTree(os.Stdout, patch.TreeOptions{
				FilterTags: tags,
				NoColor:    noColor,
				Overlays:   overlays,
			})
			return nil
		},
	}

	cmd.Flags().StringVarP(&scenario, "scenario", "s", config.DefaultScenario, "Molecule scenario to operate on")
	cmd.Flags().StringVar(&format, "format", "tree", "Output format: tree|yaml")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable ANSI colors")
	cmd.Flags().StringSliceVar(&tags, "tag", nil, "Highlight tasks with these tags (repeatable or comma-separated)")

	return cmd
}

// describePatchTask summarizes one patching task for list output.
func describePatchTask(pt *patch.PatchingTask) string {
	var parts []string
	if v := strings.TrimSpace(pt.NewModule); v != "" {
		parts = append(parts, "module->"+v)
	}
	if len(pt.NewModuleSetup) > 0 {
		parts = append(parts, fmt.Sprintf("args(%d)", len(pt.NewModuleSetup)))
	}
	if v := strings.TrimSpace(pt.NewFileSrc); v != "" {
		parts = append(parts, "file<="+v)
	}
	if v := strings.TrimSpace(pt.NewTemplateSrc); v != "" {
		parts = append(parts, "template<="+v)
	}
	if len(pt.NewConditions) > 0 {
		counts := map[string]int{}
		var order []string
		for _, c := range pt.NewConditions {
			key := strings.ToLower(strings.TrimSpace(c.Condition))
			if key == "" {
				key = "when"
			}
			if _, ok := counts[key]; !ok {
				order = append(order, key)
			}
			counts[key]++
		}
		for _, key := range order {
			short := strings.TrimSuffix(key, "_when")
			parts = append(parts, fmt.Sprintf("%s(%d)", short, counts[key]))
		}
	}
	if pt.NewBecome != nil {
		parts = append(parts, "become")
	}
	if len(pt.NewEnvironment) > 0 {
		parts = append(parts, "env")
	}
	if len(pt.NewBlock) > 0 {
		parts = append(parts, fmt.Sprintf("block(%d)", len(pt.NewBlock)))
	}
	if len(parts) == 0 {
		return "no-op"
	}
	return strings.Join(parts, " ")
}

func newPatchListCmd() *cobra.Command {
	var scenario string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List patch bundles for a scenario",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadPatchBundles(scenario)
			if err != nil {
				return err
			}
			if cfg == nil || len(cfg.Bundles) == 0 {
				fmt.Printf("No patch bundles defined for scenario %s (%s)\n", scenario, "scenarios/"+scenario+"/patch.yml")
				return nil
			}
			for _, b := range cfg.Bundles {
				fmt.Printf("\033[1m%s\033[0m (role: \033[38;2;127;255;212m%s\033[0m, scenario: %s, tasks: %d)\n",
					b.PatchBundleName, b.RoleName, b.Scenario, len(b.TasksToPatch))
				for _, pt := range b.TasksToPatch {
					fmt.Printf("  %s: %s\n", strings.TrimSpace(pt.TaskId), describePatchTask(&pt))
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&scenario, "scenario", "s", config.DefaultScenario, "Molecule scenario to operate on")

	return cmd
}

func newPatchCheckCmd() *cobra.Command {
	var scenario string

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Validate patch bundles against installed roles",
		Long: `Load scenarios/<scenario>/patch.yml, validate every bundle
(diffusion.toml role, task IDs, overlays) and verify each task ID resolves
against the installed role analysis. Exits 1 on the first failure.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadPatchBundles(scenario)
			if err != nil {
				return err
			}
			if cfg == nil || len(cfg.Bundles) == 0 {
				fmt.Printf("No patch bundles defined for scenario %s\n", scenario)
				return nil
			}
			failed := false
			for i := range cfg.Bundles {
				b := &cfg.Bundles[i]
				analysis, err := patch.AnalyzeExternalRole(scenario, b.RoleName)
				if err != nil {
					fmt.Printf("\033[31mFAIL %s: analyze role %s: %v\033[0m\n", b.PatchBundleName, b.RoleName, err)
					failed = true
					continue
				}
				if err := b.CheckAgainst(analysis); err != nil {
					fmt.Printf("\033[31mFAIL %s: %v\033[0m\n", b.PatchBundleName, err)
					failed = true
					continue
				}
				fmt.Printf("\033[32mOK %s (role %s, %d tasks)\033[0m\n",
					b.PatchBundleName, b.RoleName, len(b.TasksToPatch))
			}
			if failed {
				return fmt.Errorf("patch check failed for scenario %s", scenario)
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&scenario, "scenario", "s", config.DefaultScenario, "Molecule scenario to operate on")

	return cmd
}

// runPatchBundles applies (or dry-runs) the selected bundles inside the
// running molecule container. The role is copied out of the container,
// patched on a throwaway temp dir and bind-mounted back over
// /root/.ansible/roles/<role> — neither the host cache nor any other host
// path is modified. With dryRun nothing is written back.
func runPatchBundles(scenario, bundleName, role string, dryRun, force bool) error {
	container, err := resolvePatchContainer(role)
	if err != nil {
		return err
	}
	applied, err := patch.ApplyScenarioPatchesInContainer(container, scenario, patch.ContainerApplyOptions{
		Bundle: bundleName, DryRun: dryRun, Force: force,
	})
	if err != nil {
		return err
	}
	if len(applied) == 0 {
		fmt.Printf("No patch bundles defined for scenario %s (%s)\n",
			scenario, "scenarios/"+scenario+"/patch.yml")
		return nil
	}
	for _, ap := range applied {
		if ap.Result != nil {
			fmt.Print(ap.Result.Summary())
		}
		if !dryRun {
			fmt.Printf("  overlay: %s:%s -> %s\n", ap.Container, ap.WorkPath, ap.ContainerPath)
		}
	}
	return nil
}

// resolvePatchContainer maps --role to molecule-<role>, verifying the
// container is actually running, and falls back to auto-detecting the
// first running molecule container when --role is omitted.
func resolvePatchContainer(role string) (string, error) {
	r := strings.TrimSpace(role)
	if r == "" {
		return patch.FindMoleculeContainer()
	}
	container := patch.MoleculeContainerName(r)
	if err := dockerContainerRunning(container); err != nil {
		return "", fmt.Errorf("container %s is not running; run 'diffusion molecule -r %s --converge' first", container, r)
	}
	return container, nil
}

// dockerContainerRunning reports whether a container exists and is running.
var dockerContainerRunning = func(container string) error {
	out, err := exec.Command("docker", "inspect", "-f", "{{.State.Running}}", container).Output()
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(out)) != "true" {
		return fmt.Errorf("container %s is not running", container)
	}
	return nil
}

func newPatchDiffCmd() *cobra.Command {
	var scenario, bundle, role string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Dry-run apply: report what patching would change",
		Long: `Copy the targeted roles out of the running molecule container and
report what applying the bundles would change. Nothing is written back and
no overlay is mounted.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPatchBundles(scenario, bundle, role, true, false)
		},
	}

	cmd.Flags().StringVarP(&scenario, "scenario", "s", config.DefaultScenario, "Molecule scenario to operate on")
	cmd.Flags().StringVarP(&role, "role", "r", "", "Role under test (container molecule-<role>; default: first running molecule container)")
	cmd.Flags().StringVar(&bundle, "bundle", "", "Only process this bundle name (default: all)")

	return cmd
}

func newPatchApplyCmd() *cobra.Command {
	var scenario, bundle, role string
	var dryRun, force bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply patch bundles inside the running molecule container",
		Long: `Apply patch bundles from scenarios/<scenario>/patch.yml inside the
running molecule-<role> container.

Each targeted role is copied to a patch work directory under the
container's writable layer (<scenario>/work/<role>, a pristine copy kept
under backup/), patched via a temporary host directory and bind-mounted
over the original roles path, so Ansible picks up the patched content
without any host file being modified.
The overlay stays until the container is destroyed, 'diffusion molecule
--destroy'/'--wipe' runs, or the next molecule run replaces it.

Requires a converged container: run 'diffusion molecule --converge' first.
Use --dry-run (or the diff subcommand) to preview.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPatchBundles(scenario, bundle, role, dryRun, force)
		},
	}

	cmd.Flags().StringVarP(&scenario, "scenario", "s", config.DefaultScenario, "Molecule scenario to operate on")
	cmd.Flags().StringVarP(&role, "role", "r", "", "Role under test (container molecule-<role>; default: first running molecule container)")
	cmd.Flags().StringVar(&bundle, "bundle", "", "Only process this bundle name (default: all)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report changes without writing")
	cmd.Flags().BoolVar(&force, "force", false, "Apply despite analysis drift")

	return cmd
}
