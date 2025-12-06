package main

// diffusion - Cobra-based cross-platform CLI tool to assist with Molecule workflows,
// with Windows-only features for WSL compaction.
//
// Features:
// - Ensures required env vars are set (vault_user, vault_passwd, GIT_URL, PROJECT_ID, VAULT_ADDR)
// - Prompts for VAULT credentials if not in env
// - Gets Vault token and pulls secrets (GIT user/token)
// - Runs `yc` init
// - Implements "molecule" command with flags: role, org, tag, verify, lint, idempotence, wipe
// - Copies role files into molecule layout (if present)
// - Runs docker commands similar to your PowerShell script
// - Adds Windows-only "compact WSL" feature that stops Docker Desktop, shuts down WSL and runs Optimize-VHD
//
// NOTE: This CLI shells out to external tools: vault, yc, docker, wsl, powershell. They must be available in PATH.
// Optimize-VHD requires elevated (Administrator) powershell rights on Windows.

import (
	"bufio"

	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	RoleInitFlag      bool
	RoleFlag          string
	OrgFlag           string
	RoleScenario      string
	AddRoleFlag       string
	RoleSrcFlag       string
	RoleScmFlag       string
	RoleVersionFlag   string
	AddCollectionFlag string
	TagFlag           string
	VerifyFlag        bool
	LintFlag          bool
	IdempotenceFlag   bool
	WipeFlag          bool
	CompactWSLFlag    bool
)

