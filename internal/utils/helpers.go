package utils

import (
	"context"
	"encoding/base64"
	"fmt"
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"diffusion/internal/config"
)

// exists checks if a file or directory exists
// Exists checks if a file or directory exists
func Exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

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
	exists := Exists(path)
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
	fmt.Printf("%s%s%s\n", color, fmt.Sprintf(format, args...), config.ColorReset)
}

// ColorLog logs with color
func ColorLog(color, message string) {
	log.Printf("%s%s%s", color, message, config.ColorReset)
}

// EnsureDir creates a directory if it doesn't exist
func EnsureDir(path string) error {
	if Exists(path) {
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
	return fmt.Sprintf("%s%s", config.MoleculeContainerPrefix, role)
}

// GetRoleDirName returns the directory name for a role
func GetRoleDirName(org, role string) string {
	return fmt.Sprintf("%s.%s", org, role)
}

// GetRoleMoleculePath returns the molecule path for a role
func GetRoleMoleculePath(basePath, org, role string) string {
	return filepath.Join(basePath, config.MoleculeDir, GetRoleDirName(org, role))
}

// ValidateRegistryProvider validates the registry provider value
func ValidateRegistryProvider(provider string) error {
	switch provider {
	case config.RegistryProviderYC, config.RegistryProviderAWS, config.RegistryProviderGCP, config.RegistryProviderPublic:
		return nil
	default:
		return fmt.Errorf("%s", config.ErrInvalidRegistryProvider)
	}
}

// ValidateTestsType validates the tests configuration type
func ValidateTestsType(testsType string) error {
	switch testsType {
	case config.TestsTypeLocal, config.TestsTypeRemote, config.TestsTypeDiffusion:
		return nil
	default:
		return fmt.Errorf("invalid tests type: %s. Allowed values are: local, remote, diffusion", testsType)
	}
}

// GetImageURL constructs the full image URL
func GetImageURL(registry *config.ContainerRegistry) string {
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
		log.Printf("%swarning removing temp dir %s: %v%s", config.ColorYellow, dir, err, config.ColorReset)
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

// ParseCollectionString parses a collection string like "community.general>=7.4.0" or "community.docker"
func ParseCollectionString(col string) (name, version string) {
	// Check for version operators
	for _, op := range []string{">=", "<=", "==", ">", "<", "="} {
		if idx := strings.Index(col, op); idx != -1 {
			name = strings.TrimSpace(col[:idx])
			version = strings.TrimSpace(col[idx:])
			return
		}
	}
	// No version specified
	name = strings.TrimSpace(col)
	version = ""
	return
}

// RunCommandCapture executes a command with context and returns its output
func RunCommandCapture(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// runCommandHide runs command and discards stdout/stderr with a loading animation
func RunCommandHide(name string, args ...string) error {
	spinner := NewSpinner(fmt.Sprintf("Running %s", name))
	spinner.Start()
	defer spinner.Stop()

	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func ExportLinters(cfg *config.Config, roleMoleculePath string, CIMode bool, roleFlag string, orgFlag string) error {

	yamlrules := config.YamlLintRulesExport{
		Braces:             cfg.YamlLintConfig.Rules.Braces,
		Brackets:           cfg.YamlLintConfig.Rules.Brackets,
		NewLines:           cfg.YamlLintConfig.Rules.NewLines,
		Comments:           cfg.YamlLintConfig.Rules.Comments,
		CommentsIdentation: cfg.YamlLintConfig.Rules.CommentsIdentation,
		OctalValues:        cfg.YamlLintConfig.Rules.OctalValues,
	}

	exportYamlLint := config.YamlLintExport{
		Extends: cfg.YamlLintConfig.Extends,
		Ignore:  strings.Join(cfg.YamlLintConfig.Ignore, "\n"),
		Rules:   &yamlrules,
	}
	yamllint, err := yaml.Marshal(exportYamlLint)
	if err != nil {
		log.Printf("\033[33mwarning marshaling yamllint config: %v\033[0m", err)
	} else {
		if !CIMode {
			yamllintPath := filepath.Join(roleMoleculePath, ".yamllint")
			if err := os.WriteFile(yamllintPath, yamllint, 0o644); err != nil {
				log.Printf("\033[33mwarning writing .yamllint: %v\033[0m", err)
			}
		} else {
			// In CI mode, write to container using cat with heredoc
			roleDirName := fmt.Sprintf("%s.%s", orgFlag, roleFlag)
			containerPath := fmt.Sprintf("/opt/molecule/%s/.yamllint", roleDirName)
			// Use base64 encoding to safely transfer content
			yamllintB64 := base64.StdEncoding.EncodeToString(yamllint)
			cmdCreateFile := fmt.Sprintf("echo '%s' | base64 -d > %s", yamllintB64, containerPath)
			if err := DockerExecInteractiveHide(roleFlag, "/bin/sh", CIMode, "-c", cmdCreateFile); err != nil {
				log.Printf("\033[33mwarning writing .yamllint in CI mode: %v\033[0m", err)
			}
		}
	}

	exportAnsibleLint := config.AnsibleLintExport{
		ExcludedPaths: cfg.AnsibleLintConfig.ExcludedPaths,
		WarnList:      cfg.AnsibleLintConfig.WarnList,
		SkipList:      cfg.AnsibleLintConfig.SkipList,
	}

	ansiblelint, err := yaml.Marshal(exportAnsibleLint)
	if err != nil {
		log.Printf("\033[33mwarning marshaling ansible-lint config: %v\033[0m", err)
	} else {
		if !CIMode {
			ansiblelintPath := filepath.Join(roleMoleculePath, ".ansible-lint")
			if err := os.WriteFile(ansiblelintPath, ansiblelint, 0o644); err != nil {
				log.Printf("\033[33mwarning writing .ansible-lint: %v\033[0m", err)
			}
		} else {
			// In CI mode, write to container using cat with heredoc
			roleDirName := fmt.Sprintf("%s.%s", orgFlag, roleFlag)
			containerPath := fmt.Sprintf("/opt/molecule/%s/.ansible-lint", roleDirName)
			// Use base64 encoding to safely transfer content
			ansiblelintB64 := base64.StdEncoding.EncodeToString(ansiblelint)
			cmdCreateFile := fmt.Sprintf("echo '%s' | base64 -d > %s", ansiblelintB64, containerPath)
			if err := DockerExecInteractiveHide(roleFlag, "/bin/sh", CIMode, "-c", cmdCreateFile); err != nil {
				log.Printf("\033[33mwarning writing .ansible-lint in CI mode: %v\033[0m", err)
			}
		}
	}

	return nil
}

// dockerExecInteractive runs: docker exec -ti molecule-role <cmd...>
// In CI mode, removes -ti flags to avoid TTY errors
func DockerExecInteractive(role, command string, ciMode bool, args ...string) error {
	execFlags := []string{"exec"}
	if !ciMode {
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
func DockerExecInteractiveHide(role, command string, ciMode bool, args ...string) error {
	if !ciMode {
		spinner := NewSpinner(fmt.Sprintf("Running %s in container", command))
		spinner.Start()
		defer spinner.Stop()
	}

	execFlags := []string{"exec"}
	if !ciMode {
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

// fixContainerPermissions fixes ownership of files inside container (Unix systems only)
func FixContainerPermissions(role string, path string, ciMode bool) error {
	if runtime.GOOS == "windows" {
		return nil // Skip on Windows
	}

	uid := os.Getuid()
	gid := os.Getgid()
	chownCmd := fmt.Sprintf("chown -R %d:%d %s", uid, gid, path)
	return DockerExecInteractiveHide(role, "/bin/sh", ciMode, "-c", chownCmd)
}
