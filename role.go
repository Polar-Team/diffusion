package main

import (
	// "fmt"
	"gopkg.in/yaml.v3"
	"os"
)

type GalaxyInfo struct {
	RoleName          string              `yaml:"role_name"`
	Company           string              `yaml:"company"`
	Namespace         string              `yaml:"namespace"`
	Author            string              `yaml:"author"`
	Description       string              `yaml:"description"`
	License           string              `yaml:"license"`
	MinAnsibleVersion string              `yaml:"min_ansible_version"`
	Platforms         []map[string]string `yaml:"platforms"`
	GalaxyTags        []string            `yaml:"galaxy_tags"`
}

type Meta struct {
	GalaxyInfo  *GalaxyInfo `yaml:"galaxy_info"`
	Collections []string    `yaml:"collections"`
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

//func InitNewRole(roleName string) error {

//}
