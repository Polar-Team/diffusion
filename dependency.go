package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// CollectionRequirement represents a collection with version constraints
type CollectionRequirement struct {
	Name    string `yaml:"name"`
	Version string `yaml:"version,omitempty"` // e.g., ">=1.0.0", "1.2.3", "latest"
}

// PythonVersion represents Python version requirements
type PythonVersion struct {
	Min        string   `yaml:"min"`                  // Minimum Python version (e.g., "3.11")
	Max        string   `yaml:"max"`                  // Maximum Python version (e.g., "3.13")
	Pinned     string   `yaml:"pinned"`               // Pinned Python version (e.g., "3.13")
	Additional []string `yaml:"additional,omitempty"` // Additional Python versions to install
}

// DependencyConfig represents the dependency configuration in diffusion.toml
type DependencyConfig struct {
	Python      *PythonVersion          `toml:"python"`
	Ansible     string                  `toml:"ansible"`      // e.g., ">=10.0.0"
	AnsibleLint string                  `toml:"ansible_lint"` // e.g., ">=24.0.0"
	Molecule    string                  `toml:"molecule"`     // e.g., ">=24.0.0"
	YamlLint    string                  `toml:"yamllint"`     // e.g., ">=1.35.0"
	Collections []CollectionRequirement `toml:"collections,omitempty"`
}

// DependencyResolver resolves dependencies from requirements and meta files
type DependencyResolver struct {
	meta        *Meta
	requirement *Requirement
	config      *DependencyConfig
}

// NewDependencyResolver creates a new dependency resolver
func NewDependencyResolver(meta *Meta, req *Requirement, config *DependencyConfig) *DependencyResolver {
	return &DependencyResolver{
		meta:        meta,
		requirement: req,
		config:      config,
	}
}

// ResolveCollectionDependencies resolves all collection dependencies
func (dr *DependencyResolver) ResolveCollectionDependencies() ([]CollectionRequirement, error) {
	collectionMap := make(map[string]string)

	// Add collections from meta/main.yml
	if dr.meta != nil && dr.meta.Collections != nil {
		for _, col := range dr.meta.Collections {
			// Collection is already structured with Name and Version
			version := col.Version
			if version == "" {
				version = "latest"
			}
			collectionMap[col.Name] = version
		}
	}

	// Add collections from requirements.yml
	if dr.requirement != nil && dr.requirement.Collections != nil {
		for _, col := range dr.requirement.Collections {
			// Collection is already structured with Name and Version
			version := col.Version
			// If version is not specified in requirements, check if it exists in meta
			if version == "" || version == "latest" {
				if existingVersion, exists := collectionMap[col.Name]; exists && existingVersion != "" && existingVersion != "latest" {
					version = existingVersion
				}
			}
			collectionMap[col.Name] = version
		}
	}

	// Add collections from diffusion.toml config
	if dr.config != nil && dr.config.Collections != nil {
		for _, col := range dr.config.Collections {
			if existingVersion, exists := collectionMap[col.Name]; !exists || existingVersion == "" || existingVersion == "latest" {
				collectionMap[col.Name] = col.Version
			}
		}
	}

	// Convert map to sorted slice
	var collections []CollectionRequirement
	for name, version := range collectionMap {
		if version == "" {
			version = "latest"
		}
		collections = append(collections, CollectionRequirement{
			Name:    name,
			Version: version,
		})
	}

	// Sort by name for consistency
	sort.Slice(collections, func(i, j int) bool {
		return collections[i].Name < collections[j].Name
	})

	return collections, nil
}