func main() {

	rootCmd := &cobra.Command{
		Use:   "diffusion",
		Short: "Molecule workflow helper (cross-platform) with Windows-only WSL compact features",
	}

	roleCmd := &cobra.Command{
		Use:   "role",
		Short: "Configure role settings interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle add-role flag first
			meta, req, err := LoadRoleConfig("")
			if err != nil {
				fmt.Println("Role config not found. Would you like to initialize a new role? (y/n):")
				var response string
				_, err := fmt.Scanln(&response)
				if err != nil {
					return fmt.Errorf("failed to read input: %w", err)
				}
				if response == "y" || response == "Y" {
					roleName, err := AnsibleGalaxyInit()
					if err != nil {
						return fmt.Errorf("failed to initialize role: %w", err)
					}

					// Change working directory to the newly created role
					if err := os.Chdir(roleName); err != nil {
						return fmt.Errorf("failed to change directory to %s: %w", roleName, err)
					}

					MetaConfig := MetaConfigSetup()
					RequirementConfig := RequirementConfigSetup(MetaConfig.Collections)
					err = SaveMetaFile(MetaConfig)
					if err != nil {
						return fmt.Errorf("failed to save meta file: %w", err)
					}

					err = SaveRequirementFile(RequirementConfig, "default")
					if err != nil {
						return fmt.Errorf("failed to save requirements file: %w", err)
					}

					fmt.Println("Role initialized successfully.")
				} else {
					fmt.Println("Role initialization skipped.")
				}
				return err
			}
			// Continue with config if loaded successfully

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

	roleAddRoleCmd := &cobra.Command{
		Use:   "add-role [role-name]",
		Short: "Add a role to requirements.yml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			roleName := args[0]

			_, req, err := LoadRoleConfig(RoleScenario)
			if err != nil {
				return fmt.Errorf("failed to load requirements file: %w", err)
			}

			// Add the new role to existing roles
			newRole := RequirementRole{
				Name:    roleName,
				Src:     RoleSrcFlag,
				Scm:     RoleScmFlag,
				Version: RoleVersionFlag,
			}
			req.Roles = append(req.Roles, newRole)

			// Save the updated requirements file
			if err := SaveRequirementFile(req, RoleScenario); err != nil {
				return fmt.Errorf("failed to save role: %w", err)
			}
			fmt.Printf("\033[32mRole '%s' added successfully to requirements.yml\n\033[0m", roleName)
			return nil
		},
	}

	roleRemoveRoleCmd := &cobra.Command{
		Use:   "remove-role [role-name]",
		Short: "Remove a role to requirements.yml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			roleName := args[0]

			_, req, err := LoadRoleConfig(RoleScenario)
			if err != nil {
				return fmt.Errorf("failed to load requirements file: %w", err)
			}

			found := false
			for i, role := range req.Roles {
				if role.Name == roleName {
					// Remove the role from the slice
					req.Roles = append(req.Roles[:i], req.Roles[i+1:]...)
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("role '%s' not found in requirements.yml", roleName)
			}

			// Save the updated requirements file
			if err := SaveRequirementFile(req, RoleScenario); err != nil {
				return fmt.Errorf("failed to save role: %w", err)
			}
			fmt.Printf("\033[32mRole '%s' removed successfully from requirements.yml\n\033[0m", roleName)
			return nil
		},
	}

	roleAddCollectionCmd := &cobra.Command{
		Use:   "add-collection [collection-name]",
		Short: "Add a role to requirements.yml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collectionName := args[0]

			meta, req, err := LoadRoleConfig(RoleScenario)
			if err != nil {
				return fmt.Errorf("failed to load requirements file: %w", err)
			}

			req.Collections = append(req.Collections, collectionName)

			// Save the updated requirements file
			if err := SaveRequirementFile(req, RoleScenario); err != nil {
				return fmt.Errorf("failed to save role: %w", err)
			}
			fmt.Printf("\033[32mRole '%s' added successfully to requirements.yml\n\033[0m", collectionName)

			meta.Collections = append(meta.Collections, collectionName)

			// Save the updated meta file
			if err := SaveMetaFile(meta); err != nil {
				return fmt.Errorf("failed to save meta file: %w", err)
			}
			fmt.Printf("\033[32mCollection '%s' added successfully to meta/main.yml\n\033[0m", collectionName)

			return nil
		},
	}

	roleRemoveCollectionCmd := &cobra.Command{
		Use:   "remove-collection [collection-name]",
		Short: "Add a role to requirements.yml",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			collectionName := args[0]

			meta, req, err := LoadRoleConfig(RoleScenario)
			if err != nil {
				return fmt.Errorf("failed to load requirements file: %w", err)
			}

			found := false
			for i, coll := range req.Collections {
				if coll == collectionName {
					// Remove the collection from the slice
					req.Collections = append(req.Collections[:i], req.Collections[i+1:]...)
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("collection '%s' not found in requirements.yml", collectionName)
			}

			found = false
			for i, coll := range meta.Collections {
				if coll == collectionName {
					// Remove the collection from the slice
					meta.Collections = append(meta.Collections[:i], meta.Collections[i+1:]...)
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("collection '%s' not found in meta/main.yml", collectionName)
			}

			// Save the updated requirements file
			if err := SaveRequirementFile(req, RoleScenario); err != nil {
				return fmt.Errorf("failed to save role: %w", err)
			}
			fmt.Printf("\033[32mRole '%s' added successfully to requirements.yml\n\033[0m", collectionName)

			// Save the updated meta file
			if err := SaveMetaFile(meta); err != nil {
				return fmt.Errorf("failed to save meta file: %w", err)
			}
			fmt.Printf("\033[32mCollection '%s' added successfully to meta/main.yml\n\033[0m", collectionName)

			return nil
		},
	}

	roleCmd.AddCommand(roleRemoveRoleCmd)

	roleCmd.AddCommand(roleRemoveCollectionCmd)

	roleAddCollectionCmd.Flags().StringVarP(&RoleScenario, "scenario", "s", "default", "Molecule scenarios folder to use")

	roleCmd.AddCommand(roleAddCollectionCmd)

	roleAddRoleCmd.Flags().StringVarP(&RoleScenario, "scenario", "s", "default", "Molecule scenarios folder to use")
	roleAddRoleCmd.Flags().StringVarP(&RoleSrcFlag, "src", "", "", "Source URL of the role (required)")
	roleAddRoleCmd.Flags().StringVarP(&RoleScmFlag, "scm", "", "git", "SCM type (e.g., git) of the role (optional)")
	roleAddRoleCmd.Flags().StringVarP(&RoleVersionFlag, "version", "v", "main", "Version of the role (optional)")

	roleCmd.AddCommand(roleAddRoleCmd)

	roleCmd.Flags().StringVarP(&RoleScenario, "scenario", "s", "default", "Molecule scenarios folder to use")
	roleCmd.Flags().BoolVarP(&RoleInitFlag, "init", "i", false, "Initialize a new Ansible role using ansible-galaxy")

	rootCmd.AddCommand(roleCmd)

	// molecule command
	molCmd := &cobra.Command{
		Use:   "molecule",
		Short: "run molecule workflow (create/converge/verify/lint/idempotence/wipe)",
		RunE:  runMolecule,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Ensure some env defaults and prompt when needed

			reader := bufio.NewReader(os.Stdin)

			config, err := LoadConfig() // ignore error for now
			if err != nil {
				log.Printf("\033[33mwarning loading config: %v\033[0m", err)
				log.Printf("\033[38;2;127;255;212mNew config file will be created...\033[0m")

				YamlLintRulesDefault := &YamlLintRules{
					Braces:             map[string]any{"max-spaces-inside": 1, "level": "warning"},
					Brackets:           map[string]any{"max-spaces-inside": 1, "level": "warning"},
					NewLines:           map[string]any{"type": "platform"},
					Comments:           map[string]any{"min-spaces-from-content": 1},
					CommentsIdentation: false,
					OctalValues:        map[string]any{"forbid-implicit-octal": true},
				}
				YamlLintDefault := &YamlLint{
					Extends: "default",
					Ignore:  []string{".git/*", "molecule/**", "vars/*", "files/*", ".yamllint", ".ansible-lint"},
					Rules:   YamlLintRulesDefault,
				}

				AnsibleLintDefault := &AnsibleLint{
					ExcludedPaths: []string{"molecule/default/tests/*.yml", "molecule/default/tests/*/*/*.yml", "tests/test.yml"},
					WarnList:      []string{"meta-no-info", "yaml[line-length]"},
					SkipList:      []string{"meta-incorrect", "role-name[path]"},
				}

				fmt.Print("Enter RegistryServer: ")
				registryServer, _ := reader.ReadString('\n')
				registryServer = strings.TrimSpace(registryServer)

				fmt.Print("Enter RegistryProvider: ")
				registryProvider, _ := reader.ReadString('\n')
				registryProvider = strings.TrimSpace(registryProvider)

				if registryProvider != "YC" && registryProvider != "AWS" && registryProvider != "GCP" && registryProvider != "Public" {
					fmt.Fprintln(os.Stderr, "\033[31mInvalid RegistryProvider. Allowed values are: YC, AWS, GCP. \nIf you're using public registry, then choose Public - or choose it, if you want to authenticate externally.\033[0m")
					os.Exit(1)
				}

				fmt.Print("Enter MoleculeContainerName: ")
				moleculeContainerName, _ := reader.ReadString('\n')
				moleculeContainerName = strings.TrimSpace(moleculeContainerName)

				fmt.Print("Enter MoleculeContainerTag: ")
				moleculeContainerTag, _ := reader.ReadString('\n')
				moleculeContainerTag = strings.TrimSpace(moleculeContainerTag)

				ContainerRegistry := &ContainerRegistry{
					RegistryServer:        registryServer,
					RegistryProvider:      registryProvider,
					MoleculeContainerName: moleculeContainerName,
					MoleculeContainerTag:  moleculeContainerTag,
				}

				fmt.Print("Enable Vault Integration? (Y/n): ")
				vaultEnabledStr, _ := reader.ReadString('\n')
				vaultEnabledStr = strings.TrimSpace(vaultEnabledStr)
				if vaultEnabledStr == "" {
					vaultEnabledStr = "n"
				}
				vaultEnabled := strings.ToLower(vaultEnabledStr) == "y"

				HashicorpVaultSet := VaultConfigHelper(vaultEnabled)

				config = &Config{
					ContainerRegistry: ContainerRegistry,
					HashicorpVault:    HashicorpVaultSet,
					ArtifactUrl:       "https://example.com/repo",
					YamlLintConfig:    YamlLintDefault,
					AnsibleLintConfig: AnsibleLintDefault,
				}

				if err := SaveConfig(config); err != nil {
					log.Printf("\033[33mwarning saving new config: %v\033[0m", err)
				}

			}

			if err := os.Setenv("GIT_URL", config.ArtifactUrl); err != nil {
				log.Printf("Failed to set GIT_URL: %v", err)
			}
		},
	}

	MetaConfig, _, err := LoadRoleConfig("")
	if err != nil {
		RoleFlag = ""
		OrgFlag = ""
		log.Printf("\033[33mwarning loading role config: %v\033[0m", err)
	} else {
		if MetaConfig.GalaxyInfo.RoleName != "" {
			RoleFlag = MetaConfig.GalaxyInfo.RoleName
			OrgFlag = MetaConfig.GalaxyInfo.Namespace
		} else {
			RoleFlag = ""
			OrgFlag = ""
			log.Printf("\033[33mwarning: role name or namespace missing in meta/main.yml\033[0m")
		}
	}

	molCmd.Flags().StringVarP(&RoleFlag, "role", "r", RoleFlag, "role name")
	molCmd.Flags().StringVarP(&OrgFlag, "org", "o", OrgFlag, "organization prefix")
	molCmd.Flags().StringVarP(&TagFlag, "tag", "t", "", "ANSIBLE_RUN_TAGS value (optional)")
	molCmd.Flags().BoolVar(&VerifyFlag, "verify", false, "run molecule verify")
	molCmd.Flags().BoolVar(&LintFlag, "lint", false, "run linting (yamllint / ansible-lint)")
	molCmd.Flags().BoolVar(&IdempotenceFlag, "idempotence", false, "run molecule idempotence")
	molCmd.Flags().BoolVar(&WipeFlag, "wipe", false, "remove container and molecule role folder")

	rootCmd.AddCommand(molCmd)

	// show command
	showCmd := &cobra.Command{
		Use:   "show",
		Short: "Display all diffusion configuration in readable format",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			fmt.Println("\n\033[1m=== Diffusion Configuration ===\033[0m")

			// Container Registry
			fmt.Println("\033[35m[Container Registry]\033[0m")
			fmt.Printf("  Registry Server:         \033[38;2;127;255;212m%s\033[0m\n", config.ContainerRegistry.RegistryServer)
			fmt.Printf("  Registry Provider:       \033[38;2;127;255;212m%s\033[0m\n", config.ContainerRegistry.RegistryProvider)
			fmt.Printf("  Molecule Container Name: \033[38;2;127;255;212m%s\033[0m\n", config.ContainerRegistry.MoleculeContainerName)
			fmt.Printf("  Molecule Container Tag:  \033[38;2;127;255;212m%s\033[0m\n\n", config.ContainerRegistry.MoleculeContainerTag)

			// HashiCorp Vault
			fmt.Println("\033[35m[HashiCorp Vault]\033[0m")
			fmt.Printf("  Integration Enabled:     \033[38;2;127;255;212m%t\033[0m\n", config.HashicorpVault.HashicorpVaultIntegration)
			if config.HashicorpVault.HashicorpVaultIntegration {
				fmt.Printf("  Secret KV2 Path:         \033[38;2;127;255;212m%s\033[0m\n", config.HashicorpVault.SecretKV2Path)
				fmt.Printf("  Secret KV2 Name:         \033[38;2;127;255;212m%s\033[0m\n", config.HashicorpVault.SecretKV2Name)
				fmt.Printf("  Username Field:          \033[38;2;127;255;212m%s\033[0m\n", config.HashicorpVault.UserNameField)
				fmt.Printf("  Token Field:             \033[38;2;127;255;212m%s\033[0m\n", config.HashicorpVault.TokenField)
			}
			fmt.Println()

			// Artifact URL
			fmt.Println("\033[35m[Artifact Repository]\033[0m")
			fmt.Printf("  URL:                     \033[38;2;127;255;212m%s\033[0m\n\n", config.ArtifactUrl)

			// YAML Lint Config
			fmt.Println("\033[35m[YAML Lint Configuration]\033[0m")
			fmt.Printf("  Extends:                 \033[38;2;127;255;212m%s\033[0m\n", config.YamlLintConfig.Extends)
			fmt.Printf("  Ignore Patterns:\n")
			for _, pattern := range config.YamlLintConfig.Ignore {
				fmt.Printf("    \033[38;2;127;255;212m- %s\033[0m\n", pattern)
			}
			fmt.Println()

			// Ansible Lint Config
			fmt.Println("\033[35m[Ansible Lint Configuration]\033[0m")
			fmt.Printf("  Excluded Paths:\n")
			for _, path := range config.AnsibleLintConfig.ExcludedPaths {
				fmt.Printf("    \033[38;2;127;255;212m- %s\033[0m\n", path)
			}
			fmt.Printf("  Warn List:\n")
			for _, warn := range config.AnsibleLintConfig.WarnList {
				fmt.Printf("    \033[38;2;127;255;212m- %s\033[0m\n", warn)
			}
			fmt.Printf("  Skip List:\n")
			for _, skip := range config.AnsibleLintConfig.SkipList {
				fmt.Printf("    \033[38;2;127;255;212m- %s\033[0m\n", skip)
			}
			fmt.Println()
			fmt.Println("\033[1m=== End of Configuration ===\033[0m")
			return nil
		},
	}
	rootCmd.AddCommand(showCmd)

	// Windows-only helper: compact-wsl
	compactCmd := &cobra.Command{
		Use:   "compact-wsl",
		Short: "Windows-only: shutdown WSL / stop Docker Desktop and Optimize-VHD for Docker Desktop VHDX files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "windows" {
				return fmt.Errorf("compact-wsl is supported only on Windows")
			}
			return compactWSLAndOptimize()
		},
	}
	compactCmd.Flags().BoolVar(&CompactWSLFlag, "confirm", false, "confirm running Optimize-VHD (requires admin)")
	rootCmd.AddCommand(compactCmd)

	// Provide a top-level flag to run compact before molecule (Windows-only)
	rootCmd.PersistentFlags().BoolVar(&CompactWSLFlag, "compact-wsl", false, "on Windows: compact Docker Desktop WSL2 vhdx (runs before molecule actions)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func AnsibleGalaxyInit() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter role name: ")
	roleName, _ := reader.ReadString('\n')
	roleName = strings.TrimSpace(roleName)

	if roleName == "" {
		return "", fmt.Errorf("role name cannot be empty")
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	cmd := exec.Command("docker", "run", "--rm",
		"-v", fmt.Sprintf("%s:/ansible", currentDir),
		"-w", "/ansible",
		"cytopia/ansible:latest",
		"ansible-galaxy", "role", "init", roleName)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("Initializing Ansible role: %s\n", roleName)
	if err := cmd.Run(); err != nil {
		return "", err
	}

	// Create scenarios/default directory structure
	scenariosPath := filepath.Join(currentDir, roleName, "scenarios", "default")
	if err := os.MkdirAll(scenariosPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create scenarios directory: %w", err)
	}

	converge_content := `# Converge playbook
---
- name: Converge
  hosts: all
  tasks:
    - name: "Include Ansible role"
      ansible.builtin.include_role:
          name: "{{ lookup('env', 'MOLECULE_PROJECT_DIRECTORY') | basename }}"
      tags:
#        - YOUR_TAGS
`
	verify_content := `# Verify playbook
---
- name: Verify
  hosts: all
  gather_facts: false

`

	molecule_content := `# Molecule default scenario configuration
---
dependency:
  name: galaxy
  options:
    requirements-file: requirements.yml
driver:
  name: docker
# platforms:
#  - name: YOUR_PLATFORM_NAME
#    image: YOUR_TESTING_IMAGE_URL
#    privileged: true
#    pre_build_image: true
#    env:
#      YC_TOKEN: ${TOKEN}
#      VAULT_ADDR: ${VAULT_ADDR}
#      VAULT_TOKEN: ${VAULT_TOKEN}
#    command: /lib/systemd/systemd
#    cgroupns_mode: host
#    tmpfs:
#      - /tmp
#      - /run
#      - /run/lock
#    volumes:
#      - /etc/ssl/certs:/etc/ssl/certs
#      - /sys/fs/cgroup:/sys/fs/cgroup:rw
#      - dockerroot:/var/lib/docker:rw
provisioner:
   name: ansible
verifier:
   name: ansible
`
	// Create converge.yml
	convergePath := filepath.Join(currentDir, roleName, "scenarios", "default", "converge.yml")
	if err := os.WriteFile(convergePath, []byte(converge_content), 0644); err != nil {
		return "", fmt.Errorf("failed to create converge.yml: %w", err)
	}

	// Create molecule.yml
	moleculePath := filepath.Join(currentDir, roleName, "scenarios", "default", "molecule.yml")
	if err := os.WriteFile(moleculePath, []byte(molecule_content), 0644); err != nil {
		return "", fmt.Errorf("failed to create converge.yml: %w", err)
	}
	// Create verify.yml
	verifyPath := filepath.Join(currentDir, roleName, "scenarios", "default", "verify.yml")
	if err := os.WriteFile(verifyPath, []byte(verify_content), 0644); err != nil {
		return "", fmt.Errorf("failed to create verify.yml: %w", err)
	}

	// Create .gitignore file
	gitignoreContent := `**/molecule/*
**/roles/*
vars/secrets.yml
`
	gitignorePath := filepath.Join(currentDir, roleName, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		return "", fmt.Errorf("failed to create .gitignore: %w", err)
	}

	fmt.Printf("Created .gitignore in %s\n", roleName)
	return roleName, nil
}

