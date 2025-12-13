package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type YamlLintRules struct {
	Braces             any  `toml:"braces"`
	Brackets           any  `toml:"brackets"`
	NewLines           any  `toml:"new-lines"`
	Comments           any  `toml:"comments"`
	CommentsIdentation bool `toml:"comments-indentation"`
	OctalValues        any  `toml:"octal-values"`
}

type YamlLintRulesExport struct {
	Braces             any  `yaml:"braces"`
	Brackets           any  `yaml:"brackets"`
	NewLines           any  `yaml:"new-lines"`
	Comments           any  `yaml:"comments"`
	CommentsIdentation bool `yaml:"comments-indentation"`
	OctalValues        any  `yaml:"octal-values"`
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

type Config struct {
	ContainerRegistry *ContainerRegistry `toml:"container_registry"`
	HashicorpVault    *HashicorpVault    `toml:"vault"`
	ArtifactSources   []ArtifactSource   `toml:"artifact_sources,omitempty"`
	YamlLintConfig    *YamlLint          `toml:"yaml_lint"`
	AnsibleLintConfig *AnsibleLint       `toml:"ansible_lint"`
	TestsConfig       *TestsSettings     `toml:"tests"`
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
