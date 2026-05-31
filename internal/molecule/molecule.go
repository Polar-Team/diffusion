package molecule

import (
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"diffusion/internal/cache"
	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/registry"
	"diffusion/internal/secrets"
	"diffusion/internal/utils"
)

// MoleculeOptions holds all the parameters needed to run the molecule workflow.
// It decouples the business logic from CLI flag parsing.
type MoleculeOptions struct {
	RoleFlag        string
	OrgFlag         string
	RoleScenario    string
	TagFlag         string
	ConvergeFlag    bool
	VerifyFlag      bool
	TestsOverWrite  bool
	LintFlag        bool
	IdempotenceFlag bool
	DestroyFlag     bool
	WipeFlag        bool
	CIMode          bool
	OidcFlag        bool
	ForceFlag       bool
}

// scenarioFlag returns " -s <scenario>" if scenario is non-default, otherwise empty string.
func scenarioFlag(opts *MoleculeOptions) string {
	if opts.RoleScenario != "" && opts.RoleScenario != config.DefaultScenario {
		return fmt.Sprintf(" -s %s", opts.RoleScenario)
	}
	return ""
}

// RunMolecule is the core function that implements the molecule workflow.
// It handles wipe, converge, lint, verify, idempotence, destroy and the
// default create/converge flow.
func RunMolecule(opts *MoleculeOptions) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf(config.ColorYellow+"warning loading config: %v"+config.ColorReset, err)
	}
	if cfg == nil {
		cfg = &config.Config{}
	}
	if cfg.ContainerRegistry == nil {
		cfg.ContainerRegistry = &config.ContainerRegistry{}
	}

	// prepare path
	path, err := os.Getwd()
	if err != nil {
		return err
	}

	// Compose role path
	roleDirName := utils.GetRoleDirName(opts.OrgFlag, opts.RoleFlag)
	roleMoleculePath := filepath.Join(path, config.MoleculeDir, roleDirName)

	// handle wipe
	if opts.WipeFlag {
		return handleWipe(opts, cfg, roleDirName, roleMoleculePath)
	}

	// handle converge/lint/verify/idempotence/destroy
	if opts.ConvergeFlag || opts.LintFlag || opts.VerifyFlag || opts.IdempotenceFlag || opts.DestroyFlag {
		return handleSubcommands(opts, cfg, path, roleDirName, roleMoleculePath)
	}

	// default flow: create/run container if not exists, copy data, converge
	return handleDefaultFlow(opts, cfg, path, roleDirName, roleMoleculePath)
}

// handleWipe destroys the molecule container and removes the role folder.
// Before removing the container, it saves DinD images and (—CI mode) copies
// the cache out of the container back to the host.
func handleWipe(opts *MoleculeOptions, cfg *config.Config, roleDirName, roleMoleculePath string) error {
	log.Printf(config.ColorAquamarine+"Wiping: running molecule destroy, removing container molecule-%s and folder %s\n"+config.ColorReset, opts.RoleFlag, roleMoleculePath)

	// Run molecule destroy inside the container first
	roleDir := utils.GetRoleDirName(opts.OrgFlag, opts.RoleFlag)
	// Best-effort: container may already be destroyed or never created.
	_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "bash", opts.CIMode, "-c", fmt.Sprintf("cd ./%s && molecule destroy%s", roleDir, scenarioFlag(opts)))

	// Save DinD images before removing the container
	if cfg.CacheConfig != nil && cfg.CacheConfig.Enabled && cfg.CacheConfig.DockerCache {
		saveDinDImages(opts)
	}

	// Windows: save UV cache back to precache (NTFS mount) before container removal
	if !opts.CIMode && runtime.GOOS == "windows" && cfg.CacheConfig != nil && cfg.CacheConfig.Enabled && cfg.CacheConfig.UVCache {
		saveUVCacheToPrecache(opts)
	}

	// CI mode: copy cache from container back to host before docker rm
	if opts.CIMode {
		copyCacheFromContainer(opts, cfg)
	}

	// Remove the container
	// Best-effort: -f flag means failure is safe to ignore (container may not exist).
	_ = utils.RunCommandHide(opts.CIMode, "docker", "rm", fmt.Sprintf("molecule-%s", opts.RoleFlag), "-f")

	// Remove the role folder
	if err := os.RemoveAll(roleMoleculePath); err != nil {
		log.Printf(config.ColorYellow+"warning: failed remove role path: %v"+config.ColorReset, err)
	}
	return nil
}

// handleSubcommands handles --converge, --lint, --verify, --idempotence, --destroy flags.
func handleSubcommands(opts *MoleculeOptions, cfg *config.Config, path, roleDirName, roleMoleculePath string) error {
	if !opts.CIMode {
		if err := utils.CopyRoleData(path, roleMoleculePath, opts.CIMode); err != nil {
			log.Printf(config.ColorYellow+"warning copying data: %v"+config.ColorReset, err)
		}
		metaFixCmd := fmt.Sprintf(
			`if [ -f /opt/molecule/%s/meta/main.yml ]; then sed -i 's/^\(\s*namespace:\s*\).*/\1%s/' /opt/molecule/%s/meta/main.yml; fi`,
			roleDirName, opts.OrgFlag, roleDirName)
		// Best-effort: meta/main.yml may not exist —all roles.
		_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", metaFixCmd)
	}

	linters := roleMoleculePath
	// In CI mode, set linters to Org.Role format
	if opts.CIMode {
		linters = fmt.Sprintf("%s.%s", opts.OrgFlag, opts.RoleFlag)
	}

	err := utils.ExportLinters(cfg, linters, opts.CIMode, opts.RoleFlag, opts.OrgFlag)
	if err != nil {
		log.Printf(config.ColorYellow+"warning exporting linters: %v"+config.ColorReset, err)
	}

	// Determine scenario name for tests directory
	scenario := config.DefaultScenario
	if opts.RoleScenario != "" {
		scenario = opts.RoleScenario
	}

	// Create tests directory for verify
	moleculeDefaultTestsPath := fmt.Sprintf("molecule/%s.%s/molecule/%s/tests", opts.OrgFlag, opts.RoleFlag, scenario)
	if err := os.MkdirAll(moleculeDefaultTestsPath, 0o755); err != nil {
		log.Printf(config.ColorYellow+"warning: cannot create scenario tests dir: %v"+config.ColorReset, err)
	}
	defaultTestsDir := moleculeDefaultTestsPath
	log.Printf("Default tests dir: %s", defaultTestsDir)

	if opts.ConvergeFlag {
		return runConverge(opts, roleDirName)
	}
	if opts.LintFlag {
		return runLint(opts, roleDirName)
	}
	if opts.VerifyFlag {
		return runVerify(opts, cfg, path, roleDirName, roleMoleculePath, scenario)
	}
	if opts.IdempotenceFlag {
		return runIdempotence(opts, roleDirName)
	}
	if opts.DestroyFlag {
		return runDestroy(opts, roleDirName)
	}

	return nil
}

