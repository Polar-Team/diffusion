package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/role"
	"diffusion/internal/utils"

	"github.com/spf13/cobra"
)

// NewDepsCmd creates the deps command with subcommands
func NewDepsCmd(cli *CLI) *cobra.Command {
	depsCmd := &cobra.Command{
		Use:   "deps",
		Short: "Manage dependencies (collections, roles, Python packages)",
		Long: `Manage project dependencies including Ansible collections, roles, and Python packages.
Generates diffusion.lock file and updates pyproject.toml for the molecule container.`,
	}

	depsCmd.AddCommand(newDepsLockCmd())
	depsCmd.AddCommand(newDepsCheckCmd())
	depsCmd.AddCommand(newDepsResolveCmd())
	depsCmd.AddCommand(newDepsInitCmd())
	depsCmd.AddCommand(newDepsSyncCmd())

	return depsCmd
}

// newDepsLockCmd creates the lock subcommand
func newDepsLockCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lock",
		Short: "Generate or update diffusion.lock file",
		Long: `Generate or update the diffusion.lock file based on current dependencies
from meta/main.yml, requirements.yml, and diffusion.toml configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("Generating lock file...")
			if err := dependency.UpdateLockFile(); err != nil {
				return fmt.Errorf("failed to update lock file: %w", err)
			}
			fmt.Printf("\033[32m%s\033[0m\n", config.MsgLockFileGenerated)
			return nil
		},
	}
}

// newDepsCheckCmd creates the check subcommand
func newDepsCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check if lock file is up-to-date",
		Long:  `Check if the diffusion.lock file is up-to-date with current dependencies.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			upToDate, err := dependency.CheckLockFileStatus()
			if err != nil {
				return fmt.Errorf("failed to check lock file: %w", err)
			}
			if upToDate {
				fmt.Printf("\033[32m%s\033[0m\n", config.MsgLockFileUpToDate)
			} else {
				fmt.Printf("\033[33mLock file is not fitting yaml manifests. Run 'diffusion deps sync' to update.\033[0m\n")
				os.Exit(1)
			}
			return nil
		},
	}
}

