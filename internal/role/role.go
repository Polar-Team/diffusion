package role

import (
	"bytes"
	"os"
	"strings"

	"diffusion/internal/utils"

	"gopkg.in/yaml.v3"
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

type RequirementRole struct {
	Name    string `yaml:"name"`
	Src     string `yaml:"src,omitempty"`
	Version string `yaml:"version,omitempty"`
	Scm     string `yaml:"scm,omitempty"`
}

type RequirementCollection struct {
	Name      string `yaml:"name"`
	Type      string `yaml:"type,omitempty"`
	Source    string `yaml:"source,omitempty"`
	SourceURL string `yaml:"source_url,omitempty"`
	Version   string `yaml:"version,omitempty"`
}

// UnmarshalYAML implements custom YAML unmarshaling to support both string and structured formats
func (rc *RequirementCollection) UnmarshalYAML(value *yaml.Node) error {
	// Try to unmarshal as a string first (backward compatibility)
	if value.Kind == yaml.ScalarNode {
		// It's a simple string like "community.general" or "community.general>=7.4.0"
		collectionStr := value.Value
		name, version := utils.ParseCollectionString(collectionStr)
		rc.Name = name
		rc.Version = version
		return nil
	}

	// Otherwise, unmarshal as a structured object
	type rawCollection RequirementCollection
	var raw rawCollection
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*rc = RequirementCollection(raw)
	return nil
}

type Meta struct {
	GalaxyInfo  *GalaxyInfo `yaml:"galaxy_info"`
	Collections []string    `yaml:"collections,omitempty"`
}

type Requirement struct {
	Collections []RequirementCollection `yaml:"collections"`
	Roles       []RequirementRole       `yaml:"roles"`
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
	data, err := marshalYaml4Indent(meta)
	if err != nil {
		return err
	}
	// Prepend YAML document header for correct formatting
	output := append([]byte("---\n"), data...)
	return os.WriteFile("meta/main.yml", output, 0644)
}

func SaveRequirementFile(req *Requirement, scenarios string) error {
	path := "requirements.yml"
	if scenarios != "" {
		path = "scenarios/" + scenarios + "/requirements.yml"
	}
	data, err := marshalYaml4Indent(req)
	if err != nil {
		return err
	}
	// Prepend YAML document header for correct formatting
	output := append([]byte("---\n"), data...)
	return os.WriteFile(path, output, 0644)
}

// marshalYaml4Indent encodes a value as YAML with consistent 4-space indentation.
// gopkg.in/yaml.v3's Encoder with SetIndent(4) produces only 2-space indent for
// sequence items nested inside a mapping within a sequence item. This function
// post-processes the output to ensure all nesting uses 4-space indent.
func marshalYaml4Indent(v interface{}) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(4)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return []byte(fixYamlIndent(buf.String())), nil
}

// fixYamlIndent fixes nested sequence items inside sequence-item mappings
// where the YAML library only indents 2 spaces instead of 4.
//
// Example input (broken):
//
//	versions:
//	  - focal       <- only 2 spaces deeper than "versions:"
//
// Example output (fixed):
//
//	versions:
//	    - focal     <- 4 spaces deeper than "versions:"
func fixYamlIndent(input string) string {
	lines := strings.Split(input, "\n")
	result := make([]string, len(lines))
	copy(result, lines)

	lastKeyIndent := -1   // indent level of the most recent "key:" line
	adjustingIndent := -1 // when >= 0, we're adjusting consecutive items at this indent

	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" {
			continue
		}
		indent := len(line) - len(trimmed)

		if strings.HasPrefix(trimmed, "- ") {
			// This is a sequence item
			if adjustingIndent >= 0 && indent == adjustingIndent {
				// Continuation of an adjusted sequence
				result[i] = "  " + line
			} else if lastKeyIndent >= 0 && indent == lastKeyIndent+2 {
				// First item: only 2 more spaces than parent key — should be 4 more
				result[i] = "  " + line
				adjustingIndent = indent
			} else {
				adjustingIndent = -1
			}
			lastKeyIndent = -1 // reset key tracking (but keep adjustingIndent)
		} else if strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "#") {
			// This is a mapping key
			lastKeyIndent = indent
			adjustingIndent = -1
		} else {
			lastKeyIndent = -1
			adjustingIndent = -1
		}
	}

	return strings.Join(result, "\n")
}