// runConverge runs molecule converge inside the container.
func runConverge(opts *MoleculeOptions, roleDirName string) error {
	// Verify molecule.yml exists inside container before running
	if opts.CIMode {
		checkCmd := fmt.Sprintf("ls -la /opt/molecule/%s/molecule/default/molecule.yml", roleDirName)
		log.Printf("Checking molecule.yml in container...")
		if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", checkCmd); err != nil {
			log.Printf(config.ColorRed+"molecule.yml not found  in container at /opt/molecule/%s/molecule/default/"+config.ColorReset, roleDirName)
			log.Printf(config.ColorYellow + "Listing container directory structure:" + config.ColorReset)
			// Best-effort debug listing — output shown regardless of success/failure.
			_ = utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("ls -laR /opt/molecule/%s/", roleDirName))
			return fmt.Errorf("molecule.yml not found in container at /opt/molecule/%s/molecule/default/", roleDirName)
		}
	}

	tagEnv := ""
	if opts.TagFlag != "" {
		tagEnv = fmt.Sprintf("ANSIBLE_RUN_TAGS=%s ", opts.TagFlag)
	}
	scenario := config.DefaultScenario
	if opts.RoleScenario != "" {
		scenario = opts.RoleScenario
	}
	galaxyInstall := ""
	if opts.ForceFlag {
		galaxyInstall = fmt.Sprintf("ansible-galaxy install --force -r molecule/%s/requirements.yml 2>/dev/null || true && ", scenario)
	}
	cmdStr := fmt.Sprintf("cd ./%s && %s%smolecule converge%s", roleDirName, galaxyInstall, tagEnv, scenarioFlag(opts))
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdStr); err != nil {
		log.Printf(config.ColorRed+"Converge failed: %v"+config.ColorReset, err)
		return fmt.Errorf("converge failed: %w", err)
	}
	log.Printf(config.ColorGreen + "Converge Done Successfully!" + config.ColorReset)

	// Fix permissions on molecule directory for Unix systems (inside container)
	if runtime.GOOS != "windows" {
		uid := os.Getuid()
		gid := os.Getgid()
		log.Printf("User UID: %d, GID: %d", uid, gid)
		chownCmd := fmt.Sprintf("chown -R %d:%d /opt/molecule", uid, gid)
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", chownCmd); err != nil {
			log.Printf(config.ColorYellow+"warning: failed to fix permissions: %v"+config.ColorReset, err)
		}
	}

	return nil
}

// runLint runs yamllint and ansible-lint inside the container.
func runLint(opts *MoleculeOptions, roleDirName string) error {
	cmdStr := fmt.Sprintf(`cd ./%s && yamllint . -c .yamllint && ansible-lint -c .ansible-lint `, roleDirName)
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdStr); err != nil {
		log.Printf(config.ColorRed+"Lint failed: %v"+config.ColorReset, err)
		return fmt.Errorf("lint failed: %w", err)
	}
	log.Printf(config.ColorGreen + "Lint Done Successfully!" + config.ColorReset)
	return nil
}

// runVerify handles test source resolution (local/remote/diffusion) and runs molecule verify.
func runVerify(opts *MoleculeOptions, cfg *config.Config, path, roleDirName, roleMoleculePath, scenario string) error {
	if cfg.TestsConfig == nil {
		log.Printf(config.ColorYellow + "warning: no tests config found, defaulting to diffusion" + config.ColorReset)
		cfg.TestsConfig = &config.TestsSettings{Type: config.TestsTypeDiffusion}
	}
	switch cfg.TestsConfig.Type {
	case config.TestsTypeLocal:
		verifyLocalTests(opts, path, roleMoleculePath, scenario)
	case config.TestsTypeRemote:
		if len(cfg.TestsConfig.RemoteRepositories) == 0 {
			return fmt.Errorf("no remote repository configured for tests type 'remote'")
		}
		verifyRemoteTests(opts, cfg, roleMoleculePath, scenario)
	case config.TestsTypeDiffusion:
		if err := verifyDiffusionTests(opts, roleMoleculePath, scenario); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown tests type: %s", cfg.TestsConfig.Type)
	}

	// run molecule verify
	tagEnv := ""
	if opts.TagFlag != "" {
		tagEnv = fmt.Sprintf("ANSIBLE_RUN_TAGS=%s ", opts.TagFlag)
	}
	cmdStr := fmt.Sprintf("cd ./%s && %smolecule verify%s", roleDirName, tagEnv, scenarioFlag(opts))
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdStr); err != nil {
		log.Printf(config.ColorRed+"Verify failed: %v"+config.ColorReset, err)
		return fmt.Errorf("verify failed: %w", err)
	}
	log.Printf(config.ColorGreen + "Verify Done Successfully!" + config.ColorReset)
	return nil
}

// verifyLocalTests copies tests from local tests/ directory.
func verifyLocalTests(opts *MoleculeOptions, path, roleMoleculePath, scenario string) {
	testsSrc := filepath.Join(path, config.TestsDir)
	if opts.CIMode {
		log.Printf("CIMode detected, copying tests from /tmp/repo/tests to %s.%s/molecule/%s/tests/", opts.OrgFlag, opts.RoleFlag, scenario)
		cmdCopy := fmt.Sprintf(`
			cd /tmp && \
			rm -rf repo && \
			git clone --single-branch --branch "${GIT_BRANCH}" "${GIT_REMOTE}" repo && \
			cp -rf /tmp/repo/tests /opt/molecule/%s.%s/molecule/%s/
		`, opts.OrgFlag, opts.RoleFlag, scenario)
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdCopy); err != nil {
			log.Printf(config.ColorYellow+"warning: failed to copy tests —CI mode: %v"+config.ColorReset, err)
		}
	} else {
		if utils.Exists(testsSrc) {
			testsDst := filepath.Join(roleMoleculePath, config.MoleculeDir, scenario, config.TestsDir)
			utils.CopyIfExists(testsSrc, testsDst)
		} else {
			log.Printf(config.ColorYellow + "tests/ directory not found, skipping copy" + config.ColorReset)
		}
	}
}

// verifyRemoteTests clones test files from remote repositories.
func verifyRemoteTests(opts *MoleculeOptions, cfg *config.Config, roleMoleculePath, scenario string) {
	for _, remoteRepo := range cfg.TestsConfig.RemoteRepositories {
		log.Printf(config.ColorGreen+"Installing test files from remote repository: %s"+config.ColorReset, remoteRepo)

		if !opts.TestsOverWrite {
			if opts.CIMode {
				cmdRemoteTests := fmt.Sprintf(`
				cd /opt/molecule/%s.%s/molecule/%s && \
				if [ ! -d tests ]; then \
					mkdir -p tests && cd tests && git clone %s; \
				else \
					echo "Tests directory already exists, skipping clone"; \
				fi
			`, opts.OrgFlag, opts.RoleFlag, scenario, remoteRepo)
				if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdRemoteTests); err != nil {
					log.Printf(config.ColorYellow+"warning: failed to clone remote tests —CI mode: %v"+config.ColorReset, err)
				}
			} else {
				testsDst := filepath.Join(roleMoleculePath, config.MoleculeDir, scenario, config.TestsDir)
				if _, err := os.Stat(testsDst); os.IsNotExist(err) {
					cmdRemoteTests := fmt.Sprintf(`
					cd %s && \
					mkdir -p tests && cd tests && git clone %s;
				`, filepath.Join(roleMoleculePath, config.MoleculeDir, scenario), remoteRepo)
					if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdRemoteTests); err != nil {
						log.Printf(config.ColorYellow+"warning: failed to clone remote tests: %v"+config.ColorReset, err)
					}
				} else {
					log.Printf(config.ColorYellow + "Tests directory already exists, skipping clone" + config.ColorReset)
				}
			}
		} else {
			// Overwrite tests directory with remote repository
			if opts.CIMode {
				cmdRemoteTests := fmt.Sprintf(`
				cd /opt/molecule/%s.%s/molecule/%s && \
				rm -rf tests && \
				mkdir -p tests && cd tests && git clone %s
			`, opts.OrgFlag, opts.RoleFlag, scenario, remoteRepo)
				if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdRemoteTests); err != nil {
					log.Printf(config.ColorYellow+"warning: failed to clone remote tests —CI mode: %v"+config.ColorReset, err)
				}
			} else {
				testsDst := filepath.Join(roleMoleculePath, config.MoleculeDir, scenario, config.TestsDir)
				if err := os.RemoveAll(testsDst); err != nil {
					log.Printf(config.ColorYellow+"warning: failed to remove tests directory before overwrite: %v"+config.ColorReset, err)
				}
				cmdRemoteTests := fmt.Sprintf(`
				cd %s && \
				git clone %s tests
			`, filepath.Join(roleMoleculePath, config.MoleculeDir, scenario), remoteRepo)
				if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdRemoteTests); err != nil {
					log.Printf(config.ColorYellow+"warning: failed to clone remote tests: %v"+config.ColorReset, err)
				}
			}
		}
	}
}