// newDepsResolveCmd creates the resolve subcommand
func newDepsResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve",
		Short: "Resolve and display all dependencies with actual versions",
		Long:  `Resolve all dependencies from diffusion.lock and display them with actual resolved versions.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load lock file
			lockFile, err := dependency.LoadLockFile()
			if err != nil {
				return fmt.Errorf("failed to load lock file: %w", err)
			}
			if lockFile == nil {
				return fmt.Errorf("lock file not found. Run 'diffusion deps lock' first")
			}

			// Display resolved dependencies
			fmt.Println("\033[1m=== Resolved Dependencies ===\033[0m")
			fmt.Println()
			fmt.Println("\033[1mPython:\033[0m")
			if lockFile.Python != nil {
				fmt.Printf("  Pinned: \033[38;2;127;255;212m%s\033[0m\n", lockFile.Python.Pinned)
				fmt.Printf("  Min: \033[38;2;127;255;212m%s\033[0m (major.minor)\n", lockFile.Python.Min)
				fmt.Printf("  Max: \033[38;2;127;255;212m%s\033[0m (major.minor)\n", lockFile.Python.Max)
				// Additional versions are not used for container
			}
			fmt.Println()

			fmt.Println("\033[1mTools:\033[0m")
			for _, tool := range lockFile.Tools {
				constraint := tool.Version
				resolved := tool.ResolvedVersion
				if resolved != "" && resolved != constraint {
					fmt.Printf("  %s: \033[38;2;127;255;212m%s\033[0m (constraint: %s)\n", tool.Name, resolved, constraint)
				} else if resolved != "" {
					fmt.Printf("  %s: \033[38;2;127;255;212m%s\033[0m\n", tool.Name, resolved)
				} else {
					fmt.Printf("  %s: \033[38;2;127;255;212m%s\033[0m\n", tool.Name, constraint)
				}
			}
			fmt.Println()

			fmt.Println("\033[1mCollections:\033[0m")
			for _, col := range lockFile.Collections {
				// Display as namespace.name (scenario) format for readability
				displayName := col.Name
				scenario := ""
				if parts := strings.SplitN(col.Name, ".", 2); len(parts) == 2 {
					scenario = parts[0]
					colName := parts[1]
					if col.Namespace != "" {
						displayName = col.Namespace + "." + colName
					} else {
						displayName = colName
					}
				}
				constraint := col.Version
				resolved := col.ResolvedVersion
				scenarioLabel := ""
				if scenario != "" && scenario != "default" {
					scenarioLabel = fmt.Sprintf(" [%s]", scenario)
				}
				if resolved != "" && resolved != constraint {
					fmt.Printf("  %s%s: \033[38;2;127;255;212m%s\033[0m (constraint: %s)\n", displayName, scenarioLabel, resolved, constraint)
				} else if resolved != "" {
					fmt.Printf("  %s%s: \033[38;2;127;255;212m%s\033[0m\n", displayName, scenarioLabel, resolved)
				} else {
					fmt.Printf("  %s%s: \033[38;2;127;255;212m%s\033[0m\n", displayName, scenarioLabel, constraint)
				}
			}
			fmt.Println()

			if len(lockFile.Roles) > 0 {
				fmt.Println("\033[1mRoles:\033[0m")
				for _, r := range lockFile.Roles {
					// Display as namespace.rolename (scenario) format for readability
					displayName := r.Name
					scenario := ""
					if parts := strings.SplitN(r.Name, ".", 2); len(parts) == 2 {
						scenario = parts[0]
						roleName := parts[1]
						if r.Namespace != "" && r.Src == "" {
							displayName = r.Namespace + "." + roleName
						} else {
							displayName = roleName
						}
					}
					constraint := r.Version
					resolved := r.ResolvedVersion
					scenarioLabel := ""
					if scenario != "" && scenario != "default" {
						scenarioLabel = fmt.Sprintf(" [%s]", scenario)
					}
					if resolved != "" && resolved != constraint {
						fmt.Printf("  %s%s: \033[38;2;127;255;212m%s\033[0m (constraint: %s)\n", displayName, scenarioLabel, resolved, constraint)
					} else if resolved != "" {
						fmt.Printf("  %s%s: \033[38;2;127;255;212m%s\033[0m\n", displayName, scenarioLabel, resolved)
					} else {
						fmt.Printf("  %s%s: \033[38;2;127;255;212m%s\033[0m\n", displayName, scenarioLabel, constraint)
					}
				}
				fmt.Println()
			}

			fmt.Printf("\033[32m%s\033[0m\n", config.MsgDependenciesResolved)
			return nil
		},
	}
}

// newDepsInitCmd creates the init subcommand
func newDepsInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize dependency configuration in diffusion.toml",
		Long:  `Initialize dependency configuration section in diffusion.toml with default values and scan existing requirements.yml files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load or create config
			cfg, err := config.LoadConfig()
			if err != nil {
				cfg = &config.Config{}
			}

			// Check if dependency config already exists
			if cfg.DependencyConfig != nil {
				fmt.Println("\033[33mDependency configuration already exists in diffusion.toml\033[0m")
				return nil
			}

			// Create default dependency config
			cfg.DependencyConfig = &config.DependencyConfig{
				Python: &config.PythonVersion{
					Min:    config.DefaultMinPythonVersion,
					Max:    config.DefaultMaxPythonVersion,
					Pinned: config.PinnedPythonVersion,
				},
				Ansible:     config.DefaultAnsibleVersion,
				AnsibleLint: config.DefaultAnsibleLintVersion,
				Molecule:    config.DefaultMoleculeVersion,
				YamlLint:    config.DefaultYamlLintVersion,
				Collections: []config.CollectionRequirement{},
				Roles:       []config.RoleRequirement{},
			}

			// Scan for existing requirements.yml files in scenario folders
			fmt.Println("Scanning for existing requirements.yml files...")
			scenariosDir := "scenarios"
			if _, err := os.Stat(scenariosDir); err == nil {
				// Read all scenario folders
				entries, err := os.ReadDir(scenariosDir)
				if err == nil {
					for _, entry := range entries {
						if entry.IsDir() {
							scenarioName := entry.Name()
							reqPath := filepath.Join(scenariosDir, scenarioName, "requirements.yml")

							// Check if requirements.yml exists
							if _, err := os.Stat(reqPath); err == nil {
								fmt.Printf("  Found requirements.yml in scenario: %s\n", scenarioName)

								// Parse requirements file
								req, err := role.ParseRequirementFile(scenarioName)
								if err != nil {
									fmt.Printf("    \033[33mWarning: Failed to parse requirements.yml: %v\033[0m\n", err)
									continue
								}

								// Add collections from this scenario
								for _, col := range req.Collections {
									// Parse namespace and name from collection name (format: "namespace.name")
									colParts := strings.SplitN(col.Name, ".", 2)
									var namespace, colName string
									if len(colParts) == 2 {
										namespace = colParts[0]
										colName = colParts[1]
									} else {
										namespace = ""
										colName = col.Name
									}

									// Config name is prefixed with scenario (e.g., "default.general")
									configName := scenarioName + "." + colName

									// Check if collection already exists
									exists := false
									for _, existing := range cfg.DependencyConfig.Collections {
										if existing.Name == configName {
											exists = true
											break
										}
									}
									if !exists {
										// Add version constraint (>= if specific version found)
										version := col.Version
										if version != "" && !strings.HasPrefix(version, ">=") && !strings.HasPrefix(version, "<=") && !strings.HasPrefix(version, "==") && !strings.HasPrefix(version, ">") && !strings.HasPrefix(version, "<") {
											version = ">=" + version
										}
										cfg.DependencyConfig.Collections = append(cfg.DependencyConfig.Collections, config.CollectionRequirement{
											Name:      configName,
											Namespace: namespace,
											Version:   version,
										})
										fmt.Printf("    + Added collection: %s (namespace: %s) %s\n", configName, namespace, version)
									}
								}

								// Add roles from this scenario
								for _, role := range req.Roles {
									// Prefix role name with scenario name
									// Extract namespace from role name if present (format: "namespace.rolename")
									var namespace, actualRoleName string
									roleParts := strings.SplitN(role.Name, ".", 2)
									if len(roleParts) == 2 && role.Src == "" {
										// Looks like "namespace.rolename" Galaxy format
										namespace = roleParts[0]
										actualRoleName = roleParts[1]
									} else {
										namespace = ""
										actualRoleName = role.Name
									}
									roleNameWithScenario := scenarioName + "." + actualRoleName

									// Check if role already exists
									exists := false
									for _, existing := range cfg.DependencyConfig.Roles {
										if existing.Name == roleNameWithScenario {
											exists = true
											break
										}
									}
									if !exists {
										// Add version constraint (>= if specific version found)
										version := role.Version
										if version != "" && version != "main" && version != "master" && !strings.HasPrefix(version, ">=") && !strings.HasPrefix(version, "<=") && !strings.HasPrefix(version, "==") && !strings.HasPrefix(version, ">") && !strings.HasPrefix(version, "<") {
											version = ">=" + version
										}
										version = strings.Replace(version, "v", "", 1)
										cfg.DependencyConfig.Roles = append(cfg.DependencyConfig.Roles, config.RoleRequirement{
											Name:      roleNameWithScenario,
											Namespace: namespace,
											Src:       role.Src,
											Scm:       role.Scm,
											Version:   version,
										})
										fmt.Printf("    + Added role: %s (namespace: %s) %s\n", roleNameWithScenario, namespace, version)
									}
								}
							}
						}
					}
				}
			}

			// Also check meta/main.yml for collections
			metaPath := "meta/main.yml"
			if _, err := os.Stat(metaPath); err == nil {
				fmt.Println("  Found meta/main.yml")
				meta, err := role.ParseMetaFile()
				if err == nil {
					for _, col := range meta.Collections {
						// Parse collection string (format: "namespace.name" or "namespace.name>=version")
						fullName, version := utils.ParseCollectionString(col)

						// Extract namespace and name from the collection string
						colParts := strings.SplitN(fullName, ".", 2)
						var namespace, colName string
						if len(colParts) == 2 {
							namespace = colParts[0]
							colName = colParts[1]
						} else {
							namespace = ""
							colName = fullName
						}

						// Config name is prefixed with "default" scenario (meta.yml is for default scenario)
						configName := "default." + colName

						// Check if collection already exists
						exists := false
						for _, existing := range cfg.DependencyConfig.Collections {
							if existing.Name == configName {
								exists = true
								break
							}
						}
						if !exists {
							// Add version constraint
							if version == "" {
								version = ">=1.0.0" // Default constraint
							}
							cfg.DependencyConfig.Collections = append(cfg.DependencyConfig.Collections, config.CollectionRequirement{
								Name:      configName,
								Namespace: namespace,
								Version:   version,
							})
							fmt.Printf("    + Added collection from meta: %s (namespace: %s) %s\n", configName, namespace, version)
						}
					}
				}
			}

			// Save config
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Println("\033[32mDependency configuration initialized in diffusion.toml\033[0m")
			if len(cfg.DependencyConfig.Collections) > 0 {
				fmt.Printf("\033[32m  Collections found: %d\033[0m\n", len(cfg.DependencyConfig.Collections))
			}
			if len(cfg.DependencyConfig.Roles) > 0 {
				fmt.Printf("\033[32m  Roles found: %d\033[0m\n", len(cfg.DependencyConfig.Roles))
			}
			return nil
		},
	}
}

