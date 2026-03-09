package cli

import (
	"fmt"
	"log"
	"os"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/galaxy"
	"diffusion/internal/role"

	"github.com/spf13/cobra"
)

// NewRoleCmd creates the role command with subcommands
func NewRoleCmd(cli *CLI) *cobra.Command {
	roleCmd := &cobra.Command{
		Use:   "role",
		Short: "Configure role settings interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle --init flag
			if cli.RoleInitFlag {
				// Check if role already exists in current directory
				if _, err := os.Stat("meta/main.yml"); err == nil {
					return fmt.Errorf("role already exists in current directory (meta/main.yml found)")
				}

				roleName, err := AnsibleGalaxyInit()
				if err != nil {
					return fmt.Errorf("failed to initialize role: %w", err)
				}

				// Change working directory to the newly created role
				if err := os.Chdir(roleName); err != nil {
					return fmt.Errorf("failed to change directory to %s: %w", roleName, err)
				}

				MetaConfig := MetaConfigSetup(roleName)
				RequirementConfig := RequirementConfigSetup(MetaConfig.Collections)
				err = role.SaveMetaFile(MetaConfig)
				if err != nil {
					return fmt.Errorf("failed to save meta file: %w", err)
				}

				err = role.SaveRequirementFile(RequirementConfig, "default")
				if err != nil {
					return fmt.Errorf("failed to save requirements file: %w", err)
				}

				fmt.Println("Role initialized successfully.")
				return nil
			}

			// Load existing role config
			meta, req, err := role.LoadRoleConfig("")
			if err != nil {
				return fmt.Errorf("role config not found. Use 'diffusion role --init' to initialize a new role: %w", err)
			}

			// Display current role configuration
			if meta != nil {
				fmt.Printf("\033[35mCurrent Role Name: \033[0m\033[38;2;127;255;212m%s\033[0m\n", meta.GalaxyInfo.RoleName)
				fmt.Printf("\033[35mCurrent Namespace: \033[0m\033[38;2;127;255;212m%s\033[0m\n", meta.GalaxyInfo.Namespace)
			}
			if req != nil {
				fmt.Printf("\033[35mCurrent Collections:\n\033[0m")
				for _, collection := range req.Collections {
					fmt.Printf("\033[38;2;127;255;212m  - %v\n\033[0m", collection)
				}
				fmt.Printf("\033[35mCurrent Roles:\n\033[0m")
				for _, role := range req.Roles {
					fmt.Printf("\033[38;2;127;255;212m  - %v\n\033[0m", role)
				}
			}
			return nil
		},
	}

	// Add flags
	roleCmd.Flags().StringVarP(&cli.RoleScenario, "scenario", "s", "default", "Molecule scenarios folder to use")
	roleCmd.Flags().BoolVarP(&cli.RoleInitFlag, "init", "i", false, "Initialize a new Ansible role using ansible-galaxy")

	// Add subcommands
	roleCmd.AddCommand(newRoleAddRoleCmd(cli))
	roleCmd.AddCommand(newRoleRemoveRoleCmd(cli))
	roleCmd.AddCommand(NewRoleAddCollectionCmd(cli))
	roleCmd.AddCommand(NewRoleRemoveCollectionCmd(cli))

	return roleCmd
}