// verifyDiffusionTests clones/updates diffusion-managed test files.
func verifyDiffusionTests(opts *MoleculeOptions, roleMoleculePath, scenario string) error {
	log.Printf(config.ColorGreen + "Using diffusion-managed test files" + config.ColorReset)

	diffusionTestsPath := "/tmp/diffusion-tests-repo"
	if !opts.TestsOverWrite {
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf(`ls %s`, diffusionTestsPath)); err != nil {
			log.Printf(config.ColorGreen + "Cloning diffusion tests repository..." + config.ColorReset)
			if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "git", opts.CIMode, "clone", "https://github.com/Polar-Team/diffusion-ansible-tests-role.git", diffusionTestsPath); err != nil {
				return fmt.Errorf("failed to clone diffusion tests repository: %w", err)
			}
		} else {
			log.Printf(config.ColorGreen + "Updating diffusion tests repository..." + config.ColorReset)
			cmdPullCommand := fmt.Sprintf(`cd %s && git pull`, diffusionTestsPath)
			if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdPullCommand); err != nil {
				log.Printf(config.ColorYellow+"warning: failed to update diffusion tests repository: %v"+config.ColorReset, err)
			}
		}
	} else {
		cmdRemove := fmt.Sprintf("rm -rf %s", diffusionTestsPath)
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdRemove); err != nil {
			return fmt.Errorf("failed to remove existing diffusion tests repository: %w", err)
		}
		log.Printf(config.ColorGreen + "Cloning diffusion tests repository (overwrite mode)..." + config.ColorReset)
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "git", opts.CIMode, "clone", "https://github.com/Polar-Team/diffusion-ansible-tests-role.git", diffusionTestsPath); err != nil {
			return fmt.Errorf("failed to clone diffusion tests repository: %w", err)
		}
	}

	// Copy tests from diffusion repo to role tests directory
	destPath := fmt.Sprintf(
		"/opt/molecule/%s.%s/%s/%s/%s/diffusion_tests", opts.OrgFlag, opts.RoleFlag,
		config.MoleculeDir, scenario, config.TestsDir)
	cmdCopy := fmt.Sprintf(`mkdir -p %s && cp -rf %s/. %s`, destPath, diffusionTestsPath, destPath)
	if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdCopy); err != nil {
		log.Printf(config.ColorYellow+"warning: failed to copy diffusion tests: %v"+config.ColorReset, err)
	}

	return nil
}

// runIdempotence runs molecule idempotence inside the container.
func runIdempotence(opts *MoleculeOptions, roleDirName string) error {
	tagEnv := ""
	if opts.TagFlag != "" {
		tagEnv = fmt.Sprintf("ANSIBLE_RUN_TAGS=%s ", opts.TagFlag)
	}
	cmdStr := fmt.Sprintf("cd ./%s && %smolecule idempotence%s", roleDirName, tagEnv, scenarioFlag(opts))
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdStr); err != nil {
		log.Printf(config.ColorRed+"Idempotence failed: %v"+config.ColorReset, err)
		return fmt.Errorf("idempotence failed: %w", err)
	}
	log.Printf(config.ColorGreen + "Idempotence Done Successfully!" + config.ColorReset)
	return nil
}

// runDestroy runs molecule destroy inside the container.
func runDestroy(opts *MoleculeOptions, roleDirName string) error {
	cmdStr := fmt.Sprintf("cd ./%s && molecule destroy%s", roleDirName, scenarioFlag(opts))
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdStr); err != nil {
		log.Printf(config.ColorRed+"Destroy failed: %v"+config.ColorReset, err)
		return fmt.Errorf("destroy failed: %w", err)
	}
	log.Printf(config.ColorGreen + "Destroy Done Successfully!" + config.ColorReset)
	return nil
}

// handleDefaultFlow handles the default molecule workflow: create container, copy data, converge.
func handleDefaultFlow(opts *MoleculeOptions, cfg *config.Config, path, roleDirName, roleMoleculePath string) error {
	// check if container exists
	err := exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", opts.RoleFlag)).Run()
	if err == nil {
		fmt.Printf(config.ColorAquamarine+"Container molecule-%s already exists. To purge use --wipe.\n"+config.ColorReset, opts.RoleFlag)
	} else {
		// Container does not exist — set up credentials, auth, and run it
		if err := setupCredentials(opts, cfg); err != nil {
			return err
		}

		setupRegistryAuth(cfg, opts.OidcFlag, opts.CIMode)

		// Ensure molecule directory exists on host before mounting it into the container
		if !opts.CIMode {
			moleculeHostPath := filepath.Join(path, config.MoleculeDir)
			if err := os.MkdirAll(moleculeHostPath, 0755); err != nil {
				return fmt.Errorf("failed to create molecule directory %s: %w", moleculeHostPath, err)
			}
		}

		if err := runContainer(opts, cfg, path, roleDirName); err != nil {
			return err
		}
	}

	// CI Mode: copy cache into container (replaces volume mounts)
	if opts.CIMode {
		copyCacheIntoContainer(opts, cfg)
	}

	// Windows: copy UV precache into native container cache (non-CI only, CI uses docker cp)
	if !opts.CIMode && runtime.GOOS == "windows" && cfg.CacheConfig != nil && cfg.CacheConfig.Enabled && cfg.CacheConfig.UVCache {
		loadUVPrecache(opts)
	}

	// Load DinD images from cached tarball (both modes)
	if cfg.CacheConfig != nil && cfg.CacheConfig.Enabled && cfg.CacheConfig.DockerCache {
		loadDinDImages(opts)
	}

	// CI Mode: Clone repository and setup files inside container
	if opts.CIMode {
		if err := setupCIRepository(opts, path, roleDirName); err != nil {
			return err
		}
	}

	// ensure role exists (skip —CI mode - already handled)
	if !opts.CIMode {
		ensureRole(opts, roleMoleculePath)
	}

	// docker exec log—to registry inside container (provider-specific)
	loginInsideContainer(opts, cfg)

	// copy files into molecule structure (skip —CI mode - already handled)
	if !opts.CIMode {
		if err := utils.CopyRoleData(path, roleMoleculePath, opts.CIMode); err != nil {
			log.Printf(config.ColorYellow+"copy role data warning: %v"+config.ColorReset, err)
		}
		err := utils.ExportLinters(cfg, roleMoleculePath, opts.CIMode, opts.RoleFlag, opts.OrgFlag)
		if err != nil {
			log.Printf(config.ColorYellow+"export linters warning: %v"+config.ColorReset, err)
		}
		metaFixCmd := fmt.Sprintf(
			`if [ -f /opt/molecule/%s/meta/main.yml ]; then sed -i 's/^\(\s*namespace:\s*\).*/\1%s/' /opt/molecule/%s/meta/main.yml; fi`,
			roleDirName, opts.OrgFlag, roleDirName)
		// Best-effort: meta/main.yml may not exist —all roles.
		_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", metaFixCmd)
	}

	// finally create/converge
	scenario := config.DefaultScenario
	if opts.RoleScenario != "" {
		scenario = opts.RoleScenario
	}
	galaxyInstall := ""
	if opts.ForceFlag {
		galaxyInstall = fmt.Sprintf("ansible-galaxy install --force -r molecule/%s/requirements.yml 2>/dev/null || true && ", scenario)
	}
	err = exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", opts.RoleFlag)).Run()
	if err == nil {
		// container exists — best-effort uv-sync, then converge
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "uv-sync", opts.CIMode); err != nil {
			log.Printf(config.ColorYellow+"warning: uv-sync failed (container-exists path): %v"+config.ColorReset, err)
		}
		if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("cd ./%s && %smolecule converge%s", roleDirName, galaxyInstall, scenarioFlag(opts))); err != nil {
			log.Printf(config.ColorYellow+"warning: converge failed (container-exists path): %v"+config.ColorReset, err)
		}
	} else {
		// Sync UV dependencies with pyproject.toml from diffusion
		if err := utils.DockerExecInteractive(opts.RoleFlag, "uv-sync", opts.CIMode); err != nil {
			log.Printf(config.ColorYellow+"Warning: uv-sync failed: %v"+config.ColorReset, err)
			log.Printf(config.ColorYellow + "Continuing with existing dependencies..." + config.ColorReset)
		}
		if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("cd ./%s && molecule create%s", roleDirName, scenarioFlag(opts))); err != nil {
			log.Printf(config.ColorYellow+"warning: molecule create failed: %v"+config.ColorReset, err)
		}
		if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("cd ./%s && %smolecule converge%s", roleDirName, galaxyInstall, scenarioFlag(opts))); err != nil {
			log.Printf(config.ColorYellow+"warning: converge failed: %v"+config.ColorReset, err)
		}
	}

	// Fix permissions on molecule directory for Unix systems (skip —CI mode - no volume mount)
	if !opts.CIMode && runtime.GOOS != "windows" {
		uid := os.Getuid()
		gid := os.Getgid()
		chownCmd := fmt.Sprintf("chown -R %d:%d /opt/molecule", uid, gid)
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", chownCmd); err != nil {
			log.Printf(config.ColorYellow+"warning: failed to fix permissions: %v"+config.ColorReset, err)
		}
	}

	return nil
}

