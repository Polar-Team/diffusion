package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// PathCache caches file existence checks to avoid repeated os.Stat calls
type PathCache struct {
	cache map[string]bool
}

// NewPathCache creates a new path cache
func NewPathCache() *PathCache {
	return &PathCache{
		cache: make(map[string]bool),
	}
}

// Exists checks if a path exists, using cache when available
func (pc *PathCache) Exists(path string) bool {
	if exists, ok := pc.cache[path]; ok {
		return exists
	}
	exists := exists(path)
	pc.cache[path] = exists
	return exists
}

// Invalidate removes a path from the cache
func (pc *PathCache) Invalidate(path string) {
	delete(pc.cache, path)
}

// Clear clears the entire cache
func (pc *PathCache) Clear() {
	pc.cache = make(map[string]bool)
}

// ColorPrintf prints formatted colored output
func ColorPrintf(color, format string, args ...interface{}) {
	fmt.Printf("%s%s%s\n", color, fmt.Sprintf(format, args...), ColorReset)
}

// ColorLog logs with color
func ColorLog(color, message string) {
	log.Printf("%s%s%s", color, message, ColorReset)
}

// EnsureDir creates a directory if it doesn't exist
func EnsureDir(path string) error {
	if exists(path) {
		return nil
	}
	return os.MkdirAll(path, 0755)
}

// EnsureDirs creates multiple directories
func EnsureDirs(paths ...string) error {
	for _, path := range paths {
		if err := EnsureDir(path); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", path, err)
		}
	}
	return nil
}

// GetMoleculeContainerName returns the container name for a role
func GetMoleculeContainerName(role string) string {
	return fmt.Sprintf("%s%s", MoleculeContainerPrefix, role)
}

// GetRoleDirName returns the directory name for a role
func GetRoleDirName(org, role string) string {
	return fmt.Sprintf("%s.%s", org, role)
}

// GetRoleMoleculePath returns the molecule path for a role
func GetRoleMoleculePath(basePath, org, role string) string {
	return filepath.Join(basePath, MoleculeDir, GetRoleDirName(org, role))
}

// ValidateRegistryProvider validates the registry provider value
func ValidateRegistryProvider(provider string) error {
	switch provider {
	case RegistryProviderYC, RegistryProviderAWS, RegistryProviderGCP, RegistryProviderPublic:
		return nil
	default:
		return fmt.Errorf("%s", ErrInvalidRegistryProvider)
	}
}

// ValidateTestsType validates the tests configuration type
func ValidateTestsType(testsType string) error {
	switch testsType {
	case TestsTypeLocal, TestsTypeRemote, TestsTypeDiffusion:
		return nil
	default:
		return fmt.Errorf("invalid tests type: %s. Allowed values are: local, remote, diffusion", testsType)
	}
}

// GetImageURL constructs the full image URL
func GetImageURL(registry *ContainerRegistry) string {
	return fmt.Sprintf("%s/%s:%s",
		registry.RegistryServer,
		registry.MoleculeContainerName,
		registry.MoleculeContainerTag)
}

// SetEnvVars sets multiple environment variables
func SetEnvVars(vars map[string]string) error {
	for key, value := range vars {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to set %s: %w", key, err)
		}
	}
	return nil
}

// GetEnvOrDefault gets an environment variable or returns a default value
func GetEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// RemoveFromSlice removes an element from a string slice
func RemoveFromSlice(slice []string, element string) ([]string, bool) {
	for i, item := range slice {
		if item == element {
			return append(slice[:i], slice[i+1:]...), true
		}
	}
	return slice, false
}

// ContainsString checks if a string slice contains an element
func ContainsString(slice []string, element string) bool {
	for _, item := range slice {
		if item == element {
			return true
		}
	}
	return false
}

// CleanupTempDir removes a temporary directory and logs any errors
func CleanupTempDir(dir string) {
	if err := os.RemoveAll(dir); err != nil {
		log.Printf("%swarning removing temp dir %s: %v%s", ColorYellow, dir, err, ColorReset)
	}
}

// RemoveGitDir removes the .git directory from a path
func RemoveGitDir(path string) error {
	gitDir := filepath.Join(path, ".git")
	if err := os.RemoveAll(gitDir); err != nil {
		return fmt.Errorf("failed to remove .git folder: %w", err)
	}
	return nil
}

// GetDefaultMoleculeTag returns the default molecule container tag based on architecture
func GetDefaultMoleculeTag() string {
	arch := runtime.GOARCH

	switch arch {
	case "amd64", "arm64":
		return fmt.Sprintf("latest-%s", arch)
	default:
		// Default to amd64 for unknown architectures
		return "latest-amd64"
	}
}

// maskToken masks a token for display, showing only first and last 4 characters
func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}

// GetUserMappingArgs returns docker user mapping arguments for Unix systems
// On Unix systems, maps the current user's UID:GID to avoid permission issues
// On Windows, returns empty slice to use default root user
func GetUserMappingArgs() []string {
	if runtime.GOOS == "windows" {
		return []string{}
	}

	// On Unix systems, get current user's UID and GID
	uid := os.Getuid()
	gid := os.Getgid()

	// Return user mapping argument
	return []string{"--user", fmt.Sprintf("%d:%d", uid, gid)}
}

// GetContainerHomePath returns the home directory path inside the container
// The main molecule container always runs as root (required for Docker-in-Docker)
// so it always uses /root as the home directory
func GetContainerHomePath() string {
	return "/root"
}