func MetaConfigSetup() *Meta {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("What name of the role should be?: ")
	roleName, _ := reader.ReadString('\n')
	roleName = strings.TrimSpace(roleName)

	fmt.Print("What namespace of the role should be?: ")
	roleNamespace, _ := reader.ReadString('\n')
	roleNamespace = strings.TrimSpace(roleNamespace)

	fmt.Print("What company of the role should be?: ")
	roleCompany, _ := reader.ReadString('\n')
	roleCompany = strings.TrimSpace(roleCompany)

	fmt.Print("What author of the role should be?: ")
	roleAuthor, _ := reader.ReadString('\n')
	roleAuthor = strings.TrimSpace(roleAuthor)

	fmt.Print("Description of the role (optional): ")
	roleDescription, _ := reader.ReadString('\n')
	roleDescription = strings.TrimSpace(roleDescription)

	platformsList := []Platform{}
	fmt.Print("Enter platforms? (y/n): ")
	addPlatforms, _ := reader.ReadString('\n')
	addPlatforms = strings.TrimSpace(strings.ToLower(addPlatforms))

	for addPlatforms == "y" {
		fmt.Print("Platform OS name (e.g., ubuntu, centos): ")
		osName, _ := reader.ReadString('\n')
		osName = strings.TrimSpace(osName)

		if osName != "" {
			fmt.Print("Platform versions (comma-separated, e.g., 20.04,22.04): ")
			versionsInput, _ := reader.ReadString('\n')
			versionsInput = strings.TrimSpace(versionsInput)
			if versionsInput != "" {
				// Check if this OS already exists in the list
				existingIndex := -1
				for i, platform := range platformsList {
					if platform.OsName == osName {
						existingIndex = i
						break
					}
				}

				// Collect versions for this OS
				versions := []string{}

				// Add new versions
				for version := range strings.SplitSeq(versionsInput, ",") {
					version = strings.TrimSpace(version)
					if version != "" {
						versions = append(versions, version)
					}
				}

				if existingIndex != -1 {
					platformsList[existingIndex].Versions = versions
				} else {
					platformsList = append(platformsList, Platform{
						OsName:   osName,
						Versions: versions,
					})
				}
			}
		}

		fmt.Print("Add another platform? (y/n): ")
		addPlatforms, _ = reader.ReadString('\n')
		addPlatforms = strings.TrimSpace(strings.ToLower(addPlatforms))
	}

	fmt.Print("Galaxy Tags (comma-separated) (optional): ")
	galaxyTagsInput, _ := reader.ReadString('\n')
	galaxyTagsInput = strings.TrimSpace(galaxyTagsInput)
	galaxyTagsList := []string{}
	if galaxyTagsInput != "" {
		for t := range strings.SplitSeq(galaxyTagsInput, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				galaxyTagsList = append(galaxyTagsList, t)
			}
		}
	}

	fmt.Print("Collections required (comma-separated) (optional): ")
	collectionsInput, _ := reader.ReadString('\n')
	collectionsInput = strings.TrimSpace(collectionsInput)
	collectionsList := []string{}
	if collectionsInput != "" {
		for c := range strings.SplitSeq(collectionsInput, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				collectionsList = append(collectionsList, c)
			}
		}
	}

	roleSettings := &Meta{
		GalaxyInfo: &GalaxyInfo{
			RoleName:          roleName,
			Namespace:         roleNamespace,
			Company:           roleCompany,
			Author:            roleAuthor,
			Description:       roleDescription,
			License:           "MIT",
			MinAnsibleVersion: "2.10",
			Platforms:         platformsList,
			GalaxyTags:        galaxyTagsList,
		},
		Collections: collectionsList,
	}

	return roleSettings

}
func RequirementConfigSetup(collections []string) *Requirement {
	if collections == nil {
		collections = []string{}
	}
	reader := bufio.NewReader(os.Stdin)
	collectionsList := collections

	rolesList := []RequirementRole{}
	fmt.Print("Enter roles to add? (y/n): ")
	addRoles, _ := reader.ReadString('\n')
	addRoles = strings.TrimSpace(strings.ToLower(addRoles))
	for addRoles == "y" {
		fmt.Print("Role name (e.g., geerlingguy.nginx): ")
		roleName, _ := reader.ReadString('\n')
		roleName = strings.TrimSpace(roleName)
		if roleName != "" {
			fmt.Print("Role source (e.g., git URL) (required): ")
			roleSrc, _ := reader.ReadString('\n')
			roleSrc = strings.TrimSpace(roleSrc)
			fmt.Print("Role SCM (default: git) (optional): ")
			roleScm, _ := reader.ReadString('\n')
			roleScm = strings.TrimSpace(roleScm)
			if roleScm == "" {
				roleScm = "git"
			}
			fmt.Print("Role version (default: main) (optional): ")
			roleVersion, _ := reader.ReadString('\n')
			roleVersion = strings.TrimSpace(roleVersion)
			if roleVersion == "" {
				roleVersion = "main"
			}
			newRole := RequirementRole{
				Name:    roleName,
				Src:     roleSrc,
				Scm:     roleScm,
				Version: roleVersion,
			}
			rolesList = append(rolesList, newRole)
		}
		fmt.Print("Add another role? (y/n): ")
		addRoles, _ = reader.ReadString('\n')
		addRoles = strings.TrimSpace(strings.ToLower(addRoles))
	}

	requirementSettings := &Requirement{
		Collections: collectionsList,
		Roles:       rolesList,
	}

	return requirementSettings

}