// setupCredentials loads artifact source credentials from Vault or local storage.
func setupCredentials(opts *MoleculeOptions, cfg *config.Config) error {
	if len(cfg.ArtifactSources) > 0 {
		for i, source := range cfg.ArtifactSources {
			index := i + 1
			creds, err := secrets.GetArtifactCredentials(&source, cfg.HashicorpVault)
			if err != nil {
				log.Printf(config.ColorYellow+"warning: failed to load credentials for '%s': %v"+config.ColorReset, source.Name, err)
				continue
			}

			if err := os.Setenv(fmt.Sprintf("GIT_USER_%d", index), creds.Username); err != nil {
				log.Printf("Failed to set GIT_USER_%d: %v", index, err)
			}
			if err := os.Setenv(fmt.Sprintf("GIT_PASSWORD_%d", index), creds.Token); err != nil {
				log.Printf("Failed to set GIT_PASSWORD_%d: %v", index, err)
			}
			if err := os.Setenv(fmt.Sprintf("GIT_URL_%d", index), creds.URL); err != nil {
				log.Printf("Failed to set GIT_URL_%d: %v", index, err)
			}

			log.Printf(config.ColorGreen+"Loaded credentials for artifact source '%s' (GIT_*_%d)"+config.ColorReset, source.Name, index)
		}
	} else if cfg.HashicorpVault != nil && cfg.HashicorpVault.HashicorpVaultIntegration && cfg.HashicorpVault.SecretKV2Path != "" {
		log.Println(config.ColorRed + "ERROR: Legacy Vault configuration detected but is no longer supported." + config.ColorReset)
		log.Println(config.ColorYellow + "Please migrate to artifact_sources configuration." + config.ColorReset)
		log.Println(config.ColorYellow + "See MIGRATION_GUIDE.md for instructions." + config.ColorReset)
		log.Println(config.ColorYellow + "Use 'diffusion artifact add' to configure artifact sources with Vault." + config.ColorReset)
		return fmt.Errorf("legacy Vault configuration (secret_kv2_path) is no longer supported; migrate to artifact_sources")
	} else {
		log.Println(config.ColorMagenta + "No artifact sources configured. Use public repositories or 'diffusion artifact add' to configure." + config.ColorReset)
	}
	return nil
}

// setupRegistryAuth initializes CLI and performs docker log—based on registry provider.
// When oidc is true, it reads credentials from environment variables instead of calling cloud CLIs.
func setupRegistryAuth(cfg *config.Config, oidc bool, ciMode bool) {
	provider := cfg.ContainerRegistry.RegistryProvider
	if oidc {
		if err := registry.OidcInit(provider); err != nil {
			log.Printf(config.ColorRed+"OIDC init error: %v"+config.ColorReset, err)
			return
		}
	}
	switch provider {
	case config.RegistryProviderYC:
		if !oidc {
			if err := registry.YcCliInit(); err != nil {
				log.Printf(config.ColorYellow+"yc init warning: %v"+config.ColorReset, err)
			}
		}
		if err := utils.RunCommandHide(ciMode, "docker", "login", cfg.ContainerRegistry.RegistryServer, "--username", "iam", "--password", os.Getenv("TOKEN")); err != nil {
			log.Printf(config.ColorYellow+"docker login to registry failed: %v"+config.ColorReset, err)
		}
	case config.RegistryProviderAWS:
		if !oidc {
			if err := registry.AwsCliInit(cfg.ContainerRegistry.RegistryServer); err != nil {
				log.Printf(config.ColorYellow+"aws ecr init warning: %v"+config.ColorReset, err)
			}
		} else {
			region := os.Getenv("AWS_REGION")
			log.Printf(config.ColorGreen+"OIDC AWS: using region %s from environment"+config.ColorReset, region)
		}
		if err := utils.RunCommandHide(ciMode, "docker", "login", cfg.ContainerRegistry.RegistryServer, "--username", "AWS", "--password", os.Getenv("TOKEN")); err != nil {
			log.Printf(config.ColorYellow+"docker login to AWS ECR registry failed: %v"+config.ColorReset, err)
		}
	case config.RegistryProviderGCP:
		if !oidc {
			if err := registry.GcpCliInit(cfg.ContainerRegistry.RegistryServer); err != nil {
				log.Printf(config.ColorYellow+"gcloud init warning: %v"+config.ColorReset, err)
			}
		}
		if err := utils.RunCommandHide(ciMode, "docker", "login", cfg.ContainerRegistry.RegistryServer, "--username", "oauth2accesstoken", "--password", os.Getenv("TOKEN")); err != nil {
			log.Printf(config.ColorYellow+"docker login to GCP registry failed: %v"+config.ColorReset, err)
		}
	case config.RegistryProviderPublic:
		log.Printf(config.ColorMagenta + "Using public registry, skipping CLI initialization and authentication" + config.ColorReset)
	default:
		log.Printf(config.ColorYellow+"Unknown registry provider '%s', skipping CLI initialization"+config.ColorReset, provider)
	}
}

