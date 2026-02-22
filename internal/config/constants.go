package config

import (
	"fmt"
	"strings"
)

// Color constants for terminal output
const (
	ColorReset      = "\033[0m"
	ColorRed        = "\033[31m"
	ColorGreen      = "\033[32m"
	ColorYellow     = "\033[33m"
	ColorMagenta    = "\033[35m"
	ColorAquamarine = "\033[38;2;127;255;212m"
)

// Default values
const (
	DefaultScenario              = "default"
	DefaultMoleculeTag           = "latest"
	DefaultRoleScm               = "git"
	DefaultRoleVersion           = "main"
	DefaultMinAnsibleVersion     = "2.10"
	DefaultLicense               = "MIT"
	MoleculeContainerPrefix      = "molecule-"
	DiffusionTestsRepo           = "https://github.com/Polar-Team/diffusion-ansible-tests-role.git"
	DiffusionTestsTempPrefix     = "diffusion-tests-"
	BufferSize                   = 32 * 1024 // 32KB buffer for file I/O
	DefaultRegistryServer        = "ghcr.io"
	DefaultRegistryProvider      = "Public"
	DefaultMoleculeContainerName = "polar-team/diffusion-molecule-container"
	// Dependency management defaults
	PinnedPythonVersion       = "3.13"
	DefaultMinPythonVersion   = "3.11"     // Major.minor only
	DefaultMaxPythonVersion   = "3.13"     // Major.minor only
	DefaultAnsibleVersion     = ">=10.0.0" // Requires Python 3.10+
	DefaultAnsibleLintVersion = ">=24.0.0" // Requires Python 3.10+
	DefaultMoleculeVersion    = ">=24.0.0" // Requires Python 3.10+
	DefaultYamlLintVersion    = ">=1.35.0"
)

// File paths
const (
	ConfigFileName         = "diffusion.toml"
	MetaFilePath           = "meta/main.yml"
	RequirementsFileName   = "requirements.yml"
	YamlLintFileName       = ".yamllint"
	AnsibleLintFileName    = ".ansible-lint"
	GitIgnoreFileName      = ".gitignore"
	MoleculeDir            = "molecule"
	ScenariosDir           = "scenarios"
	TestsDir               = "tests"
	DiffusionTestsRoleName = "diffusion_tests"
	LockFileName           = "diffusion.lock"
	PyProjectFileName      = "pyproject.toml"
)

// Cache directory names and container paths
const (
	CacheRolesDir        = "roles"
	CacheCollectionsDir  = "collections"
	CacheUVDir           = "uv"
	CacheDockerDir       = "docker"
	ContainerUVCachePath = "/root/.cache/uv" // UV cache location inside the container
	DockerImageTarball   = "image.tar"       // Filename for cached Docker image tarball
)

// Registry providers
const (
	RegistryProviderYC     = "YC"
	RegistryProviderAWS    = "AWS"
	RegistryProviderGCP    = "GCP"
	RegistryProviderPublic = "Public"
)

// Test configuration types
const (
	TestsTypeLocal     = "local"
	TestsTypeRemote    = "remote"
	TestsTypeDiffusion = "diffusion"
)

// Docker commands
const (
	DockerCmdRun     = "run"
	DockerCmdExec    = "exec"
	DockerCmdLogin   = "login"
	DockerCmdInspect = "inspect"
	DockerCmdRm      = "rm"
)

// Molecule commands
const (
	MoleculeCreate      = "molecule create"
	MoleculeConverge    = "molecule converge"
	MoleculeVerify      = "molecule verify"
	MoleculeIdempotence = "molecule idempotence"
)

