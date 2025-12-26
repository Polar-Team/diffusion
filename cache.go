package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// GetCacheDir returns the cache directory for the current role
func GetCacheDir(cacheID string, customPath string) (string, error) {

	cacheDir := ""
	if _, err := os.Stat(customPath); err == nil {
		cacheDir = filepath.Join(customPath, "cache", fmt.Sprintf("role_%s", cacheID))
	} else {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}

		cacheDir = filepath.Join(homeDir, ".diffusion", "cache", fmt.Sprintf("role_%s", cacheID))
	}
	return cacheDir, nil
}

// EnsureCacheDir creates the cache directory if it doesn't exist
func EnsureCacheDir(cacheID string, customPath string) (string, error) {
	cacheDir, err := GetCacheDir(cacheID, customPath)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	return cacheDir, nil
}

// GenerateCacheID generates a random cache ID
func GenerateCacheID() (string, error) {
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate cache ID: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// GetOrCreateCacheID returns existing cache ID or creates a new one
func GetOrCreateCacheID(config *Config) (string, error) {
	if config.CacheConfig != nil && config.CacheConfig.CacheID != "" {
		return config.CacheConfig.CacheID, nil
	}

	return GenerateCacheID()
}

// CleanupCache removes the cache directory for a given cache ID
func CleanupCache(cacheID string, customPath string) error {
	cacheDir, err := GetCacheDir(cacheID, customPath)
	if err != nil {
		return err
	}

	if err := os.RemoveAll(cacheDir); err != nil {
		return fmt.Errorf("failed to remove cache directory: %w", err)
	}

	return nil
}

// ListCaches returns all cache directories
func ListCaches() ([]string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	cacheBaseDir := filepath.Join(homeDir, ".diffusion", "cache")

	entries, err := os.ReadDir(cacheBaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	var caches []string
	for _, entry := range entries {
		if entry.IsDir() {
			caches = append(caches, entry.Name())
		}
	}

	return caches, nil
}

// GetCacheSize returns the size of a cache directory in bytes
func GetCacheSize(cacheID string, customPath string) (int64, error) {
	cacheDir, err := GetCacheDir(cacheID, customPath)
	if err != nil {
		return 0, err
	}

	var size int64
	err = filepath.Walk(cacheDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}
