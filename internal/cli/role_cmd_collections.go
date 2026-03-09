package cli

import (
	"fmt"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/galaxy"
	"diffusion/internal/utils"

	"github.com/spf13/cobra"
)

// NewRoleAddCollectionCmd creates the add-collection subcommand
func NewRoleAddCollectionCmd(cli *CLI) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-collection [collection-name]",
		Short: "Add a collection to diffusion.toml (use 'deps sync' to update requirements.yml and meta.yml)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collectionName := args[0]

			// Parse collection name and version constraint
			name, versionConstraint := utils.ParseCollectionString(collectionName)

			// Validate: dots are forbidden in collection names (dots are reserved as scenario name prefixes)
			if strings.Contains(name, ".") {
				return fmt.Errorf("dots are not allowed in collection names (dots are reserved as scenario prefixes). Use --namespace/-n to specify the Galaxy namespace separately.\nExample: diffusion role add-collection %s --namespace %s",
					strings.SplitN(name, ".", 2)[1], strings.SplitN(name, ".", 2)[0])
			}

			// Require --namespace for Galaxy collections
			if cli.NamespaceFlag == "" {
				return fmt.Errorf("--namespace/-n is required for collections.\nExample: diffusion role add-collection %s --namespace <namespace>", name)
			}

			// Resolve the actual version from Galaxy API if no constraint provided
			var resolvedVersion string
			if versionConstraint == "" || versionConstraint == "latest" {
				fmt.Printf("Resolving version for %s.%s...\n", cli.NamespaceFlag, name)
				var err error
				resolvedVersion, err = galaxy.GetCollectionVersion(cli.NamespaceFlag, name, versionConstraint)
				if err != nil {
					return fmt.Errorf("failed to resolve collection version: %w", err)
				}
				fmt.Printf("Resolved %s.%s to version %s\n", cli.NamespaceFlag, name, resolvedVersion)
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

			// Collection name in config is prefixed with scenario (e.g., "default.general")
			configName := cli.RoleScenario + "." + name

			// Check if collection already exists in config
			configCollExists := false
			for i, coll := range cfg.DependencyConfig.Collections {
				if coll.Name == configName {
					cfg.DependencyConfig.Collections[i] = config.CollectionRequirement{
						Name:      configName,
						Namespace: cli.NamespaceFlag,
						Version:   configVersionConstraint,
					}
					configCollExists = true
					break
				}
			}
			if !configCollExists {
				cfg.DependencyConfig.Collections = append(cfg.DependencyConfig.Collections, config.CollectionRequirement{
					Name:      configName,
					Namespace: cli.NamespaceFlag,
					Version:   configVersionConstraint,
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
			fmt.Printf("\033[32mCollection '%s' (namespace: %s, version %s) added successfully to diffusion.toml and diffusion.lock\n\033[0m", configName, cli.NamespaceFlag, configVersionConstraint)

			return nil
		},
	}

	cmd.Flags().StringVarP(&cli.RoleScenario, "scenario", "s", "default", "Molecule scenarios folder to use")
	cmd.Flags().StringVarP(&cli.NamespaceFlag, "namespace", "n", "", "Namespace for the collection (e.g., 'community' for community.general)")
	return cmd
}

// NewRoleRemoveCollectionCmd creates the remove-collection subcommand
func NewRoleRemoveCollectionCmd(cli *CLI) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove-collection [collection-name]",
		Short: "Remove a collection from diffusion.toml (use 'deps sync' to update requirements.yml and meta.yml)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collectionName := args[0]

			// Parse collection name (ignore version for removal)
			name, _ := utils.ParseCollectionString(collectionName)

			// Validate: dots are forbidden in collection names
			if strings.Contains(name, ".") {
				return fmt.Errorf("dots are not allowed in collection names (dots are reserved as scenario prefixes)")
			}

			// Collection name in config is prefixed with scenario (e.g., "default.general")
			configName := cli.RoleScenario + "." + name

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
				if coll.Name == configName {
					cfg.DependencyConfig.Collections = append(cfg.DependencyConfig.Collections[:i], cfg.DependencyConfig.Collections[i+1:]...)
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("collection '%s' not found in diffusion.toml", configName)
			}

			// Save diffusion.toml
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save diffusion.toml: %w", err)
			}
			err = dependency.UpdateLockFile()
			if err != nil {
				return fmt.Errorf("failed to update lock file: %w", err)
			}
			fmt.Printf("\033[32mCollection '%s' removed successfully from diffusion.toml and diffusion.lock\n\033[0m", configName)
			fmt.Printf("\033[33mRun 'diffusion deps sync' to update requirements.yml and meta/main.yml\n\033[0m")

			return nil
		},
	}

	cmd.Flags().StringVarP(&cli.RoleScenario, "scenario", "s", "default", "Molecule scenarios folder to use")
	cmd.Flags().StringVarP(&cli.NamespaceFlag, "namespace", "n", "", "Namespace for the collection")
	return cmd
}
