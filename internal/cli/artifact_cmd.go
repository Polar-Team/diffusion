package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/secrets"

	"github.com/spf13/cobra"
)

// NewArtifactCmd creates the artifact command with subcommands
func NewArtifactCmd(cli *CLI) *cobra.Command {
	artifactCmd := &cobra.Command{
		Use:   "artifact",
		Short: "Manage private artifact repository credentials",
	}

	artifactCmd.AddCommand(newArtifactAddCmd())
	artifactCmd.AddCommand(newArtifactListCmd())
	artifactCmd.AddCommand(newArtifactRemoveCmd())
	artifactCmd.AddCommand(newArtifactShowCmd())

	return artifactCmd
}

func newArtifactAddCmd() *cobra.Command {
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
			source := config.ArtifactSource{
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

				creds := &config.ArtifactCredentials{
					Name:     sourceName,
					URL:      url,
					Username: username,
					Token:    token,
				}

				if err := secrets.SaveArtifactCredentials(creds); err != nil {
					return fmt.Errorf("failed to save credentials: %w", err)
				}

				roleName := secrets.GetCurrentRoleName()
				if roleName == "" {
					roleName = "default"
				}
				fmt.Printf("\033[32mCredentials for '%s' saved successfully (encrypted in ~/.diffusion/secrets/%s/%s)\033[0m\n", sourceName, roleName, sourceName)
			}

			// Load existing config or create new one
			cfg, err := config.LoadConfig()
			if err != nil {
				// Config doesn't exist, create minimal config
				cfg = &config.Config{
					ArtifactSources: []config.ArtifactSource{},
				}
			}

			// Check if source already exists
			for i, existing := range cfg.ArtifactSources {
				if existing.Name == sourceName {
					// Update existing source
					cfg.ArtifactSources[i] = source
					if err := config.SaveConfig(cfg); err != nil {
						return fmt.Errorf("failed to update config: %w", err)
					}
					fmt.Printf("\033[32mUpdated artifact source '%s' in diffusion.toml\033[0m\n", sourceName)
					return nil
				}
			}

			// Add new source
			cfg.ArtifactSources = append(cfg.ArtifactSources, source)
			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("\033[32mAdded artifact source '%s' to diffusion.toml\033[0m\n", sourceName)
			return nil
		},
	}

	return artifactAddCmd
}

func newArtifactListCmd() *cobra.Command {
	artifactListCmd := &cobra.Command{
		Use:   "list",
		Short: "List all stored artifact sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			sources, err := secrets.ListStoredCredentials()
			if err != nil {
				return fmt.Errorf("failed to list credentials: %w", err)
			}

			if len(sources) == 0 {
				fmt.Println("No stored artifact credentials found.")
				return nil
			}

			fmt.Println("\033[35mStored Artifact Sources:\033[0m")
			for _, source := range sources {
				creds, err := secrets.LoadArtifactCredentials(source)
				if err != nil {
					fmt.Printf("  \033[31m✗\033[0m %s (error loading: %v)\n", source, err)
					continue
				}
				fmt.Printf("  \033[32m✓\033[0m %s - %s\n", creds.Name, creds.URL)
			}
			return nil
		},
	}

	return artifactListCmd
}

func newArtifactRemoveCmd() *cobra.Command {
	artifactRemoveCmd := &cobra.Command{
		Use:   "remove [source-name]",
		Short: "Remove stored credentials for an artifact source",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceName := args[0]

			// Delete encrypted credentials (if they exist)
			if err := secrets.DeleteArtifactCredentials(sourceName); err != nil {
				// Don't fail if credentials don't exist - might be Vault-only
				fmt.Printf("\033[33mNote: No local credentials found for '%s' (may be using Vault)\033[0m\n", sourceName)
			} else {
				fmt.Printf("\033[32mLocal credentials for '%s' removed successfully\033[0m\n", sourceName)
			}

			// Remove from config file
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Find and remove the source
			found := false
			for i, source := range cfg.ArtifactSources {
				if source.Name == sourceName {
					cfg.ArtifactSources = append(cfg.ArtifactSources[:i], cfg.ArtifactSources[i+1:]...)
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("artifact source '%s' not found in diffusion.toml", sourceName)
			}

			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("\033[32mRemoved artifact source '%s' from diffusion.toml\033[0m\n", sourceName)
			return nil
		},
	}

	return artifactRemoveCmd
}

func newArtifactShowCmd() *cobra.Command {
	artifactShowCmd := &cobra.Command{
		Use:   "show [source-name]",
		Short: "Show details for an artifact source (without token)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceName := args[0]

			creds, err := secrets.LoadArtifactCredentials(sourceName)
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

	return artifactShowCmd
}
