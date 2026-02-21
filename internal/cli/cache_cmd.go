package cli

import (
	"fmt"

	"diffusion/internal/cache"
	"diffusion/internal/config"

	"github.com/spf13/cobra"
)

// NewCacheCmd creates the cache command with subcommands
func NewCacheCmd(cli *CLI) *cobra.Command {
	cacheCmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage Ansible cache for faster role execution",
	}

	cacheCmd.AddCommand(newCacheEnableCmd(cli))
	cacheCmd.AddCommand(newCacheDisableCmd(cli))
	cacheCmd.AddCommand(newCacheCleanCmd(cli))
	cacheCmd.AddCommand(newCacheStatusCmd(cli))
	cacheCmd.AddCommand(newCacheListCmd(cli))

	return cacheCmd
}

func newCacheEnableCmd(cli *CLI) *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "Enable Ansible cache for this role",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			// Generate or get cache ID
			cacheID, err := cache.GetOrCreateCacheID(cfg)
			if err != nil {
				return fmt.Errorf("failed to generate cache ID: %w", err)
			}

			// Create cache directory
			cacheDir, err := cache.EnsureCacheDir(cacheID, cfg.CacheConfig.CachePath)
			if err != nil {
				return fmt.Errorf("failed to create cache directory: %w", err)
			}

			// Update config
			if cfg.CacheConfig == nil {
				cfg.CacheConfig = &config.CacheSettings{}
			}
			cfg.CacheConfig.Enabled = true
			cfg.CacheConfig.CacheID = cacheID

			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("\033[32mCache enabled for this role\033[0m\n")
			fmt.Printf("\033[35mCache ID: \033[0m\033[38;2;127;255;212m%s\033[0m\n", cacheID)
			fmt.Printf("\033[35mCache Directory: \033[0m\033[38;2;127;255;212m%s\033[0m\n", cacheDir)
			return nil
		},
	}
}

func newCacheDisableCmd(cli *CLI) *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable Ansible cache for this role",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.CacheConfig == nil {
				fmt.Println("\033[33mCache is not configured\033[0m")
				return nil
			}

			cfg.CacheConfig.Enabled = false

			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("\033[32mCache disabled for this role\033[0m\n")
			fmt.Printf("\033[33mNote: Cache directory is preserved. Use 'diffusion cache clean' to remove it.\033[0m\n")
			return nil
		},
	}
}

func newCacheCleanCmd(cli *CLI) *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Clean the Ansible cache for this role",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.CacheConfig == nil || cfg.CacheConfig.CacheID == "" {
				fmt.Println("\033[33mNo cache configured for this role\033[0m")
				return nil
			}

			cacheID := cfg.CacheConfig.CacheID

			// Get cache size before cleaning
			size, _ := cache.GetCacheSize(cacheID, cfg.CacheConfig.CachePath)

			// Note: cache commands don't have access to --role flag
			// We can only clean the cache on the host filesystem
			// If you need to clean cache inside a running container, use:
			// docker exec molecule-<role> /bin/sh -c "rm -rf /root/.ansible/roles/* /root/.ansible/collections/*"

			// Clean cache on host
			if err := cache.CleanupCache(cacheID, cfg.CacheConfig.CachePath); err != nil {
				return fmt.Errorf("failed to clean cache: %w", err)
			}
			fmt.Printf("\033[32mCache cleaned on host\033[0m\n")

			if size > 0 {
				fmt.Printf("\033[35mFreed: \033[0m\033[38;2;127;255;212m%.2f MB\033[0m\n", float64(size)/(1024*1024))
			}
			return nil
		},
	}
}

func newCacheStatusCmd(cli *CLI) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show cache status for this role",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if cfg.CacheConfig == nil {
				fmt.Println("\033[33mCache is not configured for this role\033[0m")
				return nil
			}

			fmt.Println("\033[35m[Cache Status]\033[0m")
			fmt.Printf("  Enabled:     \033[38;2;127;255;212m%t\033[0m\n", cfg.CacheConfig.Enabled)
			fmt.Printf("  Cache ID:    \033[38;2;127;255;212m%s\033[0m\n", cfg.CacheConfig.CacheID)

			if cfg.CacheConfig.CacheID != "" {
				cacheDir, _ := cache.GetCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
				fmt.Printf("  Cache Path:  \033[38;2;127;255;212m%s\033[0m\n", cacheDir)

				size, err := cache.GetCacheSize(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
				if err == nil && size > 0 {
					fmt.Printf("  Cache Size:  \033[38;2;127;255;212m%.2f MB\033[0m\n", float64(size)/(1024*1024))
				} else {
					fmt.Printf("  Cache Size:  \033[38;2;127;255;212m0 MB (empty)\033[0m\n")
				}
			}

			return nil
		},
	}
}

func newCacheListCmd(cli *CLI) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all cache directories in home directory.",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.LoadConfig()
			if err != nil {
				return fmt.Errorf("failed to download config: %w", err)
			}
			caches, err := cache.ListCaches()
			if err != nil {
				return fmt.Errorf("failed to list caches: %w", err)
			}

			if len(caches) == 0 {
				fmt.Println("\033[33mNo cache directories found\033[0m")
				return nil
			}

			fmt.Println("\033[35m[Cache Directories]\033[0m")
			for _, cacheEntry := range caches {
				// Extract cache ID from directory name (role_<id>)
				cacheID := cacheEntry[5:] // Remove "role_" prefix
				size, _ := cache.GetCacheSize(cacheID, cfg.CacheConfig.CachePath)
				fmt.Printf("  \033[32m✓\033[0m %s - \033[38;2;127;255;212m%.2f MB\033[0m\n", cacheEntry, float64(size)/(1024*1024))
			}

			return nil
		},
	}
}