// runContainer builds docker run arguments and starts the molecule container.
func runContainer(opts *MoleculeOptions, cfg *config.Config, path, roleDirName string) error {
	image := utils.GetImageURL(cfg.ContainerRegistry)
	args := []string{
		"run", "--rm", "-d", "--name=" + fmt.Sprintf("molecule-%s", opts.RoleFlag),
	}

	// CI Mode: Don't mount /opt/molecule, we'll clone repo inside container
	if !opts.CIMode {
		args = append(args, "-v", fmt.Sprintf("%s/molecule:/opt/molecule", path))
	}

	args = append(args,
		"-e", "UV_VENV_CLEAR=1",
		"-e", "TOKEN="+os.Getenv("TOKEN"),
		"-e", "VAULT_TOKEN="+os.Getenv("VAULT_TOKEN"),
		"-e", "VAULT_ADDR="+os.Getenv("VAULT_ADDR"),
		"-e", "SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt",
	)

	// Get Python version from lock file if it exists, otherwise use default
	pythonVersion := config.PinnedPythonVersion
	lockFile, err := dependency.LoadLockFile()
	if err == nil && lockFile != nil && lockFile.Python != nil && lockFile.Python.Pinned != "" {
		pythonVersion = lockFile.Python.Pinned
		log.Printf(config.ColorGreen+"Using Python version from lock file: %s"+config.ColorReset, pythonVersion)
	} else {
		log.Printf(config.ColorYellow+"No lock file found, using default Python version: %s"+config.ColorReset, pythonVersion)
	}
	args = append(args, "-e", fmt.Sprintf("PYTHON_PINNED_VERSION=%s", pythonVersion))

	// Generate and pass pyproject.toml configuration
	pyprojectContent, err := dependency.GeneratePyProjectFromCurrentConfig()
	if err != nil {
		log.Printf(config.ColorYellow+"warning: failed to generate pyproject.toml config: %v"+config.ColorReset, err)
		log.Printf(config.ColorYellow + "Container will use default dependencies" + config.ColorReset)
	} else {
		pyprojectEncoded := base64.StdEncoding.EncodeToString([]byte(pyprojectContent))
		args = append(args, "-e", "PYPROJECT_TOML_CONTENT="+pyprojectEncoded)
		log.Printf(config.ColorGreen + "Passing pyproject.toml configuration to container" + config.ColorReset)
	}

	// CI Mode: Pass git remote and commit SHA for cloning inside container
	if opts.CIMode {
		gitRemoteCmd := exec.Command("git", "config", "--get", "remote.origin.url")
		gitRemoteCmd.Dir = path
		gitRemoteOutput, err := gitRemoteCmd.Output()
		if err != nil {
			return fmt.Errorf("CI mode: failed to get git remote URL: %w", err)
		}
		gitRemote := strings.TrimSpace(string(gitRemoteOutput))

		// Detect pull_request events: actions/checkout creates a synthetic
		// merge commit on a detached HEAD that doesn't exist on the remote.
		// In that case git rev-parse --abbrev-ref HEAD returns "HEAD" and
		// the SHA is unreachable from the remote, so neither can be used
		// for cloning inside the container.
		//
		// GitHub Actions sets GITHUB_HEAD_REF to the PR source branch name
		// for pull_request events (empty for push events). When present we
		// use it as the branch to clone; otherwise we fall back to the
		// local branch name.
		gitBranch := os.Getenv("GITHUB_HEAD_REF")
		if gitBranch == "" {
			gitBranchCmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
			gitBranchCmd.Dir = path
			gitBranchOutput, err := gitBranchCmd.Output()
			if err != nil {
				return fmt.Errorf("CI mode: failed to get git branch name: %w", err)
			}
			gitBranch = strings.TrimSpace(string(gitBranchOutput))
		}

		gitShaCmd := exec.Command("git", "rev-parse", "HEAD")
		gitShaCmd.Dir = path
		gitShaOutput, err := gitShaCmd.Output()
		if err != nil {
			return fmt.Errorf("CI mode: failed to get git commit SHA: %w", err)
		}
		gitSha := strings.TrimSpace(string(gitShaOutput))

		args = append(args,
			"-e", "CI_MODE=true",
			"-e", "GIT_REMOTE="+gitRemote,
			"-e", "GIT_BRANCH="+gitBranch,
			"-e", "GIT_SHA="+gitSha,
			"-e", "ROLE_NAME="+opts.RoleFlag,
			"-e", "ORG_NAME="+opts.OrgFlag,
		)
		log.Printf(config.ColorGreen+"CI Mode: Will clone %s (branch: %s, commit: %s) inside container"+config.ColorReset, gitRemote, gitBranch, gitSha[:8])
	}

	// Add cgroup mount only if it exists (may not be available —WSL2)
	if _, err := os.Stat("/sys/fs/cgroup"); err == nil {
		args = append(args, "-v", "/sys/fs/cgroup:/sys/fs/cgroup:rw")
	}

	// Add cache volume mounts if enabled (non-CI mode only; CI mode uses docker cp)
	if !opts.CIMode && cfg.CacheConfig != nil && cfg.CacheConfig.Enabled && cfg.CacheConfig.CacheID != "" {
		cacheDir, err := cache.EnsureCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
		if err != nil {
			log.Printf(config.ColorYellow+"warning: failed to create cache directory: %v"+config.ColorReset, err)
		} else {
			// Roles and collections cache (always mounted when cache is enabled)
			rolesDir := filepath.Join(cacheDir, config.CacheRolesDir)
			collectionsDir := filepath.Join(cacheDir, config.CacheCollectionsDir)

			if err := os.MkdirAll(rolesDir, 0755); err != nil {
				log.Printf(config.ColorYellow+"warning: failed to create roles cache directory: %v"+config.ColorReset, err)
			}
			if err := os.MkdirAll(collectionsDir, 0755); err != nil {
				log.Printf(config.ColorYellow+"warning: failed to create collections cache directory: %v"+config.ColorReset, err)
			}

			args = append(args, "-v", fmt.Sprintf("%s:%s", rolesDir, config.ContainerRolesCachePath))
			args = append(args, "-v", fmt.Sprintf("%s:%s", collectionsDir, config.ContainerCollectionsCachePath))
			log.Printf(config.ColorGreen+"Cache enabled: mounting roles and collections from %s"+config.ColorReset, cacheDir)

			// UV/Python package cache mount
			if cfg.CacheConfig.UVCache {
				uvDir, err := cache.EnsureUVCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
				if err != nil {
					log.Printf(config.ColorYellow+"warning: failed to create UV cache directory: %v"+config.ColorReset, err)
				} else {
					// On Windows, mount to a staging path (precache) instead of the real
					// cache path. NTFS-mounted volumes are too slow for UV operations.
					// The precache contents are copied to the native ext4 cache on start
					// and saved back on wipe.
					uvContainerPath := config.ContainerUVCachePath
					if runtime.GOOS == "windows" {
						uvContainerPath = config.ContainerUVPrecachePath
					}
					args = append(args, "-v", fmt.Sprintf("%s:%s", uvDir, uvContainerPath))
					log.Printf(config.ColorGreen+"UV cache enabled: mounting %s -> %s"+config.ColorReset, uvDir, uvContainerPath)
				}
			}

			// Docker/DinD image cache mount
			if cfg.CacheConfig.DockerCache {
				dockerDir, err := cache.EnsureDockerCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
				if err != nil {
					log.Printf(config.ColorYellow+"warning: failed to create Docker cache directory: %v"+config.ColorReset, err)
				} else {
					args = append(args, "-v", fmt.Sprintf("%s:%s", dockerDir, config.ContainerDockerCachePath))
					log.Printf(config.ColorGreen+"Docker cache enabled: mounting %s -> %s"+config.ColorReset, dockerDir, config.ContainerDockerCachePath)
				}
			}
		}
	}

	// Add all indexed GIT environment variables
	for i := 1; i <= config.MaxArtifactSources; i++ {
		gitUser := os.Getenv(fmt.Sprintf("%s%d", config.EnvGitUserPrefix, i))
		gitPassword := os.Getenv(fmt.Sprintf("%s%d", config.EnvGitPassPrefix, i))
		gitURL := os.Getenv(fmt.Sprintf("%s%d", config.EnvGitURLPrefix, i))

		if gitUser != "" || gitPassword != "" || gitURL != "" {
			args = append(args, "-e", fmt.Sprintf("%s%d=%s", config.EnvGitUserPrefix, i, gitUser))
			args = append(args, "-e", fmt.Sprintf("%s%d=%s", config.EnvGitPassPrefix, i, gitPassword))
			args = append(args, "-e", fmt.Sprintf("%s%d=%s", config.EnvGitURLPrefix, i, gitURL))
		}
	}

	args = append(args, "--cgroupns", "host", "--privileged", "--pull", "always", image)

	// Run docker with error capture for better debugging
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf(config.ColorRed+"docker run failed: %v"+config.ColorReset, err)
		if len(output) > 0 {
			log.Printf(config.ColorRed+"Docker error output: %s"+config.ColorReset, string(output))
		}

		// Check for common WSL2 credential helper issue
		if strings.Contains(string(output), "docker-credential-desktop.exe") {
			log.Printf(config.ColorYellow + "\nWSL2 Docker credential issue detected!" + config.ColorReset)
			log.Printf(config.ColorYellow + "To fix this, edit ~/.docker/config.json and either:" + config.ColorReset)
			log.Printf(config.ColorYellow + "  1. Remove the 'credsStore' line, OR" + config.ColorReset)
			log.Printf(config.ColorYellow + "  2. Change 'credsStore': 'desktop.exe' to 'credsStore': 'desktop'" + config.ColorReset)
			log.Printf(config.ColorYellow + "\nExample fix: sed -i 's/desktop.exe/desktop/g' ~/.docker/config.json" + config.ColorReset)
		}

		return err
	}

	return nil
}