func VaultConfigHelper(intergration bool) *HashicorpVault {
	reader := bufio.NewReader(os.Stdin)

	if !intergration {
		return &HashicorpVault{
			HashicorpVaultIntegration: false,
		}
	}
	fmt.Print("Enter SecretKV2Path (e.g., secret/data/diffusion): ")
	secretKV2Path, _ := reader.ReadString('\n')
	secretKV2Path = strings.TrimSpace(secretKV2Path)

	fmt.Print("Enter Git Username Field in Vault (default: git_username): ")
	gitUsernameField, _ := reader.ReadString('\n')
	gitUsernameField = strings.TrimSpace(gitUsernameField)

	fmt.Print("Enter Git Token Field in Vault (default: git_token): ")
	gitTokenField, _ := reader.ReadString('\n')
	gitTokenField = strings.TrimSpace(gitTokenField)

	HashicorpVaultSet := &HashicorpVault{
		HashicorpVaultIntegration: true,
		SecretKV2Path:             secretKV2Path,
		UserNameField:             gitUsernameField,
		TokenField:                gitTokenField,
	}

	return HashicorpVaultSet
}
func PromptInput(prompt string) string {
	fmt.Print(prompt)
	r := bufio.NewReader(os.Stdin)
	val, _ := r.ReadString('\n')
	return strings.TrimSpace(val)
}

