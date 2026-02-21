package cli

import (
	"bufio"
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
	"diffusion/internal/role"
	"diffusion/internal/secrets"
	"diffusion/internal/utils"

	"github.com/spf13/cobra"
)

// NewMoleculeCmd creates the molecule command
func NewMoleculeCmd(cli *CLI) *cobra.Command {
	molCmd := &cobra.Command{
		Use:   "molecule",
		Short: "run molecule workflow (create/converge/verify/lint/idempotence/wipe)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMolecule(cmd, args, cli)
		},
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Ensure some env defaults and prompt when needed

			reader := bufio.NewReader(os.Stdin)

			cfg, err := config.LoadConfig() // ignore error for now
			if err != nil {
				log.Printf("\033[33mwarning loading config: %v\033[0m", err)
				log.Printf("\033[38;2;127;255;212mNew config file will be created...\033[0m")

				YamlLintRulesDefault := &config.YamlLintRules{
					Braces:             map[string]any{"max-spaces-inside": 1, "level": "warning"},
					Brackets:           map[string]any{"max-spaces-inside": 1, "level": "warning"},
					NewLines:           map[string]any{"type": "platform"},
					Comments:           map[string]any{"min-spaces-from-content": 1},
					CommentsIdentation: false,
					OctalValues:        map[string]any{"forbid-implicit-octal": true},
				}
				YamlLintDefault := &config.YamlLint{
					Extends: "default",
					Ignore:  []string{".git/*", "molecule/**", "vars/*", "files/*", ".yamllint", ".ansible-lint"},
					Rules:   YamlLintRulesDefault,
				}

				AnsibleLintDefault := &config.AnsibleLint{
					ExcludedPaths: []string{"molecule/default/tests/*.yml", "molecule/default/tests/*/*/*.yml", "tests/test.yml"},
					WarnList:      []string{"meta-no-info", "yaml[line-length]"},
					SkipList:      []string{"meta-incorrect", "role-name[path]"},
				}

				fmt.Printf("Enter RegistryServer (%s): ", config.DefaultRegistryServer)
				registryServer, _ := reader.ReadString('\n')
				registryServer = strings.TrimSpace(registryServer)
				if registryServer == "" {
					registryServer = config.DefaultRegistryServer
				}

				fmt.Printf("Enter RegistryProvider (%s): ", config.DefaultRegistryProvider)
				registryProvider, _ := reader.ReadString('\n')
				registryProvider = strings.TrimSpace(registryProvider)
				if registryProvider == "" {
					registryProvider = config.DefaultRegistryProvider
				}

				if registryProvider != "YC" && registryProvider != "AWS" && registryProvider != "GCP" && registryProvider != "Public" {
					fmt.Fprintln(os.Stderr, "\033[31mInvalid RegistryProvider. Allowed values are: YC, AWS, GCP. \nIf you're using public registry, then choose Public - or choose it, if you want to authenticate externally.\033[0m")
					os.Exit(1)
				}

				fmt.Printf("Enter MoleculeContainerName (%s): ", config.DefaultMoleculeContainerName)
				moleculeContainerName, _ := reader.ReadString('\n')
				moleculeContainerName = strings.TrimSpace(moleculeContainerName)
				if moleculeContainerName == "" {
					moleculeContainerName = config.DefaultMoleculeContainerName
				}

				defaultTag := utils.GetDefaultMoleculeTag()
				fmt.Printf("Enter MoleculeContainerTag (%s): ", defaultTag)
				moleculeContainerTag, _ := reader.ReadString('\n')
				moleculeContainerTag = strings.TrimSpace(moleculeContainerTag)
				if moleculeContainerTag == "" {
					moleculeContainerTag = defaultTag
				}

				ContainerRegistry := &config.ContainerRegistry{
					RegistryServer:        registryServer,
					RegistryProvider:      registryProvider,
					MoleculeContainerName: moleculeContainerName,
					MoleculeContainerTag:  moleculeContainerTag,
				}

				fmt.Print("Enable Vault Integration for artifact sources? (y/N): ")
				vaultEnabledStr, _ := reader.ReadString('\n')
				vaultEnabledStr = strings.TrimSpace(vaultEnabledStr)
				if vaultEnabledStr == "" {
					vaultEnabledStr = "n"
				}
				vaultEnabled := strings.ToLower(vaultEnabledStr) == "y"

				HashicorpVaultSet := VaultConfigHelper(vaultEnabled)

				// Configure artifact sources
				ArtifactSourcesList := ArtifactSourcesHelper()

				TestsSettings := TestsConfigSetup()

				cfg = &config.Config{
					ContainerRegistry: ContainerRegistry,
					HashicorpVault:    HashicorpVaultSet,
					ArtifactSources:   ArtifactSourcesList,
					YamlLintConfig:    YamlLintDefault,
					AnsibleLintConfig: AnsibleLintDefault,
					TestsConfig:       TestsSettings,
				}

				if err := config.SaveConfig(cfg); err != nil {
					log.Printf("\033[33mwarning saving new config: %v\033[0m", err)
				}

			}
		},
	}

	MetaConfig, _, err := role.LoadRoleConfig("")
	if err != nil {
		cli.RoleFlag = ""
		cli.OrgFlag = ""
		log.Printf("\033[33mwarning loading role config: %v\033[0m", err)
	} else {
		if MetaConfig.GalaxyInfo.RoleName != "" {
			cli.RoleFlag = MetaConfig.GalaxyInfo.RoleName
			cli.OrgFlag = MetaConfig.GalaxyInfo.Namespace
		} else {
			cli.RoleFlag = ""
			cli.OrgFlag = ""
			log.Printf("\033[33mwarning: role name or namespace missing in meta/main.yml\033[0m")
		}
	}

	molCmd.Flags().StringVarP(&cli.RoleFlag, "role", "r", cli.RoleFlag, "role name")
	molCmd.Flags().StringVarP(&cli.OrgFlag, "org", "o", cli.OrgFlag, "organization prefix")
	molCmd.Flags().StringVarP(&cli.TagFlag, "tag", "t", "", "Ansible tags to run (comma-separated, e.g., 'install,configure')")
	molCmd.Flags().BoolVar(&cli.ConvergeFlag, "converge", false, "run molecule converge")
	molCmd.Flags().BoolVar(&cli.VerifyFlag, "verify", false, "run molecule verify")
	molCmd.Flags().BoolVar(&cli.TestsOverWriteFlag, "testsoverwrite", false, "overwrite molecule tests folder for remote or diffusion type")
	molCmd.Flags().BoolVar(&cli.LintFlag, "lint", false, "run linting (yamllint / ansible-lint)")
	molCmd.Flags().BoolVar(&cli.IdempotenceFlag, "idempotence", false, "run molecule idempotence")
	molCmd.Flags().BoolVar(&cli.DestroyFlag, "destroy", false, "run molecule destroy")
	molCmd.Flags().BoolVar(&cli.WipeFlag, "wipe", false, "remove container and molecule role folder")
	molCmd.Flags().BoolVar(&cli.CIMode, "ci", false, "CI/CD mode (non-interactive, skip TTY and permission fixes)")

	return molCmd
}

