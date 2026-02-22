package cli

import (
	"fmt"
	"runtime"

	"diffusion/internal/config"
	"github.com/spf13/cobra"
)

// NewShowCmd creates the show command
func NewShowCmd(cli *CLI) *cobra.Command {
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Display all diffusion configuration in readable format",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			fmt.Println("\n\033[1m=== Diffusion Configuration ===\033[0m")

			// Container Registry
			fmt.Println("\033[35m[Container Registry]\033[0m")
			fmt.Printf("  Registry Server:         \033[38;2;127;255;212m%s\033[0m\n", cfg.ContainerRegistry.RegistryServer)
			fmt.Printf("  Registry Provider:       \033[38;2;127;255;212m%s\033[0m\n", cfg.ContainerRegistry.RegistryProvider)
			fmt.Printf("  Molecule Container Name: \033[38;2;127;255;212m%s\033[0m\n", cfg.ContainerRegistry.MoleculeContainerName)
			fmt.Printf("  Molecule Container Tag:  \033[38;2;127;255;212m%s\033[0m\n\n", cfg.ContainerRegistry.MoleculeContainerTag)

			// HashiCorp Vault
			fmt.Println("\033[35m[HashiCorp Vault]\033[0m")
			fmt.Printf("  Integration Enabled:     \033[38;2;127;255;212m%t\033[0m\n", cfg.HashicorpVault.HashicorpVaultIntegration)
			if cfg.HashicorpVault.HashicorpVaultIntegration {
				if cfg.HashicorpVault.SecretKV2Path != "" {
					fmt.Printf("  \033[33mLegacy Config Detected (deprecated):\033[0m\n")
					fmt.Printf("    Secret KV2 Path:       \033[38;2;127;255;212m%s\033[0m\n", cfg.HashicorpVault.SecretKV2Path)
					fmt.Printf("    Secret KV2 Name:       \033[38;2;127;255;212m%s\033[0m\n", cfg.HashicorpVault.SecretKV2Name)
					fmt.Printf("  \033[33mPlease migrate to artifact_sources configuration\033[0m\n")
				} else {
					fmt.Printf("  \033[32mVault configured per artifact source\033[0m\n")
				}
			}
			fmt.Println()

			// Artifact Sources
			fmt.Println("\033[35m[Artifact Sources]\033[0m")
			if len(cfg.ArtifactSources) > 0 {
				for i, source := range cfg.ArtifactSources {
					fmt.Printf("  Source %d:\n", i+1)
					fmt.Printf("    Name:                  \033[38;2;127;255;212m%s\033[0m\n", source.Name)
					fmt.Printf("    URL:                   \033[38;2;127;255;212m%s\033[0m\n", source.URL)
					if source.UseVault {
						fmt.Printf("    Storage:               \033[38;2;127;255;212mVault (%s/%s)\033[0m\n", source.VaultPath, source.VaultSecretName)
						fmt.Printf("    Username Field:        \033[38;2;127;255;212m%s\033[0m\n", source.VaultUsernameField)
						fmt.Printf("    Token Field:           \033[38;2;127;255;212m%s\033[0m\n", source.VaultTokenField)
					} else {
						fmt.Printf("    Storage:               \033[38;2;127;255;212mLocal (encrypted)\033[0m\n")
					}
				}
			} else {
				fmt.Printf("  \033[33mNo artifact sources configured\033[0m\n")
				fmt.Printf("  \033[33mUse 'diffusion artifact add' to configure private repositories\033[0m\n")
			}
			fmt.Println()

			// YAML Lint Config
			fmt.Println("\033[35m[YAML Lint Configuration]\033[0m")
			fmt.Printf("  Extends:                 \033[38;2;127;255;212m%s\033[0m\n", cfg.YamlLintConfig.Extends)
			fmt.Printf("  Ignore Patterns:\n")
			for _, pattern := range cfg.YamlLintConfig.Ignore {
				fmt.Printf("    \033[38;2;127;255;212m- %s\033[0m\n", pattern)
			}
			if cfg.YamlLintConfig.Rules != nil {
				fmt.Printf("  Rules:\n")
				if cfg.YamlLintConfig.Rules.Braces != nil {
					fmt.Printf("    Braces:                \033[38;2;127;255;212m%v\033[0m\n", cfg.YamlLintConfig.Rules.Braces)
				}
				if cfg.YamlLintConfig.Rules.Brackets != nil {
					fmt.Printf("    Brackets:              \033[38;2;127;255;212m%v\033[0m\n", cfg.YamlLintConfig.Rules.Brackets)
				}
				if cfg.YamlLintConfig.Rules.NewLines != nil {
					fmt.Printf("    New Lines:             \033[38;2;127;255;212m%v\033[0m\n", cfg.YamlLintConfig.Rules.NewLines)
				}
				if cfg.YamlLintConfig.Rules.Comments != nil {
					fmt.Printf("    Comments:              \033[38;2;127;255;212m%v\033[0m\n", cfg.YamlLintConfig.Rules.Comments)
				}
				fmt.Printf("    Comments Indentation:  \033[38;2;127;255;212m%t\033[0m\n", cfg.YamlLintConfig.Rules.CommentsIdentation)
				if cfg.YamlLintConfig.Rules.OctalValues != nil {
					fmt.Printf("    Octal Values:          \033[38;2;127;255;212m%v\033[0m\n", cfg.YamlLintConfig.Rules.OctalValues)
				}
			}
			fmt.Println()

			// Ansible Lint Config
			fmt.Println("\033[35m[Ansible Lint Configuration]\033[0m")
			fmt.Printf("  Excluded Paths:\n")
			for _, path := range cfg.AnsibleLintConfig.ExcludedPaths {
				fmt.Printf("    \033[38;2;127;255;212m- %s\033[0m\n", path)
			}
			fmt.Printf("  Warn List:\n")
			for _, warn := range cfg.AnsibleLintConfig.WarnList {
				fmt.Printf("    \033[38;2;127;255;212m- %s\033[0m\n", warn)
			}
			fmt.Printf("  Skip List:\n")
			for _, skip := range cfg.AnsibleLintConfig.SkipList {
				fmt.Printf("    \033[38;2;127;255;212m- %s\033[0m\n", skip)
			}
			fmt.Println()
			fmt.Println("\033[1m=== End of Configuration ===\033[0m")
			return nil
		},
	}

	return showCmd
}

// NewVersionCmd creates the version command
func NewVersionCmd(cli *CLI) *cobra.Command {
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Diffusion version %s\n", Version)
			fmt.Printf("Go version: %s\n", runtime.Version())
			fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}

	return versionCmd
}
