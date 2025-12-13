package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGenerateCacheID(t *testing.T) {
	id1, err := GenerateCacheID()
	if err != nil {
		t.Fatalf("GenerateCacheID failed: %v", err)
	}

	if len(id1) != 16 { // 8 bytes = 16 hex characters
		t.Errorf("expected cache ID length 16, got %d", len(id1))
	}

	// Generate another ID and ensure it's different
	id2, err := GenerateCacheID()
	if err != nil {
		t.Fatalf("GenerateCacheID failed: %v", err)
	}

	if id1 == id2 {
		t.Error("expected different cache IDs, got same")
	}
}

func TestGetCacheDir(t *testing.T) {
	cacheID := "test123"
	cacheDir, err := GetCacheDir(cacheID)
	if err != nil {
		t.Fatalf("GetCacheDir failed: %v", err)
	}

	if cacheDir == "" {
		t.Error("cache directory should not be empty")
	}

	// Should contain .diffusion/cache/role_<id>
	if !contains(cacheDir, ".diffusion") {
		t.Errorf("cache directory should contain '.diffusion', got %s", cacheDir)
	}

	if !contains(cacheDir, "cache") {
		t.Errorf("cache directory should contain 'cache', got %s", cacheDir)
	}

	if !contains(cacheDir, "role_test123") {
		t.Errorf("cache directory should contain 'role_test123', got %s", cacheDir)
	}
}

func TestEnsureCacheDir(t *testing.T) {
	cacheID := "testcache"
	cacheDir, err := EnsureCacheDir(cacheID)
	if err != nil {
		t.Fatalf("EnsureCacheDir failed: %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(cacheDir)
	if err != nil {
		t.Fatalf("cache directory should exist: %v", err)
	}

	if !info.IsDir() {
		t.Error("cache path should be a directory")
	}

	// Cleanup
	homeDir, _ := os.UserHomeDir()
	testCacheBase := filepath.Join(homeDir, ".diffusion", "cache", "role_testcache")
	os.RemoveAll(testCacheBase)
}

func TestCleanupCache(t *testing.T) {
	cacheID := "testcleanup"

	// Create cache directory
	cacheDir, err := EnsureCacheDir(cacheID)
	if err != nil {
		t.Fatalf("EnsureCacheDir failed: %v", err)
	}

	// Create a test file in cache
	testFile := filepath.Join(cacheDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Cleanup cache
	if err := CleanupCache(cacheID); err != nil {
		t.Fatalf("CleanupCache failed: %v", err)
	}

	// Verify directory was removed
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("cache directory should be removed")
	}
}

func TestGetCacheSize(t *testing.T) {
	cacheID := "testsize"

	// Create cache directory
	cacheDir, err := EnsureCacheDir(cacheID)
	if err != nil {
		t.Fatalf("EnsureCacheDir failed: %v", err)
	}
	defer CleanupCache(cacheID)

	// Initially should be 0 or very small
	size, err := GetCacheSize(cacheID)
	if err != nil {
		t.Fatalf("GetCacheSize failed: %v", err)
	}

	if size < 0 {
		t.Error("cache size should not be negative")
	}

	// Create a test file
	testFile := filepath.Join(cacheDir, "test.txt")
	testData := []byte("test data for size calculation")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Get size again
	size, err = GetCacheSize(cacheID)
	if err != nil {
		t.Fatalf("GetCacheSize failed: %v", err)
	}

	if size < int64(len(testData)) {
		t.Errorf("expected cache size >= %d, got %d", len(testData), size)
	}
}

func TestGetOrCreateCacheID(t *testing.T) {
	// Test with nil cache config
	config := &Config{}
	id1, err := GetOrCreateCacheID(config)
	if err != nil {
		t.Fatalf("GetOrCreateCacheID failed: %v", err)
	}

	if id1 == "" {
		t.Error("cache ID should not be empty")
	}

	// Test with existing cache ID
	config.CacheConfig = &CacheSettings{
		CacheID: "existing123",
	}

	id2, err := GetOrCreateCacheID(config)
	if err != nil {
		t.Fatalf("GetOrCreateCacheID failed: %v", err)
	}

	if id2 != "existing123" {
		t.Errorf("expected existing cache ID 'existing123', got %s", id2)
	}
}

func TestCacheConfigInConfig(t *testing.T) {
	config := &Config{
		ContainerRegistry: &ContainerRegistry{
			RegistryServer:        "ghcr.io",
			RegistryProvider:      "Public",
			MoleculeContainerName: "test",
			MoleculeContainerTag:  "latest",
		},
		HashicorpVault: &HashicorpVault{
			HashicorpVaultIntegration: false,
		},
		CacheConfig: &CacheSettings{
			Enabled: true,
			CacheID: "abc123",
		},
		YamlLintConfig: &YamlLint{
			Extends: "default",
			Ignore:  []string{},
			Rules:   &YamlLintRules{},
		},
		AnsibleLintConfig: &AnsibleLint{
			ExcludedPaths: []string{},
			WarnList:      []string{},
			SkipList:      []string{},
		},
		TestsConfig: &TestsSettings{
			Type: "diffusion",
		},
	}

	if config.CacheConfig == nil {
		t.Error("cache config should not be nil")
	}

	if !config.CacheConfig.Enabled {
		t.Error("cache should be enabled")
	}

	if config.CacheConfig.CacheID != "abc123" {
		t.Errorf("expected cache ID 'abc123', got %s", config.CacheConfig.CacheID)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
