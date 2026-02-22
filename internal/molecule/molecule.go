package molecule

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

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
}

// RunMolecule is the core function that implements the molecule workflow.
// It handles wipe, converge, lint, verify, idempotence, destroy and the
// default create/converge flow.
func RunMolecule(opts *MoleculeOptions) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("\033[33mwarning loading config: %v\033[0m", err)
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
// Before removing the container, it saves DinD images and (in CI mode) copies
// the cache out of the container back to the host.
func handleWipe(opts *MoleculeOptions, cfg *config.Config, roleDirName, roleMoleculePath string) error {
	log.Printf("\033[38;2;127;255;212mWiping: running molecule destroy, removing container molecule-%s and folder %s\n\033[0m", opts.RoleFlag, roleMoleculePath)

	// Run molecule destroy inside the container first
	roleDir := utils.GetRoleDirName(opts.OrgFlag, opts.RoleFlag)
	_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "bash", opts.CIMode, "-c", fmt.Sprintf("cd ./%s && molecule destroy", roleDir))

	// Save DinD images before removing the container
	if cfg.CacheConfig != nil && cfg.CacheConfig.Enabled && cfg.CacheConfig.DockerCache {
		saveDinDImages(opts)
	}

	// CI mode: copy cache from container back to host before docker rm
	if opts.CIMode {
		copyCacheFromContainer(opts, cfg)
	}

	// Remove the container
	_ = utils.RunCommandHide("docker", "rm", fmt.Sprintf("molecule-%s", opts.RoleFlag), "-f")

	// Remove the role folder
	if err := os.RemoveAll(roleMoleculePath); err != nil {
		log.Printf("\033[33mwarning: failed remove role path: %v\033[0m", err)
	}
	return nil
}

// handleSubcommands handles --converge, --lint, --verify, --idempotence, --destroy flags.
func handleSubcommands(opts *MoleculeOptions, cfg *config.Config, path, roleDirName, roleMoleculePath string) error {
	if !opts.CIMode {
		if err := utils.CopyRoleData(path, roleMoleculePath, opts.CIMode); err != nil {
			log.Printf("\033[33mwarning copying data: %v\033[0m", err)
		}
	}

	linters := roleMoleculePath
	// In CI mode, set linters to Org.Role format
	if opts.CIMode {
		linters = fmt.Sprintf("%s.%s", opts.OrgFlag, opts.RoleFlag)
	}

	utils.ExportLinters(cfg, linters, opts.CIMode, opts.RoleFlag, opts.OrgFlag)

	// Determine scenario name for tests directory
	scenario := config.DefaultScenario
	if opts.RoleScenario != "" {
		scenario = opts.RoleScenario
	}

	// Create tests directory for verify/lint
	moleculeDefaultTestsPath := fmt.Sprintf("molecule/%s.%s/molecule/%s/tests", opts.OrgFlag, opts.RoleFlag, scenario)
	if err := os.MkdirAll(moleculeDefaultTestsPath, 0o755); err != nil {
		log.Printf("\033[33mwarning: cannot create scenario tests dir: %v\033[0m", err)
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
			log.Printf("\033[31mmolecule.yml not found in container at /opt/molecule/%s/molecule/default/\033[0m", roleDirName)
			log.Printf("\033[33mListing container directory structure:\033[0m")
			_ = utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("ls -laR /opt/molecule/%s/", roleDirName))
			os.Exit(1)
		}
	}

	tagEnv := ""
	if opts.TagFlag != "" {
		tagEnv = fmt.Sprintf("ANSIBLE_RUN_TAGS=%s ", opts.TagFlag)
	}
	cmdStr := fmt.Sprintf("cd ./%s && %smolecule converge", roleDirName, tagEnv)
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdStr); err != nil {
		log.Printf("\033[31mConverge failed: %v\033[0m", err)
		os.Exit(1)
	}
	log.Printf("\033[32mConverge Done Successfully!\033[0m")

	// Fix permissions on molecule directory for Unix systems (inside container)
	if runtime.GOOS != "windows" {
		uid := os.Getuid()
		gid := os.Getgid()
		log.Printf("User UID: %d, GID: %d", uid, gid)
		chownCmd := fmt.Sprintf("chown -R %d:%d /opt/molecule", uid, gid)
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", chownCmd); err != nil {
			log.Printf("\033[33mwarning: failed to fix permissions: %v\033[0m", err)
		}
	}

	return nil
}

