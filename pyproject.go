package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// PyProjectTOML represents the structure of pyproject.toml
type PyProjectTOML struct {
	Project ProjectSection `toml:"project"`
	ToolUV  *ToolUVSection `toml:"tool,omitempty"`
}

// ProjectSection represents the [project] section
type ProjectSection struct {
	Name           string   `toml:"name"`
	Version        string   `toml:"version"`
	Description    string   `toml:"description"`
	RequiresPython string   `toml:"requires-python"`
	Dependencies   []string `toml:"dependencies"`
}

// ToolUVSection represents the [tool.uv] section
type ToolUVSection struct {
	UV UVConfig `toml:"uv"`
}

// UVConfig represents UV-specific configuration
type UVConfig struct {
	Sources map[string]UVSource `toml:"sources,omitempty"`
}

// UVSource represents a UV source configuration
type UVSource struct {
	Git    string `toml:"git,omitempty"`
	Branch string `toml:"branch,omitempty"`
	Tag    string `toml:"tag,omitempty"`
	Rev    string `toml:"rev,omitempty"`
}

// GeneratePyProjectContent generates pyproject.toml content as a string
func GeneratePyProjectContent(collections []CollectionRequirement, toolVersions map[string]string, pythonVersion *PythonVersion) (string, error) {
	// Create project section
	project := ProjectSection{
		Name:           "diffusion-molecule-container",
		Version:        "1.0.0",
		Description:    "Docker-in-Docker container for Ansible Molecule testing",
		RequiresPython: formatPythonRequirement(pythonVersion),
		Dependencies:   make([]string, 0),
	}

	// Add tool dependencies
	var tools = []string{"ansible", "ansible-lint", "molecule", "yamllint"}
	for _, tool := range tools {
		if version, ok := toolVersions[tool]; ok {
			project.Dependencies = append(project.Dependencies, formatDependency(tool, version))
		}
	}

	// if version, ok := toolVersions["ansible"]; ok {
	// 	project.Dependencies = append(project.Dependencies, formatDependency("ansible", version))
	// }
	// if version, ok := toolVersions["ansible-lint"]; ok {
	// 	project.Dependencies = append(project.Dependencies, formatDependency("ansible-lint", version))
	// }
	// if version, ok := toolVersions["molecule"]; ok {
	// 	project.Dependencies = append(project.Dependencies, formatDependency("molecule", version))
	// }
	// if version, ok := toolVersions["yamllint"]; ok {
	// 	project.Dependencies = append(project.Dependencies, formatDependency("yamllint", version))
	// }

	// Add molecule-plugins
	project.Dependencies = append(project.Dependencies, "molecule-plugins[docker]>=23.5.0")

	// Add collection-specific dependencies
	for _, col := range collections {
		deps := getCollectionPythonDependencies(col.Name)
		project.Dependencies = append(project.Dependencies, deps...)
	}

	// Create pyproject.toml structure (no custom sources needed, all from PyPI)
	pyproject := PyProjectTOML{
		Project: project,
		ToolUV:  nil, // No custom sources needed
	}

	// Marshal to TOML
	data, err := toml.Marshal(pyproject)
	if err != nil {
		return "", fmt.Errorf("failed to marshal pyproject.toml: %w", err)
	}

	return string(data), nil
}

// GeneratePyProjectTOML generates pyproject.toml from dependencies
func GeneratePyProjectTOML(collections []CollectionRequirement, toolVersions map[string]string, pythonVersion *PythonVersion, outputPath string) error {
	content, err := GeneratePyProjectContent(collections, toolVersions, pythonVersion)
	if err != nil {
		return err
	}

	// Ensure output directory exists
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Write to file
	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write pyproject.toml: %w", err)
	}

	return nil
}

// formatPythonRequirement formats Python version requirement
func formatPythonRequirement(pythonVersion *PythonVersion) string {
	if pythonVersion == nil {
		return ">=3.10"
	}

	// Use the pinned Python version (major.minor format)
	pinnedVer := pythonVersion.Pinned

	// If no pinned version specified, use min
	if pinnedVer == "" {
		min := pythonVersion.Min
		if min == "" {
			min = "3.10"
		}
		return fmt.Sprintf(">=%s", min)
	}

	// Pinned version is already in major.minor format (e.g., "3.13")
	return fmt.Sprintf(">=%s", pinnedVer)
}

// formatDependency formats a dependency string
func formatDependency(name, version string) string {
	if version == "" || version == "latest" {
		return name
	}

	// If version already has operator, use as-is
	if strings.HasPrefix(version, ">=") || strings.HasPrefix(version, "<=") ||
		strings.HasPrefix(version, "==") || strings.HasPrefix(version, ">") ||
		strings.HasPrefix(version, "<") || strings.HasPrefix(version, "=") {
		return fmt.Sprintf("%s%s", name, version)
	}

	// Default to >= for version constraints
	return fmt.Sprintf("%s>=%s", name, version)
}

