package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// CollectionRequirement represents a collection with version constraints
type CollectionRequirement struct {
	Name      string `toml:"Name" yaml:"name"`
	Namespace string `toml:"Namespace,omitempty" yaml:"namespace,omitempty"`  // Galaxy namespace (e.g., "community" for community.general)
	Version   string `toml:"Version,omitempty" yaml:"version,omitempty"`      // e.g., ">=1.0.0", "1.2.3", "latest"
	Source    string `toml:"Source,omitempty" yaml:"source,omitempty"`        // Optional type of source (e.g., "galaxy", "git") default is galaxy
	SourceURL string `toml:"SourceURL,omitempty" yaml:"source_url,omitempty"` // Optional URL of the source (e.g., git repo URL)
}

// PythonVersion represents Python version requirements
type PythonVersion struct {
	Min        string   `toml:"Min" yaml:"min"`                                   // Minimum Python version (e.g., "3.11")
	Max        string   `toml:"Max" yaml:"max"`                                   // Maximum Python version (e.g., "3.13")
	Pinned     string   `toml:"Pinned,omitempty" yaml:"pinned,omitempty"`         // Pinned Python version (e.g., "3.13")
	Additional []string `toml:"Additional,omitempty" yaml:"additional,omitempty"` // Additional Python versions to install
}

// DependencyConfig represents the dependency configuration in diffusion.toml
type DependencyConfig struct {
	Python      *PythonVersion          `toml:"python"`
	Ansible     string                  `toml:"ansible,omitempty"`      // e.g., ">=10.0.0"
	AnsibleLint string                  `toml:"ansible_lint,omitempty"` // e.g., ">=24.0.0"
	Molecule    string                  `toml:"molecule,omitempty"`     // e.g., ">=24.0.0"
	YamlLint    string                  `toml:"yamllint,omitempty"`     // e.g., ">=1.35.0"
	Collections []CollectionRequirement `toml:"collections,omitempty"`
	Roles       []RoleRequirement       `toml:"roles,omitempty"` // Roles per scenario: scenario.role_name
}

// RoleRequirement represents a role with version constraints
type RoleRequirement struct {
	Name      string `toml:"Name" yaml:"name"`
	Namespace string `toml:"Namespace,omitempty" yaml:"namespace,omitempty"` // Galaxy namespace (e.g., "geerlingguy" for geerlingguy.docker)
	Src       string `toml:"Src,omitempty" yaml:"src,omitempty"`
	Scm       string `toml:"Scm,omitempty" yaml:"scm,omitempty"`
	Version   string `toml:"Version,omitempty" yaml:"version,omitempty"` // e.g., ">=1.0.0", "1.2.3", "main"
}

// HashicorpVault configuration
type HashicorpVault struct {
	HashicorpVaultIntegration bool   `toml:"enabled"`
	SecretKV2Path             string `toml:"secret_kv2_path,omitempty"` // Deprecated: use per-source vault configuration
	SecretKV2Name             string `toml:"secret_kv2_name,omitempty"` // Deprecated: use per-source vault configuration
}

// ArtifactSource represents a private artifact source configuration
type ArtifactSource struct {
	Name               string `toml:"name"`
	URL                string `toml:"url"`
	Type               string `toml:"type"` // "galaxy" or "git"
	VaultPath          string `toml:"vault_path,omitempty"`
	VaultSecretName    string `toml:"vault_secret_name,omitempty"`
	VaultUsernameField string `toml:"vault_username_field,omitempty"`
	VaultTokenField    string `toml:"vault_token_field,omitempty"`
	UseVault           bool   `toml:"use_vault"`
}

// ArtifactCredentials stores credentials for a private artifact repository
type ArtifactCredentials struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Username string `json:"username"`
	Password string `json:"password"` // Encrypted
	Token    string `json:"token"`    // Alternative to password
}

type YamlLintRules struct {
	Anchors             any `toml:"anchors,omitempty"`
	Braces              any `toml:"braces,omitempty"`
	Brackets            any `toml:"brackets,omitempty"`
	Colons              any `toml:"colons,omitempty"`
	Commas              any `toml:"commas,omitempty"`
	Comments            any `toml:"comments,omitempty"`
	CommentsIndentation any `toml:"comments-indentation,omitempty"`
	DocumentEnd         any `toml:"document-end,omitempty"`
	DocumentStart       any `toml:"document-start,omitempty"`
	EmptyLines          any `toml:"empty-lines,omitempty"`
	EmptyValues         any `toml:"empty-values,omitempty"`
	FloatValues         any `toml:"float-values,omitempty"`
	Hyphens             any `toml:"hyphens,omitempty"`
	Indentation         any `toml:"indentation,omitempty"`
	KeyDuplicates       any `toml:"key-duplicates,omitempty"`
	KeyOrdering         any `toml:"key-ordering,omitempty"`
	LineLength          any `toml:"line-length,omitempty"`
	NewLineAtEndOfFile  any `toml:"new-line-at-end-of-file,omitempty"`
	NewLines            any `toml:"new-lines,omitempty"`
	OctalValues         any `toml:"octal-values,omitempty"`
	QuotedStrings       any `toml:"quoted-strings,omitempty"`
	TrailingSpaces      any `toml:"trailing-spaces,omitempty"`
	Truthy              any `toml:"truthy,omitempty"`
}

