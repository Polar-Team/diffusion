package cli

import (
	"fmt"
	"os"
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
under scenarios/<scenario>/patch/files|templates/. Patches apply to staged
or installed role copies — the source of truth is never mutated.`,
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
copies, Galaxy roles path). When missing, it is installed into a running
molecule container first and analyzed from a temporary copy.

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
		parts = append(parts, fmt.Sprintf("when(%d)", len(pt.NewConditions)))
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

// runPatchBundles analyzes each selected bundle's role and applies it.
// dryRun only reports; otherwise the target role copy is patched in place
// (a backup path is printed for revert).
func runPatchBundles(scenario, bundleName, targetPath string, dryRun, force bool, backupDir string) error {
	cfg, err := loadPatchBundles(scenario)
	if err != nil {
		return err
	}
	if cfg == nil || len(cfg.Bundles) == 0 {
		return fmt.Errorf("no patch bundles defined for scenario %s", scenario)
	}
	failed := false
	for i := range cfg.Bundles {
		b := &cfg.Bundles[i]
		if bundleName != "" && b.PatchBundleName != bundleName {
			continue
		}
		target := strings.TrimSpace(targetPath)
		if target == "" {
			target, err = patch.ResolveInstalledRolePath(b.RoleName)
			if err != nil {
				fmt.Printf("\033[31mFAIL %s: %v\033[0m\n", b.PatchBundleName, err)
				failed = true
				continue
			}
		}
		analysis, err := patch.AnalyzeRoleAtPath(target, scenario, b.RoleName)
		if err != nil {
			fmt.Printf("\033[31mFAIL %s: analyze %s: %v\033[0m\n", b.PatchBundleName, target, err)
			failed = true
			continue
		}
		res, err := patch.ApplyBundle(b, analysis, patch.ApplyOptions{
			Scenario: scenario, RolePath: target,
			DryRun: dryRun, Force: force, BackupDir: backupDir,
		})
		if err != nil {
			fmt.Printf("\033[31mFAIL %s: %v\033[0m\n", b.PatchBundleName, err)
			failed = true
			continue
		}
		fmt.Print(res.Summary())
	}
	if bundleName != "" && !failed {
		found := false
		for _, b := range cfg.Bundles {
			if b.PatchBundleName == bundleName {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("bundle %q not found in scenario %s", bundleName, scenario)
		}
	}
	if failed {
		return fmt.Errorf("patch %s failed for scenario %s", map[bool]string{true: "diff", false: "apply"}[dryRun], scenario)
	}
	return nil
}

func newPatchDiffCmd() *cobra.Command {
	var scenario, bundle string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Dry-run apply: report what patching would change",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPatchBundles(scenario, bundle, "", true, false, "")
		},
	}

	cmd.Flags().StringVarP(&scenario, "scenario", "s", config.DefaultScenario, "Molecule scenario to operate on")
	cmd.Flags().StringVar(&bundle, "bundle", "", "Only process this bundle name (default: all)")

	return cmd
}

func newPatchApplyCmd() *cobra.Command {
	var scenario, bundle, targetPath, backupDir string
	var dryRun, force bool

	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Apply patch bundles to a role copy",
		Long: `Apply patch bundles from scenarios/<scenario>/patch.yml.

The target defaults to the resolved installed role. Prefer staging a copy
(diffusion molecule does this automatically) — manual applies print a
backup path usable with revert semantics. Use --dry-run (or the diff
subcommand) to preview.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPatchBundles(scenario, bundle, targetPath, dryRun, force, backupDir)
		},
	}

	cmd.Flags().StringVarP(&scenario, "scenario", "s", config.DefaultScenario, "Molecule scenario to operate on")
	cmd.Flags().StringVar(&bundle, "bundle", "", "Only process this bundle name (default: all)")
	cmd.Flags().StringVar(&targetPath, "path", "", "Role directory to patch (default: resolved installed role)")
	cmd.Flags().StringVar(&backupDir, "backup-dir", "", "Directory for pre-apply backups (default: temp dir)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Report changes without writing")
	cmd.Flags().BoolVar(&force, "force", false, "Apply despite analysis drift")

	return cmd
}