// newDepsSyncCmd creates the sync subcommand
func newDepsSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Sync dependencies from lock file to requirements.yml and meta.yml",
		Long:  `Restore dependency versions from diffusion.lock to requirements.yml and meta.yml. Useful for rollback scenarios.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Load lock file
			lockFile, err := dependency.LoadLockFile()
			if err != nil {
				return fmt.Errorf("failed to load lock file: %w", err)
			}
			if lockFile == nil {
				return fmt.Errorf("lock file not found. Run 'diffusion deps lock' first")
			}
			scenarios := []string{}
			scenariosDir := "scenarios"
			if _, err := os.Stat(scenariosDir); err == nil {
				// Read all scenario folders
				entries, err := os.ReadDir(scenariosDir)
				if err == nil {
					for _, entry := range entries {
						if entry.IsDir() {
							scenarios = append(scenarios, entry.Name())
						}
					}
				}
			}
			if len(scenarios) == 0 {
				scenarios = append(scenarios, "default")
			}

			// Sync roles and collections to requirements.yml for each scenario
			for _, scenario := range scenarios {
				// Load current role config
				_, req, err := role.LoadRoleConfig(scenario)
				if err != nil {
					return fmt.Errorf("failed to load role config: %w", err)
				}

				// Sync collections to requirements.yml (structured format with resolved versions)
				// Collections are scenario-prefixed in lock file (e.g., "default.general")
				fmt.Printf("Syncing collections to requirements.yml for %s...\n", scenario)
				req.Collections = []role.RequirementCollection{}
				colPrefix := scenario + "."
				for _, col := range lockFile.Collections {
					if !strings.HasPrefix(col.Name, colPrefix) {
						continue
					}
					// Strip scenario prefix from collection name
					colName := strings.TrimPrefix(col.Name, colPrefix)
					// Reconstruct namespace.name format for YAML output
					yamlName := colName
					if col.Namespace != "" {
						yamlName = col.Namespace + "." + colName
					}

					version := col.ResolvedVersion
					if version == "" {
						version = col.Version
					}
					req.Collections = append(req.Collections, role.RequirementCollection{
						Name:    yamlName,
						Version: version,
					})
					fmt.Printf("  + %s: %s\n", yamlName, version)
				}

				// Sync roles to requirements.yml
				fmt.Printf("Syncing roles to requirements.yml for %s...\n", scenario)
				req.Roles = []role.RequirementRole{}
				scenarioRoles := []dependency.LockFileEntry{}
				rolePrefix := scenario + "."
				// Filter roles for this scenario
				for _, lockRole := range lockFile.Roles {
					if strings.HasPrefix(lockRole.Name, rolePrefix) {
						scenarioRoles = append(scenarioRoles, lockRole)
					}
				}
				// Now add the filtered roles to requirements.yml
				for _, lockRole := range scenarioRoles {
					version := lockRole.ResolvedVersion
					if version == "" {
						version = lockRole.Version
					}
					// If still no version, default to "main"
					if version == "" || version == "latest" {
						version = "main"
					}

					// Remove scenario prefix from role name
					roleName := strings.TrimPrefix(lockRole.Name, rolePrefix)

					// Reconstruct namespace.rolename for YAML output (Galaxy roles)
					yamlRoleName := roleName
					if lockRole.Namespace != "" && lockRole.Src == "" {
						yamlRoleName = lockRole.Namespace + "." + roleName
					}

					req.Roles = append(req.Roles, role.RequirementRole{
						Name:    yamlRoleName,
						Version: version,
						Src:     lockRole.Src,    // Restore git URL
						Scm:     lockRole.Source, // Restore SCM type
					})
					fmt.Printf("  + %s: %s\n", yamlRoleName, version)
				}
				// Save requirements.yml
				if err := role.SaveRequirementFile(req, scenario); err != nil {
					return fmt.Errorf("failed to save requirements.yml: %w", err)
				}
				fmt.Printf("\033[32m+ requirements.yml updated for %s\033[0m\n", scenario)

			}
			// Sync collections to meta/main.yml
			// Only collections from the "default" scenario go into meta.yml

			meta, _, err := role.LoadRoleConfig("")
			if err != nil {
				return fmt.Errorf("failed to load meta config: %w", err)
			}

			// Sync collections to meta.yml (simple string format - namespace.name, no versions)
			fmt.Println("Syncing collections to meta.yml (default scenario only)...")
			meta.Collections = []string{}
			defaultColPrefix := "default."
			for _, col := range lockFile.Collections {
				if !strings.HasPrefix(col.Name, defaultColPrefix) {
					continue // Only default scenario collections go into meta.yml
				}
				// Strip scenario prefix and reconstruct namespace.name format
				colName := strings.TrimPrefix(col.Name, defaultColPrefix)
				metaName := colName
				if col.Namespace != "" {
					metaName = col.Namespace + "." + colName
				}
				meta.Collections = append(meta.Collections, metaName)
				fmt.Printf("  + %s\n", metaName)
			}

			// Save meta.yml
			if err := role.SaveMetaFile(meta); err != nil {
				return fmt.Errorf("failed to save meta.yml: %w", err)
			}
			fmt.Printf("\033[32m+ meta.yml updated\033[0m\n")

			fmt.Printf("\033[32mDependencies synced successfully from lock file\033[0m\n")
			return nil
		},
	}
}