// setupCIRepository clones the repo and sets up role files inside the container.
func setupCIRepository(opts *MoleculeOptions, hostPath, roleDirName string) error {
	log.Printf(config.ColorGreen + "CI Mode: Setting up repository inside container..." + config.ColorReset)

	// GIT_BRANCH is now reliable for both push and pull_request events:
	// - push: resolved from git rev-parse --abbrev-ref HEAD
	// - pull_request: resolved from GITHUB_HEAD_REF (the PR source branch)
	// We clone only that branch (--single-branch) for speed, then the
	// checkout lands on the correct branch tip.
	cloneCmd := `cd /tmp && rm -rf repo && git clone --single-branch --branch "$GIT_BRANCH" "$GIT_REMOTE" repo`
	if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cloneCmd); err != nil {
		return fmt.Errorf("failed to clone repository —container: %w", err)
	}
	log.Printf(config.ColorGreen + "CI Mode: Repository cloned to /tmp/repo (commit: $GIT_SHA)" + config.ColorReset)

	mkdirCmd := fmt.Sprintf("mkdir -p /opt/molecule/%s", roleDirName)
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", mkdirCmd); err != nil {
		return fmt.Errorf("failed to create role directory —container: %w", err)
	}

	copyDirs := []string{"tasks", "defaults", "meta", "handlers", "templates", "files", "vars"}
	for _, dir := range copyDirs {
		copyCmd := fmt.Sprintf("if [ -d /tmp/repo/%s ]; then cp -r /tmp/repo/%s /opt/molecule/%s/; fi", dir, dir, roleDirName)
		// Best-effort: role subdirs (tasks, defaults, etc.) may not all exist.
		_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", copyCmd)
	}

	// Lowercase the namespace field —meta/main.yml inside the container so it
	// matches the lowercased roleDirName used for the directory structure.
	metaFixCmd := fmt.Sprintf(
		`if [ -f /opt/molecule/%s/meta/main.yml ]; then sed -i 's/^\(\s*namespace:\s*\).*/\1%s/' /opt/molecule/%s/meta/main.yml; fi`,
		roleDirName, opts.OrgFlag, roleDirName)
	// Best-effort: meta/main.yml may not exist —all roles.
	_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", metaFixCmd)

	copyScenarios := fmt.Sprintf("mkdir -p /opt/molecule/%s/molecule && if [ -d /tmp/repo/scenarios ]; then cp -r /tmp/repo/scenarios/. /opt/molecule/%s/molecule/; fi", roleDirName, roleDirName)
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", copyScenarios); err != nil {
		return fmt.Errorf("failed to copy scenarios —container: %w", err)
	}

	copyLintCmd := fmt.Sprintf("if [ -f /tmp/repo/.ansible-lint ]; then cp /tmp/repo/.ansible-lint /opt/molecule/%s/; fi && if [ -f /tmp/repo/.yamllint ]; then cp /tmp/repo/.yamllint /opt/molecule/%s/; fi", roleDirName, roleDirName)
	// Best-effort: linter config files may not be present —the repository.
	_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", copyLintCmd)

	// Copy host-side files that may have been updated by commands like
	// "diffusion deps lock" / "diffusion deps sync" before molecule was invoked.
	// The cloned repo inside the container has the remote (stale) versions;
	// overwrite them with the runner's current copies so tests use the
	// freshly resolved dependencies.
	containerName := fmt.Sprintf("molecule-%s", opts.RoleFlag)
	hostOverrideFiles := []string{config.LockFileName, "diffusion.toml"}
	for _, fname := range hostOverrideFiles {
		hostFile := filepath.Join(hostPath, fname)
		if _, err := os.Stat(hostFile); err == nil {
			dest := fmt.Sprintf("%s:/opt/molecule/%s/%s", containerName, roleDirName, fname)
			if cpErr := exec.Command("docker", "cp", hostFile, dest).Run(); cpErr != nil {
				log.Printf(config.ColorYellow+"warning: failed to copy %s into container: %v"+config.ColorReset, fname, cpErr)
			} else {
				log.Printf(config.ColorGreen+"CI Mode: copied host %s into container"+config.ColorReset, fname)
			}
		}
	}
	// Also copy scenario-level requirements.yml files that deps sync may have updated
	scenariosHostPath := filepath.Join(hostPath, config.ScenariosDir)
	if entries, err := os.ReadDir(scenariosHostPath); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			reqFile := filepath.Join(scenariosHostPath, entry.Name(), "requirements.yml")
			if _, err := os.Stat(reqFile); err == nil {
				dest := fmt.Sprintf("%s:/opt/molecule/%s/molecule/%s/requirements.yml", containerName, roleDirName, entry.Name())
				if cpErr := exec.Command("docker", "cp", reqFile, dest).Run(); cpErr != nil {
					log.Printf(config.ColorYellow+"warning: failed to copy scenarios/%s/requirements.yml into container: %v"+config.ColorReset, entry.Name(), cpErr)
				} else {
					log.Printf(config.ColorGreen+"CI Mode: copied host scenarios/%s/requirements.yml into container"+config.ColorReset, entry.Name())
				}
			}
		}
	}

	log.Printf(config.ColorGreen+"CI Mode: Role files copied to /opt/molecule/%s"+config.ColorReset, roleDirName)

	verifyCmd := fmt.Sprintf("ls -la /opt/molecule/%s/molecule/default/molecule.yml", roleDirName)
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", verifyCmd); err != nil {
		log.Printf(config.ColorRed + "CI Mode: molecule.yml not found!" + config.ColorReset)
		// Best-effort debug listing — output shown regardless of success/failure.
		_ = utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("ls -laR /opt/molecule/%s/", roleDirName))
		return fmt.Errorf("molecule.yml not found —container")
	}
	log.Printf(config.ColorGreen + "CI Mode: Setup complete!" + config.ColorReset)

	return nil
}

