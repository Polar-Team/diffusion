package utils

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"diffusion/internal/config"
)

func TestPathCache(t *testing.T) {
	cache := NewPathCache()
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// First call should check filesystem
	if !cache.Exists(testFile) {
		t.Error("expected file to exist")
	}

	// Second call should use cache
	if !cache.Exists(testFile) {
		t.Error("expected cached result to be true")
	}

	// Test non-existent file
	nonExistent := filepath.Join(tmpDir, "nonexistent.txt")
	if cache.Exists(nonExistent) {
		t.Error("expected file to not exist")
	}

	// Test invalidation
	cache.Invalidate(testFile)
	if !cache.Exists(testFile) {
		t.Error("expected file to exist after invalidation")
	}

	// Test clear
	cache.Clear()
	if !cache.Exists(testFile) {
		t.Error("expected file to exist after clear")
	}
}

func TestEnsureDir(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "test", "nested", "dir")

	if err := EnsureDir(testDir); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}

	if !Exists(testDir) {
		t.Error("directory was not created")
	}

	// Test idempotency
	if err := EnsureDir(testDir); err != nil {
		t.Errorf("EnsureDir should be idempotent: %v", err)
	}
}

func TestEnsureDirs(t *testing.T) {
	tmpDir := t.TempDir()
	dir1 := filepath.Join(tmpDir, "dir1")
	dir2 := filepath.Join(tmpDir, "dir2")
	dir3 := filepath.Join(tmpDir, "dir3")

	if err := EnsureDirs(dir1, dir2, dir3); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	for _, dir := range []string{dir1, dir2, dir3} {
		if !Exists(dir) {
			t.Errorf("directory %s was not created", dir)
		}
	}
}