type YamlLintRulesExport struct {
	Anchors             any `yaml:"anchors,omitempty"`
	Braces              any `yaml:"braces,omitempty"`
	Brackets            any `yaml:"brackets,omitempty"`
	Colons              any `yaml:"colons,omitempty"`
	Commas              any `yaml:"commas,omitempty"`
	Comments            any `yaml:"comments,omitempty"`
	CommentsIndentation any `yaml:"comments-indentation,omitempty"`
	DocumentEnd         any `yaml:"document-end,omitempty"`
	DocumentStart       any `yaml:"document-start,omitempty"`
	EmptyLines          any `yaml:"empty-lines,omitempty"`
	EmptyValues         any `yaml:"empty-values,omitempty"`
	FloatValues         any `yaml:"float-values,omitempty"`
	Hyphens             any `yaml:"hyphens,omitempty"`
	Indentation         any `yaml:"indentation,omitempty"`
	KeyDuplicates       any `yaml:"key-duplicates,omitempty"`
	KeyOrdering         any `yaml:"key-ordering,omitempty"`
	LineLength          any `yaml:"line-length,omitempty"`
	NewLineAtEndOfFile  any `yaml:"new-line-at-end-of-file,omitempty"`
	NewLines            any `yaml:"new-lines,omitempty"`
	OctalValues         any `yaml:"octal-values,omitempty"`
	QuotedStrings       any `yaml:"quoted-strings,omitempty"`
	TrailingSpaces      any `yaml:"trailing-spaces,omitempty"`
	Truthy              any `yaml:"truthy,omitempty"`
}

type YamlLint struct {
	Extends string         `toml:"extends"`
	Ignore  []string       `toml:"ignore"`
	Rules   *YamlLintRules `toml:"rules"`
}

type YamlLintExport struct {
	Extends string               `yaml:"extends"`
	Ignore  string               `yaml:"ignore"`
	Rules   *YamlLintRulesExport `yaml:"rules"`
}

type AnsibleLintExport struct {
	ExcludedPaths []string `yaml:"exclude_paths"`
	WarnList      []string `yaml:"warn_list"`
	SkipList      []string `yaml:"skip_list"`
}

type AnsibleLint struct {
	ExcludedPaths []string `toml:"exclude_paths"`
	WarnList      []string `toml:"warn_list"`
	SkipList      []string `toml:"skip_list"`
}

type ContainerRegistry struct {
	RegistryServer        string `toml:"registry_server"`
	RegistryProvider      string `toml:"registry_provider"`
	MoleculeContainerName string `toml:"molecule_container_name"`
	MoleculeContainerTag  string `toml:"molecule_container_tag"`
}

type TestsSettings struct {
	Type               string   `toml:"type"`
	RemoteRepositories []string `toml:"remote_repositories,omitempty"`
}

type CacheSettings struct {
	Enabled     bool   `toml:"enabled"`
	CacheID     string `toml:"cache_id,omitempty"`     // Unique identifier for this role's cache
	CachePath   string `toml:"cache_path,omitempty"`   // Custom cache path (optional)
	DockerCache bool   `toml:"docker_cache,omitempty"` // Cache Docker images as tarballs
	UVCache     bool   `toml:"uv_cache,omitempty"`     // Cache UV/Python packages
}

type Config struct {
	ContainerRegistry *ContainerRegistry `toml:"container_registry"`
	HashicorpVault    *HashicorpVault    `toml:"vault"`
	ArtifactSources   []ArtifactSource   `toml:"artifact_sources,omitempty"`
	YamlLintConfig    *YamlLint          `toml:"yaml_lint"`
	AnsibleLintConfig *AnsibleLint       `toml:"ansible_lint"`
	TestsConfig       *TestsSettings     `toml:"tests"`
	CacheConfig       *CacheSettings     `toml:"cache,omitempty"`
	DependencyConfig  *DependencyConfig  `toml:"dependencies,omitempty"`
}

// LoadConfig reads configuration from a TOML file in the project directory
func LoadConfig() (*Config, error) {
	projectDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get project directory: %w", err)
	}
	configPath := filepath.Join(projectDir, "diffusion.toml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var configMap *Config
	if err := toml.Unmarshal(data, &configMap); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return configMap, nil
}

func SaveConfig(config *Config) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get project directory: %w", err)
	}
	configPath := filepath.Join(projectDir, "diffusion.toml")

	var configMap *Config

	// Read existing config if present
	data, err := os.ReadFile(configPath)
	if err == nil {
		_ = toml.Unmarshal(data, &configMap)
	} else {
		configMap = nil
	}

	configMap = config

	newData, err := toml.Marshal(configMap)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, newData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
