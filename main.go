package main

// diffusion - Cobra-based cross-platform CLI tool to assist with Molecule workflows.
//
// Features:
// - Ensures required env vars are set (vault_user, vault_passwd, GIT_URL, PROJECT_ID, VAULT_ADDR)
// - Prompts for VAULT credentials if not in env
// - Gets Vault token and pulls secrets (GIT user/token)
// - Runs `yc` init
// - Implements "molecule" command with flags: role, org, tag, verify, lint, idempotence, wipe
// - Copies role files into molecule layout (if present)
// - Runs docker commands similar to your PowerShell script
//
// NOTE: This CLI shells out to external tools: vault, yc, docker. They must be available in PATH.

import (
	"bufio"
	"runtime"

	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Version is set at build time via ldflags
var Version = "dev"

var (
	RoleInitFlag       bool
	RoleFlag           string
	OrgFlag            string
	RoleScenario       string
	AddRoleFlag        string
	RoleSrcFlag        string
	RoleScmFlag        string
	RoleVersionFlag    string
	AddCollectionFlag  string
	TagFlag            string
	ConvergeFlag       bool
	VerifyFlag         bool
	TestsOverWriteFlag bool
	LintFlag           bool
	IdempotenceFlag    bool
	DestroyFlag        bool
	WipeFlag           bool
	CIMode             bool
)

func main() {

	rootCmd := &cobra.Command{
		Use:   "diffusion",
		Short: "Molecule workflow helper (cross-platform)",
	}

	roleCmd := &cobra.Command{
		Use:   "role",
		Short: "Configure role settings interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle --init flag
			if RoleInitFlag {
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
				err = SaveMetaFile(MetaConfig)
				if err != nil {
					return fmt.Errorf("failed to save meta file: %w", err)
				}

				err = SaveRequirementFile(RequirementConfig, "default")
				if err != nil {
					return fmt.Errorf("failed to save requirements file: %w", err)
				}

				fmt.Println("Role initialized successfully.")
				return nil
			}

			// Load existing role config
			meta, req, err := LoadRoleConfig("")
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
					// Remove the role from the slice using slices package
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

	// artifact command for managing private artifact sources
	artifactCmd := &cobra.Command{
		Use:   "artifact",
		Short: "Manage private artifact repository credentials",
	}

	artifactAddCmd := &cobra.Command{
		Use:   "add [source-name]",
		Short: "Add credentials for a private artifact source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceName := args[0]
			reader := bufio.NewReader(os.Stdin)

			fmt.Printf("Enter URL for %s: ", sourceName)
			url, _ := reader.ReadString('\n')
			url = strings.TrimSpace(url)

			fmt.Print("Store credentials in Vault? (y/N): ")
			useVaultStr, _ := reader.ReadString('\n')
			useVaultStr = strings.TrimSpace(strings.ToLower(useVaultStr))
			useVault := useVaultStr == "y"

			// Create artifact source
			source := ArtifactSource{
				Name:     sourceName,
				URL:      url,
				UseVault: useVault,
			}

			if useVault {
				fmt.Printf("Enter Vault path for %s (e.g., secret/data/artifacts): ", sourceName)
				vaultPath, _ := reader.ReadString('\n')
				source.VaultPath = strings.TrimSpace(vaultPath)

				fmt.Printf("Enter Vault secret name for %s: ", sourceName)
				vaultSecret, _ := reader.ReadString('\n')
				source.VaultSecretName = strings.TrimSpace(vaultSecret)

				fmt.Print("Enter Username Field in Vault (default: username): ")
				usernameField, _ := reader.ReadString('\n')
				usernameField = strings.TrimSpace(usernameField)
				if usernameField == "" {
					usernameField = "username"
				}
				source.VaultUsernameField = usernameField

				fmt.Print("Enter Token Field in Vault (default: token): ")
				tokenField, _ := reader.ReadString('\n')
				tokenField = strings.TrimSpace(tokenField)
				if tokenField == "" {
					tokenField = "token"
				}
				source.VaultTokenField = tokenField

				fmt.Printf("\033[32mArtifact source '%s' configured to use Vault at %s/%s\033[0m\n", sourceName, source.VaultPath, source.VaultSecretName)
			} else {
				// Local storage - prompt for credentials
				fmt.Printf("Enter Username: ")
				username, _ := reader.ReadString('\n')
				username = strings.TrimSpace(username)

				fmt.Printf("Enter Token/Password: ")
				token, _ := reader.ReadString('\n')
				token = strings.TrimSpace(token)

				creds := &ArtifactCredentials{
					Name:     sourceName,
					URL:      url,
					Username: username,
					Token:    token,
				}

				if err := SaveArtifactCredentials(creds); err != nil {
					return fmt.Errorf("failed to save credentials: %w", err)
				}

				roleName := getCurrentRoleName()
				if roleName == "" {
					roleName = "default"
				}
				fmt.Printf("\033[32mCredentials for '%s' saved successfully (encrypted in ~/.diffusion/secrets/%s/%s)\033[0m\n", sourceName, roleName, sourceName)
			}

			// Load existing config or create new one
			config, err := LoadConfig()
			if err != nil {
				// Config doesn't exist, create minimal config
				config = &Config{
					ArtifactSources: []ArtifactSource{},
				}
			}

			// Check if source already exists
			for i, existing := range config.ArtifactSources {
				if existing.Name == sourceName {
					// Update existing source
					config.ArtifactSources[i] = source
					if err := SaveConfig(config); err != nil {
						return fmt.Errorf("failed to update config: %w", err)
					}
					fmt.Printf("\033[32mUpdated artifact source '%s' in diffusion.toml\033[0m\n", sourceName)
					return nil
				}
			}

			// Add new source
			config.ArtifactSources = append(config.ArtifactSources, source)
			if err := SaveConfig(config); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("\033[32mAdded artifact source '%s' to diffusion.toml\033[0m\n", sourceName)
			return nil
		},
	}

	artifactListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all stored artifact sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			sources, err := ListStoredCredentials()
			if err != nil {
				return fmt.Errorf("failed to list credentials: %w", err)
			}

			if len(sources) == 0 {
				fmt.Println("No stored artifact credentials found.")
				return nil
			}

			fmt.Println("\033[35mStored Artifact Sources:\033[0m")
			for _, source := range sources {
				creds, err := LoadArtifactCredentials(source)
				if err != nil {
					fmt.Printf("  \033[31m✗\033[0m %s (error loading: %v)\n", source, err)
					continue
				}
				fmt.Printf("  \033[32m✓\033[0m %s - %s\n", creds.Name, creds.URL)
			}
			return nil
		},
	}

	artifactRemoveCmd := &cobra.Command{
		Use:   "remove [source-name]",
		Short: "Remove stored credentials for an artifact source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceName := args[0]

			// Delete encrypted credentials (if they exist)
			if err := DeleteArtifactCredentials(sourceName); err != nil {
				// Don't fail if credentials don't exist - might be Vault-only
				fmt.Printf("\033[33mNote: No local credentials found for '%s' (may be using Vault)\033[0m\n", sourceName)
			} else {
				fmt.Printf("\033[32mLocal credentials for '%s' removed successfully\033[0m\n", sourceName)
			}

			// Remove from config file
			config, err := LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Find and remove the source
			found := false
			for i, source := range config.ArtifactSources {
				if source.Name == sourceName {
					config.ArtifactSources = append(config.ArtifactSources[:i], config.ArtifactSources[i+1:]...)
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("artifact source '%s' not found in diffusion.toml", sourceName)
			}

			if err := SaveConfig(config); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("\033[32mRemoved artifact source '%s' from diffusion.toml\033[0m\n", sourceName)
			return nil
		},
	}

	artifactShowCmd := &cobra.Command{
		Use:   "show [source-name]",
		Short: "Show details for an artifact source (without token)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceName := args[0]

			creds, err := LoadArtifactCredentials(sourceName)
			if err != nil {
				return fmt.Errorf("failed to load credentials: %w", err)
			}

			fmt.Printf("\033[35mArtifact Source: \033[0m\033[38;2;127;255;212m%s\033[0m\n", creds.Name)
			fmt.Printf("\033[35mURL: \033[0m\033[38;2;127;255;212m%s\033[0m\n", creds.URL)
			fmt.Printf("\033[35mUsername: \033[0m\033[38;2;127;255;212m%s\033[0m\n", creds.Username)
			fmt.Printf("\033[35mToken: \033[0m\033[38;2;127;255;212m%s\033[0m\n", maskToken(creds.Token))
			return nil
		},
	}

	artifactCmd.AddCommand(artifactAddCmd)
	artifactCmd.AddCommand(artifactListCmd)
	artifactCmd.AddCommand(artifactRemoveCmd)
	artifactCmd.AddCommand(artifactShowCmd)

	rootCmd.AddCommand(artifactCmd)

	// cache command for managing Ansible cache
	cacheCmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage Ansible cache for faster role execution",
	}

	cacheEnableCmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable Ansible cache for this role",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Generate or get cache ID
			cacheID, err := GetOrCreateCacheID(config)
			if err != nil {
				return fmt.Errorf("failed to generate cache ID: %w", err)
			}

			// Create cache directory
			cacheDir, err := EnsureCacheDir(cacheID)
			if err != nil {
				return fmt.Errorf("failed to create cache directory: %w", err)
			}

			// Update config
			if config.CacheConfig == nil {
				config.CacheConfig = &CacheSettings{}
			}
			config.CacheConfig.Enabled = true
			config.CacheConfig.CacheID = cacheID

			if err := SaveConfig(config); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("\033[32mCache enabled for this role\033[0m\n")
			fmt.Printf("\033[35mCache ID: \033[0m\033[38;2;127;255;212m%s\033[0m\n", cacheID)
			fmt.Printf("\033[35mCache Directory: \033[0m\033[38;2;127;255;212m%s\033[0m\n", cacheDir)
			return nil
		},
	}

	cacheDisableCmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable Ansible cache for this role",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if config.CacheConfig == nil {
				fmt.Println("\033[33mCache is not configured\033[0m")
				return nil
			}

			config.CacheConfig.Enabled = false

			if err := SaveConfig(config); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("\033[32mCache disabled for this role\033[0m\n")
			fmt.Printf("\033[33mNote: Cache directory is preserved. Use 'diffusion cache clean' to remove it.\033[0m\n")
			return nil
		},
	}

	cacheCleanCmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean the Ansible cache for this role",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if config.CacheConfig == nil || config.CacheConfig.CacheID == "" {
				fmt.Println("\033[33mNo cache configured for this role\033[0m")
				return nil
			}

			cacheID := config.CacheConfig.CacheID

			// Get cache size before cleaning
			size, _ := GetCacheSize(cacheID)

			// Check if molecule container is running
			containerName := fmt.Sprintf("molecule-%s", RoleFlag)
			err = exec.Command("docker", "inspect", containerName).Run()
			if err == nil {
				// Container exists - clean cache inside container
				cleanCmd := "rm -rf /root/.ansible/roles/* /root/.ansible/collections/*"
				if err := dockerExecInteractiveHide(RoleFlag, "/bin/sh", "-c", cleanCmd); err != nil {
					log.Printf("\033[33mwarning: failed to clean cache inside container: %v\033[0m", err)
				}
				fmt.Printf("\033[32mCache cleaned inside container\033[0m\n")
			} else {
				// Container doesn't exist - clean cache on host
				if err := CleanupCache(cacheID); err != nil {
					return fmt.Errorf("failed to clean cache: %w", err)
				}
				fmt.Printf("\033[32mCache cleaned on host\033[0m\n")
			}

			if size > 0 {
				fmt.Printf("\033[35mFreed: \033[0m\033[38;2;127;255;212m%.2f MB\033[0m\n", float64(size)/(1024*1024))
			}
			return nil
		},
	}

	cacheStatusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show cache status for this role",
		RunE: func(cmd *cobra.Command, args []string) error {
			config, err := LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if config.CacheConfig == nil {
				fmt.Println("\033[33mCache is not configured for this role\033[0m")
				return nil
			}

			fmt.Println("\033[35m[Cache Status]\033[0m")
			fmt.Printf("  Enabled:     \033[38;2;127;255;212m%t\033[0m\n", config.CacheConfig.Enabled)
			fmt.Printf("  Cache ID:    \033[38;2;127;255;212m%s\033[0m\n", config.CacheConfig.CacheID)

			if config.CacheConfig.CacheID != "" {
				cacheDir, _ := GetCacheDir(config.CacheConfig.CacheID)
				fmt.Printf("  Cache Path:  \033[38;2;127;255;212m%s\033[0m\n", cacheDir)

				size, err := GetCacheSize(config.CacheConfig.CacheID)
				if err == nil && size > 0 {
					fmt.Printf("  Cache Size:  \033[38;2;127;255;212m%.2f MB\033[0m\n", float64(size)/(1024*1024))
				} else {
					fmt.Printf("  Cache Size:  \033[38;2;127;255;212m0 MB (empty)\033[0m\n")
				}
			}

			return nil
		},
	}

	cacheListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all cache directories",
		RunE: func(cmd *cobra.Command, args []string) error {
			caches, err := ListCaches()
			if err != nil {
				return fmt.Errorf("failed to list caches: %w", err)
			}

			if len(caches) == 0 {
				fmt.Println("\033[33mNo cache directories found\033[0m")
				return nil
			}

			fmt.Println("\033[35m[Cache Directories]\033[0m")
			for _, cache := range caches {
				// Extract cache ID from directory name (role_<id>)
				cacheID := cache[5:] // Remove "role_" prefix
				size, _ := GetCacheSize(cacheID)
				fmt.Printf("  \033[32m✓\033[0m %s - \033[38;2;127;255;212m%.2f MB\033[0m\n", cache, float64(size)/(1024*1024))
			}

			return nil
		},
	}

	cacheCmd.AddCommand(cacheEnableCmd)
	cacheCmd.AddCommand(cacheDisableCmd)
	cacheCmd.AddCommand(cacheCleanCmd)
	cacheCmd.AddCommand(cacheStatusCmd)
	cacheCmd.AddCommand(cacheListCmd)

	rootCmd.AddCommand(cacheCmd)

	// m
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

				fmt.Printf("Enter RegistryServer (%s): ", DefaultRegistryServer)
				registryServer, _ := reader.ReadString('\n')
				registryServer = strings.TrimSpace(registryServer)
				if registryServer == "" {
					registryServer = DefaultRegistryServer
				}

				fmt.Printf("Enter RegistryProvider (%s): ", DefaultRegistryProvider)
				registryProvider, _ := reader.ReadString('\n')
				registryProvider = strings.TrimSpace(registryProvider)
				if registryProvider == "" {
					registryProvider = DefaultRegistryProvider
				}

				if registryProvider != "YC" && registryProvider != "AWS" && registryProvider != "GCP" && registryProvider != "Public" {
					fmt.Fprintln(os.Stderr, "\033[31mInvalid RegistryProvider. Allowed values are: YC, AWS, GCP. \nIf you're using public registry, then choose Public - or choose it, if you want to authenticate externally.\033[0m")
					os.Exit(1)
				}

				fmt.Printf("Enter MoleculeContainerName (%s): ", DefaultMoleculeContainerName)
				moleculeContainerName, _ := reader.ReadString('\n')
				moleculeContainerName = strings.TrimSpace(moleculeContainerName)
				if moleculeContainerName == "" {
					moleculeContainerName = DefaultMoleculeContainerName
				}

				defaultTag := GetDefaultMoleculeTag()
				fmt.Printf("Enter MoleculeContainerTag (%s): ", defaultTag)
				moleculeContainerTag, _ := reader.ReadString('\n')
				moleculeContainerTag = strings.TrimSpace(moleculeContainerTag)
				if moleculeContainerTag == "" {
					moleculeContainerTag = defaultTag
				}

				ContainerRegistry := &ContainerRegistry{
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

				config = &Config{
					ContainerRegistry: ContainerRegistry,
					HashicorpVault:    HashicorpVaultSet,
					ArtifactSources:   ArtifactSourcesList,
					YamlLintConfig:    YamlLintDefault,
					AnsibleLintConfig: AnsibleLintDefault,
					TestsConfig:       TestsSettings,
				}

				if err := SaveConfig(config); err != nil {
					log.Printf("\033[33mwarning saving new config: %v\033[0m", err)
				}

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
	molCmd.Flags().StringVarP(&TagFlag, "tag", "t", "", "Ansible tags to run (comma-separated, e.g., 'install,configure')")
	molCmd.Flags().BoolVar(&ConvergeFlag, "converge", false, "run molecule converge")
	molCmd.Flags().BoolVar(&VerifyFlag, "verify", false, "run molecule verify")
	molCmd.Flags().BoolVar(&TestsOverWriteFlag, "testsoverwrite", false, "overwrite molecule tests folder for remote or diffusion type")
	molCmd.Flags().BoolVar(&LintFlag, "lint", false, "run linting (yamllint / ansible-lint)")
	molCmd.Flags().BoolVar(&IdempotenceFlag, "idempotence", false, "run molecule idempotence")
	molCmd.Flags().BoolVar(&DestroyFlag, "destroy", false, "run molecule destroy")
	molCmd.Flags().BoolVar(&WipeFlag, "wipe", false, "remove container and molecule role folder")
	molCmd.Flags().BoolVar(&CIMode, "ci", false, "CI/CD mode (non-interactive, skip TTY and permission fixes)")

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
				if config.HashicorpVault.SecretKV2Path != "" {
					fmt.Printf("  \033[33mLegacy Config Detected (deprecated):\033[0m\n")
					fmt.Printf("    Secret KV2 Path:       \033[38;2;127;255;212m%s\033[0m\n", config.HashicorpVault.SecretKV2Path)
					fmt.Printf("    Secret KV2 Name:       \033[38;2;127;255;212m%s\033[0m\n", config.HashicorpVault.SecretKV2Name)
					fmt.Printf("  \033[33mPlease migrate to artifact_sources configuration\033[0m\n")
				} else {
					fmt.Printf("  \033[32mVault configured per artifact source\033[0m\n")
				}
			}
			fmt.Println()

			// Artifact Sources
			fmt.Println("\033[35m[Artifact Sources]\033[0m")
			if len(config.ArtifactSources) > 0 {
				for i, source := range config.ArtifactSources {
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
			fmt.Printf("  Extends:                 \033[38;2;127;255;212m%s\033[0m\n", config.YamlLintConfig.Extends)
			fmt.Printf("  Ignore Patterns:\n")
			for _, pattern := range config.YamlLintConfig.Ignore {
				fmt.Printf("    \033[38;2;127;255;212m- %s\033[0m\n", pattern)
			}
			if config.YamlLintConfig.Rules != nil {
				fmt.Printf("  Rules:\n")
				if config.YamlLintConfig.Rules.Braces != nil {
					fmt.Printf("    Braces:                \033[38;2;127;255;212m%v\033[0m\n", config.YamlLintConfig.Rules.Braces)
				}
				if config.YamlLintConfig.Rules.Brackets != nil {
					fmt.Printf("    Brackets:              \033[38;2;127;255;212m%v\033[0m\n", config.YamlLintConfig.Rules.Brackets)
				}
				if config.YamlLintConfig.Rules.NewLines != nil {
					fmt.Printf("    New Lines:             \033[38;2;127;255;212m%v\033[0m\n", config.YamlLintConfig.Rules.NewLines)
				}
				if config.YamlLintConfig.Rules.Comments != nil {
					fmt.Printf("    Comments:              \033[38;2;127;255;212m%v\033[0m\n", config.YamlLintConfig.Rules.Comments)
				}
				fmt.Printf("    Comments Indentation:  \033[38;2;127;255;212m%t\033[0m\n", config.YamlLintConfig.Rules.CommentsIdentation)
				if config.YamlLintConfig.Rules.OctalValues != nil {
					fmt.Printf("    Octal Values:          \033[38;2;127;255;212m%v\033[0m\n", config.YamlLintConfig.Rules.OctalValues)
				}
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

	// version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Diffusion version %s\n", Version)
			fmt.Printf("Go version: %s\n", runtime.Version())
			fmt.Printf("OS/Arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		},
	}
	rootCmd.AddCommand(versionCmd)

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

	args := []string{
		"run",
	}

	// Add user mapping for Unix systems to avoid permission issues
	args = append(args, GetUserMappingArgs()...)

	// Set HOME environment variable for ansible-galaxy config
	// containerHome := GetContainerHomePath()

	args = append(args,
		// "-e", fmt.Sprintf("HOME=%s", containerHome),
		"-v", fmt.Sprintf("%s:/ansible", currentDir),
		"-w", "/ansible",
		fmt.Sprintf("ghcr.io/polar-team/diffusion-molecule-container:%s", GetDefaultMoleculeTag()),
		"ansible-galaxy", "role", "init", roleName,
	)

	fmt.Printf("Initializing Ansible role: %s\n", roleName)

	err = runCommandHide("docker", args...)
	if err != nil {
		log.Printf("\033[31mInitializing of new role were failed: %v\033[0m", err)
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
# vars:
#    system_name: YOUR_PLATFORM_NAME
#    docker_user_uid: 1000
#  roles:
#    - role: tests/diffusion_tests
#      vars:
#        port_test: true
#        port: YOUR_PORT_NUMBER
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

func MetaConfigSetup(roleName string) *Meta {
	reader := bufio.NewReader(os.Stdin)

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

func VaultConfigHelper(integration bool) *HashicorpVault {
	return &HashicorpVault{
		HashicorpVaultIntegration: integration,
	}
}

func ArtifactSourcesHelper() []ArtifactSource {
	reader := bufio.NewReader(os.Stdin)
	var sources []ArtifactSource

	fmt.Print("Configure artifact sources for private repositories? (y/N): ")
	configureStr, _ := reader.ReadString('\n')
	configureStr = strings.TrimSpace(strings.ToLower(configureStr))

	if configureStr != "y" {
		fmt.Println("Skipping artifact source configuration. You can add sources later with 'diffusion artifact add'")
		return sources
	}

	for {
		fmt.Print("\nEnter artifact source name (or press Enter to finish): ")
		name, _ := reader.ReadString('\n')
		name = strings.TrimSpace(name)

		if name == "" {
			break
		}

		fmt.Printf("Enter URL for %s: ", name)
		url, _ := reader.ReadString('\n')
		url = strings.TrimSpace(url)

		fmt.Print("Store credentials in Vault? (y/N): ")
		useVaultStr, _ := reader.ReadString('\n')
		useVaultStr = strings.TrimSpace(strings.ToLower(useVaultStr))
		useVault := useVaultStr == "y"

		source := ArtifactSource{
			Name:     name,
			URL:      url,
			UseVault: useVault,
		}

		if useVault {
			fmt.Printf("Enter Vault path for %s (e.g., secret/data/artifacts): ", name)
			vaultPath, _ := reader.ReadString('\n')
			source.VaultPath = strings.TrimSpace(vaultPath)

			fmt.Printf("Enter Vault secret name for %s: ", name)
			vaultSecret, _ := reader.ReadString('\n')
			source.VaultSecretName = strings.TrimSpace(vaultSecret)

			fmt.Print("Enter Username Field in Vault (default: username): ")
			usernameField, _ := reader.ReadString('\n')
			usernameField = strings.TrimSpace(usernameField)
			if usernameField == "" {
				usernameField = "username"
			}
			source.VaultUsernameField = usernameField

			fmt.Print("Enter Token Field in Vault (default: token): ")
			tokenField, _ := reader.ReadString('\n')
			tokenField = strings.TrimSpace(tokenField)
			if tokenField == "" {
				tokenField = "token"
			}
			source.VaultTokenField = tokenField

			fmt.Printf("Credentials for '%s' will be retrieved from Vault at %s/%s (fields: %s, %s)\n",
				name, source.VaultPath, source.VaultSecretName, source.VaultUsernameField, source.VaultTokenField)
		} else {
			// Prompt for credentials and save locally
			fmt.Printf("Enter Username for %s: ", name)
			username, _ := reader.ReadString('\n')
			username = strings.TrimSpace(username)

			fmt.Printf("Enter Token/Password for %s: ", name)
			token, _ := reader.ReadString('\n')
			token = strings.TrimSpace(token)

			// Save credentials locally
			creds := &ArtifactCredentials{
				Name:     name,
				URL:      url,
				Username: username,
				Token:    token,
			}

			if err := SaveArtifactCredentials(creds); err != nil {
				fmt.Printf("\033[33mWarning: failed to save credentials for '%s': %v\033[0m\n", name, err)
			} else {
				fmt.Printf("\033[32mCredentials for '%s' saved locally (encrypted)\033[0m\n", name)
			}
		}

		sources = append(sources, source)
		fmt.Printf("\033[32mAdded artifact source: %s\033[0m\n", name)

		fmt.Print("Add another artifact source? (y/N): ")
		addAnother, _ := reader.ReadString('\n')
		addAnother = strings.TrimSpace(strings.ToLower(addAnother))
		if addAnother != "y" {
			break
		}
	}

	return sources
}

func TestsConfigSetup() *TestsSettings {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("What type of configuration you want use? remote / local / diffusion(Default): ")
	configType, _ := reader.ReadString('\n')
	configType = strings.TrimSpace(strings.ToLower(configType))

	if configType == "" {
		configType = "diffusion"
	}
	if configType != "remote" && configType != "local" && configType != "diffusion" {
		fmt.Fprintln(os.Stderr, "\033[31mInvalid configuration type. Allowed values are: remote, local, diffusion.\033[0m")
		os.Exit(1)
	}

	remoteReposList := []string{}
	if configType == "remote" {
		fmt.Print("Enter remote repository URLs seperated by comma, it should be public or from artifact URL path (if remote selected): ")
		remoteReposInput, _ := reader.ReadString('\n')
		remoteReposInput = strings.TrimSpace(remoteReposInput)
		if remoteReposInput != "" {
			for r := range strings.SplitSeq(remoteReposInput, ",") {
				r = strings.TrimSpace(r)
				if r != "" {
					remoteReposList = append(remoteReposList, r)
				}
			}
		}
	}
	testsSettings := &TestsSettings{
		Type:               configType,
		RemoteRepositories: remoteReposList,
	}

	return testsSettings
}

func PromptInput(prompt string) string {
	fmt.Print(prompt)
	r := bufio.NewReader(os.Stdin)
	val, _ := r.ReadString('\n')
	return strings.TrimSpace(val)
}

// runCommandHide runs command and discards stdout/stderr with a loading animation
func runCommandHide(name string, args ...string) error {
	spinner := NewSpinner(fmt.Sprintf("Running %s", name))
	spinner.Start()
	defer spinner.Stop()

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
		log.Printf("\033[38;2;127;255;212mWiping: running molecule destroy, removing container molecule-%s and folder %s\n\033[0m", RoleFlag, roleMoleculePath)

		// Run molecule destroy inside the container first
		roleDir := GetRoleDirName(OrgFlag, RoleFlag)
		_ = dockerExecInteractiveHide(RoleFlag, "bash", "-c", fmt.Sprintf("cd ./%s && molecule destroy", roleDir))

		// Remove the container
		_ = runCommandHide("docker", "rm", fmt.Sprintf("molecule-%s", RoleFlag), "-f")

		// Remove the role folder
		if err := os.RemoveAll(roleMoleculePath); err != nil {
			log.Printf("\033[33mwarning: failed remove role path: %v\033[0m", err)
		}
		return nil
	}

	// handle converge/lint/verify/idempotence/destroy by ensuring files are copied and running docker exec commands
	if ConvergeFlag || LintFlag || VerifyFlag || IdempotenceFlag || DestroyFlag {
		if err := copyRoleData(path, roleMoleculePath); err != nil {
			log.Printf("\033[33mwarning copying data: %v\033[0m", err)
		}
		// ensure tests dir exists for verify/lint
		defaultTestsDir := filepath.Join(roleMoleculePath, "molecule", "default", "tests")
		if err := os.MkdirAll(defaultTestsDir, 0o755); err != nil {
			log.Printf("\033[33mwarning: cannot create tests dir: %v\033[0m", err)
		}
		if ConvergeFlag {
			// Verify molecule.yml exists inside container before running
			if CIMode {
				checkCmd := fmt.Sprintf("ls -la /opt/molecule/%s/molecule/default/molecule.yml", roleDirName)
				log.Printf("Checking molecule.yml in container...")
				if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", checkCmd); err != nil {
					log.Printf("\033[31mmolecule.yml not found in container at /opt/molecule/%s/molecule/default/\033[0m", roleDirName)
					log.Printf("\033[33mListing container directory structure:\033[0m")
					_ = dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("ls -laR /opt/molecule/%s/", roleDirName))
					os.Exit(1)
				}
			}

			tagEnv := ""
			if TagFlag != "" {
				tagEnv = fmt.Sprintf("ANSIBLE_RUN_TAGS=%s ", TagFlag)
			}
			cmdStr := fmt.Sprintf("cd ./%s && %smolecule converge", roleDirName, tagEnv)
			if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", cmdStr); err != nil {
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
				if err := dockerExecInteractiveHide(RoleFlag, "/bin/sh", "-c", chownCmd); err != nil {
					log.Printf("\033[33mwarning: failed to fix permissions: %v\033[0m", err)
				}
			}

			return nil
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

			switch config.TestsConfig.Type {
			case "local":
				testsSrc := filepath.Join(path, "tests")
				copyIfExists(testsSrc, defaultTestsDir)
			case "remote":
				for _, repo := range config.TestsConfig.RemoteRepositories {
					roleName := strings.TrimPrefix(strings.TrimSuffix(filepath.Base(repo), ".git"), "/")
					testsRolePath := fmt.Sprintf("%s/%s", defaultTestsDir, roleName)

					// Check if path exists before cloning
					if _, err := os.Stat(testsRolePath); err == nil && !TestsOverWriteFlag {
						log.Printf("\033[35mTests role %s already exist in %s, skipping clone. To overwrite use --testsoverwrite flag\033[0m", roleName, defaultTestsDir)
					} else {
						log.Printf("Cloning remote tests repository: %s", repo)
						tmpDir, err := os.MkdirTemp("", "diffusion-tests-")
						if err != nil {
							log.Printf("\033[33mwarning creating temp dir for tests repo: %v\033[0m", err)
							continue
						}

						if err := runCommandHide("git", "clone", repo, tmpDir); err != nil {
							log.Printf("\033[33mwarning cloning tests repo %s: %v\033[0m", repo, err)
							continue
						}

						testsSrc := tmpDir

						// Remove .git folder from cloned repository
						gitDir := filepath.Join(testsSrc, ".git")
						if err := os.RemoveAll(gitDir); err != nil {
							log.Printf("\033[33mwarning removing .git folder: %v\033[0m", err)
						}

						copyIfExists(testsSrc, testsRolePath)
						if err := os.RemoveAll(tmpDir); err != nil {
							log.Printf("\033[33mwarning removing temp dir for tests repo %s: %v\033[0m", repo, err)
						}
					}
				}
			case "diffusion":
				// copy from diffusion_tests repository
				diffusionTestsPath := fmt.Sprintf("%s/diffusion_tests", defaultTestsDir)

				// Check if path exists and is not empty
				if _, err := os.Stat(diffusionTestsPath); err == nil && !TestsOverWriteFlag {
					log.Printf("\033[35mDiffusion tests role already exist in %s, skipping clone. To overwrite use --testsoverwrite flag\033[0m", diffusionTestsPath)
				} else {
					diffusionTestsRepo := "https://github.com/Polar-Team/diffusion-ansible-tests-role.git"
					log.Printf("\033[35mCloning diffusion tests repository:\033[0m \033[38;2;127;255;212m%s\033[0m", diffusionTestsRepo)
					tmpDir, err := os.MkdirTemp("", "diffusion-tests-")
					if err != nil {
						log.Printf("\033[33mwarning creating temp dir for diffusion tests repo: %v\033[0m", err)
					}
					if err := runCommandHide("git", "clone", diffusionTestsRepo, tmpDir); err != nil {
						log.Printf("\033[33mwarning cloning diffusion tests repo: %v\033[0m", err)
					}
					testsSrc := tmpDir

					// Remove .git folder from cloned repository
					gitDir := filepath.Join(testsSrc, ".git")
					if err := os.RemoveAll(gitDir); err != nil {
						log.Printf("\033[33mwarning removing .git folder: %v\033[0m", err)
					}

					copyIfExists(testsSrc, diffusionTestsPath)
					if err := os.RemoveAll(tmpDir); err != nil {
						log.Printf("\033[33mwarning removing temp dir for diffusion tests repo: %v\033[0m", err)
					}
				}
			}

			cmdStr := fmt.Sprintf("cd ./%s && molecule verify", roleDirName)
			if TagFlag != "" {
				cmdStr = fmt.Sprintf("cd ./%s && ANSIBLE_RUN_TAGS=%s molecule verify", roleDirName, TagFlag)
			}
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
		if DestroyFlag {
			cmdStr := fmt.Sprintf("cd ./%s && molecule destroy", roleDirName)
			if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", cmdStr); err != nil {
				log.Printf("\033[31mDestroy failed: %v\033[0m", err)
				os.Exit(1)
			}
			log.Printf("\033[32mDestroy Done Successfully!\033[0m")
			return nil
		}
	}

	// default flow: create/run container if not exists, copy data, converge
	// check if container exists
	err = exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", RoleFlag)).Run()
	if err == nil {
		fmt.Printf("\033[38;2;127;255;212mContainer molecule-%s already exists. To purge use --wipe.\n\033[0m", RoleFlag)
	} else {
		// Load credentials for all configured artifact sources
		if len(config.ArtifactSources) > 0 {
			for i, source := range config.ArtifactSources {
				index := i + 1
				var creds *ArtifactCredentials
				var err error

				// Get credentials from Vault or local storage
				creds, err = GetArtifactCredentials(&source, config.HashicorpVault)
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
		} else if config.HashicorpVault != nil && config.HashicorpVault.HashicorpVaultIntegration && config.HashicorpVault.SecretKV2Path != "" {
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
		switch config.ContainerRegistry.RegistryProvider {
		case "YC":
			// Yandex Cloud: Initialize yc CLI and login
			if err := YcCliInit(); err != nil {
				log.Printf("\033[33myc init warning: %v\033[0m", err)
			}
			if err := runCommandHide("docker", "login", config.ContainerRegistry.RegistryServer, "--username", "iam", "--password", os.Getenv("TOKEN")); err != nil {
				log.Printf("\033[33mdocker login to registry failed: %v\033[0m", err)
			}
		case "AWS":
			// AWS: Initialize AWS CLI and login to ECR (placeholder)
			// if err := runCommand("aws", "configure", "..."); err != nil {
			//     log.Printf("\033[33maws configure warning: %v\033[0m", err)
			// }
			// if err := runCommand("aws", "ecr", "get-login-password", "..."); err != nil {
			//     log.Printf("\033[33maws ecr login failed: %v\033[0m", err)
			// }
			log.Printf("\033[33mAWS ECR authentication not yet implemented\033[0m")
		case "GCP":
			// GCP: Initialize gcloud CLI and login to Artifact Registry (placeholder)
			// if err := runCommand("gcloud", "init", "..."); err != nil {
			//     log.Printf("\033[33mgcloud init warning: %v\033[0m", err)
			// }
			// if err := runCommand("gcloud", "auth", "configure-docker", "..."); err != nil {
			//     log.Printf("\033[33mgcloud docker auth failed: %v\033[0m", err)
			// }
			log.Printf("\033[33mGCP Artifact Registry authentication not yet implemented\033[0m")
		case "Public":
			// Public registry: No CLI init or authentication needed
			log.Printf("\033[35mUsing public registry, skipping CLI initialization and authentication\033[0m")
		default:
			log.Printf("\033[33mUnknown registry provider '%s', skipping CLI initialization\033[0m", config.ContainerRegistry.RegistryProvider)
		}
		// run container
		// docker run --rm -d --name=molecule-$role -v "$path/molecule:/opt/molecule" -v /sys/fs/cgroup:/sys/fs/cgroup:rw -e ... --privileged --pull always cr.yandex/...
		image := fmt.Sprintf("%s/%s:%s", config.ContainerRegistry.RegistryServer, config.ContainerRegistry.MoleculeContainerName, config.ContainerRegistry.MoleculeContainerTag)
		args := []string{
			"run", "--rm", "-d", "--name=" + fmt.Sprintf("molecule-%s", RoleFlag),
		}

		// Note: Not using user mapping here because DinD (Docker-in-Docker) requires root
		// We'll fix permissions on the mounted volume after operations instead

		volumeMount := fmt.Sprintf("%s/molecule:/opt/molecule", path)
		args = append(args,
			"-v", volumeMount,
			"-e", "TOKEN="+os.Getenv("TOKEN"),
			"-e", "VAULT_TOKEN="+os.Getenv("VAULT_TOKEN"),
			"-e", "VAULT_ADDR="+os.Getenv("VAULT_ADDR"),
		)

		if CIMode {
			log.Printf("\033[35mCI Mode: Volume mount: %s\033[0m", volumeMount)
			log.Printf("\033[35mCI Mode: Role will be at: /opt/molecule/%s\033[0m", roleDirName)
		}

		// Add cgroup mount only if it exists (may not be available in WSL2)
		if _, err := os.Stat("/sys/fs/cgroup"); err == nil {
			args = append(args, "-v", "/sys/fs/cgroup:/sys/fs/cgroup:rw")
		}

		// Add cache volume mounts if enabled (roles and collections only)
		if config.CacheConfig != nil && config.CacheConfig.Enabled && config.CacheConfig.CacheID != "" {
			cacheDir, err := EnsureCacheDir(config.CacheConfig.CacheID)
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
				containerHome := GetContainerHomePath()
				args = append(args, "-v", fmt.Sprintf("%s:%s/.ansible/roles", rolesDir, containerHome))
				args = append(args, "-v", fmt.Sprintf("%s:%s/.ansible/collections", collectionsDir, containerHome))
				log.Printf("\033[32mCache enabled: mounting roles and collections from %s\033[0m", cacheDir)
			}
		}

		// Add all indexed GIT environment variables
		for i := 1; i <= MaxArtifactSources; i++ {
			gitUser := os.Getenv(fmt.Sprintf("%s%d", EnvGitUserPrefix, i))
			gitPassword := os.Getenv(fmt.Sprintf("%s%d", EnvGitPassPrefix, i))
			gitURL := os.Getenv(fmt.Sprintf("%s%d", EnvGitURLPrefix, i))

			if gitUser != "" || gitPassword != "" || gitURL != "" {
				args = append(args, "-e", fmt.Sprintf("%s%d=%s", EnvGitUserPrefix, i, gitUser))
				args = append(args, "-e", fmt.Sprintf("%s%d=%s", EnvGitPassPrefix, i, gitPassword))
				args = append(args, "-e", fmt.Sprintf("%s%d=%s", EnvGitURLPrefix, i, gitURL))
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

	// ensure role exists
	if exists(roleMoleculePath) {
		fmt.Println("\033[35mThis role already exists in molecule\033[0m")
	} else {
		// In CI mode, skip ansible-galaxy role init and copy files directly
		if CIMode {
			log.Printf("\033[35mCI Mode: Creating role directory structure directly\033[0m")
			// Create the role directory on host (will be visible in container via volume mount)
			if err := os.MkdirAll(roleMoleculePath, 0o755); err != nil {
				log.Printf("\033[33mwarning: failed to create role directory: %v\033[0m", err)
			}
		} else {
			// Normal mode: use ansible-galaxy role init
			// docker exec -ti molecule-$role /bin/sh -c "cd /opt/molecule && ansible-galaxy role init $org.$role"
			if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("ansible-galaxy role init %s.%s", OrgFlag, RoleFlag)); err != nil {
				log.Printf("\033[33mrole init warning: %v\033[0m", err)
			}

			// Fix ownership inside container after role init (Unix systems only)
			if runtime.GOOS != "windows" {
				uid := os.Getuid()
				gid := os.Getgid()
				chownCmd := fmt.Sprintf("chown -R %d:%d /opt/molecule/%s.%s", uid, gid, OrgFlag, RoleFlag)
				if err := dockerExecInteractiveHide(RoleFlag, "/bin/sh", "-c", chownCmd); err != nil {
					log.Printf("\033[33mwarning: failed to fix ownership after role init: %v\033[0m", err)
				}
			}

			// Clean up ansible-galaxy skeleton files
			if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("rm -f %s.%s/*/*", OrgFlag, RoleFlag)); err != nil {
				log.Printf("\033[33mclean role dir warning: %v\033[0m", err)
			}
		}
	}

	// docker exec login to registry inside container (provider-specific)
	switch config.ContainerRegistry.RegistryProvider {
	case "YC":
		// Yandex Cloud registry login
		_ = dockerExecInteractiveHide(RoleFlag, "/bin/sh", "-c", `echo $TOKEN | docker login cr.yandex --username iam --password-stdin`)
	case "AWS":
		// AWS ECR login would go here if needed
		// _ = dockerExecInteractiveHide(RoleFlag, "/bin/sh", "-c", `aws ecr get-login-password | docker login ...`)
	case "GCP":
		// GCP Artifact Registry login would go here if needed
		// _ = dockerExecInteractiveHide(RoleFlag, "/bin/sh", "-c", `gcloud auth print-access-token | docker login ...`)
	case "Public":
		// No login needed for public registries
		log.Printf("\033[35mUsing public registry, skipping authentication\033[0m")
	default:
		// Unknown provider, skip login
		log.Printf("\033[33mUnknown registry provider '%s', skipping authentication\033[0m", config.ContainerRegistry.RegistryProvider)
	}

	// copy files into molecule structure
	if err := copyRoleData(path, roleMoleculePath); err != nil {
		log.Printf("\033[33mcopy role data warning: %v\033[0m", err)
	}

	// In CI mode, verify files exist on host before checking container
	if CIMode {
		log.Printf("\033[35mCI Mode: Verifying files on host filesystem\033[0m")
		log.Printf("Host path: %s", roleMoleculePath)
		if entries, err := os.ReadDir(roleMoleculePath); err == nil {
			log.Printf("Host directory contents:")
			for _, entry := range entries {
				log.Printf("  - %s (isDir: %v)", entry.Name(), entry.IsDir())
			}
		} else {
			log.Printf("\033[31mFailed to read host directory: %v\033[0m", err)
		}

		// Check if molecule.yml exists on host
		hostMoleculeYml := filepath.Join(roleMoleculePath, "molecule", "default", "molecule.yml")
		if _, err := os.Stat(hostMoleculeYml); err == nil {
			log.Printf("\033[32m✓ molecule.yml exists on host at: %s\033[0m", hostMoleculeYml)
		} else {
			log.Printf("\033[31m✗ molecule.yml NOT found on host at: %s\033[0m", hostMoleculeYml)
		}
	}

	// finally create/converge
	err = exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", RoleFlag)).Run()
	if err == nil {
		// container exists
		// Verify molecule.yml exists inside container before running (CI mode)
		if CIMode {
			checkCmd := fmt.Sprintf("ls -la /opt/molecule/%s/molecule/default/molecule.yml", roleDirName)
			log.Printf("Checking molecule.yml in container...")
			if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", checkCmd); err != nil {
				log.Printf("\033[31mmolecule.yml not found in container at /opt/molecule/%s/molecule/default/\033[0m", roleDirName)
				log.Printf("\033[33mListing container directory structure:\033[0m")
				_ = dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("ls -laR /opt/molecule/%s/", roleDirName))
				os.Exit(1)
			}
		}
		_ = dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("cd ./%s && molecule converge", roleDirName))
	} else {
		// Verify molecule.yml exists inside container before running (CI mode)
		if CIMode {
			checkCmd := fmt.Sprintf("ls -la /opt/molecule/%s/molecule/default/molecule.yml", roleDirName)
			log.Printf("Checking molecule.yml in container...")
			if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", checkCmd); err != nil {
				log.Printf("\033[31mmolecule.yml not found in container at /opt/molecule/%s/molecule/default/\033[0m", roleDirName)
				log.Printf("\033[33mListing container directory structure:\033[0m")
				_ = dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("ls -laR /opt/molecule/%s/", roleDirName))
				os.Exit(1)
			}
		}
		_ = dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("cd ./%s && molecule create", roleDirName))
		_ = dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("cd ./%s && molecule converge", roleDirName))
	}

	// Fix permissions on molecule directory for Unix systems
	// Container runs as root (for DinD), so we need to fix ownership inside the container
	if runtime.GOOS != "windows" {
		uid := os.Getuid()
		gid := os.Getgid()
		chownCmd := fmt.Sprintf("chown -R %d:%d /opt/molecule", uid, gid)
		if err := dockerExecInteractiveHide(RoleFlag, "/bin/sh", "-c", chownCmd); err != nil {
			log.Printf("\033[33mwarning: failed to fix permissions: %v\033[0m", err)
		}
	}

	return nil
}