// Environment variables
const (
	EnvToken           = "TOKEN"
	EnvVaultToken      = "VAULT_TOKEN"
	EnvVaultAddr       = "VAULT_ADDR"
	EnvGitUserPrefix   = "GIT_USER_"     // Indexed: GIT_USER_1, GIT_USER_2, etc.
	EnvGitPassPrefix   = "GIT_PASSWORD_" // Indexed: GIT_PASSWORD_1, GIT_PASSWORD_2, etc.
	EnvGitURLPrefix    = "GIT_URL_"      // Indexed: GIT_URL_1, GIT_URL_2, etc.
	EnvYCCloudID       = "YC_CLOUD_ID"
	EnvYCFolderID      = "YC_FOLDER_ID"
	EnvGCPProjectID    = "GCP_PROJECT_ID"
	EnvAnsibleRunTags  = "ANSIBLE_RUN_TAGS"
	MaxArtifactSources = 10 // Maximum number of artifact sources supported
)

// GCP-specific constants
const (
	GcloudUnsetValue = "(unset)" // Value returned by gcloud when config is not set
)

// Error messages
const (
	ErrInvalidRegistryProvider = "invalid RegistryProvider. Allowed values are: YC, AWS, GCP, Public"
	ErrRoleNameEmpty           = "role name cannot be empty"
)

// Success messages
const (
	MsgRoleInitSuccess      = "Role initialized successfully."
	MsgRoleAddSuccess       = "Role '%s' added successfully to requirements.yml"
	MsgRoleRemoveSuccess    = "Role '%s' removed successfully from requirements.yml"
	MsgCollectionAddSuccess = "Collection '%s' added successfully"
	MsgLintDone             = "Lint Done Successfully!"
	MsgVerifyDone           = "Verify Done Successfully!"
	MsgIdempotenceDone      = "Idempotence Done Successfully!"
	MsgLockFileGenerated    = "Lock file generated successfully"
	MsgLockFileUpToDate     = "YAML manifests is up-to-date"
	MsgPyProjectGenerated   = "pyproject.toml generated successfully"
	MsgDependenciesResolved = "Dependencies resolved successfully"
)

// Warning messages
const (
	WarnLoadingConfig        = "warning loading config: %v"
	WarnSavingConfig         = "warning saving new config: %v"
	WarnLoadingRoleConfig    = "warning loading role config: %v"
	WarnRoleNameMissing      = "warning: role name or namespace missing in meta/main.yml"
	WarnCopyingData          = "warning copying data: %v"
	WarnCannotCreateTestsDir = "warning: cannot create tests dir: %v"
	WarnDockerLoginFailed    = "docker login to registry failed: %v"
	WarnDockerRunFailed      = "docker run failed: %v"
	WarnRoleInitFailed       = "role init warning: %v"
	WarnCleanRoleDirFailed   = "clean role dir warning: %v"
	WarnCopyRoleDataFailed   = "copy role data warning: %v"
)

// AllowedPythonVersions contains the only allowed Python versions (major.minor)
var AllowedPythonVersions = []string{"3.13", "3.12", "3.11"}

// ValidatePythonVersion validates a Python version (major.minor format only)
// Returns the version if valid, error if not allowed
func ValidatePythonVersion(version string) (string, error) {
	// Normalize to major.minor if patch version provided
	normalized := ExtractMajorMinor(version)

	// Check if it's an allowed version
	for _, allowed := range AllowedPythonVersions {
		if normalized == allowed {
			return normalized, nil
		}
	}

	// Not allowed
	return "", fmt.Errorf("Python version %s is not allowed. Allowed versions: 3.13, 3.12, 3.11", version)
}

// ExtractMajorMinor extracts major.minor from a version string
// e.g., "3.13.11" -> "3.13", "3.13" -> "3.13"
func ExtractMajorMinor(version string) string {
	// Remove any version operators first
	cleaned := version
	for _, op := range []string{">=", "<=", "==", ">", "<", "="} {
		cleaned = strings.TrimPrefix(cleaned, op)
	}
	cleaned = strings.TrimSpace(cleaned)

	parts := strings.Split(cleaned, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return cleaned
}