// ensureRole initializes or validates the role directory inside the container.
func ensureRole(opts *MoleculeOptions, roleMoleculePath string) {
	if utils.Exists(roleMoleculePath) {
		fmt.Println(config.ColorMagenta + "This role already exists  in molecule" + config.ColorReset)
	} else {
		if err := utils.DockerExecInteractive(
			opts.RoleFlag, "/bin/sh", opts.CIMode,
			"-c", fmt.Sprintf("ansible-galaxy role init %s.%s", opts.OrgFlag, opts.RoleFlag)); err != nil {
			log.Printf(config.ColorYellow+"role init warning: %v"+config.ColorReset, err)
		}

		// Fix ownership inside container after role init (Unix systems only)
		if runtime.GOOS != "windows" {
			uid := os.Getuid()
			gid := os.Getgid()
			chownCmd := fmt.Sprintf("chown -R %d:%d /opt/molecule/%s.%s", uid, gid, opts.OrgFlag, opts.RoleFlag)
			if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", chownCmd); err != nil {
				log.Printf(config.ColorYellow+"warning: failed to fix ownership after role init: %v"+config.ColorReset, err)
			}
		}

		if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("rm -f %s.%s/*/*", opts.OrgFlag, opts.RoleFlag)); err != nil {
			log.Printf(config.ColorYellow+"clean role dir warning: %v"+config.ColorReset, err)
		}
	}
}

// loginInsideContainer performs docker log—inside the container (provider-specific).
func loginInsideContainer(opts *MoleculeOptions, cfg *config.Config) {
	switch cfg.ContainerRegistry.RegistryProvider {
	case config.RegistryProviderYC:
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", `echo $TOKEN | docker login cr.yandex --username iam --password-stdin`); err != nil {
			log.Printf(config.ColorYellow+"warning: docker login inside container (YC) failed: %v"+config.ColorReset, err)
		}
	case config.RegistryProviderAWS:
		loginCmd := fmt.Sprintf(`echo $TOKEN | docker login %s --username AWS --password-stdin`, cfg.ContainerRegistry.RegistryServer)
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", loginCmd); err != nil {
			log.Printf(config.ColorYellow+"warning: docker login inside container (AWS) failed: %v"+config.ColorReset, err)
		}
	case config.RegistryProviderGCP:
		loginCmd := fmt.Sprintf(`echo $TOKEN | docker login %s --username oauth2accesstoken --password-stdin`, cfg.ContainerRegistry.RegistryServer)
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", loginCmd); err != nil {
			log.Printf(config.ColorYellow+"warning: docker login inside container (GCP) failed: %v"+config.ColorReset, err)
		}
	case config.RegistryProviderPublic:
		log.Printf(config.ColorMagenta + "Using public registry, skipping authentication" + config.ColorReset)
	default:
		log.Printf(config.ColorYellow+"Unknown registry provider '%s', skipping authentication"+config.ColorReset, cfg.ContainerRegistry.RegistryProvider)
	}
}

// copyCacheIntoContainer copies cached roles, collections, UV packages, and Docker
// image tarballs FROM the host cache directory INTO the running container using
// "docker cp". This is used —CI mode where volume mounts (-v) are unavailable.
func copyCacheIntoContainer(opts *MoleculeOptions, cfg *config.Config) {
	if cfg.CacheConfig == nil || !cfg.CacheConfig.Enabled || cfg.CacheConfig.CacheID == "" {
		return
	}

	cacheDir, err := cache.GetCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
	if err != nil {
		log.Printf(config.ColorYellow+"warning: failed to resolve cache directory: %v"+config.ColorReset, err)
		return
	}

	containerName := fmt.Sprintf("molecule-%s", opts.RoleFlag)

	// Helper: docker cp <hostPath>/. <container>:<containerPath>
	// The "/." suffix copies the directory *contents* (not the directory itself).
	copyDir := func(hostSubdir, containerPath, label string) {
		hostPath := filepath.Join(cacheDir, hostSubdir)
		if info, err := os.Stat(hostPath); err != nil || !info.IsDir() {
			return // nothing to copy
		}
		// Ensure target directory exists inside container
		mkdirCmd := fmt.Sprintf("mkdir -p %s", containerPath)
		// Best-effort: mkdir -p never fails on a running container.
		_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", mkdirCmd)

		src := hostPath + string(os.PathSeparator) + "."
		if err := exec.Command("docker", "cp", src, containerName+":"+containerPath).Run(); err != nil {
			log.Printf(config.ColorYellow+"warning: failed to copy %s cache into container: %v"+config.ColorReset, label, err)
		} else {
			log.Printf(config.ColorGreen+"CI cache: copied %s into container"+config.ColorReset, label)
		}
	}

	// Roles & collections (always when cache is enabled)
	copyDir(config.CacheRolesDir, config.ContainerRolesCachePath, "roles")
	copyDir(config.CacheCollectionsDir, config.ContainerCollectionsCachePath, "collections")

	// UV cache
	if cfg.CacheConfig.UVCache {
		copyDir(config.CacheUVDir, config.ContainerUVCachePath, "uv")
	}

	// Docker image tarball — copy the whole docker/ directory so that
	// loadDinDImages can find images.tar at /root/.cache/docker/images.tar
	if cfg.CacheConfig.DockerCache {
		copyDir(config.CacheDockerDir, config.ContainerDockerCachePath, "docker")
	}
}

// copyCacheFromContainer copies cached roles, collections, UV packages, and
// Docker image tarballs FROM the running container back to the host cache
// directory using "docker cp". Called —CI mode during --wipe, before the
// container is removed.
func copyCacheFromContainer(opts *MoleculeOptions, cfg *config.Config) {
	if cfg.CacheConfig == nil || !cfg.CacheConfig.Enabled || cfg.CacheConfig.CacheID == "" {
		return
	}

	cacheDir, err := cache.EnsureCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
	if err != nil {
		log.Printf(config.ColorYellow+"warning: failed to ensure cache directory: %v"+config.ColorReset, err)
		return
	}

	containerName := fmt.Sprintf("molecule-%s", opts.RoleFlag)

	// Helper: docker cp <container>:<containerPath>/. <hostPath>
	copyDir := func(containerPath, hostSubdir, label string) {
		hostPath := filepath.Join(cacheDir, hostSubdir)
		if err := os.MkdirAll(hostPath, 0755); err != nil {
			log.Printf(config.ColorYellow+"warning: failed to create host cache dir for %s: %v"+config.ColorReset, label, err)
			return
		}

		src := containerName + ":" + containerPath + "/."
		if err := exec.Command("docker", "cp", src, hostPath).Run(); err != nil {
			log.Printf(config.ColorYellow+"warning: failed to copy %s cache from container: %v"+config.ColorReset, label, err)
		} else {
			log.Printf(config.ColorGreen+"CI cache: saved %s from container"+config.ColorReset, label)
		}
	}

	// Roles & collections
	copyDir(config.ContainerRolesCachePath, config.CacheRolesDir, "roles")
	copyDir(config.ContainerCollectionsCachePath, config.CacheCollectionsDir, "collections")

	// UV cache
	if cfg.CacheConfig.UVCache {
		copyDir(config.ContainerUVCachePath, config.CacheUVDir, "uv")
	}

	// Docker image tarball directory
	if cfg.CacheConfig.DockerCache {
		copyDir(config.ContainerDockerCachePath, config.CacheDockerDir, "docker")
	}
}