// runMolecule is the core function that implements the behavior
func runMolecule(cmd *cobra.Command, args []string, cli *CLI) error {

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Printf("\033[33mwarning loading config: %v\033[0m", err)
	}

	// prepare path
	path, err := os.Getwd()
	if err != nil {
		return err
	}
	// use forward slashes in mounts where required (docker on windows expects Windows paths but we keep raw)
	// Compose role path
	roleDirName := fmt.Sprintf("%s.%s", cli.OrgFlag, cli.RoleFlag)
	roleMoleculePath := filepath.Join(path, "molecule", roleDirName)

	// handle wipe
	if cli.WipeFlag {
		log.Printf("\033[38;2;127;255;212mWiping: running molecule destroy, removing container molecule-%s and folder %s\n\033[0m", cli.RoleFlag, roleMoleculePath)

		// Run molecule destroy inside the container first
		roleDir := utils.GetRoleDirName(cli.OrgFlag, cli.RoleFlag)
		_ = utils.DockerExecInteractiveHide(cli.RoleFlag, "bash", cli.CIMode, "-c", fmt.Sprintf("cd ./%s && molecule destroy", roleDir))

		// Remove the container
		_ = utils.RunCommandHide("docker", "rm", fmt.Sprintf("molecule-%s", cli.RoleFlag), "-f")

		// Remove the role folder
		if err := os.RemoveAll(roleMoleculePath); err != nil {
			log.Printf("\033[33mwarning: failed remove role path: %v\033[0m", err)
		}
		return nil
	}

	// handle converge/lint/verify/idempotence/destroy by ensuring files are copied and running docker exec commands
	if cli.ConvergeFlag || cli.LintFlag || cli.VerifyFlag || cli.IdempotenceFlag || cli.DestroyFlag {

		if !cli.CIMode {
			if err := copyRoleData(path, roleMoleculePath, cli.CIMode); err != nil {
				log.Printf("\033[33mwarning copying data: %v\033[0m", err)
			}
		}

		linters := roleMoleculePath
		// In CI mode, set linters to Org.Role format
		if cli.CIMode {
			linters = fmt.Sprintf("%s.%s", cli.OrgFlag, cli.RoleFlag)
		}

		utils.ExportLinters(cfg, linters, cli.CIMode, cli.RoleFlag, cli.OrgFlag)

		// Determine scenario name for tests directory
		scenario := "default"
		if cli.RoleScenario != "" {
			scenario = cli.RoleScenario
		}

		// Create tests directory for verify/lint
		moleculeDefaultTestsPath := fmt.Sprintf("molecule/%s.%s/molecule/%s/tests", cli.OrgFlag, cli.RoleFlag, scenario)
		if err := os.MkdirAll(moleculeDefaultTestsPath, 0o755); err != nil {
			log.Printf("\033[33mwarning: cannot create scenario tests dir: %v\033[0m", err)
		}
		defaultTestsDir := moleculeDefaultTestsPath
		log.Printf("Default tests dir: %s", defaultTestsDir)

		if cli.ConvergeFlag {
			// Verify molecule.yml exists inside container before running
			if cli.CIMode {
				checkCmd := fmt.Sprintf("ls -la /opt/molecule/%s/molecule/default/molecule.yml", roleDirName)
				log.Printf("Checking molecule.yml in container...")
				if err := utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", checkCmd); err != nil {
					log.Printf("\033[31mmolecule.yml not found in container at /opt/molecule/%s/molecule/default/\033[0m", roleDirName)
					log.Printf("\033[33mListing container directory structure:\033[0m")
					_ = utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", fmt.Sprintf("ls -laR /opt/molecule/%s/", roleDirName))
					os.Exit(1)
				}
			}

			tagEnv := ""
			if cli.TagFlag != "" {
				tagEnv = fmt.Sprintf("ANSIBLE_RUN_TAGS=%s ", cli.TagFlag)
			}
			cmdStr := fmt.Sprintf("cd ./%s && %smolecule converge", roleDirName, tagEnv)
			if err := utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", cmdStr); err != nil {
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
				if err := utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", chownCmd); err != nil {
					log.Printf("\033[33mwarning: failed to fix permissions: %v\033[0m", err)
				}
			}

			return nil
		}
		if cli.LintFlag {
			// run yamllint and ansible-lint inside container
			cmdStr := fmt.Sprintf(`cd ./%s && yamllint . -c .yamllint && ansible-lint -c .ansible-lint `, roleDirName)
			if err := utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", cmdStr); err != nil {
				log.Printf("\033[31mLint failed: %v\033[0m", err)
				os.Exit(1)
			}
			log.Printf("\033[32mLint Done Successfully!\033[0m")
			return nil
		}
		if cli.VerifyFlag {
			// copy tests/*

			switch cfg.TestsConfig.Type {
			case "local":
				testsSrc := filepath.Join(path, "tests")
				if cli.CIMode {
					log.Printf("CIMode detected, copying tests from /tmp/repo/tests to %s.%s/molecule/%s/tests/", cli.OrgFlag, cli.RoleFlag, scenario)
					cmdCopy := fmt.Sprintf(`
						cd /tmp && \
						rm -rf repo && \
						git clone "${GIT_REMOTE}" repo && \
						cd repo && \
						git checkout "${GIT_SHA}" && \
						cp -rf /tmp/repo/tests /opt/molecule/%s.%s/molecule/%s/
					`, cli.OrgFlag, cli.RoleFlag, scenario)
					if err := utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", cmdCopy); err != nil {
						log.Printf("\033[33mwarning: failed to copy tests in CI mode: %v\033[0m", err)
					}
				} else {
					if exists(testsSrc) {
						testsDst := filepath.Join(roleMoleculePath, "molecule", scenario, "tests")
						copyIfExists(testsSrc, testsDst)
					} else {
						log.Printf("\033[33mtests/ directory not found, skipping copy\033[0m")
					}
				}
			case "remote":
				// Ensure a remote repository URL is provided
				if len(cfg.TestsConfig.RemoteRepositories) == 0 {
					return fmt.Errorf("\033[31mno remote repository configured for tests type 'remote'\033[0m")
				}

				// Loop through each remote repository and install to tests directory
				for _, remoteRepo := range cfg.TestsConfig.RemoteRepositories {
					log.Printf("\033[32mInstalling test files from remote repository: %s\033[0m", remoteRepo)

					// Install tests from remote repository using git clone inside container
					if !cli.TestsOverWriteFlag {
						if cli.CIMode {
							cmdRemoteTests := fmt.Sprintf(`
							cd /opt/molecule/%s.%s/molecule/%s && \
							if [ ! -d tests ]; then \
								git clone %s tests; \
							else \
								echo "Tests directory already exists, skipping clone"; \
							fi
						`, cli.OrgFlag, cli.RoleFlag, scenario, remoteRepo)
							if err := utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", cmdRemoteTests); err != nil {
								log.Printf("\033[33mwarning: failed to clone remote tests in CI mode: %v\033[0m", err)
							}
						} else {
							testsDst := filepath.Join(roleMoleculePath, "molecule", scenario, "tests")
							if _, err := os.Stat(testsDst); os.IsNotExist(err) {
								cmdRemoteTests := fmt.Sprintf(`
								cd %s && \
								git clone %s tests
							`, filepath.Join(roleMoleculePath, "molecule", scenario), remoteRepo)
								if err := utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", cmdRemoteTests); err != nil {
									log.Printf("\033[33mwarning: failed to clone remote tests: %v\033[0m", err)
								}
							} else {
								log.Printf("\033[33mTests directory already exists, skipping clone\033[0m")
							}
						}
					} else {
						// Overwrite tests directory with remote repository
						if cli.CIMode {
							cmdRemoteTests := fmt.Sprintf(`
							cd /opt/molecule/%s.%s/molecule/%s && \
							rm -rf tests && \
							git clone %s tests
						`, cli.OrgFlag, cli.RoleFlag, scenario, remoteRepo)
							if err := utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", cmdRemoteTests); err != nil {
								log.Printf("\033[33mwarning: failed to clone remote tests in CI mode: %v\033[0m", err)
							}
						} else {
							testsDst := filepath.Join(roleMoleculePath, "molecule", scenario, "tests")
							_ = os.RemoveAll(testsDst)
							cmdRemoteTests := fmt.Sprintf(`
							cd %s && \
							git clone %s tests
						`, filepath.Join(roleMoleculePath, "molecule", scenario), remoteRepo)
							if err := utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", cmdRemoteTests); err != nil {
								log.Printf("\033[33mwarning: failed to clone remote tests: %v\033[0m", err)
							}
						}
					}
				}
			case "diffusion":
				// Use tests from diffusion's internal repository
				log.Printf("\033[32mUsing diffusion-managed test files\033[0m")

				// Check if tests-repo directory exists, if not clone it
				diffusionTestsPath := filepath.Join(os.TempDir(), "diffusion-tests-repo")
				if !cli.TestsOverWriteFlag {
					if _, err := os.Stat(diffusionTestsPath); os.IsNotExist(err) {
						// Clone the tests repository
						log.Printf("\033[32mCloning diffusion tests repository...\033[0m")
						if err := exec.Command("git", "clone", "https://github.com/your-org/diffusion-tests", diffusionTestsPath).Run(); err != nil {
							return fmt.Errorf("\033[31mfailed to clone diffusion tests repository: %w\033[0m", err)
						}
					} else {
						// Pull latest changes
						log.Printf("\033[32mUpdating diffusion tests repository...\033[0m")
						gitPullCmd := exec.Command("git", "pull")
						gitPullCmd.Dir = diffusionTestsPath
						if err := gitPullCmd.Run(); err != nil {
							log.Printf("\033[33mwarning: failed to update diffusion tests repository: %v\033[0m", err)
						}
					}
				} else {
					// Overwrite tests directory with diffusion tests
					_ = os.RemoveAll(diffusionTestsPath)
					log.Printf("\033[32mCloning diffusion tests repository (overwrite mode)...\033[0m")
					if err := exec.Command("git", "clone", "https://github.com/your-org/diffusion-tests", diffusionTestsPath).Run(); err != nil {
						return fmt.Errorf("\033[31mfailed to clone diffusion tests repository: %w\033[0m", err)
					}
				}

				// Copy tests from diffusion repo to role tests directory
				if cli.CIMode {
					cmdCopy := fmt.Sprintf(`
						cp -rf %s /opt/molecule/%s.%s/molecule/%s/tests
					`, diffusionTestsPath, cli.OrgFlag, cli.RoleFlag, scenario)
					if err := utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", cmdCopy); err != nil {
						log.Printf("\033[33mwarning: failed to copy diffusion tests in CI mode: %v\033[0m", err)
					}
				} else {
					testsDst := filepath.Join(roleMoleculePath, "molecule", scenario, "tests")
					copyIfExists(diffusionTestsPath, testsDst)
				}
			default:
				return fmt.Errorf("\033[31munknown tests type: %s\033[0m", cfg.TestsConfig.Type)
			}

			// run molecule verify
			cmdStr := fmt.Sprintf("cd ./%s && molecule verify", roleDirName)
			if err := utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", cmdStr); err != nil {
				log.Printf("\033[31mVerify failed: %v\033[0m", err)
				os.Exit(1)
			}
			log.Printf("\033[32mVerify Done Successfully!\033[0m")
			return nil
		}
		if cli.IdempotenceFlag {
			tagEnv := ""
			if cli.TagFlag != "" {
				tagEnv = fmt.Sprintf("ANSIBLE_RUN_TAGS=%s ", cli.TagFlag)
			}
			cmdStr := fmt.Sprintf("cd ./%s && %smolecule idempotence", roleDirName, tagEnv)
			if err := utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", cmdStr); err != nil {
				log.Printf("\033[31mIdempotence failed: %v\033[0m", err)
				os.Exit(1)
			}
			log.Printf("\033[32mIdempotence Done Successfully!\033[0m")
			return nil
		}
		if cli.DestroyFlag {
			cmdStr := fmt.Sprintf("cd ./%s && molecule destroy", roleDirName)
			if err := utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", cmdStr); err != nil {
				log.Printf("\033[31mDestroy failed: %v\033[0m", err)
				os.Exit(1)
			}
			log.Printf("\033[32mDestroy Done Successfully!\033[0m")
			return nil
		}
	}

	// default flow: create/run container if not exists, copy data, converge
	// check if container exists
	err = exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", cli.RoleFlag)).Run()
	if err == nil {
		fmt.Printf("\033[38;2;127;255;212mContainer molecule-%s already exists. To purge use --wipe.\n\033[0m", cli.RoleFlag)
	} else {
		// Load credentials for all configured artifact sources
		if len(cfg.ArtifactSources) > 0 {
			for i, source := range cfg.ArtifactSources {
				index := i + 1
				var creds *config.ArtifactCredentials
				var err error

				// Get credentials from Vault or local storage
				creds, err = secrets.GetArtifactCredentials(&source, cfg.HashicorpVault)
				if err != nil {
					log.Printf("\033[33mwarning: failed to load credentials for '%s': %v\033[0m", source.Name, err)
					continue
				}

				// Set indexed environment variables
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
			// Legacy Vault configuration is no longer supported
			log.Println("\033[31mERROR: Legacy Vault configuration detected but is no longer supported.\033[0m")
			log.Println("\033[33mPlease migrate to artifact_sources configuration.\033[0m")
			log.Println("\033[33mSee MIGRATION_GUIDE.md for instructions.\033[0m")
			log.Println("\033[33mUse 'diffusion artifact add' to configure artifact sources with Vault.\033[0m")
			os.Exit(1)
		} else {
			log.Println("\033[35mNo artifact sources configured. Use public repositories or 'diffusion artifact add' to configure.\033[0m")
		}

		// Initialize CLI and login based on registry provider
		switch cfg.ContainerRegistry.RegistryProvider {
		case "YC":
			// Yandex Cloud: Initialize yc CLI and login
			if err := registry.YcCliInit(); err != nil {
				log.Printf("\033[33myc init warning: %v\033[0m", err)
			}
			if err := utils.RunCommandHide("docker", "login", cfg.ContainerRegistry.RegistryServer, "--username", "iam", "--password", os.Getenv("TOKEN")); err != nil {
				log.Printf("\033[33mdocker login to registry failed: %v\033[0m", err)
			}
		case "AWS":
			// AWS: Initialize AWS CLI and login to ECR
			if err := registry.AwsCliInit(cfg.ContainerRegistry.RegistryServer); err != nil {
				log.Printf("\033[33maws ecr init warning: %v\033[0m", err)
			}
			if err := utils.RunCommandHide("docker", "login", cfg.ContainerRegistry.RegistryServer, "--username", "AWS", "--password", os.Getenv("TOKEN")); err != nil {
				log.Printf("\033[33mdocker login to AWS ECR registry failed: %v\033[0m", err)
			}
		case "GCP":
			// GCP: Initialize gcloud CLI and login to Artifact Registry or GCR
			if err := registry.GcpCliInit(cfg.ContainerRegistry.RegistryServer); err != nil {
				log.Printf("\033[33mgcloud init warning: %v\033[0m", err)
			}
			if err := utils.RunCommandHide("docker", "login", cfg.ContainerRegistry.RegistryServer, "--username", "oauth2accesstoken", "--password", os.Getenv("TOKEN")); err != nil {
				log.Printf("\033[33mdocker login to GCP registry failed: %v\033[0m", err)
			}
		case "Public":
			// Public registry: No CLI init or authentication needed
			log.Printf("\033[35mUsing public registry, skipping CLI initialization and authentication\033[0m")
		default:
			log.Printf("\033[33mUnknown registry provider '%s', skipping CLI initialization\033[0m", cfg.ContainerRegistry.RegistryProvider)
		}
		// run container
		// docker run --rm -d --name=molecule-$role -v "$path/molecule:/opt/molecule" -v /sys/fs/cgroup:/sys/fs/cgroup:rw -e ... --privileged --pull always cr.yandex/...
		image := fmt.Sprintf("%s/%s:%s", cfg.ContainerRegistry.RegistryServer, cfg.ContainerRegistry.MoleculeContainerName, cfg.ContainerRegistry.MoleculeContainerTag)
		args := []string{
			"run", "--rm", "-d", "--name=" + fmt.Sprintf("molecule-%s", cli.RoleFlag),
		}

		// Note: Not using user mapping here because DinD (Docker-in-Docker) requires root
		// We'll fix permissions on the mounted volume after operations instead

		// CI Mode: Don't mount /opt/molecule, we'll clone repo inside container
		if !cli.CIMode {
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
			// Pass pyproject.toml content as environment variable (base64 encoded to handle special characters)
			pyprojectEncoded := base64.StdEncoding.EncodeToString([]byte(pyprojectContent))
			args = append(args, "-e", "PYPROJECT_TOML_CONTENT="+pyprojectEncoded)
			log.Printf("\033[32mPassing pyproject.toml configuration to container\033[0m")
		}

		// CI Mode: Pass git remote and commit SHA for cloning inside container
		if cli.CIMode {
			// Get git remote URL from current repository
			gitRemoteCmd := exec.Command("git", "config", "--get", "remote.origin.url")
			gitRemoteCmd.Dir = path
			gitRemoteOutput, err := gitRemoteCmd.Output()
			if err != nil {
				return fmt.Errorf("CI mode: failed to get git remote URL: %w", err)
			}
			gitRemote := strings.TrimSpace(string(gitRemoteOutput))

			// Get current commit SHA
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
				"-e", "ROLE_NAME="+cli.RoleFlag,
				"-e", "ORG_NAME="+cli.OrgFlag,
			)
			log.Printf("\033[32mCI Mode: Will clone %s (commit: %s) inside container\033[0m", gitRemote, gitSha[:8])
		}

		// Add cgroup mount only if it exists (may not be available in WSL2)
		if _, err := os.Stat("/sys/fs/cgroup"); err == nil {
			args = append(args, "-v", "/sys/fs/cgroup:/sys/fs/cgroup:rw")
		}

		// Add cache volume mounts if enabled (roles and collections only)
		if cfg.CacheConfig != nil && cfg.CacheConfig.Enabled && cfg.CacheConfig.CacheID != "" {
			cacheDir, err := cache.EnsureCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
			if err != nil {
				log.Printf("\033[33mwarning: failed to create cache directory: %v\033[0m", err)
			} else {
				// Create subdirectories for roles and collections
				rolesDir := filepath.Join(cacheDir, "roles")
				collectionsDir := filepath.Join(cacheDir, "collections")

				if err := os.MkdirAll(rolesDir, 0755); err != nil {
					log.Printf("\033[33mwarning: failed to create roles cache directory: %v\033[0m", err)
				}
				if err := os.MkdirAll(collectionsDir, 0755); err != nil {
					log.Printf("\033[33mwarning: failed to create collections cache directory: %v\033[0m", err)
				}

				// Mount only roles and collections directories
				// Use appropriate home path based on OS (root for Windows, ansible user for Unix)
				containerHome := utils.GetContainerHomePath()
				args = append(args, "-v", fmt.Sprintf("%s:%s/.ansible/roles", rolesDir, containerHome))
				args = append(args, "-v", fmt.Sprintf("%s:%s/.ansible/collections", collectionsDir, containerHome))
				log.Printf("\033[32mCache enabled: mounting roles and collections from %s\033[0m", cacheDir)
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
	}

	// CI Mode: Clone repository and setup files inside container
	if cli.CIMode {
		log.Printf("\033[32mCI Mode: Setting up repository inside container...\033[0m")

		// Clone repository to /tmp/repo and checkout specific commit
		cloneCmd := `cd /tmp && rm -rf repo && git clone "$GIT_REMOTE" repo && cd repo && git checkout "$GIT_SHA"`
		if err := utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", cloneCmd); err != nil {
			return fmt.Errorf("failed to clone repository in container: %w", err)
		}
		log.Printf("\033[32mCI Mode: Repository cloned to /tmp/repo\033[0m")

		// Create role directory structure
		roleDirName := fmt.Sprintf("%s.%s", cli.OrgFlag, cli.RoleFlag)
		mkdirCmd := fmt.Sprintf("mkdir -p /opt/molecule/%s", roleDirName)
		if err := utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", mkdirCmd); err != nil {
			return fmt.Errorf("failed to create role directory in container: %w", err)
		}

		// Copy role files from /tmp/repo to /opt/molecule/org.role/
		copyDirs := []string{"tasks", "defaults", "meta", "handlers", "templates", "files", "vars"}
		for _, dir := range copyDirs {
			copyCmd := fmt.Sprintf("if [ -d /tmp/repo/%s ]; then cp -r /tmp/repo/%s /opt/molecule/%s/; fi", dir, dir, roleDirName)
			_ = utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", copyCmd)
		}

		// Copy scenarios to molecule directory
		copyScenarios := fmt.Sprintf("if [ -d /tmp/repo/scenarios ]; then cp -r /tmp/repo/scenarios /opt/molecule/%s/molecule; fi", roleDirName)
		if err := utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", copyScenarios); err != nil {
			return fmt.Errorf("failed to copy scenarios in container: %w", err)
		}

		// Copy lint configs
		copyLintCmd := fmt.Sprintf("if [ -f /tmp/repo/.ansible-lint ]; then cp /tmp/repo/.ansible-lint /opt/molecule/%s/; fi && if [ -f /tmp/repo/.yamllint ]; then cp /tmp/repo/.yamllint /opt/molecule/%s/; fi", roleDirName, roleDirName)
		_ = utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", copyLintCmd)

		log.Printf("\033[32mCI Mode: Role files copied to /opt/molecule/%s\033[0m", roleDirName)

		// Verify molecule.yml exists
		verifyCmd := fmt.Sprintf("ls -la /opt/molecule/%s/molecule/default/molecule.yml", roleDirName)
		if err := utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", verifyCmd); err != nil {
			log.Printf("\033[31mCI Mode: molecule.yml not found!\033[0m")
			_ = utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", fmt.Sprintf("ls -laR /opt/molecule/%s/", roleDirName))
			return fmt.Errorf("molecule.yml not found in container")
		}
		log.Printf("\033[32mCI Mode: Setup complete!\033[0m")
	}

	// ensure role exists (skip in CI mode - already handled)
	if !cli.CIMode {
		if exists(roleMoleculePath) {
			fmt.Println("\033[35mThis role already exists in molecule\033[0m")
		} else {
			// docker exec -ti molecule-$role /bin/sh -c "cd /opt/molecule && ansible-galaxy role init $org.$role"
			// Ensure we're in the correct directory with write permissions
			if err := utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", fmt.Sprintf("ansible-galaxy role init %s.%s", cli.OrgFlag, cli.RoleFlag)); err != nil {
				log.Printf("\033[33mrole init warning: %v\033[0m", err)
			}

			// Fix ownership inside container after role init (Unix systems only)
			if runtime.GOOS != "windows" {
				uid := os.Getuid()
				gid := os.Getgid()
				chownCmd := fmt.Sprintf("chown -R %d:%d /opt/molecule/%s.%s", uid, gid, cli.OrgFlag, cli.RoleFlag)
				if err := utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", chownCmd); err != nil {
					log.Printf("\033[33mwarning: failed to fix ownership after role init: %v\033[0m", err)
				}
			}

			if err := utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", fmt.Sprintf("rm -f %s.%s/*/*", cli.OrgFlag, cli.RoleFlag)); err != nil {
				log.Printf("\033[33mclean role dir warning: %v\033[0m", err)
			}
		}
	}

	// docker exec login to registry inside container (provider-specific)
	switch cfg.ContainerRegistry.RegistryProvider {
	case "YC":
		// Yandex Cloud registry login
		_ = utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", `echo $TOKEN | docker login cr.yandex --username iam --password-stdin`)
	case "AWS":
		// AWS ECR login inside container
		loginCmd := fmt.Sprintf(`echo $TOKEN | docker login %s --username AWS --password-stdin`, cfg.ContainerRegistry.RegistryServer)
		_ = utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", loginCmd)
	case "GCP":
		// GCP Artifact Registry/GCR login inside container
		loginCmd := fmt.Sprintf(`echo $TOKEN | docker login %s --username oauth2accesstoken --password-stdin`, cfg.ContainerRegistry.RegistryServer)
		_ = utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", loginCmd)
	case "Public":
		// No login needed for public registries
		log.Printf("\033[35mUsing public registry, skipping authentication\033[0m")
	default:
		// Unknown provider, skip login
		log.Printf("\033[33mUnknown registry provider '%s', skipping authentication\033[0m", cfg.ContainerRegistry.RegistryProvider)
	}

	// copy files into molecule structure (skip in CI mode - already handled)
	if !cli.CIMode {
		if err := copyRoleData(path, roleMoleculePath, cli.CIMode); err != nil {
			log.Printf("\033[33mcopy role data warning: %v\033[0m", err)
		}
		utils.ExportLinters(cfg, roleMoleculePath, cli.CIMode, cli.RoleFlag, cli.OrgFlag)
	}

	// finally create/converge
	err = exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", cli.RoleFlag)).Run()
	if err == nil {
		// container exists
		_ = utils.DockerExecInteractiveHide(cli.RoleFlag, "uv-sync", cli.CIMode)
		_ = utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", fmt.Sprintf("cd ./%s && molecule converge", roleDirName))
	} else {
		// Sync UV dependencies with pyproject.toml from diffusion
		if err := utils.DockerExecInteractive(cli.RoleFlag, "uv-sync", cli.CIMode); err != nil {
			log.Printf("\033[33mWarning: uv-sync failed: %v\033[0m", err)
			log.Printf("\033[33mContinuing with existing dependencies...\033[0m")
		}
		_ = utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", fmt.Sprintf("cd ./%s && molecule create", roleDirName))
		_ = utils.DockerExecInteractive(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", fmt.Sprintf("cd ./%s && molecule converge", roleDirName))
	}

	// Fix permissions on molecule directory for Unix systems (skip in CI mode - no volume mount)
	// Container runs as root (for DinD), so we need to fix ownership inside the container
	if !cli.CIMode && runtime.GOOS != "windows" {
		uid := os.Getuid()
		gid := os.Getgid()
		chownCmd := fmt.Sprintf("chown -R %d:%d /opt/molecule", uid, gid)
		if err := utils.DockerExecInteractiveHide(cli.RoleFlag, "/bin/sh", cli.CIMode, "-c", chownCmd); err != nil {
			log.Printf("\033[33mwarning: failed to fix permissions: %v\033[0m", err)
		}
	}

	return nil
}