// parseCollectionString parses a collection string like "community.general>=7.4.0" or "community.docker"
func parseCollectionString(col string) (name, version string) {
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

// ResolvePythonVersion resolves Python version requirements
func (dr *DependencyResolver) ResolvePythonVersion() *PythonVersion {
	if dr.config != nil && dr.config.Python != nil {
		return dr.config.Python
	}

	// Return pinned Python version
	return &PythonVersion{
		Min:    DefaultMinPythonVersion,
		Max:    DefaultMaxPythonVersion,
		Pinned: PinnedPythonVersion,
	}
}

// ResolveToolVersions resolves versions for ansible, molecule, ansible-lint, yamllint
func (dr *DependencyResolver) ResolveToolVersions() map[string]string {
	versions := make(map[string]string)

	if dr.config != nil {
		if dr.config.Ansible != "" {
			versions["ansible"] = dr.config.Ansible
		}
		if dr.config.AnsibleLint != "" {
			versions["ansible-lint"] = dr.config.AnsibleLint
		}
		if dr.config.Molecule != "" {
			versions["molecule"] = dr.config.Molecule
		}
		if dr.config.YamlLint != "" {
			versions["yamllint"] = dr.config.YamlLint
		}
	}

	// Set defaults if not specified
	if versions["ansible"] == "" {
		versions["ansible"] = DefaultAnsibleVersion
	}
	if versions["ansible-lint"] == "" {
		versions["ansible-lint"] = DefaultAnsibleLintVersion
	}
	if versions["molecule"] == "" {
		versions["molecule"] = DefaultMoleculeVersion
	}
	if versions["yamllint"] == "" {
		versions["yamllint"] = DefaultYamlLintVersion
	}

	// Adjust versions for Python compatibility
	pythonVersion := dr.ResolvePythonVersion()
	if pythonVersion != nil && pythonVersion.Pinned != "" {
		adjusted, warnings := AdjustToolVersionsForPython(versions, pythonVersion.Pinned)

		// Print warnings
		for _, warning := range warnings {
			fmt.Printf("\033[33m%s\033[0m\n", warning)
		}

		return adjusted
	}

	return versions
}

// ComputeDependencyHash computes a hash of all dependencies for lock file
func ComputeDependencyHash(collections []CollectionRequirement, roles []RequirementRole, toolVersions map[string]string, pythonVersion *PythonVersion) string {
	h := sha256.New()

	// Sort and hash collections
	sort.Slice(collections, func(i, j int) bool {
		return collections[i].Name < collections[j].Name
	})
	for _, col := range collections {
		h.Write([]byte(fmt.Sprintf("collection:%s:%s\n", col.Name, col.Version)))
	}

	// Sort and hash roles
	sort.Slice(roles, func(i, j int) bool {
		return roles[i].Name < roles[j].Name
	})
	for _, role := range roles {
		h.Write([]byte(fmt.Sprintf("role:%s:%s:%s\n", role.Name, role.Version, role.Src)))
	}

	// Sort and hash tool versions
	var tools []string
	for tool := range toolVersions {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	for _, tool := range tools {
		h.Write([]byte(fmt.Sprintf("tool:%s:%s\n", tool, toolVersions[tool])))
	}

	// Hash Python version
	if pythonVersion != nil {
		h.Write([]byte(fmt.Sprintf("python:%s:%s:%s\n", pythonVersion.Min, pythonVersion.Max, pythonVersion.Pinned)))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// LoadDependencyConfig loads dependency configuration from diffusion.toml
func LoadDependencyConfig() (*DependencyConfig, error) {
	config, err := LoadConfig()
	if err != nil {
		return nil, err
	}

	if config.DependencyConfig == nil {
		// Return default configuration
		return &DependencyConfig{
			Python: &PythonVersion{
				Min:    ExtractMajorMinor(DefaultMinPythonVersion),
				Max:    ExtractMajorMinor(DefaultMaxPythonVersion),
				Pinned: PinnedPythonVersion,
			},
			Ansible:     DefaultAnsibleVersion,
			AnsibleLint: DefaultAnsibleLintVersion,
			Molecule:    DefaultMoleculeVersion,
			YamlLint:    DefaultYamlLintVersion,
		}, nil
	}

	// Validate and normalize Python versions
	if config.DependencyConfig.Python != nil {
		python := config.DependencyConfig.Python

		// Validate and normalize Pinned version
		if python.Pinned != "" {
			validated, err := ValidatePythonVersion(python.Pinned)
			if err != nil {
				return nil, fmt.Errorf("invalid pinned Python version: %w", err)
			}
			python.Pinned = validated
		} else {
			python.Pinned = PinnedPythonVersion
		}

		// Validate and normalize Min version
		if python.Min != "" {
			validated, err := ValidatePythonVersion(python.Min)
			if err != nil {
				return nil, fmt.Errorf("invalid min Python version: %w", err)
			}
			python.Min = validated
		} else {
			python.Min = DefaultMinPythonVersion
		}

		// Validate and normalize Max version
		if python.Max != "" {
			validated, err := ValidatePythonVersion(python.Max)
			if err != nil {
				return nil, fmt.Errorf("invalid max Python version: %w", err)
			}
			python.Max = validated
		} else {
			python.Max = DefaultMaxPythonVersion
		}

		// Clear Additional versions - not supported for container
		python.Additional = nil
	}

	return config.DependencyConfig, nil
}

// SaveDependencyConfig saves dependency configuration to diffusion.toml
func SaveDependencyConfig(depConfig *DependencyConfig) error {
	config, err := LoadConfig()
	if err != nil {
		// Create new config if it doesn't exist
		config = &Config{}
	}

	config.DependencyConfig = depConfig
	return SaveConfig(config)
}

// FetchCollectionMetadata fetches metadata for a collection (placeholder for future implementation)
func FetchCollectionMetadata(collectionName string) (map[string]interface{}, error) {
	// TODO: Implement fetching from Ansible Galaxy API
	// For now, return empty metadata
	return map[string]interface{}{
		"name":    collectionName,
		"version": "latest",
	}, nil
}

// ValidateCollectionVersion validates if a collection version exists
func ValidateCollectionVersion(collectionName, version string) error {
	// TODO: Implement validation against Ansible Galaxy API
	// For now, accept all versions
	if version == "" || version == "latest" {
		return nil
	}
	return nil
}
