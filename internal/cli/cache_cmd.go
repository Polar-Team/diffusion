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

	cacheCmd.AddCommand(newCacheEnableCmd())
	cacheCmd.AddCommand(newCacheDisableCmd())
	cacheCmd.AddCommand(newCacheCleanCmd())
	cacheCmd.AddCommand(newCacheStatusCmd())
	cacheCmd.AddCommand(newCacheListCmd())

	return cacheCmd
}

func newCacheEnableCmd() *cobra.Command {
	var dockerCache bool
	var uvCache bool

	cmd := &cobra.Command{
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

			// Initialize CacheConfig if needed
			if cfg.CacheConfig == nil {
				cfg.CacheConfig = &config.CacheSettings{}
			}

			// Create cache directory
			cacheDir, err := cache.EnsureCacheDir(cacheID, cfg.CacheConfig.CachePath)
			if err != nil {
				return fmt.Errorf("failed to create cache directory: %w", err)
			}

			// Update config
			cfg.CacheConfig.Enabled = true
			cfg.CacheConfig.CacheID = cacheID

			// Apply flags if explicitly set, otherwise preserve existing values
			if cmd.Flags().Changed("docker") {
				cfg.CacheConfig.DockerCache = dockerCache
			}
			if cmd.Flags().Changed("uv") {
				cfg.CacheConfig.UVCache = uvCache
			}

			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("\033[32mCache enabled for this role\033[0m\n")
			fmt.Printf("\033[35mCache ID:      \033[0m\033[38;2;127;255;212m%s\033[0m\n", cacheID)
			fmt.Printf("\033[35mCache Path:    \033[0m\033[38;2;127;255;212m%s\033[0m\n", cacheDir)
			fmt.Printf("\033[35mDocker cache:  \033[0m\033[38;2;127;255;212m%t\033[0m\n", cfg.CacheConfig.DockerCache)
			fmt.Printf("\033[35mUV cache:      \033[0m\033[38;2;127;255;212m%t\033[0m\n", cfg.CacheConfig.UVCache)
			return nil
		},
	}

	cmd.Flags().BoolVar(&dockerCache, "docker", false, "Enable Docker image caching (saves/loads image tarballs)")
	cmd.Flags().BoolVar(&uvCache, "uv", false, "Enable UV/Python package caching")

	return cmd
}

func newCacheDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Disable cache for this role",
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
			cfg.CacheConfig.DockerCache = false
			cfg.CacheConfig.UVCache = false

			if err := config.SaveConfig(cfg); err != nil {
				return fmt.Errorf("failed to save config: %w", err)
			}

			fmt.Printf("\033[32mCache disabled for this role (roles, collections, Docker, UV)\033[0m\n")
			fmt.Printf("\033[33mNote: Cache directory is preserved. Use 'diffusion cache clean' to remove it.\033[0m\n")
			return nil
		},
	}
}

func newCacheCleanCmd() *cobra.Command {
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
			cachePath := cfg.CacheConfig.CachePath

			// Get per-type sizes before cleaning
			rolesSize, _ := cache.GetSubdirSize(cacheID, cachePath, config.CacheRolesDir)
			collectionsSize, _ := cache.GetSubdirSize(cacheID, cachePath, config.CacheCollectionsDir)
			uvSize, _ := cache.GetSubdirSize(cacheID, cachePath, config.CacheUVDir)
			dockerSize, _ := cache.GetSubdirSize(cacheID, cachePath, config.CacheDockerDir)
			totalSize := rolesSize + collectionsSize + uvSize + dockerSize

			// Clean cache on host
			if err := cache.CleanupCache(cacheID, cachePath); err != nil {
				return fmt.Errorf("failed to clean cache: %w", err)
			}
			fmt.Printf("\033[32mCache cleaned on host\033[0m\n")

			if totalSize > 0 {
				fmt.Printf("\033[35mFreed: \033[0m\033[38;2;127;255;212m%.2f MB\033[0m\n", float64(totalSize)/(1024*1024))
				if rolesSize > 0 {
					fmt.Printf("  Roles:       %.2f MB\n", float64(rolesSize)/(1024*1024))
				}
				if collectionsSize > 0 {
					fmt.Printf("  Collections: %.2f MB\n", float64(collectionsSize)/(1024*1024))
				}
				if uvSize > 0 {
					fmt.Printf("  UV packages: %.2f MB\n", float64(uvSize)/(1024*1024))
				}
				if dockerSize > 0 {
					fmt.Printf("  Docker image: %.2f MB\n", float64(dockerSize)/(1024*1024))
				}
			}
			return nil
		},
	}
}

func newCacheStatusCmd() *cobra.Command {
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
			fmt.Printf("  Enabled:       \033[38;2;127;255;212m%t\033[0m\n", cfg.CacheConfig.Enabled)
			fmt.Printf("  Cache ID:      \033[38;2;127;255;212m%s\033[0m\n", cfg.CacheConfig.CacheID)
			fmt.Printf("  Docker cache:  \033[38;2;127;255;212m%t\033[0m\n", cfg.CacheConfig.DockerCache)
			fmt.Printf("  UV cache:      \033[38;2;127;255;212m%t\033[0m\n", cfg.CacheConfig.UVCache)

			if cfg.CacheConfig.CacheID != "" {
				cacheDir, _ := cache.GetCacheDir(cfg.CacheConfig.CacheID, cfg.CacheConfig.CachePath)
				fmt.Printf("  Cache Path:    \033[38;2;127;255;212m%s\033[0m\n", cacheDir)

				cacheID := cfg.CacheConfig.CacheID
				cachePath := cfg.CacheConfig.CachePath

				// Per-type size breakdown
				rolesSize, _ := cache.GetSubdirSize(cacheID, cachePath, config.CacheRolesDir)
				collectionsSize, _ := cache.GetSubdirSize(cacheID, cachePath, config.CacheCollectionsDir)
				uvSize, _ := cache.GetSubdirSize(cacheID, cachePath, config.CacheUVDir)
				dockerSize, _ := cache.GetSubdirSize(cacheID, cachePath, config.CacheDockerDir)
				totalSize := rolesSize + collectionsSize + uvSize + dockerSize

				fmt.Println("\033[35m  [Size Breakdown]\033[0m")
				fmt.Printf("    Roles:        \033[38;2;127;255;212m%.2f MB\033[0m\n", float64(rolesSize)/(1024*1024))
				fmt.Printf("    Collections:  \033[38;2;127;255;212m%.2f MB\033[0m\n", float64(collectionsSize)/(1024*1024))
				fmt.Printf("    UV packages:  \033[38;2;127;255;212m%.2f MB\033[0m\n", float64(uvSize)/(1024*1024))
				fmt.Printf("    Docker image: \033[38;2;127;255;212m%.2f MB\033[0m\n", float64(dockerSize)/(1024*1024))
				fmt.Printf("    Total:        \033[38;2;127;255;212m%.2f MB\033[0m\n", float64(totalSize)/(1024*1024))

				// Show Docker image cache status
				if cfg.CacheConfig.DockerCache {
					if cache.HasCachedDockerImage(cacheID, cachePath) {
						fmt.Printf("  Docker image:  \033[32mcached\033[0m\n")
					} else {
						fmt.Printf("  Docker image:  \033[33mnot cached\033[0m\n")
					}
				}
			}

			return nil
		},
	}
}

func newCacheListCmd() *cobra.Command {
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