// getCollectionPythonDependencies returns Python dependencies for known collections
func getCollectionPythonDependencies(collectionName string) []string {
	// Map of known collections to their Python dependencies
	collectionDeps := map[string][]string{
		"community.postgresql": {"psycopg2-binary>=2.9.0"},
		"community.mysql":      {"PyMySQL>=1.0.0"},
		"community.mongodb":    {"pymongo>=4.0.0"},
		"community.docker":     {"docker>=6.0.0"},
		"kubernetes.core":      {"kubernetes>=25.0.0", "PyYAML>=6.0"},
		"amazon.aws":           {"boto3>=1.26.0", "botocore>=1.29.0"},
		"google.cloud":         {"google-auth>=2.16.0"},
		"azure.azcollection":   {"azure-cli-core>=2.45.0"},
	}

	if deps, ok := collectionDeps[collectionName]; ok {
		return deps
	}

	return []string{}
}

// UpdatePyProjectForContainer updates pyproject.toml in the container project
func UpdatePyProjectForContainer(containerProjectPath string) error {
	// Load dependency configuration
	depConfig, err := LoadDependencyConfig()
	if err != nil {
		return fmt.Errorf("failed to load dependency config: %w", err)
	}

	// Load meta and requirements
	meta, req, err := LoadRoleConfig("")
	if err != nil {
		return fmt.Errorf("failed to load role config: %w", err)
	}

	// Resolve dependencies
	resolver := NewDependencyResolver(meta, req, depConfig)
	collections, err := resolver.ResolveCollectionDependencies()
	if err != nil {
		return fmt.Errorf("failed to resolve collections: %w", err)
	}

	pythonVersion := resolver.ResolvePythonVersion()
	toolVersions := resolver.ResolveToolVersions()

	// Generate pyproject.toml
	pyprojectPath := filepath.Join(containerProjectPath, "pyproject.toml")
	if err := GeneratePyProjectTOML(collections, toolVersions, pythonVersion, pyprojectPath); err != nil {
		return fmt.Errorf("failed to generate pyproject.toml: %w", err)
	}

	return nil
}

// SyncPyProjectWithLock syncs pyproject.toml with lock file
func SyncPyProjectWithLock(lockFile *LockFile, containerProjectPath string) error {
	if lockFile == nil {
		return fmt.Errorf("lock file is nil")
	}

	// Convert lock file entries to collections and tool versions
	collections := make([]CollectionRequirement, 0)
	for _, entry := range lockFile.Collections {
		collections = append(collections, CollectionRequirement{
			Name:    entry.Name,
			Version: entry.Version,
		})
	}

	toolVersions := make(map[string]string)
	for _, entry := range lockFile.Tools {
		toolVersions[entry.Name] = entry.Version
	}

	// Generate pyproject.toml
	pyprojectPath := filepath.Join(containerProjectPath, "pyproject.toml")
	if err := GeneratePyProjectTOML(collections, toolVersions, lockFile.Python, pyprojectPath); err != nil {
		return fmt.Errorf("failed to generate pyproject.toml: %w", err)
	}

	return nil
}

// GeneratePyProjectFromCurrentConfig generates pyproject.toml content from current configuration
// This is used to pass to the container at runtime
func GeneratePyProjectFromCurrentConfig() (string, error) {
	// Load dependency configuration
	depConfig, err := LoadDependencyConfig()
	if err != nil {
		// Use defaults if config doesn't exist
		depConfig = &DependencyConfig{
			Python: &PythonVersion{
				Min:    DefaultMinPythonVersion,
				Max:    DefaultMaxPythonVersion,
				Pinned: PinnedPythonVersion,
			},
			Ansible:     DefaultAnsibleVersion,
			AnsibleLint: DefaultAnsibleLintVersion,
			Molecule:    DefaultMoleculeVersion,
			YamlLint:    DefaultYamlLintVersion,
		}
	}

	// Try to load meta and requirements
	meta, req, err := LoadRoleConfig("")
	if err != nil {
		// If no role config, use empty
		meta = &Meta{}
		req = &Requirement{}
	}

	// Resolve dependencies
	resolver := NewDependencyResolver(meta, req, depConfig)
	collections, err := resolver.ResolveCollectionDependencies()
	if err != nil {
		return "", fmt.Errorf("failed to resolve collections: %w", err)
	}

	pythonVersion := resolver.ResolvePythonVersion()
	toolVersions := resolver.ResolveToolVersions()

	// Generate pyproject.toml content
	return GeneratePyProjectContent(collections, toolVersions, pythonVersion)
}
