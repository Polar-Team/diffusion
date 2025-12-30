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
	Name      string `toml:"Name" yaml:"name"`
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
	Name    string `toml:"Name" yaml:"name"`
	Src     string `toml:"Src,omitempty" yaml:"src,omitempty"`
	Scm     string `toml:"Scm,omitempty" yaml:"scm,omitempty"`
	Version string `toml:"Version,omitempty" yaml:"version,omitempty"` // e.g., ">=1.0.0", "1.2.3", "main"
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
// diffusion.toml provides version constraints, meta/requirements provide collection names
func (dr *DependencyResolver) ResolveCollectionDependencies() ([]CollectionRequirement, error) {
	collectionMap := make(map[string]CollectionRequirement)

	// Add collections from meta/main.yml (simple string format)
	if dr.meta != nil && dr.meta.Collections != nil {
		for _, col := range dr.meta.Collections {
			// Parse collection string like "community.general>=7.4.0"
			name, version := parseCollectionString(col)
			collectionMap[name] = CollectionRequirement{
				Name:    name,
				Version: version,
			}
		}
	}

	// Add collections from requirements.yml (structured format)
	if dr.requirement != nil && dr.requirement.Collections != nil {
		for _, col := range dr.requirement.Collections {
			// If collection already exists, keep existing unless this has a constraint
			if existing, exists := collectionMap[col.Name]; exists {
				// If existing has no constraint or is "latest", use this one
				if existing.Version == "" || existing.Version == "latest" {
					collectionMap[col.Name] = CollectionRequirement{
						Name:    col.Name,
						Version: col.Version,
					}
				}
			} else {
				collectionMap[col.Name] = CollectionRequirement{
					Name:    col.Name,
					Version: col.Version,
				}
			}
		}
	}

	// Override with collections from diffusion.toml config (highest priority)
	// diffusion.toml is the source of truth for version constraints
	if dr.config != nil && dr.config.Collections != nil {
		for _, col := range dr.config.Collections {
			collectionMap[col.Name] = CollectionRequirement{
				Name:    col.Name,
				Version: col.Version, // This is the constraint from diffusion.toml
			}
		}
	}

	// Convert map to sorted slice
	var collections []CollectionRequirement
	for _, col := range collectionMap {
		if col.Version == "" {
			col.Version = "latest"
		}
		collections = append(collections, col)
	}

	// Sort by name for consistency
	sort.Slice(collections, func(i, j int) bool {
		return collections[i].Name < collections[j].Name
	})

	return collections, nil
}

// ResolveRoleDependencies resolves all role dependencies
// For lock file generation, only roles in diffusion.toml are included
func (dr *DependencyResolver) ResolveRoleDependencies() ([]RoleRequirement, error) {
	roleMap := make(map[string]RoleRequirement)

	// Build a map of roles from requirements.yml for reference (src, scm info)
	requirementRoles := make(map[string]RequirementRole)
	if dr.requirement != nil && dr.requirement.Roles != nil {
		for _, role := range dr.requirement.Roles {
			requirementRoles[role.Name] = role
		}
	}

	// Only include roles from diffusion.toml config (source of truth for lock file)
	if dr.config != nil && dr.config.Roles != nil {
		for _, role := range dr.config.Roles {
			// Role names in config are prefixed with scenario (e.g., "default.rolename")
			// Extract the actual role name
			parts := strings.SplitN(role.Name, ".", 2)
			var roleName string
			if len(parts) == 2 {
				roleName = parts[1]
			} else {
				roleName = role.Name
			}

			// Get src and scm from requirements.yml if not in config
			src := role.Src
			scm := role.Scm
			if reqRole, exists := requirementRoles[roleName]; exists {
				if src == "" {
					src = reqRole.Src
				}
				if scm == "" {
					scm = reqRole.Scm
				}
			}

			// Add role with constraint from config
			roleMap[roleName] = RoleRequirement{
				Name:    roleName,
				Src:     src,
				Scm:     scm,
				Version: role.Version,
			}
		}
	}

	// Convert map to sorted slice
	var roles []RoleRequirement
	for _, role := range roleMap {
		if role.Version == "" {
			role.Version = "main"
		}
		roles = append(roles, role)
	}

	// Sort by name for consistency
	sort.Slice(roles, func(i, j int) bool {
		return roles[i].Name < roles[j].Name
	})

	return roles, nil
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
func ComputeDependencyHash(collections []CollectionRequirement, roles []RoleRequirement, toolVersions map[string]string, pythonVersion *PythonVersion) string {
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
