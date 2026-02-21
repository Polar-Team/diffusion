package dependency

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"sort"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/galaxy"
	"diffusion/internal/role"
	"diffusion/internal/utils"
)

// DependencyResolver resolves dependencies from requirements and meta files
type DependencyResolver struct {
	meta        *role.Meta
	requirement *role.Requirement
	config      *config.DependencyConfig
}

// NewDependencyResolver creates a new dependency resolver
func NewDependencyResolver(meta *role.Meta, req *role.Requirement, cfg *config.DependencyConfig) *DependencyResolver {
	return &DependencyResolver{
		meta:        meta,
		requirement: req,
		config:      cfg,
	}
}

// ResolveCollectionDependencies resolves all collection dependencies
// diffusion.toml provides version constraints, meta/requirements provide collection names
func (dr *DependencyResolver) ResolveCollectionDependencies() ([]config.CollectionRequirement, error) {
	collectionMap := make(map[string]config.CollectionRequirement)

	// Add collections from meta/main.yml (simple string format)
	if dr.meta != nil && dr.meta.Collections != nil {
		for _, col := range dr.meta.Collections {
			// Parse collection string like "community.general>=7.4.0"
			name, version := utils.ParseCollectionString(col)
			collectionMap[name] = config.CollectionRequirement{
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
					collectionMap[col.Name] = config.CollectionRequirement{
						Name:    col.Name,
						Version: col.Version,
					}
				}
			} else {
				collectionMap[col.Name] = config.CollectionRequirement{
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
			collectionMap[col.Name] = config.CollectionRequirement{
				Name:    col.Name,
				Version: col.Version, // This is the constraint from diffusion.toml
			}
		}
	}

	// Convert map to sorted slice
	var collections []config.CollectionRequirement
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
func (dr *DependencyResolver) ResolveRoleDependencies() ([]config.RoleRequirement, error) {
	roleMap := make(map[string]config.RoleRequirement)

	// Build a map of roles from requirements.yml for reference (src, scm info)
	requirementRoles := make(map[string]role.RequirementRole)
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
			roleMap[roleName] = config.RoleRequirement{
				Name:    roleName,
				Src:     src,
				Scm:     scm,
				Version: role.Version,
			}
		}
	}

	// Convert map to sorted slice
	var roles []config.RoleRequirement
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

// ParseCollectionString parses a collection string like "community.general>=7.4.0" or "community.docker"
func ParseCollectionString(col string) (name, version string) {
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
func (dr *DependencyResolver) ResolvePythonVersion() *config.PythonVersion {
	if dr.config != nil && dr.config.Python != nil {
		return dr.config.Python
	}

	// Return pinned Python version
	return &config.PythonVersion{
		Min:    config.DefaultMinPythonVersion,
		Max:    config.DefaultMaxPythonVersion,
		Pinned: config.PinnedPythonVersion,
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
		versions["ansible"] = config.DefaultAnsibleVersion
	}
	if versions["ansible-lint"] == "" {
		versions["ansible-lint"] = config.DefaultAnsibleLintVersion
	}
	if versions["molecule"] == "" {
		versions["molecule"] = config.DefaultMoleculeVersion
	}
	if versions["yamllint"] == "" {
		versions["yamllint"] = config.DefaultYamlLintVersion
	}

	// Adjust versions for Python compatibility
	pythonVersion := dr.ResolvePythonVersion()
	if pythonVersion != nil && pythonVersion.Pinned != "" {
		adjusted, warnings := utils.AdjustToolVersionsForPython(versions, pythonVersion.Pinned)

		// Print warnings
		for _, warning := range warnings {
			fmt.Printf("\033[33m%s\033[0m\n", warning)
		}

		return adjusted
	}

	return versions
}

// ComputeDependencyHash computes a hash of all dependencies for lock file
func ComputeDependencyHash(collections []config.CollectionRequirement, roles []config.RoleRequirement, toolVersions map[string]string, pythonVersion *config.PythonVersion) string {
	h := sha256.New()

	// Sort and hash collections
	sort.Slice(collections, func(i, j int) bool {
		return collections[i].Name < collections[j].Name
	})
	for _, col := range collections {
		galaxyAPI := galaxy.NewGalaxyAPI()
		resolvedVersion, err := galaxyAPI.ResolveVersion(col.Name, "collection", col.Version)
		if err == nil {
			col.Version = resolvedVersion
		}
		_, err = fmt.Fprintf(h, "collection:%s:%s\n", col.Name, col.Version)
		if err != nil {
			fmt.Printf("Error hashing collection: %v\n", err)
		}

	}

	// Sort and hash roles
	sort.Slice(roles, func(i, j int) bool {
		return roles[i].Name < roles[j].Name
	})
	for _, role := range roles {
		// Resolve git versions
		resolvedVersion, err := galaxy.ResolveVersionFromGit(role.Src, role.Version)
		if err == nil {
			role.Version = resolvedVersion
		}
		// Normalize role names by stripping scenario prefixes
		parts := strings.SplitN(role.Name, ".", 2)
		var prefix string
		if len(parts) == 2 {
			prefix = fmt.Sprintf("%s.", parts[0])
		} else if prefix == "default" || prefix == "" {
			prefix = "default."
		}

		role.Name = strings.TrimPrefix(role.Name, prefix)

		_, err = fmt.Fprintf(h, "role:%s:%s:%s:%s\n", strings.Replace(prefix, ".", "", 1), role.Name, role.Version, role.Src)
		if err != nil {
			fmt.Printf("Error hashing role: %v\n", err)
		}
	}

	// Sort and hash tool versions
	var tools []string
	for tool := range toolVersions {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	for _, tool := range tools {
		_, err := fmt.Fprintf(h, "tool:%s:%s\n", tool, toolVersions[tool])
		if err != nil {
			fmt.Printf("Error hashing tool: %v\n", err)
		}

	}

	// Hash Python version
	if pythonVersion != nil {
		_, err := fmt.Fprintf(h, "python:%s:%s:%s\n", pythonVersion.Min, pythonVersion.Max, pythonVersion.Pinned)
		if err != nil {
			fmt.Printf("Error hashing python version: %v\n", err)
		}
	}

	return hex.EncodeToString(h.Sum(nil))
}

// LoadDependencyConfig loads dependency configuration from diffusion.toml
func LoadDependencyConfig() (*config.DependencyConfig, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, err
	}

	if cfg.DependencyConfig == nil {
		// Return default configuration
		return &config.DependencyConfig{
			Python: &config.PythonVersion{
				Min:    config.ExtractMajorMinor(config.DefaultMinPythonVersion),
				Max:    config.ExtractMajorMinor(config.DefaultMaxPythonVersion),
				Pinned: config.PinnedPythonVersion,
			},
			Ansible:     config.DefaultAnsibleVersion,
			AnsibleLint: config.DefaultAnsibleLintVersion,
			Molecule:    config.DefaultMoleculeVersion,
			YamlLint:    config.DefaultYamlLintVersion,
		}, nil

	}

	// Validate and normalize Python versions
	if cfg.DependencyConfig.Python != nil {
		python := cfg.DependencyConfig.Python

		// Validate and normalize Pinned version
		if python.Pinned != "" {
			validated, err := config.ValidatePythonVersion(python.Pinned)
			if err != nil {
				return nil, fmt.Errorf("invalid pinned Python version: %w", err)
			}
			python.Pinned = validated
		} else {
			python.Pinned = config.PinnedPythonVersion
		}

		// Validate and normalize Min version
		if python.Min != "" {
			validated, err := config.ValidatePythonVersion(python.Min)
			if err != nil {
				return nil, fmt.Errorf("invalid min Python version: %w", err)
			}
			python.Min = validated
		} else {
			python.Min = config.DefaultMinPythonVersion
		}

		// Validate and normalize Max version
		if python.Max != "" {
			validated, err := config.ValidatePythonVersion(python.Max)
			if err != nil {
				return nil, fmt.Errorf("invalid max Python version: %w", err)
			}
			python.Max = validated
		} else {
			python.Max = config.DefaultMaxPythonVersion
		}

		// Clear Additional versions - not supported for container
		python.Additional = nil

	}
	// Validate and normalize Roles versions
	if cfg.DependencyConfig.Roles != nil {
		for i, role := range cfg.DependencyConfig.Roles {
			// Extract actual role name from scenario prefix (e.g., "default.rolename" -> "rolename")
			// Roles in diffusion.toml are stored as "scenario.rolename" or "scenario.namespace.rolename"

			// Role names in config are prefixed with scenario (e.g., "default.rolename")
			// Extract the actual role name
			parts := strings.SplitN(role.Name, ".", 2)
			var roleName string
			if len(parts) == 2 {
				roleName = parts[1]
			} else {
				roleName = role.Name
			}

			// Store the cleaned role name
			cfg.DependencyConfig.Roles[i].Name = roleName

			if role.Scm == "git" && role.Src != "" {
				cfg.DependencyConfig.Roles[i].Src = role.Src
				cfg.DependencyConfig.Roles[i].Scm = "git"
				cfg.DependencyConfig.Roles[i].Version = role.Version
			} else {
				// Galaxy role - need to split namespace and name
				cfg.DependencyConfig.Roles[i].Scm = "galaxy"
				cfg.DependencyConfig.Roles[i].Version = role.Version
			}

			if roleName == "" {
				return nil, fmt.Errorf("role name cannot be empty")
			}
		}
	}

	// Validate and normalize Collections versions
	if cfg.DependencyConfig.Collections != nil {
		for i, collection := range cfg.DependencyConfig.Collections {
			if collection.Source == "git" && collection.SourceURL != "" {
				cfg.DependencyConfig.Collections[i].SourceURL = collection.SourceURL
				cfg.DependencyConfig.Collections[i].Source = "git"
				cfg.DependencyConfig.Collections[i].Version = collection.Version
			} else {
				cfg.DependencyConfig.Collections[i].Source = "galaxy"
				cfg.DependencyConfig.Collections[i].Version = collection.Version
			}

			if collection.Name == "" {
				return nil, fmt.Errorf("collection name cannot be empty")
			}
		}
	}

	return cfg.DependencyConfig, nil
}

// SaveDependencyConfig saves dependency configuration to diffusion.toml
func SaveDependencyConfig(depConfig *config.DependencyConfig) error {
	cfg, err := config.LoadConfig()
	if err != nil {
		// Create new config if it doesn't exist
		cfg = &config.Config{}
	}

	cfg.DependencyConfig = depConfig
	return config.SaveConfig(cfg)
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
