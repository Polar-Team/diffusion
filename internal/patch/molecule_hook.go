package patch

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/utils"
)

// AppliedPatch describes one bundle applied by ApplyScenarioPatches: the
// bundle name, the targeted role and the host-side patched role directory.
type AppliedPatch struct {
	Bundle string
	Role   string
	Path   string
}

// ApplyScenarioPatches loads scenarios/<scenario>/patch.yml and applies every
// bundle to its resolved installed role. It is a no-op returning (nil, nil)
// when no patch.yml exists, so molecule runs can call it unconditionally.
//
// Patched roles live in the static <cacheDir>/patch/<scenario> tree
// (role_<cache_id> suffix) when caching is configured, otherwise in a temp
// dir — see ResolveInstalledRolePath. Callers copy AppliedPatch paths into
// the molecule container where Ansible looks first.
func ApplyScenarioPatches(scenario string) ([]AppliedPatch, error) {
	scenario = strings.TrimSpace(scenario)
	if scenario == "" {
		scenario = config.DefaultScenario
	}
	if err := utils.ValidateCLIArgument("scenario", scenario); err != nil {
		return nil, err
	}
	if strings.Contains(scenario, "/") || strings.Contains(scenario, "\\") || strings.Contains(scenario, "..") {
		return nil, fmt.Errorf("invalid scenario %q: must be a plain scenario name", scenario)
	}

	if _, err := os.Stat(filepath.Join("scenarios", scenario, "patch.yml")); os.IsNotExist(err) {
		return nil, nil
	}

	pc := &PatchesConfig{}
	cfg, err := pc.LoadPatchConfigFrom(scenario)
	if err != nil {
		return nil, err
	}
	if cfg == nil || len(cfg.Bundles) == 0 {
		return nil, nil
	}

	var applied []AppliedPatch
	for i := range cfg.Bundles {
		b := &cfg.Bundles[i]
		resolveScenario := strings.TrimSpace(b.Scenario)
		if resolveScenario == "" {
			resolveScenario = scenario
		}
		target, err := ResolveInstalledRolePath(b.RoleName, resolveScenario)
		if err != nil {
			return nil, fmt.Errorf("bundle %q: %w", b.PatchBundleName, err)
		}
		analysis, err := AnalyzeRoleAtPath(target, scenario, b.RoleName)
		if err != nil {
			return nil, fmt.Errorf("bundle %q: analyze %s: %w", b.PatchBundleName, target, err)
		}
		if _, err := ApplyBundle(b, analysis, ApplyOptions{Scenario: scenario, RolePath: target}); err != nil {
			return nil, fmt.Errorf("bundle %q: %w", b.PatchBundleName, err)
		}
		applied = append(applied, AppliedPatch{
			Bundle: b.PatchBundleName,
			Role:   b.RoleName,
			Path:   target,
		})
		log.Printf(config.ColorGreen+"patch: applied bundle %q to %s"+config.ColorReset, b.PatchBundleName, target)
	}
	return applied, nil
}
