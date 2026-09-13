package patch

import (
	"fmt"
	yaml "gopkg.in/yaml.v3"
	"os"
	"path/filepath"

	"diffusion/internal/config"
)

type PatchesConfig struct {
	Bundles []PatchBundle `yaml:"Bundles,omitempty"`
}

type PatchBundle struct {
	PatchBundleName string
	TaskToPatch     []PatchingTask
	Scenario        string
}

type PatchingTask struct {
	TaskId         string             `yaml:"task_id"`
	NewModule      string             `yaml:"new_module,omitempty"`
	NewModuleSetup []PatchModuleSetup `yaml:"new_module_setup,omitempty"`
	NewConditions  []PatchConditions  `yaml:"new_conditions,omitempty"`
	NewBecome      *PatchBecomeSetup  `yaml:"new_become_user,omitempty"`
	NewEnvironment []PatchEnvironment `yaml:"new_environment,omitempty"`
	NewBlock       []PatchingTask     `yaml:"new_block,omitempty"`
}

type PatchConditions struct {
	Condition string `yaml:"condition,omitempty"`
	Body      string `yaml:"body,omitempty"`
}

type PatchModuleSetup struct {
	Key   any `yaml:"key"`
	Value any `yaml:"value"`
}

type PatchBecomeSetup struct {
	User   string `yaml:"user,omitempty"`
	Become bool   `yaml:"become,omitempty"`
}

type PatchEnvironment struct {
	Key   any `yaml:"key"`
	Value any `yaml:"value"`
}

func (p *PatchesConfig) LoadPatchConfigFrom(configPath string) (*PatchesConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf(config.ColorRed+"failed to read patch config file: %v"+config.ColorReset, err)
	}

	var configMap *PatchesConfig
	if err := yaml.Unmarshal(data, &configMap); err != nil {
		return nil, fmt.Errorf(config.ColorRed+"failed to unmarshal patch config: %v"+config.ColorReset, err)
	}
	return configMap, nil
}

func (p *PatchesConfig) SavePatchConfigTo(scenarioName string) error {
	projectDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get project directory: %v", err)
	}
	configPath := filepath.Join(projectDir, "scenarios", scenarioName, "patch.yml")

	newData, err := yaml.Marshal(p)
	if err != nil {
		return fmt.Errorf(config.ColorRed+"failed to marshal patch config: %v"+config.ColorReset, err)
	}

	if err := os.WriteFile(configPath, newData, 0644); err != nil {
		return fmt.Errorf(config.ColorRed+"failed to write patch config file: %v"+config.ColorReset, err)
	}

	return nil

}