func newRoleAddRoleCmd(cli *CLI) *cobra.Command {
	roleAddRoleCmd := &cobra.Command{
		Use:   "add-role [role-name]",
		Short: "Add a role and diffusion.toml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			roleName := args[0]

			// Validate: dots are forbidden in role names (dots are reserved as scenario name prefixes)
			if strings.Contains(roleName, ".") {
				return fmt.Errorf("dots are not allowed in role names (dots are reserved as scenario prefixes). Use --namespace/-n to specify the Galaxy namespace separately.\nExample: diffusion role add-role %s --namespace %s",
					strings.SplitN(roleName, ".", 2)[1], strings.SplitN(roleName, ".", 2)[0])
			}

			if strings.HasSuffix(cli.RoleSrcFlag, ".git") {
				cli.RoleScmFlag = "git"

			} else {
				cli.RoleScmFlag = "galaxy"
			}

			// Require --namespace when using Galaxy source (no --src git URL provided)
			if cli.RoleScmFlag == "galaxy" && cli.NamespaceFlag == "" {
				return fmt.Errorf("--namespace/-n is required for Galaxy roles (no --src git URL provided).\nExample: diffusion role add-role %s --namespace <namespace>", roleName)
			}

			// Resolve the actual version from Galaxy API or Git if no version provided
			resolvedVersion := cli.RoleVersionFlag
			if resolvedVersion == "" || resolvedVersion == "latest" || resolvedVersion == "main" || resolvedVersion == "master" {
				fmt.Printf("Resolving version for role %s...\n", roleName)

				// If src is provided and it's a git URL, try to resolve from git
				switch cli.RoleScmFlag {
				case "git":
					if resolvedVersion == "" {
						resolvedVersion = "main"
					}
					resolved, err := galaxy.ResolveVersionFromGit(cli.RoleSrcFlag, resolvedVersion)
					if err != nil {
						fmt.Printf("\033[33mWarning: Failed to resolve role version from git: %v\033[0m\n", err)
						fmt.Printf("\033[33mTrying Galaxy API...\033[0m\n")
						resolved, err = galaxy.GetRoleVersion(cli.NamespaceFlag, roleName, resolvedVersion)
						if err != nil {
							fmt.Printf("\033[33mWarning: Failed to resolve role version from Galaxy: %v\033[0m\n", err)
							fmt.Printf("\033[33mUsing 'main' as default version\033[0m\n")
							resolved, err = galaxy.ResolveVersionFromGit(cli.RoleSrcFlag, resolvedVersion)
							if err == nil {
								resolvedVersion = galaxy.NormalizeVersion(resolved)
							} else {
								resolvedVersion = "not-defined"
							}
						} else {
							resolvedVersion = resolved
							fmt.Printf("Resolved %s to version %s from Galaxy\n", roleName, resolvedVersion)
						}
					} else {
						resolvedVersion = resolved
						fmt.Printf("Resolved %s to version %s from git\n", roleName, resolvedVersion)
					}
				case "galaxy":
					// Try Galaxy API with separate namespace and name
					resolved, err := galaxy.GetRoleVersion(cli.NamespaceFlag, roleName, resolvedVersion)
					if err != nil {
						fmt.Printf("\033[33mWarning: Failed to resolve role version: %v\033[0m\n", err)
						fmt.Printf("\033[33mUsing 'main' as default version\033[0m\n")
						resolvedVersion = "main"
					} else {
						resolvedVersion = resolved
						fmt.Printf("Resolved %s to version %s\n", roleName, resolvedVersion)
					}
				}
			}

			// Add to diffusion.toml dependencies
			cfg, err := config.LoadConfig()
			if err != nil {
				// Create new config if it doesn't exist
				cfg = &config.Config{}
			}

			if cfg.DependencyConfig == nil {
				cfg.DependencyConfig = &config.DependencyConfig{}
			}

			// Determine version constraint for diffusion.toml
			// If no constraint was provided, use >=<resolved_version>
			configVersionConstraint := cli.RoleVersionFlag
			if strings.Contains(cli.RoleVersionFlag, "<") ||
				strings.Contains(cli.RoleVersionFlag, ">") ||
				strings.Contains(cli.RoleVersionFlag, "==") ||
				strings.Contains(cli.RoleVersionFlag, "=") ||
				strings.Contains(cli.RoleVersionFlag, ">=") ||
				strings.Contains(cli.RoleVersionFlag, "<=") {
				configVersionConstraint = cli.RoleVersionFlag
			} else {
				switch cli.RoleScmFlag {
				case "git":
					configVersionConstraint, err = galaxy.ResolveVersionFromGit(cli.RoleSrcFlag, cli.RoleVersionFlag)
					if err != nil {
						log.Printf("Warning: Failed to get role version from Galaxy: %v\n", err)
						configVersionConstraint = "1.0.0"
					}
				case "galaxy":
					configVersionConstraint, err = galaxy.GetRoleVersion(cli.NamespaceFlag, roleName, cli.RoleVersionFlag)
					if err != nil {
						log.Printf("Warning: Failed to get role version from Galaxy: %v\n", err)
						configVersionConstraint = "1.0.0"
					}
				}

			}
			if cli.RoleVersionFlag == "" || cli.RoleVersionFlag == "latest" || cli.RoleVersionFlag == "main" || cli.RoleVersionFlag == "master" {
				// Strip 'v' prefix for constraint if present
				constraintVersion := strings.TrimPrefix(resolvedVersion, "v")
				configVersionConstraint = ">=" + constraintVersion
			}

			// Check if role already exists in config
			// Note: Roles are stored per scenario
			roleKey := cli.RoleScenario + "." + roleName
			configRoleExists := false
			for i, role := range cfg.DependencyConfig.Roles {
				if role.Name == roleKey {
					cfg.DependencyConfig.Roles[i] = config.RoleRequirement{
						Name:      roleKey,
						Namespace: cli.NamespaceFlag,
						Src:       cli.RoleSrcFlag,
						Version:   configVersionConstraint,
						Scm:       cli.RoleScmFlag,
					}
					configRoleExists = true
					break
				}
			}
			if !configRoleExists {
				if cfg.DependencyConfig.Roles == nil {
					cfg.DependencyConfig.Roles = []config.RoleRequirement{}
				}
				cfg.DependencyConfig.Roles = append(cfg.DependencyConfig.Roles, config.RoleRequirement{
					Name:      roleKey,
					Namespace: cli.NamespaceFlag,
					Src:       cli.RoleSrcFlag,
					Version:   configVersionConstraint,
					Scm:       cli.RoleScmFlag,
				})
			}

			// Save diffusion.toml
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save diffusion.toml: %w", err)
			}

			err = dependency.UpdateLockFile()
			if err != nil {
				return fmt.Errorf("failed to update lock file: %w", err)
			}
			fmt.Printf("\033[32mRole '%s' (version %s) added successfully to diffusion.toml and diffusion.lock\n\033[0m", roleKey, configVersionConstraint)

			return nil
		},
	}

	roleAddRoleCmd.Flags().StringVarP(&cli.RoleScenario, "scenario", "s", "default", "Molecule scenarios folder to use")
	roleAddRoleCmd.Flags().StringVarP(&cli.RoleSrcFlag, "src", "", "", "Source URL of the role (required)")
	roleAddRoleCmd.Flags().StringVarP(&cli.RoleScmFlag, "scm", "", "git", "SCM type (e.g., git) of the role (optional)")
	roleAddRoleCmd.Flags().StringVarP(&cli.RoleVersionFlag, "version", "v", "main", "Version of the role (optional)")
	roleAddRoleCmd.Flags().StringVarP(&cli.NamespaceFlag, "namespace", "n", "", "Namespace for galaxy roles (optional)")

	return roleAddRoleCmd
}