// runLint runs yamllint and ansible-lint inside the container.
func runLint(opts *MoleculeOptions, roleDirName string) error {
	cmdStr := fmt.Sprintf(`cd ./%s && yamllint . -c .yamllint && ansible-lint -c .ansible-lint `, roleDirName)
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdStr); err != nil {
		log.Printf("\033[31mLint failed: %v\033[0m", err)
		os.Exit(1)
	}
	log.Printf("\033[32mLint Done Successfully!\033[0m")
	return nil
}

// runVerify handles test source resolution (local/remote/diffusion) and runs molecule verify.
func runVerify(opts *MoleculeOptions, cfg *config.Config, path, roleDirName, roleMoleculePath, scenario string) error {
	switch cfg.TestsConfig.Type {
	case config.TestsTypeLocal:
		verifyLocalTests(opts, path, roleMoleculePath, scenario)
	case config.TestsTypeRemote:
		if len(cfg.TestsConfig.RemoteRepositories) == 0 {
			return fmt.Errorf("\033[31mno remote repository configured for tests type 'remote'\033[0m")
		}
		verifyRemoteTests(opts, cfg, roleMoleculePath, scenario)
	case config.TestsTypeDiffusion:
		if err := verifyDiffusionTests(opts, roleMoleculePath, scenario); err != nil {
			return err
		}
	default:
		return fmt.Errorf("\033[31munknown tests type: %s\033[0m", cfg.TestsConfig.Type)
	}

	// run molecule verify
	cmdStr := fmt.Sprintf("cd ./%s && molecule verify", roleDirName)
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdStr); err != nil {
		log.Printf("\033[31mVerify failed: %v\033[0m", err)
		os.Exit(1)
	}
	log.Printf("\033[32mVerify Done Successfully!\033[0m")
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
			git clone "${GIT_REMOTE}" repo && \
			cd repo && \
			git checkout "${GIT_SHA}" && \
			cp -rf /tmp/repo/tests /opt/molecule/%s.%s/molecule/%s/
		`, opts.OrgFlag, opts.RoleFlag, scenario)
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdCopy); err != nil {
			log.Printf("\033[33mwarning: failed to copy tests in CI mode: %v\033[0m", err)
		}
	} else {
		if utils.Exists(testsSrc) {
			testsDst := filepath.Join(roleMoleculePath, config.MoleculeDir, scenario, config.TestsDir)
			utils.CopyIfExists(testsSrc, testsDst)
		} else {
			log.Printf("\033[33mtests/ directory not found, skipping copy\033[0m")
		}
	}
}

// verifyRemoteTests clones test files from remote repositories.
func verifyRemoteTests(opts *MoleculeOptions, cfg *config.Config, roleMoleculePath, scenario string) {
	for _, remoteRepo := range cfg.TestsConfig.RemoteRepositories {
		log.Printf("\033[32mInstalling test files from remote repository: %s\033[0m", remoteRepo)

		if !opts.TestsOverWrite {
			if opts.CIMode {
				cmdRemoteTests := fmt.Sprintf(`
				cd /opt/molecule/%s.%s/molecule/%s && \
				if [ ! -d tests ]; then \
					git clone %s tests; \
				else \
					echo "Tests directory already exists, skipping clone"; \
				fi
			`, opts.OrgFlag, opts.RoleFlag, scenario, remoteRepo)
				if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdRemoteTests); err != nil {
					log.Printf("\033[33mwarning: failed to clone remote tests in CI mode: %v\033[0m", err)
				}
			} else {
				testsDst := filepath.Join(roleMoleculePath, config.MoleculeDir, scenario, config.TestsDir)
				if _, err := os.Stat(testsDst); os.IsNotExist(err) {
					cmdRemoteTests := fmt.Sprintf(`
					cd %s && \
					git clone %s tests
				`, filepath.Join(roleMoleculePath, config.MoleculeDir, scenario), remoteRepo)
					if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdRemoteTests); err != nil {
						log.Printf("\033[33mwarning: failed to clone remote tests: %v\033[0m", err)
					}
				} else {
					log.Printf("\033[33mTests directory already exists, skipping clone\033[0m")
				}
			}
		} else {
			// Overwrite tests directory with remote repository
			if opts.CIMode {
				cmdRemoteTests := fmt.Sprintf(`
				cd /opt/molecule/%s.%s/molecule/%s && \
				rm -rf tests && \
				git clone %s tests
			`, opts.OrgFlag, opts.RoleFlag, scenario, remoteRepo)
				if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdRemoteTests); err != nil {
					log.Printf("\033[33mwarning: failed to clone remote tests in CI mode: %v\033[0m", err)
				}
			} else {
				testsDst := filepath.Join(roleMoleculePath, config.MoleculeDir, scenario, config.TestsDir)
				_ = os.RemoveAll(testsDst)
				cmdRemoteTests := fmt.Sprintf(`
				cd %s && \
				git clone %s tests
			`, filepath.Join(roleMoleculePath, config.MoleculeDir, scenario), remoteRepo)
				if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdRemoteTests); err != nil {
					log.Printf("\033[33mwarning: failed to clone remote tests: %v\033[0m", err)
				}
			}
		}
	}
}

// verifyDiffusionTests clones/updates diffusion-managed test files.
func verifyDiffusionTests(opts *MoleculeOptions, roleMoleculePath, scenario string) error {
	log.Printf("\033[32mUsing diffusion-managed test files\033[0m")

	diffusionTestsPath := filepath.Join(os.TempDir(), "diffusion-tests-repo")
	if !opts.TestsOverWrite {
		if _, err := os.Stat(diffusionTestsPath); os.IsNotExist(err) {
			log.Printf("\033[32mCloning diffusion tests repository...\033[0m")
			if err := exec.Command("git", "clone", "https://github.com/your-org/diffusion-tests", diffusionTestsPath).Run(); err != nil {
				return fmt.Errorf("\033[31mfailed to clone diffusion tests repository: %w\033[0m", err)
			}
		} else {
			log.Printf("\033[32mUpdating diffusion tests repository...\033[0m")
			gitPullCmd := exec.Command("git", "pull")
			gitPullCmd.Dir = diffusionTestsPath
			if err := gitPullCmd.Run(); err != nil {
				log.Printf("\033[33mwarning: failed to update diffusion tests repository: %v\033[0m", err)
			}
		}
	} else {
		_ = os.RemoveAll(diffusionTestsPath)
		log.Printf("\033[32mCloning diffusion tests repository (overwrite mode)...\033[0m")
		if err := exec.Command("git", "clone", "https://github.com/your-org/diffusion-tests", diffusionTestsPath).Run(); err != nil {
			return fmt.Errorf("\033[31mfailed to clone diffusion tests repository: %w\033[0m", err)
		}
	}

	// Copy tests from diffusion repo to role tests directory
	if opts.CIMode {
		cmdCopy := fmt.Sprintf(`
			cp -rf %s /opt/molecule/%s.%s/molecule/%s/tests
		`, diffusionTestsPath, opts.OrgFlag, opts.RoleFlag, scenario)
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdCopy); err != nil {
			log.Printf("\033[33mwarning: failed to copy diffusion tests in CI mode: %v\033[0m", err)
		}
	} else {
		testsDst := filepath.Join(roleMoleculePath, config.MoleculeDir, scenario, config.TestsDir)
		utils.CopyIfExists(diffusionTestsPath, testsDst)
	}

	return nil
}

// runIdempotence runs molecule idempotence inside the container.
func runIdempotence(opts *MoleculeOptions, roleDirName string) error {
	tagEnv := ""
	if opts.TagFlag != "" {
		tagEnv = fmt.Sprintf("ANSIBLE_RUN_TAGS=%s ", opts.TagFlag)
	}
	cmdStr := fmt.Sprintf("cd ./%s && %smolecule idempotence", roleDirName, tagEnv)
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdStr); err != nil {
		log.Printf("\033[31mIdempotence failed: %v\033[0m", err)
		os.Exit(1)
	}
	log.Printf("\033[32mIdempotence Done Successfully!\033[0m")
	return nil
}

// runDestroy runs molecule destroy inside the container.
func runDestroy(opts *MoleculeOptions, roleDirName string) error {
	cmdStr := fmt.Sprintf("cd ./%s && molecule destroy", roleDirName)
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cmdStr); err != nil {
		log.Printf("\033[31mDestroy failed: %v\033[0m", err)
		os.Exit(1)
	}
	log.Printf("\033[32mDestroy Done Successfully!\033[0m")
	return nil
}

// handleDefaultFlow handles the default molecule workflow: create container, copy data, converge.
func handleDefaultFlow(opts *MoleculeOptions, cfg *config.Config, path, roleDirName, roleMoleculePath string) error {
	// check if container exists
	err := exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", opts.RoleFlag)).Run()
	if err == nil {
		fmt.Printf("\033[38;2;127;255;212mContainer molecule-%s already exists. To purge use --wipe.\n\033[0m", opts.RoleFlag)
	} else {
		// Container does not exist — set up credentials, auth, and run it
		if err := setupCredentials(opts, cfg); err != nil {
			return err
		}

		setupRegistryAuth(cfg)

		if err := runContainer(opts, cfg, path, roleDirName); err != nil {
			return err
		}
	}

	// CI Mode: copy cache into container (replaces volume mounts)
	if opts.CIMode {
		copyCacheIntoContainer(opts, cfg)
	}

	// Load DinD images from cached tarball (both modes)
	if cfg.CacheConfig != nil && cfg.CacheConfig.Enabled && cfg.CacheConfig.DockerCache {
		loadDinDImages(opts)
	}

	// CI Mode: Clone repository and setup files inside container
	if opts.CIMode {
		if err := setupCIRepository(opts, roleDirName); err != nil {
			return err
		}
	}

	// ensure role exists (skip in CI mode - already handled)
	if !opts.CIMode {
		ensureRole(opts, roleMoleculePath)
	}

	// docker exec login to registry inside container (provider-specific)
	loginInsideContainer(opts, cfg)

	// copy files into molecule structure (skip in CI mode - already handled)
	if !opts.CIMode {
		if err := utils.CopyRoleData(path, roleMoleculePath, opts.CIMode); err != nil {
			log.Printf("\033[33mcopy role data warning: %v\033[0m", err)
		}
		utils.ExportLinters(cfg, roleMoleculePath, opts.CIMode, opts.RoleFlag, opts.OrgFlag)
	}

	// finally create/converge
	err = exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", opts.RoleFlag)).Run()
	if err == nil {
		// container exists
		_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "uv-sync", opts.CIMode)
		_ = utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("cd ./%s && molecule converge", roleDirName))
	} else {
		// Sync UV dependencies with pyproject.toml from diffusion
		if err := utils.DockerExecInteractive(opts.RoleFlag, "uv-sync", opts.CIMode); err != nil {
			log.Printf("\033[33mWarning: uv-sync failed: %v\033[0m", err)
			log.Printf("\033[33mContinuing with existing dependencies...\033[0m")
		}
		_ = utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("cd ./%s && molecule create", roleDirName))
		_ = utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("cd ./%s && molecule converge", roleDirName))
	}

	// Fix permissions on molecule directory for Unix systems (skip in CI mode - no volume mount)
	if !opts.CIMode && runtime.GOOS != "windows" {
		uid := os.Getuid()
		gid := os.Getgid()
		chownCmd := fmt.Sprintf("chown -R %d:%d /opt/molecule", uid, gid)
		if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", chownCmd); err != nil {
			log.Printf("\033[33mwarning: failed to fix permissions: %v\033[0m", err)
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
				log.Printf("\033[33mwarning: failed to load credentials for '%s': %v\033[0m", source.Name, err)
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

			log.Printf("\033[32mLoaded credentials for artifact source '%s' (GIT_*_%d)\033[0m", source.Name, index)
		}
	} else if cfg.HashicorpVault != nil && cfg.HashicorpVault.HashicorpVaultIntegration && cfg.HashicorpVault.SecretKV2Path != "" {
		log.Println("\033[31mERROR: Legacy Vault configuration detected but is no longer supported.\033[0m")
		log.Println("\033[33mPlease migrate to artifact_sources configuration.\033[0m")
		log.Println("\033[33mSee MIGRATION_GUIDE.md for instructions.\033[0m")
		log.Println("\033[33mUse 'diffusion artifact add' to configure artifact sources with Vault.\033[0m")
		os.Exit(1)
	} else {
		log.Println("\033[35mNo artifact sources configured. Use public repositories or 'diffusion artifact add' to configure.\033[0m")
	}
	return nil
}

// setupRegistryAuth initializes CLI and performs docker login based on registry provider.
func setupRegistryAuth(cfg *config.Config) {
	switch cfg.ContainerRegistry.RegistryProvider {
	case config.RegistryProviderYC:
		if err := registry.YcCliInit(); err != nil {
			log.Printf("\033[33myc init warning: %v\033[0m", err)
		}
		if err := utils.RunCommandHide("docker", "login", cfg.ContainerRegistry.RegistryServer, "--username", "iam", "--password", os.Getenv("TOKEN")); err != nil {
			log.Printf("\033[33mdocker login to registry failed: %v\033[0m", err)
		}
	case config.RegistryProviderAWS:
		if err := registry.AwsCliInit(cfg.ContainerRegistry.RegistryServer); err != nil {
			log.Printf("\033[33maws ecr init warning: %v\033[0m", err)
		}
		if err := utils.RunCommandHide("docker", "login", cfg.ContainerRegistry.RegistryServer, "--username", "AWS", "--password", os.Getenv("TOKEN")); err != nil {
			log.Printf("\033[33mdocker login to AWS ECR registry failed: %v\033[0m", err)
		}
	case config.RegistryProviderGCP:
		if err := registry.GcpCliInit(cfg.ContainerRegistry.RegistryServer); err != nil {
			log.Printf("\033[33mgcloud init warning: %v\033[0m", err)
		}
		if err := utils.RunCommandHide("docker", "login", cfg.ContainerRegistry.RegistryServer, "--username", "oauth2accesstoken", "--password", os.Getenv("TOKEN")); err != nil {
			log.Printf("\033[33mdocker login to GCP registry failed: %v\033[0m", err)
		}
	case config.RegistryProviderPublic:
		log.Printf("\033[35mUsing public registry, skipping CLI initialization and authentication\033[0m")
	default:
		log.Printf("\033[33mUnknown registry provider '%s', skipping CLI initialization\033[0m", cfg.ContainerRegistry.RegistryProvider)
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
	)

	// Get Python version from lock file if it exists, otherwise use default
	pythonVersion := config.PinnedPythonVersion
	lockFile, err := dependency.LoadLockFile()
	if err == nil && lockFile != nil && lockFile.Python != nil && lockFile.Python.Pinned != "" {
		pythonVersion = lockFile.Python.Pinned
		log.Printf("\033[32mUsing Python version from lock file: %s\033[0m", pythonVersion)
	} else {
		log.Printf("\033[33mNo lock file found, using default Python version: %s\033[0m", pythonVersion)
	}
	args = append(args, "-e", fmt.Sprintf("PYTHON_PINNED_VERSION=%s", pythonVersion))

	// Generate and pass pyproject.toml configuration
	pyprojectContent, err := dependency.GeneratePyProjectFromCurrentConfig()
	if err != nil {
		log.Printf("\033[33mwarning: failed to generate pyproject.toml config: %v\033[0m", err)
		log.Printf("\033[33mContainer will use default dependencies\033[0m")
	} else {
		pyprojectEncoded := base64.StdEncoding.EncodeToString([]byte(pyprojectContent))
		args = append(args, "-e", "PYPROJECT_TOML_CONTENT="+pyprojectEncoded)
		log.Printf("\033[32mPassing pyproject.toml configuration to container\033[0m")
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
			"-e", "GIT_SHA="+gitSha,
			"-e", "ROLE_NAME="+opts.RoleFlag,
			"-e", "ORG_NAME="+opts.OrgFlag,
		)
		log.Printf("\033[32mCI Mode: Will clone %s (commit: %s) inside container\033[0m", gitRemote, gitSha[:8])
	}

	// Add cgroup mount only if it exists (may not be available in WSL2)
	if _, err := os.Stat("/sys/fs/cgroup"); err == nil {
		args = append(args, "-v", "/sys/fs/cgroup:/sys/fs/cgroup:rw")
	}

	// Add cache volume mounts if enabled (non-CI mode only; CI mode uses docker cp)
	if !opts.CIMode && cfg.CacheConfig != nil && cfg.CacheConfig.Enabled && cfg.CacheConfig.CacheID != "" {
		cacheDir, err := cache.EnsureCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
		if err != nil {
			log.Printf("\033[33mwarning: failed to create cache directory: %v\033[0m", err)
		} else {
			// Roles and collections cache (always mounted when cache is enabled)
			rolesDir := filepath.Join(cacheDir, config.CacheRolesDir)
			collectionsDir := filepath.Join(cacheDir, config.CacheCollectionsDir)

			if err := os.MkdirAll(rolesDir, 0755); err != nil {
				log.Printf("\033[33mwarning: failed to create roles cache directory: %v\033[0m", err)
			}
			if err := os.MkdirAll(collectionsDir, 0755); err != nil {
				log.Printf("\033[33mwarning: failed to create collections cache directory: %v\033[0m", err)
			}

			args = append(args, "-v", fmt.Sprintf("%s:%s", rolesDir, config.ContainerRolesCachePath))
			args = append(args, "-v", fmt.Sprintf("%s:%s", collectionsDir, config.ContainerCollectionsCachePath))
			log.Printf("\033[32mCache enabled: mounting roles and collections from %s\033[0m", cacheDir)

			// UV/Python package cache mount
			if cfg.CacheConfig.UVCache {
				uvDir, err := cache.EnsureUVCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
				if err != nil {
					log.Printf("\033[33mwarning: failed to create UV cache directory: %v\033[0m", err)
				} else {
					args = append(args, "-v", fmt.Sprintf("%s:%s", uvDir, config.ContainerUVCachePath))
					log.Printf("\033[32mUV cache enabled: mounting %s -> %s\033[0m", uvDir, config.ContainerUVCachePath)
				}
			}

			// Docker/DinD image cache mount
			if cfg.CacheConfig.DockerCache {
				dockerDir, err := cache.EnsureDockerCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
				if err != nil {
					log.Printf("\033[33mwarning: failed to create Docker cache directory: %v\033[0m", err)
				} else {
					args = append(args, "-v", fmt.Sprintf("%s:%s", dockerDir, config.ContainerDockerCachePath))
					log.Printf("\033[32mDocker cache enabled: mounting %s -> %s\033[0m", dockerDir, config.ContainerDockerCachePath)
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
		log.Printf("\033[31mdocker run failed: %v\033[0m", err)
		if len(output) > 0 {
			log.Printf("\033[31mDocker error output: %s\033[0m", string(output))
		}

		// Check for common WSL2 credential helper issue
		if strings.Contains(string(output), "docker-credential-desktop.exe") {
			log.Printf("\033[33m\nWSL2 Docker credential issue detected!\033[0m")
			log.Printf("\033[33mTo fix this, edit ~/.docker/config.json and either:\033[0m")
			log.Printf("\033[33m  1. Remove the 'credsStore' line, OR\033[0m")
			log.Printf("\033[33m  2. Change 'credsStore': 'desktop.exe' to 'credsStore': 'desktop'\033[0m")
			log.Printf("\033[33m\nExample fix: sed -i 's/desktop.exe/desktop/g' ~/.docker/config.json\033[0m")
		}

		return err
	}

	return nil
}

// setupCIRepository clones the repo and sets up role files inside the container.
func setupCIRepository(opts *MoleculeOptions, roleDirName string) error {
	log.Printf("\033[32mCI Mode: Setting up repository inside container...\033[0m")

	cloneCmd := `cd /tmp && rm -rf repo && git clone "$GIT_REMOTE" repo && cd repo && git checkout "$GIT_SHA"`
	if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", cloneCmd); err != nil {
		return fmt.Errorf("failed to clone repository in container: %w", err)
	}
	log.Printf("\033[32mCI Mode: Repository cloned to /tmp/repo\033[0m")

	mkdirCmd := fmt.Sprintf("mkdir -p /opt/molecule/%s", roleDirName)
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", mkdirCmd); err != nil {
		return fmt.Errorf("failed to create role directory in container: %w", err)
	}

	copyDirs := []string{"tasks", "defaults", "meta", "handlers", "templates", "files", "vars"}
	for _, dir := range copyDirs {
		copyCmd := fmt.Sprintf("if [ -d /tmp/repo/%s ]; then cp -r /tmp/repo/%s /opt/molecule/%s/; fi", dir, dir, roleDirName)
		_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", copyCmd)
	}

	copyScenarios := fmt.Sprintf("if [ -d /tmp/repo/scenarios ]; then cp -r /tmp/repo/scenarios /opt/molecule/%s/molecule; fi", roleDirName)
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", copyScenarios); err != nil {
		return fmt.Errorf("failed to copy scenarios in container: %w", err)
	}

	copyLintCmd := fmt.Sprintf("if [ -f /tmp/repo/.ansible-lint ]; then cp /tmp/repo/.ansible-lint /opt/molecule/%s/; fi && if [ -f /tmp/repo/.yamllint ]; then cp /tmp/repo/.yamllint /opt/molecule/%s/; fi", roleDirName, roleDirName)
	_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", copyLintCmd)

	log.Printf("\033[32mCI Mode: Role files copied to /opt/molecule/%s\033[0m", roleDirName)

	verifyCmd := fmt.Sprintf("ls -la /opt/molecule/%s/molecule/default/molecule.yml", roleDirName)
	if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", verifyCmd); err != nil {
		log.Printf("\033[31mCI Mode: molecule.yml not found!\033[0m")
		_ = utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("ls -laR /opt/molecule/%s/", roleDirName))
		return fmt.Errorf("molecule.yml not found in container")
	}
	log.Printf("\033[32mCI Mode: Setup complete!\033[0m")

	return nil
}

// ensureRole initializes or validates the role directory inside the container.
func ensureRole(opts *MoleculeOptions, roleMoleculePath string) {
	if utils.Exists(roleMoleculePath) {
		fmt.Println("\033[35mThis role already exists in molecule\033[0m")
	} else {
		if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("ansible-galaxy role init %s.%s", opts.OrgFlag, opts.RoleFlag)); err != nil {
			log.Printf("\033[33mrole init warning: %v\033[0m", err)
		}

		// Fix ownership inside container after role init (Unix systems only)
		if runtime.GOOS != "windows" {
			uid := os.Getuid()
			gid := os.Getgid()
			chownCmd := fmt.Sprintf("chown -R %d:%d /opt/molecule/%s.%s", uid, gid, opts.OrgFlag, opts.RoleFlag)
			if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", chownCmd); err != nil {
				log.Printf("\033[33mwarning: failed to fix ownership after role init: %v\033[0m", err)
			}
		}

		if err := utils.DockerExecInteractive(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", fmt.Sprintf("rm -f %s.%s/*/*", opts.OrgFlag, opts.RoleFlag)); err != nil {
			log.Printf("\033[33mclean role dir warning: %v\033[0m", err)
		}
	}
}

// loginInsideContainer performs docker login inside the container (provider-specific).
func loginInsideContainer(opts *MoleculeOptions, cfg *config.Config) {
	switch cfg.ContainerRegistry.RegistryProvider {
	case config.RegistryProviderYC:
		_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", `echo $TOKEN | docker login cr.yandex --username iam --password-stdin`)
	case config.RegistryProviderAWS:
		loginCmd := fmt.Sprintf(`echo $TOKEN | docker login %s --username AWS --password-stdin`, cfg.ContainerRegistry.RegistryServer)
		_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", loginCmd)
	case config.RegistryProviderGCP:
		loginCmd := fmt.Sprintf(`echo $TOKEN | docker login %s --username oauth2accesstoken --password-stdin`, cfg.ContainerRegistry.RegistryServer)
		_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "/bin/sh", opts.CIMode, "-c", loginCmd)
	case config.RegistryProviderPublic:
		log.Printf("\033[35mUsing public registry, skipping authentication\033[0m")
	default:
		log.Printf("\033[33mUnknown registry provider '%s', skipping authentication\033[0m", cfg.ContainerRegistry.RegistryProvider)
	}
}

// copyCacheIntoContainer copies cached roles, collections, UV packages, and Docker
// image tarballs FROM the host cache directory INTO the running container using
// "docker cp". This is used in CI mode where volume mounts (-v) are unavailable.
func copyCacheIntoContainer(opts *MoleculeOptions, cfg *config.Config) {
	if cfg.CacheConfig == nil || !cfg.CacheConfig.Enabled || cfg.CacheConfig.CacheID == "" {
		return
	}

	cacheDir, err := cache.GetCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
	if err != nil {
		log.Printf("\033[33mwarning: failed to resolve cache directory: %v\033[0m", err)
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
		_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", mkdirCmd)

		src := hostPath + string(os.PathSeparator) + "."
		if err := exec.Command("docker", "cp", src, containerName+":"+containerPath).Run(); err != nil {
			log.Printf("\033[33mwarning: failed to copy %s cache into container: %v\033[0m", label, err)
		} else {
			log.Printf("\033[32mCI cache: copied %s into container\033[0m", label)
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
// directory using "docker cp". Called in CI mode during --wipe, before the
// container is removed.
func copyCacheFromContainer(opts *MoleculeOptions, cfg *config.Config) {
	if cfg.CacheConfig == nil || !cfg.CacheConfig.Enabled || cfg.CacheConfig.CacheID == "" {
		return
	}

	cacheDir, err := cache.EnsureCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
	if err != nil {
		log.Printf("\033[33mwarning: failed to ensure cache directory: %v\033[0m", err)
		return
	}

	containerName := fmt.Sprintf("molecule-%s", opts.RoleFlag)

	// Helper: docker cp <container>:<containerPath>/. <hostPath>
	copyDir := func(containerPath, hostSubdir, label string) {
		hostPath := filepath.Join(cacheDir, hostSubdir)
		if err := os.MkdirAll(hostPath, 0755); err != nil {
			log.Printf("\033[33mwarning: failed to create host cache dir for %s: %v\033[0m", label, err)
			return
		}

		src := containerName + ":" + containerPath + "/."
		if err := exec.Command("docker", "cp", src, hostPath).Run(); err != nil {
			log.Printf("\033[33mwarning: failed to copy %s cache from container: %v\033[0m", label, err)
		} else {
			log.Printf("\033[32mCI cache: saved %s from container\033[0m", label)
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
// This works in both CI and non-CI modes — the tarball is either volume-mounted
// (non-CI) or copied in via copyCacheIntoContainer (CI).
func loadDinDImages(opts *MoleculeOptions) {
	// Container paths are always Linux — use forward slashes, never filepath.Join.
	tarballPath := fmt.Sprintf("%s/%s", config.ContainerDockerCachePath, config.DockerImageTarball)

	// Check if the tarball exists inside the container
	checkCmd := fmt.Sprintf("test -f %s", tarballPath)
	if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", checkCmd); err != nil {
		log.Printf("\033[33mNo cached Docker images found at %s, skipping load\033[0m", tarballPath)
		return
	}

	// Load the images into the inner Docker daemon

	if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", fmt.Sprintf("docker load -i %s", tarballPath)); err != nil {
		log.Printf("\033[33mwarning: failed to load DinD images from cache (%s): %v\033[0m", tarballPath, err)
	} else {
		log.Printf("\033[32mDinD images loaded from cache\033[0m")
	}
}

// saveDinDImages discovers all Docker images inside the DinD daemon running in
// the molecule container and saves them to a single tarball at
// /root/.cache/docker/images.tar.  It runs:
//
//	docker exec <container> docker images --format '{{.Repository}}:{{.Tag}}'
//	docker exec <container> docker save -o /root/.cache/docker/images.tar <image1> <image2> ...
//
// This works in both CI and non-CI modes. In non-CI mode the tarball persists
// on the host automatically via the volume mount. In CI mode the caller must
// follow up with copyCacheFromContainer to pull it out.
func saveDinDImages(opts *MoleculeOptions) {
	containerName := fmt.Sprintf("molecule-%s", opts.RoleFlag)

	// Discover images inside the DinD daemon.
	// We need to capture stdout, so we use exec.Command directly here.
	// Build flags the same way DockerExecInteractiveHide does.
	execFlags := []string{"exec"}
	if !opts.CIMode {
		execFlags = append(execFlags, "-ti")
	}
	execFlags = append(execFlags, containerName, "sh", "-c",
		`docker images --format '{{.Repository}}:{{.Tag}}'`)
	out, err := exec.Command("docker", execFlags...).Output()
	if err != nil {
		log.Printf("\033[33mwarning: failed to list DinD images: %v\033[0m", err)
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
		log.Printf("\033[33mNo DinD images to cache\033[0m")
		return
	}

	log.Printf("\033[32mSaving %d DinD image(s) to cache: %s\033[0m", len(images), strings.Join(images, ", "))

	// Ensure the cache directory exists inside the container
	mkdirCmd := fmt.Sprintf("mkdir -p %s", config.ContainerDockerCachePath)
	_ = utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", mkdirCmd)

	// Container paths are always Linux — use forward slashes, never filepath.Join.
	tarballPath := fmt.Sprintf("%s/%s", config.ContainerDockerCachePath, config.DockerImageTarball)
	saveCmd := fmt.Sprintf("docker save -o %s %s", tarballPath, strings.Join(images, " "))
	if err := utils.DockerExecInteractiveHide(opts.RoleFlag, "sh", opts.CIMode, "-c", saveCmd); err != nil {
		log.Printf("\033[33mwarning: failed to save DinD images to cache: %v\033[0m", err)
	} else {
		log.Printf("\033[32mDinD images saved to %s\033[0m", tarballPath)
	}
}
