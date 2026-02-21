package cli

import (
	"fmt"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/galaxy"
	"diffusion/internal/utils"

	"github.com/spf13/cobra"
)

// newRoleAddCollectionCmd creates the add-collection subcommand
func newRoleAddCollectionCmd(cli *CLI) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-collection [collection-name]",
		Short: "Add a collection to diffusion.toml (use 'deps sync' to update requirements.yml and meta.yml)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collectionName := args[0]

			// Parse collection name and version constraint
			name, versionConstraint := utils.ParseCollectionString(collectionName)

			// Resolve the actual version from Galaxy API if no constraint provided
			var resolvedVersion string
			if versionConstraint == "" || versionConstraint == "latest" {
				fmt.Printf("Resolving version for %s...\n", name)
				var err error
				resolvedVersion, err = galaxy.GetCollectionVersion(name, versionConstraint)
				if err != nil {
					return fmt.Errorf("failed to resolve collection version: %w", err)
				}
				fmt.Printf("Resolved %s to version %s\n", name, resolvedVersion)
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
			configVersionConstraint := versionConstraint
			if configVersionConstraint == "" || configVersionConstraint == "latest" {
				configVersionConstraint = ">=" + resolvedVersion
			}

			// Check if collection already exists in config
			configCollExists := false
			for i, coll := range cfg.DependencyConfig.Collections {
				if coll.Name == name {
					cfg.DependencyConfig.Collections[i] = config.CollectionRequirement{Name: name, Version: configVersionConstraint}
					configCollExists = true
					break
				}
			}
			if !configCollExists {
				cfg.DependencyConfig.Collections = append(cfg.DependencyConfig.Collections, config.CollectionRequirement{Name: name, Version: configVersionConstraint})
			}

			// Save diffusion.toml
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save diffusion.toml: %w", err)
			}
			err = dependency.UpdateLockFile()
			if err != nil {
				return fmt.Errorf("failed to update lock file: %w", err)
			}
			fmt.Printf("\033[32mCollection '%s' (version %s) added successfully to diffusion.toml and diffusion.lock\n\033[0m", name, configVersionConstraint)

			return nil
		},
	}

	cmd.Flags().StringVarP(&cli.RoleScenario, "scenario", "s", "default", "Molecule scenarios folder to use")
	return cmd
}

func newRoleRemoveCollectionCmd(cli *CLI) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-collection [collection-name]",
		Short: "Remove a collection from diffusion.toml (use 'deps sync' to update requirements.yml and meta.yml)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collectionName := args[0]

			// Parse collection name (ignore version for removal)
			name, _ := dependency.ParseCollectionString(collectionName)

			// Remove from diffusion.toml
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load diffusion.toml: %w", err)
			}

			if cfg.DependencyConfig == nil {
				return fmt.Errorf("no dependencies found in diffusion.toml")
			}

			found := false
			for i, coll := range cfg.DependencyConfig.Collections {
				if coll.Name == name {
					cfg.DependencyConfig.Collections = append(cfg.DependencyConfig.Collections[:i], cfg.DependencyConfig.Collections[i+1:]...)
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("collection '%s' not found in diffusion.toml", name)
			}

			// Save diffusion.toml
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save diffusion.toml: %w", err)
			}
			err = dependency.UpdateLockFile()
			if err != nil {
				return fmt.Errorf("failed to update lock file: %w", err)
			}
			fmt.Printf("\033[32mCollection '%s' removed successfully from diffusion.toml and diffusion.lock\n\033[0m", name)
			fmt.Printf("\033[33mRun 'diffusion deps sync' to update requirements.yml and meta/main.yml\n\033[0m")

			return nil
		},
	}

	cmd.Flags().StringVarP(&cli.RoleScenario, "scenario", "s", "default", "Molecule scenarios folder to use")
	return cmd
}
