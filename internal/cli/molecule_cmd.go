package cli

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/molecule"
	"diffusion/internal/role"
	"diffusion/internal/utils"

	"github.com/spf13/cobra"
)

// NewMoleculeCmd creates the molecule command
func NewMoleculeCmd(cli *CLI) *cobra.Command {
	molCmd := &cobra.Command{
		Use:   "molecule",
		Short: "run molecule workflow (create/converge/verify/lint/idempotence/wipe)",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := &molecule.MoleculeOptions{
				RoleFlag:        cli.RoleFlag,
				OrgFlag:         cli.OrgFlag,
				RoleScenario:    cli.RoleScenario,
				TagFlag:         cli.TagFlag,
				ConvergeFlag:    cli.ConvergeFlag,
				VerifyFlag:      cli.VerifyFlag,
				TestsOverWrite:  cli.TestsOverWriteFlag,
				LintFlag:        cli.LintFlag,
				IdempotenceFlag: cli.IdempotenceFlag,
				DestroyFlag:     cli.DestroyFlag,
				WipeFlag:        cli.WipeFlag,
				CIMode:          cli.CIMode,
			}
			return molecule.RunMolecule(opts)
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