func TestGetMoleculeContainerName(t *testing.T) {
	tests := []struct {
		role     string
		expected string
	}{
		{"test-role", "molecule-test-role"},
		{"my_role", "molecule-my_role"},
		{"role123", "molecule-role123"},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			result := GetMoleculeContainerName(tt.role)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetRoleDirName(t *testing.T) {
	tests := []struct {
		org      string
		role     string
		expected string
	}{
		{"myorg", "myrole", "myorg.myrole"},
		{"test", "role", "test.role"},
		{"company", "ansible-role", "company.ansible-role"},
	}

	for _, tt := range tests {
		t.Run(tt.org+"-"+tt.role, func(t *testing.T) {
			result := GetRoleDirName(tt.org, tt.role)
			if result != tt.expected {
				t.Errorf("got %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestValidateRegistryProvider(t *testing.T) {
	validProviders := []string{"YC", "AWS", "GCP", "Public"}
	for _, provider := range validProviders {
		t.Run(provider, func(t *testing.T) {
			if err := ValidateRegistryProvider(provider); err != nil {
				t.Errorf("expected %s to be valid, got error: %v", provider, err)
			}
		})
	}

	invalidProviders := []string{"invalid", "azure", "docker", ""}
	for _, provider := range invalidProviders {
		t.Run(provider, func(t *testing.T) {
			if err := ValidateRegistryProvider(provider); err == nil {
				t.Errorf("expected %s to be invalid", provider)
			}
		})
	}
}

func TestValidateTestsType(t *testing.T) {
	validTypes := []string{"local", "remote", "diffusion"}
	for _, testsType := range validTypes {
		t.Run(testsType, func(t *testing.T) {
			if err := ValidateTestsType(testsType); err != nil {
				t.Errorf("expected %s to be valid, got error: %v", testsType, err)
			}
		})
	}

	invalidTypes := []string{"invalid", "custom", ""}
	for _, testsType := range invalidTypes {
		t.Run(testsType, func(t *testing.T) {
			if err := ValidateTestsType(testsType); err == nil {
				t.Errorf("expected %s to be invalid", testsType)
			}
		})
	}
}

func TestGetImageURL(t *testing.T) {
	registry := &config.ContainerRegistry{
		RegistryServer:        "cr.example.com",
		MoleculeContainerName: "molecule",
		MoleculeContainerTag:  "v1.0",
	}

	expected := "cr.example.com/molecule:v1.0"
	result := GetImageURL(registry)

	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestSetEnvVars(t *testing.T) {
	vars := map[string]string{
		"TEST_VAR1": "value1",
		"TEST_VAR2": "value2",
		"TEST_VAR3": "value3",
	}

	if err := SetEnvVars(vars); err != nil {
		t.Fatalf("SetEnvVars failed: %v", err)
	}

	for key, expectedValue := range vars {
		actualValue := os.Getenv(key)
		if actualValue != expectedValue {
			t.Errorf("env var %s: got %q, want %q", key, actualValue, expectedValue)
		}
	}

	// Cleanup
	for key := range vars {
		os.Unsetenv(key)
	}
}

func TestGetEnvOrDefault(t *testing.T) {
	testKey := "TEST_ENV_VAR_UNIQUE"
	defaultValue := "default"

	// Test with unset variable
	result := GetEnvOrDefault(testKey, defaultValue)
	if result != defaultValue {
		t.Errorf("got %q, want %q", result, defaultValue)
	}

	// Test with set variable
	expectedValue := "custom"
	os.Setenv(testKey, expectedValue)
	defer os.Unsetenv(testKey)

	result = GetEnvOrDefault(testKey, defaultValue)
	if result != expectedValue {
		t.Errorf("got %q, want %q", result, expectedValue)
	}
}

func TestRemoveFromSlice(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		element  string
		expected []string
		found    bool
	}{
		{
			name:     "remove existing element",
			slice:    []string{"a", "b", "c", "d"},
			element:  "b",
			expected: []string{"a", "c", "d"},
			found:    true,
		},
		{
			name:     "remove non-existing element",
			slice:    []string{"a", "b", "c"},
			element:  "d",
			expected: []string{"a", "b", "c"},
			found:    false,
		},
		{
			name:     "remove from empty slice",
			slice:    []string{},
			element:  "a",
			expected: []string{},
			found:    false,
		},
		{
			name:     "remove first element",
			slice:    []string{"a", "b", "c"},
			element:  "a",
			expected: []string{"b", "c"},
			found:    true,
		},
		{
			name:     "remove last element",
			slice:    []string{"a", "b", "c"},
			element:  "c",
			expected: []string{"a", "b"},
			found:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, found := RemoveFromSlice(tt.slice, tt.element)
			if found != tt.found {
				t.Errorf("found: got %v, want %v", found, tt.found)
			}
			if len(result) != len(tt.expected) {
				t.Errorf("length mismatch: got %d, want %d", len(result), len(tt.expected))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("element %d: got %q, want %q", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestContainsString(t *testing.T) {
	tests := []struct {
		name     string
		slice    []string
		element  string
		expected bool
	}{
		{"element exists", []string{"a", "b", "c"}, "b", true},
		{"element does not exist", []string{"a", "b", "c"}, "d", false},
		{"empty slice", []string{}, "a", false},
		{"first element", []string{"a", "b", "c"}, "a", true},
		{"last element", []string{"a", "b", "c"}, "c", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContainsString(tt.slice, tt.element)
			if result != tt.expected {
				t.Errorf("got %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestRemoveGitDir(t *testing.T) {
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")

	// Create .git directory
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a file inside .git
	testFile := filepath.Join(gitDir, "config")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	// Remove .git directory
	if err := RemoveGitDir(tmpDir); err != nil {
		t.Fatalf("RemoveGitDir failed: %v", err)
	}

	// Verify .git was removed
	if Exists(gitDir) {
		t.Error(".git directory still exists")
	}
}

func BenchmarkPathCacheExists(b *testing.B) {
	cache := NewPathCache()
	tmpFile, _ := os.CreateTemp("", "bench-*.txt")
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	b.Run("WithCache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			cache.Exists(tmpFile.Name())
		}
	})

	b.Run("WithoutCache", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Exists(tmpFile.Name())
		}
	})
}

func TestGetDefaultMoleculeTag(t *testing.T) {
	tag := GetDefaultMoleculeTag()

	// Should return either latest-amd64 or latest-arm64
	if tag != "latest-amd64" && tag != "latest-arm64" {
		t.Errorf("unexpected tag: got %q, want 'latest-amd64' or 'latest-arm64'", tag)
	}

	// Verify format
	if !strings.HasPrefix(tag, "latest-") {
		t.Errorf("tag should start with 'latest-', got %q", tag)
	}
}

func TestGetUserMappingArgs(t *testing.T) {
	args := GetUserMappingArgs()

	if runtime.GOOS == "windows" {
		// On Windows, should return empty slice
		if len(args) != 0 {
			t.Errorf("expected empty slice on Windows, got %v", args)
		}
	} else {
		// On Unix systems, should return user mapping
		if len(args) != 2 {
			t.Errorf("expected 2 arguments, got %d", len(args))
		}
		if len(args) == 2 {
			if args[0] != "--user" {
				t.Errorf("expected first arg to be '--user', got %q", args[0])
			}
			// Second arg should be in format "uid:gid"
			if !strings.Contains(args[1], ":") {
				t.Errorf("expected second arg to contain ':', got %q", args[1])
			}
		}
	}
}

func TestGetContainerHomePath(t *testing.T) {
	homePath := GetContainerHomePath()

	// Main molecule container always runs as root (for DinD), so always uses /root
	if homePath != "/root" {
		t.Errorf("expected '/root', got %q", homePath)
	}
}