// runCommand runs command and streams combined stdout/stderr to our stdout/stderr.
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runCommandHide runs command and discards stdout/stderr
func runCommandHide(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// runCommandCapture returns stdout (trimmed) and error
func runCommandCapture(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ycCliInit runs yc commands and sets env variables YC_TOKEN, YC_CLOUD_ID, YC_FOLDER_ID
func YcCliInit() error {
	// yc iam create-token
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	token, err := runCommandCapture(ctx, "yc", "iam", "create-token")
	if err != nil {
		return fmt.Errorf("yc iam create-token failed: %v (%s)", err, token)
	}
	_ = os.Setenv("TOKEN", token)

	cloudID, _ := runCommandCapture(ctx, "yc", "config", "get", "cloud-id")
	if cloudID != "" {
		_ = os.Setenv("YC_CLOUD_ID", cloudID)
	}

	folderID, _ := runCommandCapture(ctx, "yc", "config", "get", "folder-id")
	if folderID != "" {
		_ = os.Setenv("YC_FOLDER_ID", folderID)
	}
	return nil
}

// runMolecule is the core function that implements the behavior from your PS script
func runMolecule(cmd *cobra.Command, args []string) error {

	config, err := LoadConfig()
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
	roleDirName := fmt.Sprintf("%s.%s", OrgFlag, RoleFlag)
	roleMoleculePath := filepath.Join(path, "molecule", roleDirName)

	// handle wipe
	if WipeFlag {
		log.Printf("\033[38;2;127;255;212mWiping: removing container molecule-%s and folder %s\n\033[0m", RoleFlag, roleMoleculePath)
		_ = runCommand("docker", "rm", fmt.Sprintf("molecule-%s", RoleFlag), "-f")
		if err := os.RemoveAll(roleMoleculePath); err != nil {
			log.Printf("\033[33mwarning: failed remove role path: %v\033[0m", err)
		}
		return nil
	}

	// handle lint/verify/idempotence by ensuring files are copied and running docker exec commands
	if LintFlag || VerifyFlag || IdempotenceFlag {
		if err := copyRoleData(path, roleMoleculePath); err != nil {
			log.Printf("\033[33mwarning copying data: %v\033[0m", err)
		}
		// ensure tests dir exists for verify/lint
		defaultTestsDir := filepath.Join(roleMoleculePath, "molecule", "default", "tests")
		if err := os.MkdirAll(defaultTestsDir, 0o755); err != nil {
			log.Printf("\033[33mwarning: cannot create tests dir: %v\033[0m", err)
		}
		if LintFlag {
			// run yamllint and ansible-lint inside container
			cmdStr := fmt.Sprintf(`cd ./%s && yamllint . -c .yamllint && ansible-lint -c .ansible-lint `, roleDirName)
			if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", cmdStr); err != nil {
				log.Printf("\033[31mLint failed: %v\033[0m", err)
				os.Exit(1)
			}
			log.Printf("\033[32mLint Done Successfully!\033[0m")
			return nil
		}
		if VerifyFlag {
			// copy tests/*
			testsSrc := filepath.Join(path, "tests")
			copyIfExists(testsSrc, defaultTestsDir)
			cmdStr := fmt.Sprintf("cd ./%s && molecule verify", roleDirName)
			if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", cmdStr); err != nil {
				log.Printf("\033[31mVerify failed: %v\033[0m", err)
				os.Exit(1)
			}
			log.Printf("\033[32mVerify Done Successfully!\033[0m")
			return nil
		}
		if IdempotenceFlag {
			tagEnv := ""
			if TagFlag != "" {
				tagEnv = fmt.Sprintf("ANSIBLE_RUN_TAGS=%s ", TagFlag)
			}
			cmdStr := fmt.Sprintf("cd ./%s && %smolecule idempotence", roleDirName, tagEnv)
			if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", cmdStr); err != nil {
				log.Printf("\033[31mIdempotence failed: %v\033[0m", err)
				os.Exit(1)
			}
			log.Printf("\033[32mIdempotence Done Successfully!\033[0m")
			return nil
		}
	}

	// default flow: create/run container if not exists, copy data, converge
	// check if container exists
	err = exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", RoleFlag)).Run()
	if err == nil {
		fmt.Printf("\033[38;2;127;255;212mContainer molecule-%s already exists. To purge use --wipe.\n\033[0m", RoleFlag)
	} else {

		if config.HashicorpVault.HashicorpVaultIntegration {

			git_raw := vault_client(context.Background(), config.HashicorpVault.SecretKV2Path, config.HashicorpVault.SecretKV2Name)

			gitUser := git_raw.Data.Data[config.HashicorpVault.UserNameField].(string)

			if err := os.Setenv("GIT_USER", gitUser); err != nil {
				log.Printf("Failed to set GIT_USER: %v", err)
			}

			gitToken := git_raw.Data.Data[config.HashicorpVault.TokenField].(string)

			if err := os.Setenv("GIT_PASSWORD", gitToken); err != nil {
				log.Printf("Failed to set GIT_PASSWORD: %v", err)
			}
		} else {
			log.Println("\033[35mHashiCorp Vault integration is disabled in config. Use public repositories.\033[0m")
		}

		// If user requested Windows compaction before running molecule
		if CompactWSLFlag && runtime.GOOS == "windows" {
			log.Println("Running Windows WSL compact prior to molecule (requested)...")
			if err := compactWSLAndOptimize(); err != nil {
				log.Printf("compact-wsl failed: %v", err)
			}
		}
		// create
		if err := YcCliInit(); err != nil {
			log.Printf("\033[32myc init warning: %v\033[0m", err)
		}
		if config.ContainerRegistry.RegistryProvider != "Public" {

			switch config.ContainerRegistry.RegistryProvider {
			case "YC":
				if err := runCommandHide("docker", "login", config.ContainerRegistry.RegistryServer, "--username", "iam", "--password", os.Getenv("TOKEN")); err != nil {
					log.Printf("\033[33mdocker login to registry failed: %v\033[0m", err)
				}

			}
		}
		// run container
		// docker run --rm -d --name=molecule-$role -v "$path/molecule:/opt/molecule" -v /sys/fs/cgroup:/sys/fs/cgroup:rw -e ... --privileged --pull always cr.yandex/...
		image := fmt.Sprintf("%s/%s:%s", config.ContainerRegistry.RegistryServer, config.ContainerRegistry.MoleculeContainerName, config.ContainerRegistry.MoleculeContainerTag)
		args := []string{
			"run", "--rm", "-d", "--name=" + fmt.Sprintf("molecule-%s", RoleFlag),
			"-v", fmt.Sprintf("%s/molecule:/opt/molecule", path),
			"-v", "/sys/fs/cgroup:/sys/fs/cgroup:rw",
			"-e", "TOKEN=" + os.Getenv("TOKEN"),
			"-e", "VAULT_TOKEN=" + os.Getenv("VAULT_TOKEN"),
			"-e", "VAULT_ADDR=" + os.Getenv("VAULT_ADDR"),
			"-e", "GIT_USER=" + os.Getenv("GIT_USER"),
			"-e", "GIT_PASSWORD=" + os.Getenv("GIT_PASSWORD"),
			"-e", "GIT_URL=" + os.Getenv("GIT_URL"),
			"--cgroupns", "host",
			"--privileged", "--pull", "always",
			image,
		}
		if err := runCommand("docker", args...); err != nil {
			log.Printf("\033[33mdocker run failed: %v\033[0m", err)
		}
	}

	// ensure role exists
	if exists(roleMoleculePath) {
		fmt.Println("\033[35mThis role already exists in molecule\033[0m")
	} else {
		// docker exec -ti molecule-$role /bin/sh -c "ansible-galaxy role init $org.$role"
		if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("ansible-galaxy role init %s.%s", OrgFlag, RoleFlag)); err != nil {
			log.Printf("\033[33mrole init warning: %v\033[0m", err)
		}
		if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("rm -f %s.%s/*/*", OrgFlag, RoleFlag)); err != nil {
			log.Printf("\033mclean role dir warning: %v\033[0m", err)
		}
	}

	// docker exec login to cr.yandex inside container
	_ = dockerExecInteractiveHide(RoleFlag, "/bin/sh", "-c", `echo $TOKEN | docker login cr.yandex --username iam --password-stdin`)

	// copy files into molecule structure
	if err := copyRoleData(path, roleMoleculePath); err != nil {
		log.Printf("\033[33mcopy role data warning: %v\033[0m", err)
	}

	// finally create/converge
	err = exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", RoleFlag)).Run()
	if err == nil {
		// container exists
		_ = dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("cd ./%s && molecule converge", roleDirName))
	} else {
		_ = dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("cd ./%s && molecule create", roleDirName))
		_ = dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("cd ./%s && molecule converge", roleDirName))
	} // copy dotfiles

	return nil
}

