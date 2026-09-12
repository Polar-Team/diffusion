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

// collectionSpec holds the resolved user intent for `role add-collection`.
type collectionSpec struct {
	Scenario  string // molecule scenario the collection belongs to
	Name      string // collection short name (no dots)
	Namespace string // Galaxy namespace; optional for git collections
	Src       string // git URL; empty means a Galaxy collection
	Scm       string // SCM type, only meaningful when Src is set
	Version   string // version constraint, branch or tag ("" means resolve)
}

// IsGit reports whether the spec describes a git-sourced collection.
func (s collectionSpec) IsGit() bool { return s.Src != "" }

// ConfigName is the diffusion.toml key for the collection ("<scenario>.<name>").
func (s collectionSpec) ConfigName() string { return s.Scenario + "." + s.Name }

// looksLikeVersion reports whether v is a version constraint or a concrete
// (semver-ish) version rather than a branch name such as "main" or "develop".
func looksLikeVersion(v string) bool {
	if v == "" {
		return false
	}
	if strings.ContainsAny(v, "<>=~^") {
		return true
	}
	v = strings.TrimPrefix(v, "v")
	return v != "" && v[0] >= '0' && v[0] <= '9'
}

// resolveCollectionRequirement turns a collectionSpec into the
// config.CollectionRequirement that is persisted into diffusion.toml.
//
// For git collections a network call is only made when no version was given:
// in that case the highest tag is resolved from the remote and stored as a
// ">=<version>" constraint, mirroring `role add-role`. Branch names and
// explicit versions/constraints are stored verbatim.
func resolveCollectionRequirement(spec collectionSpec) (config.CollectionRequirement, error) {
	if !spec.IsGit() {
		// Galaxy collection — namespace is mandatory.
		if spec.Namespace == "" {
			return config.CollectionRequirement{}, fmt.Errorf("--namespace/-n is required for Galaxy collections.\nExample: diffusion role add-collection %s --namespace <namespace>", spec.Name)
		}

		version := spec.Version
		if version == "" || version == "latest" {
			fmt.Printf("Resolving version for %s.%s...\n", spec.Namespace, spec.Name)
			resolved, err := galaxy.GetCollectionVersion(spec.Namespace, spec.Name, spec.Version)
			if err != nil {
				return config.CollectionRequirement{}, fmt.Errorf("failed to resolve collection version: %w", err)
			}
			fmt.Printf("Resolved %s.%s to version %s\n", spec.Namespace, spec.Name, resolved)
			version = ">=" + strings.TrimPrefix(resolved, "v")
		}

		return config.CollectionRequirement{
			Name:      spec.ConfigName(),
			Namespace: spec.Namespace,
			Version:   version,
		}, nil
	}

	// Git collection — namespace is optional and only stored for reference.
	scm := spec.Scm
	if scm == "" {
		scm = "git"
	}

	version := spec.Version
	switch {
	case version == "" || version == "latest":
		fmt.Printf("Resolving latest tag for %s...\n", spec.Src)
		resolved, err := galaxy.ResolveVersionFromGit(spec.Src, "")
		if err != nil || resolved == "" || resolved == "not-defined" {
			if err != nil {
				fmt.Printf("\033[33mWarning: failed to resolve version from git: %v\033[0m\n", err)
			}
			fmt.Printf("\033[33mNo usable tag found for %s — using branch 'main'\033[0m\n", spec.Src)
			version = "main"
		} else {
			version = ">=" + strings.TrimPrefix(resolved, "v")
			fmt.Printf("Resolved %s to %s\n", spec.Src, version)
		}
	case looksLikeVersion(version):
		fmt.Printf("Using version constraint %s for %s\n", version, spec.Src)
	default:
		fmt.Printf("Using git ref '%s' for %s\n", version, spec.Src)
	}

	return config.CollectionRequirement{
		Name:      spec.ConfigName(),
		Namespace: spec.Namespace,
		Version:   version,
		Source:    scm,
		SourceURL: spec.Src,
	}, nil
}

// upsertCollection inserts or replaces a collection requirement in the config,
// matching on the scenario-prefixed name.
func upsertCollection(depCfg *config.DependencyConfig, req config.CollectionRequirement) {
	for i, coll := range depCfg.Collections {
		if coll.Name == req.Name {
			depCfg.Collections[i] = req
			return
		}
	}
	depCfg.Collections = append(depCfg.Collections, req)
}

// NewRoleAddCollectionCmd creates the add-collection subcommand
func NewRoleAddCollectionCmd(cli *CLI) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add-collection [collection-name]",
		Short: "Add a collection to diffusion.toml (use 'deps sync' to update requirements.yml and meta.yml)",
		Long: `Add an Ansible collection to diffusion.toml.

Galaxy collection:
  diffusion role add-collection general --namespace community

Git collection:
  diffusion role add-collection foo --src https://github.com/org/ansible-collection-foo.git --version main`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Parse collection name and version constraint
			name, versionConstraint := utils.ParseCollectionString(args[0])

			// Validate: dots are forbidden in collection names (dots are reserved as scenario name prefixes)
			if strings.Contains(name, ".") {
				return fmt.Errorf("dots are not allowed in collection names (dots are reserved as scenario prefixes). Use --namespace/-n to specify the Galaxy namespace separately.\nExample: diffusion role add-collection %s --namespace %s",
					strings.SplitN(name, ".", 2)[1], strings.SplitN(name, ".", 2)[0])
			}

			// An explicit --version wins over a constraint appended to the name.
			version := cli.CollectionVersionFlag
			if version == "" {
				version = versionConstraint
			}

			spec := collectionSpec{
				Scenario:  cli.RoleScenario,
				Name:      name,
				Namespace: cli.NamespaceFlag,
				Src:       strings.TrimSpace(cli.CollectionSrcFlag),
				Scm:       cli.CollectionScmFlag,
				Version:   version,
			}

			req, err := resolveCollectionRequirement(spec)
			if err != nil {
				return err
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

			upsertCollection(cfg.DependencyConfig, req)

			// Save diffusion.toml
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save diffusion.toml: %w", err)
			}
			if err := dependency.UpdateLockFile(cli.RoleScenario); err != nil {
				return fmt.Errorf("failed to update lock file: %w", err)
			}

			if spec.IsGit() {
				fmt.Printf("\033[32mCollection '%s' (%s: %s, version %s) added successfully to diffusion.toml and diffusion.lock\n\033[0m", req.Name, req.Source, req.SourceURL, req.Version)
				fmt.Printf("\033[33mNote: git collections cannot be listed in meta/main.yml — they are written to requirements.yml only\n\033[0m")
			} else {
				fmt.Printf("\033[32mCollection '%s' (namespace: %s, version %s) added successfully to diffusion.toml and diffusion.lock\n\033[0m", req.Name, req.Namespace, req.Version)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&cli.RoleScenario, "scenario", "s", "default", "Molecule scenarios folder to use")
	cmd.Flags().StringVarP(&cli.NamespaceFlag, "namespace", "n", "", "Namespace for the collection (required for Galaxy collections, optional for git)")
	cmd.Flags().StringVar(&cli.CollectionSrcFlag, "src", "", "Git URL of the collection (switches the collection to a git source)")
	cmd.Flags().StringVar(&cli.CollectionScmFlag, "scm", "git", "SCM type of the collection, only used together with --src")
	cmd.Flags().StringVarP(&cli.CollectionVersionFlag, "version", "v", "", "Version, tag, branch or constraint (empty resolves the highest tag)")
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
			err = dependency.UpdateLockFile(cli.RoleScenario)
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