func newRoleRemoveRoleCmd(cli *CLI) *cobra.Command {
	roleRemoveRoleCmd := &cobra.Command{
		Use:   "remove-role [role-name]",
		Short: "Remove a role from diffusion.toml (keeps it in requirements.yml)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			roleName := args[0]

			// Validate: dots are forbidden in role names
			if strings.Contains(roleName, ".") {
				return fmt.Errorf("dots are not allowed in role names (dots are reserved as scenario prefixes)")
			}

			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.DependencyConfig == nil {
				return fmt.Errorf("no dependencies configured in diffusion.toml")
			}

			// Find and remove the role from diffusion.toml
			roleKey := cli.RoleScenario + "." + roleName
			found := false
			for i, role := range cfg.DependencyConfig.Roles {
				if role.Name == roleKey {
					// Remove the role from the slice completely
					cfg.DependencyConfig.Roles = append(cfg.DependencyConfig.Roles[:i], cfg.DependencyConfig.Roles[i+1:]...)
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("role '%s' not found in diffusion.toml", roleKey)
			}

			// Save diffusion.toml
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save diffusion.toml: %w", err)
			}
			err = dependency.UpdateLockFile()
			if err != nil {
				return fmt.Errorf("failed to update lock file: %w", err)
			}
			fmt.Printf("\033[32mRole '%s' removed successfully from diffusion.toml and diffusion.lock\n\033[0m", roleKey)
			fmt.Printf("\033[33mNote: Role remains in requirements.yml (use 'deps sync' to update if needed)\n\033[0m")
			return nil
		},
	}

	roleRemoveRoleCmd.Flags().StringVarP(&cli.RoleScenario, "scenario", "s", "default", "Molecule scenarios folder to use")
	roleRemoveRoleCmd.Flags().StringVarP(&cli.NamespaceFlag, "namespace", "n", "", "Namespace for galaxy roles (optional)")

	return roleRemoveRoleCmd
}