// copyRoleData copies tasks, handlers, templates, files, vars, defaults, meta, scenarios, .ansible-lint, .yamllint
func copyRoleData(basePath, roleMoleculePath string) error {
	config, err := LoadConfig()
	if err != nil {
		log.Printf("\033[33mwarning loading config: %v\033[0m", err)
	}
	// create role dir base
	if err := os.MkdirAll(roleMoleculePath, 0o755); err != nil {
		return err
	}
	// helper copy pairs
	pairs := []struct{ src, dst string }{
		{"tasks", "tasks"},
		{"handlers", "handlers"},
		{"templates", "templates"},
		{"files", "files"},
		{"vars", "vars"},
		{"defaults", "defaults"},
		{"meta", "meta"},
		{"scenarios", "molecule"}, // copy scenarios into molecule/<role>/molecule/
	}
	for _, p := range pairs {
		src := filepath.Join(basePath, p.src)
		dst := filepath.Join(roleMoleculePath, p.dst)
		if p.src == "scenarios" {
			dst = filepath.Join(roleMoleculePath, "molecule")
		}
		copyIfExists(src, dst)
	}

	yamlrules := YamlLintRulesExport{
		Braces:             config.YamlLintConfig.Rules.Braces,
		Brackets:           config.YamlLintConfig.Rules.Brackets,
		NewLines:           config.YamlLintConfig.Rules.NewLines,
		Comments:           config.YamlLintConfig.Rules.Comments,
		CommentsIdentation: config.YamlLintConfig.Rules.CommentsIdentation,
		OctalValues:        config.YamlLintConfig.Rules.OctalValues,
	}

	exportYamlLint := YamlLintExport{
		Extends: config.YamlLintConfig.Extends,
		Ignore:  strings.Join(config.YamlLintConfig.Ignore, "\n"),
		Rules:   &yamlrules,
	}
	yamllint, err := yaml.Marshal(exportYamlLint)
	if err != nil {
		log.Printf("\033[33mwarning marshaling yamllint config: %v\033[0m", err)
	} else {
		yamllintPath := filepath.Join(roleMoleculePath, ".yamllint")
		if err := os.WriteFile(yamllintPath, yamllint, 0o644); err != nil {
			log.Printf("\033[33mwarning writing .yamllint: %v\033[0m", err)
		}
	}

	exportAnsibleLint := AnsibleLintExport{
		ExcludedPaths: config.AnsibleLintConfig.ExcludedPaths,
		WarnList:      config.AnsibleLintConfig.WarnList,
		SkipList:      config.AnsibleLintConfig.SkipList,
	}

	ansiblelint, err := yaml.Marshal(exportAnsibleLint)
	if err != nil {
		log.Printf("\033[33mwarning marshaling ansible-lint config: %v\033[0m", err)
	} else {
		ansiblelintPath := filepath.Join(roleMoleculePath, ".ansible-lint")
		if err := os.WriteFile(ansiblelintPath, ansiblelint, 0o644); err != nil {
			log.Printf("\033[33mwarning writing .ansible-lint: %v\033[0m", err)
		}
	}

	return nil
}

