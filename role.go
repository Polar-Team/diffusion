package main

import (
	// "fmt"
	"gopkg.in/yaml.v3"
	"os"
)

type Platform struct {
	OsName   string   `yaml:"name"`
	Versions []string `yaml:"versions"`
}

type GalaxyInfo struct {
	RoleName          string     `yaml:"role_name"`
	Company           string     `yaml:"company"`
	Namespace         string     `yaml:"namespace"`
	Author            string     `yaml:"author"`
	Description       string     `yaml:"description"`
	License           string     `yaml:"license"`
	MinAnsibleVersion string     `yaml:"min_ansible_version"`
	Platforms         []Platform `yaml:"platforms"`
	GalaxyTags        []string   `yaml:"galaxy_tags"`
}

type Meta struct {
	GalaxyInfo  *GalaxyInfo `yaml:"galaxy_info"`
	Collections []string    `yaml:"collections,omitempty"`
}

type RequirementRole struct {
	Name    string `yaml:"name"`
	Src     string `yaml:"src,omitempty"`
	Version string `yaml:"version,omitempty"`
	Scm     string `yaml:"scm,omitempty"`
}

type Requirement struct {
	Collections []string          `yaml:"collections"`
	Roles       []RequirementRole `yaml:"roles"`
}

func ParseMetaFile() (*Meta, error) {
	file, err := os.ReadFile("meta/main.yml")
	if err != nil {
		return nil, err
	}
	var meta Meta
	err = yaml.Unmarshal(file, &meta)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

func ParseRequirementFile(scenarios string) (*Requirement, error) {
	path := "requirements.yml"
	if scenarios != "" {
		path = "scenarios/" + scenarios + "/requirements.yml"
	}
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var req Requirement
	err = yaml.Unmarshal(file, &req)
	if err != nil {
		return nil, err
	}
	return &req, nil
}

func LoadRoleConfig(scenarios string) (*Meta, *Requirement, error) {
	if scenarios == "" {
		scenarios = "default"
	}
	meta, err := ParseMetaFile()
	if err != nil {
		return nil, nil, err
	}
	req, err := ParseRequirementFile(scenarios)
	if err != nil {
		return nil, nil, err
	}
	return meta, req, nil
}

func SaveMetaFile(meta *Meta) error {
	data, err := yaml.Marshal(meta)
	if err != nil {
		return err
	}
	err = os.WriteFile("meta/main.yml", data, 0644)
	if err != nil {
		return err
	}
	return nil
}

func SaveRequirementFile(req *Requirement, scenarios string) error {
	path := "requirements.yml"
	if scenarios != "" {
		path = "scenarios/" + scenarios + "/requirements.yml"
	}
	data, err := yaml.Marshal(req)
	if err != nil {
		return err
	}
	err = os.WriteFile(path, data, 0644)
	if err != nil {
		return err
	}
	return nil
}