// loadDinDImages loads cached Docker images into the DinD daemon running
// inside the molecule container. It runs:
//
//	docker exec <container> docker load -i /root/.cache/docker/images.tar
//
// This works —both CI and non-CI modes — the tarball is either volume-mounted
// (non-CI) or copied —via copyCacheIntoContainer (CI).
func loadDinDImages(opts *MoleculeOptions) {
	// Container paths are always Linux — use forward slashes, never filepath.Join.
	tarballPath := fmt.Sprintf("%s/%s", config.ContainerDockerCachePath, config.DockerImageTarball)

	// Check if the tarball exists inside the container
	checkCmd := fmt.Sprintf("test -f %s", tarballPath)
	if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", checkCmd); err != nil {
		log.Printf(config.ColorYellow+"No cached Docker images found at %s, skipping load"+config.ColorReset, tarballPath)
		return
	}

	// Wait for the inner DinD Docker daemon to be ready.
	// The container may have just started and dockerd needs time to initialize.
	containerName := fmt.Sprintf("molecule-%s", opts.RoleFlag)
	const maxRetries = 30
	dockerReady := false
	for i := range maxRetries {
		checkDocker := exec.Command("docker", "exec", containerName, "docker", "info")
		checkDocker.Stdout = io.Discard
		checkDocker.Stderr = io.Discard
		if err := checkDocker.Run(); err == nil {
			dockerReady = true
			break
		}
		log.Printf(config.ColorYellow+"Waiting for DinD daemon to start... (%d/%d)"+config.ColorReset, i+1, maxRetries)
		time.Sleep(2 * time.Second)
	}
	if !dockerReady {
		log.Printf(config.ColorYellow + "warning: DinD daemon did not start —time, skipping image load" + config.ColorReset)
		return
	}

	// Load the images into the inner Docker daemon.
	// Never use -ti here: shell redirection (< file) conflicts with TTY allocation,
	// and we don't need interactive terminal for this operation.
	loadCmd := fmt.Sprintf("docker load < %s", tarballPath)
	execFlags := []string{"exec", containerName, "sh", "-c", loadCmd}
	cmd := exec.Command("docker", execFlags...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf(config.ColorYellow+"warning: failed to load DinD images from cache (%s): %v"+config.ColorReset, tarballPath, err)
		if len(output) > 0 {
			log.Printf(config.ColorYellow+"Docker load output: %s"+config.ColorReset, strings.TrimSpace(string(output)))
		}
	} else {
		log.Printf(config.ColorGreen + "DinD images loaded from cache" + config.ColorReset)
	}
}

// saveDinDImages discovers all Docker images inside the DinD daemon running in
// the molecule container and saves them to a single tarball at
// /root/.cache/docker/images.tar.  It runs:
//
//	docker exec <container> docker images --format '{{.Repository}}:{{.Tag}}'
//	docker exec <container> docker save -o /root/.cache/docker/images.tar <image1> <image2> ...
//
// This works —both CI and non-CI modes. In non-CI mode the tarball persists
// on the host automatically via the volume mount. In CI mode the caller must
// follow up with copyCacheFromContainer to pull it out.
func saveDinDImages(opts *MoleculeOptions) {
	containerName := fmt.Sprintf("molecule-%s", opts.RoleFlag)

	// Discover images inside the DinD daemon.
	// We need to capture stdout, so we use exec.Command directly here.
	// Build flags the same way DockerExecInteractiveHide does.
	// Never use -ti here: we need to capture stdout programmatically via .Output(),
	// and -t (TTY allocation) fails when stdout is not a real terminal.
	execFlags := []string{"exec", containerName, "sh", "-c",
		`docker images --format '{{.Repository}}:{{.Tag}}'`}
	out, err := exec.Command("docker", execFlags...).Output()
	if err != nil {
		log.Printf(config.ColorYellow+"warning: failed to list DinD images: %v"+config.ColorReset, err)
		return
	}

	// Parse and filter out <none>:<none> entries
	var images []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "<none>:<none>" {
			continue
		}
		images = append(images, line)
	}

	if len(images) == 0 {
		log.Printf(config.ColorYellow + "No DinD images to cache" + config.ColorReset)
		return
	}

	log.Printf(config.ColorGreen+"Saving %d DinD image(s) to cache: %s"+config.ColorReset, len(images), strings.Join(images, ", "))

	// Ensure the cache directory exists inside the container
	mkdirCmd := fmt.Sprintf("mkdir -p %s", config.ContainerDockerCachePath)
	// Best-effort: mkdir -p never fails on a running container.
	_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", mkdirCmd)

	// Container paths are always Linux — use forward slashes, never filepath.Join.
	tarballPath := fmt.Sprintf("%s/%s", config.ContainerDockerCachePath, config.DockerImageTarball)
	saveCmd := fmt.Sprintf("docker save %s > %s", strings.Join(images, " "), tarballPath)
	if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", saveCmd); err != nil {
		log.Printf(config.ColorYellow+"warning: failed to save DinD images to cache: %v"+config.ColorReset, err)
	} else {
		log.Printf(config.ColorGreen+"DinD images saved to %s"+config.ColorReset, tarballPath)
	}
}

// loadUVPrecache extracts the cached UV tarball from the NTFS-mounted staging
// path (/root/.precache/uv/uv-cache.tar) into the native ext4 cache path
// (/root/.cache/uv). Using a single tarball instead of copying thousands of
// tiny files is significantly faster across the NTFS filesystem boundary.
// This is only needed on Windows where direct NTFS volume mounts are too slow
// for UV operations. On Linux/macOS the cache is mounted directly.
func loadUVPrecache(opts *MoleculeOptions) {
	tarball := fmt.Sprintf("%s/%s", config.ContainerUVPrecachePath, config.UVCacheTarball)
	cachePath := config.ContainerUVCachePath

	// Check if tarball exists
	checkCmd := fmt.Sprintf("test -f %s", tarball)
	if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", checkCmd); err != nil {
		log.Printf(config.ColorYellow+"No UV cache tarball found at %s, skipping"+config.ColorReset, tarball)
		return
	}

	// Extract tarball into native cache path
	extractCmd := fmt.Sprintf("mkdir -p %s && tar xf %s -C %s", cachePath, tarball, cachePath)
	if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", extractCmd); err != nil {
		log.Printf(config.ColorYellow+"warning: failed to extract UV cache tarball: %v"+config.ColorReset, err)
	} else {
		log.Printf(config.ColorGreen + "UV cache extracted from tarball into native cache" + config.ColorReset)
	}
}

// saveUVCacheToPrecache archives the UV cache from the native ext4 path
// (/root/.cache/uv) into a single tarball at the NTFS-mounted staging path
// (/root/.precache/uv/uv-cache.tar). Writing one large file to NTFS is much
// faster than copying many small files. The volume mount syncs the tarball
// back to the Windows host automatically.
// Called during --wipe before the container is removed.
func saveUVCacheToPrecache(opts *MoleculeOptions) {
	tarball := fmt.Sprintf("%s/%s", config.ContainerUVPrecachePath, config.UVCacheTarball)
	cachePath := config.ContainerUVCachePath

	// Check if cache has any content
	checkCmd := fmt.Sprintf("test -d %s && [ \"$(ls -A %s 2>/dev/null)\" ]", cachePath, cachePath)
	if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", checkCmd); err != nil {
		log.Printf(config.ColorYellow + "UV cache is empty, skipping save" + config.ColorReset)
		return
	}

	// Archive cache into a single tarball on the NTFS mount
	archiveCmd := fmt.Sprintf("tar cf %s -C %s .", tarball, cachePath)
	if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", archiveCmd); err != nil {
		log.Printf(config.ColorYellow+"warning: failed to archive UV cache to tarball: %v"+config.ColorReset, err)
	} else {
		log.Printf(config.ColorGreen+"UV cache archived to %s (synced to host)"+config.ColorReset, tarball)
	}
}