// copyIfExists copies file/directory if it exists (recursively when directory)
func copyIfExists(src, dst string) {
	if !exists(src) {
		log.Printf("\033[38;2;127;255;212mnote: %s does not exist, skipping\033[0m", src)
		return
	}
	fi, err := os.Stat(src)
	if err != nil {
		log.Printf("copy stat error: %v", err)
		return
	}
	if fi.IsDir() {
		if err := copyDir(src, dst); err != nil {
			log.Printf("copy dir error %s -> %s: %v", src, dst, err)
		}
	} else {
		// file
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			log.Printf("mkdir for file: %v", err)
		}
		if err := copyFile(src, dst); err != nil {
			log.Printf("copy file error %v", err)
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := in.Close(); cerr != nil {
			log.Printf("Failed to close source file: %v", cerr)
		}
	}()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil {
			log.Printf("Failed to close destination file: %v", cerr)
		}
	}()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		// file
		return copyFile(path, target)
	})
}

// dockerExecInteractive runs: docker exec -ti molecule-role <cmd...>
func dockerExecInteractive(role, command string, args ...string) error {
	all := []string{"exec", "-ti", fmt.Sprintf("molecule-%s", role), command}
	all = append(all, args...)
	cmd := exec.Command("docker", all...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// dockerExecInteractiveHide runs: docker exec -ti molecule-role <cmd...>
func dockerExecInteractiveHide(role, command string, args ...string) error {
	all := []string{"exec", "-ti", fmt.Sprintf("molecule-%s", role), command}
	all = append(all, args...)
	cmd := exec.Command("docker", all...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
func YcCliInitWrapper() error {
	return YcCliInit()
}

// Windows-only: compact WSL and Optimize-VHD for Docker Desktop VHDX files.
// This will:
// - Stop Docker Desktop process
// - wsl --shutdown
// - run Optimize-VHD on $env:LOCALAPPDATA\Docker\wsl\data\*.vhdx (docker-desktop-data vhdx and docker-desktop vhdx)
// - restart Docker Desktop
func compactWSLAndOptimize() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("compactWSLAndOptimize is Windows only")
	}

	// stop Docker Desktop (graceful quit)
	log.Println("Stopping Docker Desktop (if running)...")
	// Stop-Process -Name "Docker Desktop" -Force
	psStop := `if (Get-Process -Name "Docker Desktop" -ErrorAction SilentlyContinue) { Stop-Process -Name "Docker Desktop" -Force }`
	if err := runPowerShell(psStop); err != nil {
		log.Printf("\033[33mwarning stopping Docker Desktop: %v\033[0m", err)
	}

	// shutdown WSL
	log.Println("Shutting down WSL...")
	if err := runCommand("wsl", "--shutdown"); err != nil {
		log.Printf("\033[33mwarning: wsl --shutdown returned: %v\033[0m", err)
	}

	// small wait
	time.Sleep(2 * time.Second)

	// build VHDX paths
	// $env:LOCALAPPDATA\Docker\wsl\data\docker-desktop-data.vhdx
	paths := []string{
		`$env:LOCALAPPDATA\Docker\wsl\disk\docker_data.vhdx`,
	}
	for _, p := range paths {
		log.Printf("Running Optimize-VHD for %s (requires admin)...", p)
		cmd := fmt.Sprintf("Optimize-VHD -Path %s -Mode Full", p)
		if err := runPowerShell(cmd); err != nil {
			log.Printf("Optimize-VHD failed for %s: %v", p, err)
		}
	}

	// restart Docker Desktop
	log.Println("Starting Docker Desktop...")
	startCmd := `Start-Process "$env:ProgramFiles\Docker\Docker\Docker Desktop.exe"`
	if err := runPowerShell(startCmd); err != nil {
		log.Printf("\033[33mwarning starting Docker Desktop: %v\033[0m", err)
	}

	log.Println("WSL compact/Optimize-VHD completed (check for errors above).")
	return nil
}

// runPowerShell executes a powershell command and streams its output.
func runPowerShell(cmd string) error {
	// Use powershell -NoProfile -Command "<cmd>"
	c := exec.Command("powershell", "-NoProfile", "-Command", cmd)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
