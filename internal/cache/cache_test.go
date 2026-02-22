package cache

import (
	"os"
	"path/filepath"
	"testing"

	"diffusion/internal/config"
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
	customPath := ""
	cacheDir, err := GetCacheDir(cacheID, customPath)
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
	customPath := ""
	cacheDir, err := EnsureCacheDir(cacheID, customPath)
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
	customPath := ""

	// Create cache directory
	cacheDir, err := EnsureCacheDir(cacheID, customPath)
	if err != nil {
		t.Fatalf("EnsureCacheDir failed: %v", err)
	}

	// Create a test file in cache
	testFile := filepath.Join(cacheDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Cleanup cache
	if err := CleanupCache(cacheID, customPath); err != nil {
		t.Fatalf("CleanupCache failed: %v", err)
	}

	// Verify directory was removed
	if _, err := os.Stat(cacheDir); !os.IsNotExist(err) {
		t.Error("cache directory should be removed")
	}
}

func TestGetCacheSize(t *testing.T) {
	cacheID := "testsize"
	customPath := ""

	// Create cache directory
	cacheDir, err := EnsureCacheDir(cacheID, customPath)
	if err != nil {
		t.Fatalf("EnsureCacheDir failed: %v", err)
	}
	defer CleanupCache(cacheID, customPath)

	// Initially should be 0 or very small
	size, err := GetCacheSize(cacheID, customPath)
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
	size, err = GetCacheSize(cacheID, customPath)
	if err != nil {
		t.Fatalf("GetCacheSize failed: %v", err)
	}

	if size < int64(len(testData)) {
		t.Errorf("expected cache size >= %d, got %d", len(testData), size)
	}
}

func TestGetOrCreateCacheID(t *testing.T) {
	// Test with nil cache config
	cfg := &config.Config{}
	id1, err := GetOrCreateCacheID(cfg)
	if err != nil {
		t.Fatalf("GetOrCreateCacheID failed: %v", err)
	}

	if id1 == "" {
		t.Error("cache ID should not be empty")
	}

	// Test with existing cache ID
	cfg.CacheConfig = &config.CacheSettings{
		CacheID: "existing123",
	}

	id2, err := GetOrCreateCacheID(cfg)
	if err != nil {
		t.Fatalf("GetOrCreateCacheID failed: %v", err)
	}

	if id2 != "existing123" {
		t.Errorf("expected existing cache ID 'existing123', got %s", id2)
	}
}

func TestCacheConfigInConfig(t *testing.T) {
	cfg := &config.Config{
		ContainerRegistry: &config.ContainerRegistry{
			RegistryServer:        "ghcr.io",
			RegistryProvider:      "Public",
			MoleculeContainerName: "test",
			MoleculeContainerTag:  "latest",
		},
		HashicorpVault: &config.HashicorpVault{
			HashicorpVaultIntegration: false,
		},
		CacheConfig: &config.CacheSettings{
			Enabled: true,
			CacheID: "abc123",
		},
		YamlLintConfig: &config.YamlLint{
			Extends: "default",
			Ignore:  []string{},
			Rules:   &config.YamlLintRules{},
		},
		AnsibleLintConfig: &config.AnsibleLint{
			ExcludedPaths: []string{},
			WarnList:      []string{},
			SkipList:      []string{},
		},
		TestsConfig: &config.TestsSettings{
			Type: "diffusion",
		},
	}

	if cfg.CacheConfig == nil {
		t.Error("cache config should not be nil")
	}

	if !cfg.CacheConfig.Enabled {
		t.Error("cache should be enabled")
	}

	if cfg.CacheConfig.CacheID != "abc123" {
		t.Errorf("expected cache ID 'abc123', got %s", cfg.CacheConfig.CacheID)
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

func TestEnsureUVCacheDir(t *testing.T) {
	cacheID := "testuvcache"
	customPath := ""

	uvDir, err := EnsureUVCacheDir(cacheID, customPath)
	if err != nil {
		t.Fatalf("EnsureUVCacheDir failed: %v", err)
	}
	defer CleanupCache(cacheID, customPath)

	// Verify directory was created
	info, err := os.Stat(uvDir)
	if err != nil {
		t.Fatalf("UV cache directory should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("UV cache path should be a directory")
	}

	// Should end with /uv
	if filepath.Base(uvDir) != config.CacheUVDir {
		t.Errorf("UV cache dir should end with '%s', got %s", config.CacheUVDir, filepath.Base(uvDir))
	}
}

func TestEnsureDockerCacheDir(t *testing.T) {
	cacheID := "testdockercache"
	customPath := ""

	dockerDir, err := EnsureDockerCacheDir(cacheID, customPath)
	if err != nil {
		t.Fatalf("EnsureDockerCacheDir failed: %v", err)
	}
	defer CleanupCache(cacheID, customPath)

	// Verify directory was created
	info, err := os.Stat(dockerDir)
	if err != nil {
		t.Fatalf("Docker cache directory should exist: %v", err)
	}
	if !info.IsDir() {
		t.Error("Docker cache path should be a directory")
	}

	// Should end with /docker
	if filepath.Base(dockerDir) != config.CacheDockerDir {
		t.Errorf("Docker cache dir should end with '%s', got %s", config.CacheDockerDir, filepath.Base(dockerDir))
	}
}

func TestGetDockerImageTarballPath(t *testing.T) {
	cacheID := "testtarball"
	customPath := ""

	tarballPath, err := GetDockerImageTarballPath(cacheID, customPath)
	if err != nil {
		t.Fatalf("GetDockerImageTarballPath failed: %v", err)
	}

	// Should end with docker/image.tar
	if filepath.Base(tarballPath) != config.DockerImageTarball {
		t.Errorf("tarball path should end with '%s', got %s", config.DockerImageTarball, filepath.Base(tarballPath))
	}
	parentDir := filepath.Base(filepath.Dir(tarballPath))
	if parentDir != config.CacheDockerDir {
		t.Errorf("tarball parent dir should be '%s', got %s", config.CacheDockerDir, parentDir)
	}
}

func TestHasCachedDockerImage(t *testing.T) {
	cacheID := "testhascached"
	customPath := ""

	// Should return false when no tarball exists
	if HasCachedDockerImage(cacheID, customPath) {
		t.Error("HasCachedDockerImage should return false when no tarball exists")
	}

	// Create the tarball file
	dockerDir, err := EnsureDockerCacheDir(cacheID, customPath)
	if err != nil {
		t.Fatalf("EnsureDockerCacheDir failed: %v", err)
	}
	defer CleanupCache(cacheID, customPath)

	tarballPath := filepath.Join(dockerDir, config.DockerImageTarball)
	if err := os.WriteFile(tarballPath, []byte("fake tarball"), 0644); err != nil {
		t.Fatalf("failed to create fake tarball: %v", err)
	}

	// Should return true now
	if !HasCachedDockerImage(cacheID, customPath) {
		t.Error("HasCachedDockerImage should return true when tarball exists")
	}
}

func TestGetSubdirSize(t *testing.T) {
	cacheID := "testsubdir"
	customPath := ""

	// Create cache with subdirectories
	cacheDir, err := EnsureCacheDir(cacheID, customPath)
	if err != nil {
		t.Fatalf("EnsureCacheDir failed: %v", err)
	}
	defer CleanupCache(cacheID, customPath)

	// Non-existent subdir should return 0
	size, err := GetSubdirSize(cacheID, customPath, "nonexistent")
	if err != nil {
		t.Fatalf("GetSubdirSize failed for nonexistent dir: %v", err)
	}
	if size != 0 {
		t.Errorf("expected size 0 for nonexistent subdir, got %d", size)
	}

	// Create a subdir with a file
	rolesDir := filepath.Join(cacheDir, config.CacheRolesDir)
	if err := os.MkdirAll(rolesDir, 0755); err != nil {
		t.Fatalf("failed to create roles dir: %v", err)
	}
	testData := []byte("test role data for size measurement")
	if err := os.WriteFile(filepath.Join(rolesDir, "test.yml"), testData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	size, err = GetSubdirSize(cacheID, customPath, config.CacheRolesDir)
	if err != nil {
		t.Fatalf("GetSubdirSize failed: %v", err)
	}
	if size < int64(len(testData)) {
		t.Errorf("expected size >= %d, got %d", len(testData), size)
	}
}

func TestCacheConfigDockerUVFields(t *testing.T) {
	cfg := &config.Config{
		CacheConfig: &config.CacheSettings{
			Enabled:     true,
			CacheID:     "test456",
			DockerCache: true,
			UVCache:     true,
		},
	}

	if !cfg.CacheConfig.DockerCache {
		t.Error("DockerCache should be true")
	}
	if !cfg.CacheConfig.UVCache {
		t.Error("UVCache should be true")
	}
}
