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

	return roleCmd
}

func newRoleAddRoleCmd(cli *CLI) *cobra.Command {
	roleAddRoleCmd := &cobra.Command{
		Use:   "add-role [role-name]",
		Short: "Add a role and diffusion.toml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			roleName := args[0]

			// Resolve the actual version from Galaxy API or Git if no version provided
			resolvedVersion := cli.RoleVersionFlag
			if resolvedVersion == "" || resolvedVersion == "latest" || resolvedVersion == "main" || resolvedVersion == "master" {
				fmt.Printf("Resolving version for role %s...\n", roleName)

				// If src is provided and it's a git URL, try to resolve from git
				if cli.RoleSrcFlag != "" && (strings.HasSuffix(cli.RoleSrcFlag, ".git")) {
					resolved, err := galaxy.ResolveVersionFromGit(cli.RoleSrcFlag, resolvedVersion)
					if err != nil {
						fmt.Printf("\033[33mWarning: Failed to resolve role version from git: %v\033[0m\n", err)
						fmt.Printf("\033[33mTrying Galaxy API...\033[0m\n")
						resolved, err = galaxy.GetRoleVersion(roleName, resolvedVersion)
						if err != nil {
							fmt.Printf("\033[33mWarning: Failed to resolve role version from Galaxy: %v\033[0m\n", err)
							fmt.Printf("\033[33mUsing 'main' as default version\033[0m\n")
							// try to resolve version from git repository
							resolved, err = galaxy.ResolveVersionFromGit(cli.RoleSrcFlag, resolvedVersion)
							if err == nil {
								resolvedVersion = resolved
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
				} else {
					// Try Galaxy API
					resolved, err := galaxy.GetRoleVersion(roleName, resolvedVersion)
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

			// Normalize version (add 'v' prefix if needed for git tags)
			if cli.RoleSrcFlag != "" && (strings.HasSuffix(cli.RoleSrcFlag, ".git")) {
				resolvedVersion = galaxy.NormalizeVersion(resolvedVersion)
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
			configVersionConstraint, err := galaxy.GetRoleVersion(roleName, cli.RoleVersionFlag)
			if err != nil {
				// If GetRoleVersion from galaxy failed trying git
				log.Printf("Warning: Failed to get role version from Galaxy: %v\n", err)
				configVersionConstraint, err = galaxy.ResolveVersionFromGit(cli.RoleSrcFlag, cli.RoleVersionFlag)
				if err != nil {
					log.Printf("Warning: Failed to get role version from git: %v\n", err)
					configVersionConstraint = "1.0.0"
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
						Name:    roleKey,
						Src:     cli.RoleSrcFlag,
						Version: configVersionConstraint,
						Scm:     cli.RoleScmFlag,
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
					Name:    roleKey,
					Src:     cli.RoleSrcFlag,
					Version: configVersionConstraint,
					Scm:     cli.RoleScmFlag,
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

	return roleAddRoleCmd
}

func newRoleRemoveRoleCmd(cli *CLI) *cobra.Command {
	roleRemoveRoleCmd := &cobra.Command{
		Use:   "remove-role [role-name]",
		Short: "Remove a role from diffusion.toml (keeps it in requirements.yml)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			roleName := args[0]

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

	return roleRemoveRoleCmd
}