// copyRoleData copies tasks, handlers, templates, files, vars, defaults, meta, scenarios, .ansible-lint, .yamllint
func copyRoleData(basePath, roleMoleculePath string) error {
	config, err := LoadConfig()
	if err != nil {
		log.Printf("\033[33mwarning loading config: %v\033[0m", err)
	}

	// Validate that scenarios/default directory exists
	scenariosPath := filepath.Join(basePath, "scenarios", "default")
	if _, err := os.Stat(scenariosPath); os.IsNotExist(err) {
		return fmt.Errorf("\033[31mscenarios/default directory not found in %s\n\nTo fix this:\n1. Initialize a new role: diffusion role --init\n2. Or create the directory structure manually:\n   mkdir -p scenarios/default\n   # Add molecule.yml, converge.yml, verify.yml to scenarios/default/\033[0m", basePath)
	}

	// Validate that molecule.yml exists
	moleculeYml := filepath.Join(scenariosPath, "molecule.yml")
	if _, err := os.Stat(moleculeYml); os.IsNotExist(err) {
		return fmt.Errorf("\033[31mscenarios/default/molecule.yml not found in %s\n\nThis file is required for Molecule testing.\nTo fix this:\n1. Initialize a new role: diffusion role --init\n2. Or create molecule.yml manually in scenarios/default/\033[0m", basePath)
	}

	if CIMode {
		log.Printf("\033[35mCI Mode: Copying role data from %s to %s\033[0m", basePath, roleMoleculePath)
	} else {
		log.Printf("\033[38;2;127;255;212mCopying role data from %s to %s\033[0m", basePath, roleMoleculePath)
	}

	// create role dir base
	if err := os.MkdirAll(roleMoleculePath, 0o755); err != nil {
		return err
	}

	// helper copy pairs - copy scenarios/ to molecule/ in destination
	pairs := []struct{ src, dst string }{
		{"tasks", "tasks"},
		{"handlers", "handlers"},
		{"templates", "templates"},
		{"files", "files"},
		{"vars", "vars"},
		{"defaults", "defaults"},
		{"meta", "meta"},
		{"scenarios", "molecule"}, // copy scenarios/ into molecule/<role>/molecule/
	}

	for _, p := range pairs {
		src := filepath.Join(basePath, p.src)
		dst := filepath.Join(roleMoleculePath, p.dst)
		if CIMode {
			log.Printf("Copying %s -> %s", src, dst)
		}
		copyIfExists(src, dst)
	}

	// Verify that molecule.yml was copied successfully
	copiedMoleculeYml := filepath.Join(roleMoleculePath, "molecule", "default", "molecule.yml")
	if CIMode {
		log.Printf("Verifying molecule.yml exists at: %s", copiedMoleculeYml)
	}
	if _, err := os.Stat(copiedMoleculeYml); os.IsNotExist(err) {
		// List what's actually in the molecule directory for debugging
		moleculeDir := filepath.Join(roleMoleculePath, "molecule")
		if entries, err := os.ReadDir(moleculeDir); err == nil {
			log.Printf("\033[33mContents of %s:\033[0m", moleculeDir)
			for _, entry := range entries {
				log.Printf("  - %s (isDir: %v)", entry.Name(), entry.IsDir())
			}
		}
		return fmt.Errorf("\033[31mFailed to copy molecule.yml to %s\nSource: %s\n\nThis may be a permission or file system issue.\033[0m", copiedMoleculeYml, moleculeYml)
	}

	if CIMode {
		log.Printf("\033[32m✓ molecule.yml successfully copied\033[0m")
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
// Performance optimization: cache os.Stat result to avoid duplicate calls
func copyIfExists(src, dst string) {
	fi, err := os.Stat(src)
	if os.IsNotExist(err) {
		log.Printf("\033[38;2;127;255;212mnote: %s does not exist, skipping\033[0m", src)
		return
	}
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

	// Use buffered I/O for better performance
	bufIn := bufio.NewReaderSize(in, 32*1024) // 32KB buffer
	bufOut := bufio.NewWriterSize(out, 32*1024)

	if _, err := io.Copy(bufOut, bufIn); err != nil {
		return err
	}

	if err := bufOut.Flush(); err != nil {
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
// In CI mode, removes -ti flags to avoid TTY errors
func dockerExecInteractive(role, command string, args ...string) error {
	execFlags := []string{"exec"}
	if !CIMode {
		execFlags = append(execFlags, "-ti")
	}
	execFlags = append(execFlags, fmt.Sprintf("molecule-%s", role), command)
	all := append(execFlags, args...)
	cmd := exec.Command("docker", all...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

// dockerExecInteractiveHide runs: docker exec -ti molecule-role <cmd...>
// In CI mode, removes -ti flags to avoid TTY errors
func dockerExecInteractiveHide(role, command string, args ...string) error {
	if !CIMode {
		spinner := NewSpinner(fmt.Sprintf("Running %s in container", command))
		spinner.Start()
		defer spinner.Stop()
	}

	execFlags := []string{"exec"}
	if !CIMode {
		execFlags = append(execFlags, "-ti")
	}
	execFlags = append(execFlags, fmt.Sprintf("molecule-%s", role), command)
	all := append(execFlags, args...)
	cmd := exec.Command("docker", all...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
