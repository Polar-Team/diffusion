package diffusion

import (
	"fmt"
	"github.com/BurntSushi/toml"

	"os"
	"path/filepath"
)

type YamlLintRules struct {
	Braces   any `toml:"braces"`
	Brackets any `toml:"brackets"`
	NewLines any `toml:"new-lines"`
}

type YamlLint struct {
	Extends string        `toml:"extends"`
	Ignore  []string      `toml:"ignore"`
	Rules   YamlLintRules `toml:"rules"`
}

type AnsibleLint struct {
	ExcludedPaths []string `toml:"exclude_paths"`
	WarnList      []string `toml:"warn_list"`
	SkipList      []string `toml:"skip_list"`
}

type Config struct {
	HashicorpVaultIntegration bool         `toml:"vault"`
	ArtifactUrl               string       `toml:"url"`
	YamlLintConfig            *YamlLint    `toml:"yaml_lint"`
	AnsibleLintConfig         *AnsibleLint `toml:"ansible_lint"`
}

// LoadConfig reads configuration from a TOML file in the project directory
func LoadConfig(profileName string) (*Config, error) {
	projectDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get project directory: %w", err)
	}
	configPath := filepath.Join(projectDir, "diffusion.toml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var configMap map[string]*Config
	if err := toml.Unmarshal(data, &configMap); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	config, ok := configMap[profileName]
	if !ok {
		return nil, fmt.Errorf("profile %s not found in config", profileName)
	}

	return config, nil
}

func SaveConfig(profileName string, config *Config) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get project directory: %w", err)
	}
	configPath := filepath.Join(projectDir, "diffusion.toml")

	var configMap map[string]*Config

	// Read existing config if present
	data, err := os.ReadFile(configPath)
	if err == nil {
		_ = toml.Unmarshal(data, &configMap)
	} else {
		configMap = make(map[string]*Config)
	}

	configMap[profileName] = config

	newData, err := toml.Marshal(configMap)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(configPath, newData, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
