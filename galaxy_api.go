package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// GalaxyAPI handles interactions with Ansible Galaxy API
type GalaxyAPI struct {
	BaseURL string
	Client  *http.Client
}

// NewGalaxyAPI creates a new Galaxy API client
func NewGalaxyAPI() *GalaxyAPI {
	return &GalaxyAPI{
		BaseURL: "https://galaxy.ansible.com/api/v3",
		Client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// GetCollectionLatestVersion fetches the latest version of a collection
func (g *GalaxyAPI) GetCollectionLatestVersion(namespace, name string) (string, error) {
	// Use the correct v3 API endpoint
	url := fmt.Sprintf("https://galaxy.ansible.com/api/v3/plugin/ansible/content/published/collections/index/%s/%s/",
		namespace, name)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := g.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch collection info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var collectionResp struct {
		HighestVersion struct {
			Version string `json:"version"`
		} `json:"highest_version"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&collectionResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if collectionResp.HighestVersion.Version == "" {
		return "", fmt.Errorf("no version found for %s.%s", namespace, name)
	}

	return collectionResp.HighestVersion.Version, nil
}

// CompareVersions compares two semantic versions
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func CompareVersions(v1, v2 string) int {
	// Remove any 'v' prefix
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	// Split by dots
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int

		if i < len(parts1) {
			// Parse number, ignoring any non-numeric suffix (like -alpha, -beta)
			numStr := parts1[i]
			for j, c := range numStr {
				if c < '0' || c > '9' {
					numStr = numStr[:j]
					break
				}
			}
			if numStr != "" {
				n1, _ = strconv.Atoi(numStr)
			}
		}

		if i < len(parts2) {
			numStr := parts2[i]
			for j, c := range numStr {
				if c < '0' || c > '9' {
					numStr = numStr[:j]
					break
				}
			}
			if numStr != "" {
				n2, _ = strconv.Atoi(numStr)
			}
		}

		if n1 > n2 {
			return 1
		} else if n1 < n2 {
			return -1
		}
	}

	return 0
}

// GetRoleLatestVersion fetches the latest version of a role
func (g *GalaxyAPI) GetRoleLatestVersion(namespace, name string) (string, error) {
	// Galaxy v1 API for roles
	url := fmt.Sprintf("https://galaxy.ansible.com/api/v1/roles/?owner__username=%s&name=%s",
		namespace, name)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")

	resp, err := g.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch role info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	var result struct {
		Results []struct {
			SummaryFields struct {
				Versions []struct {
					Name string `json:"name"`
				} `json:"versions"`
			} `json:"summary_fields"`
		} `json:"results"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(result.Results) == 0 || len(result.Results[0].SummaryFields.Versions) == 0 {
		return "", fmt.Errorf("no versions found for %s.%s", namespace, name)
	}

	// First version is the latest
	return result.Results[0].SummaryFields.Versions[0].Name, nil
}

// ResolveCollectionVersion resolves a collection version constraint to an actual version
func (g *GalaxyAPI) ResolveCollectionVersion(collectionName, versionConstraint string) (string, error) {
	// Parse collection name (namespace.name)
	parts := strings.Split(collectionName, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid collection name format: %s (expected namespace.name)", collectionName)
	}

	namespace, name := parts[0], parts[1]

	// If version is "latest" or empty, fetch latest
	if versionConstraint == "" || versionConstraint == "latest" {
		return g.GetCollectionLatestVersion(namespace, name)
	}

	// If version has operators (>=, <=, etc.), fetch latest and validate
	if strings.ContainsAny(versionConstraint, ">=<") {
		// For now, just fetch latest version
		// TODO: Implement proper version constraint resolution
		return g.GetCollectionLatestVersion(namespace, name)
	}

	// If it's a specific version, return as-is
	return versionConstraint, nil
}

// ResolveRoleVersion resolves a role version constraint to an actual version
func (g *GalaxyAPI) ResolveRoleVersion(namespace, name, versionConstraint string) (string, error) {
	// If version is "latest", "main", or empty, fetch latest
	if versionConstraint == "" || versionConstraint == "latest" || versionConstraint == "main" {
		return g.GetRoleLatestVersion(namespace, name)
	}

	// If it's a specific version or branch, return as-is
	return versionConstraint, nil
}

// GetPythonPackageVersion fetches the latest version of a Python package from PyPI
func GetPythonPackageVersion(packageName string) (string, error) {
	// Remove version constraints if present
	pkgName := packageName
	var operand string
	var constraintVersion string

	for _, op := range []string{">=", "<=", "==", ">", "<", "="} {
		if idx := strings.Index(pkgName, op); idx != -1 {
			operand = op
			constraintVersion = strings.TrimSpace(pkgName[idx+len(op):])
			pkgName = pkgName[:idx]
			break
		}
	}
	pkgName = strings.TrimSpace(pkgName)

	// Handle extras like "package[extra]"
	if idx := strings.Index(pkgName, "["); idx != -1 {
		pkgName = pkgName[:idx]
	}

	url := fmt.Sprintf("https://pypi.org/pypi/%s/json", pkgName)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to fetch package info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PyPI returned status %d for package %s", resp.StatusCode, pkgName)
	}

	var result struct {
		Releases map[string][]struct {
			RequiresPython string `json:"requires_python"`
			URL            string `json:"url"`
			Digests        struct {
				Sha256 string `json:"sha256"`
			} `json:"digests"`
		} `json:"releases"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	// Get all version strings from releases map keys
	versions := make([]string, 0, len(result.Releases))
	for version := range result.Releases {
		versions = append(versions, version)
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no releases found for package %s", pkgName)
	}

	// Sort versions in descending order (highest first)
	sort.Slice(versions, func(i, j int) bool {
		return CompareVersions(versions[i], versions[j]) > 0
	})

	// If no constraint, return latest version
	if operand == "" {
		return versions[0], nil
	}

	// Validate that latest version satisfies the constraint
	latestVersion := versions[0]
	cmp := CompareVersions(latestVersion, constraintVersion)

	switch operand {
	case ">=":
		if cmp >= 0 {
			return latestVersion, nil
		}
		return constraintVersion, nil
	case "<=":
		// Find the highest version that is <= constraint
		for _, v := range versions {
			if CompareVersions(v, constraintVersion) <= 0 {
				return v, nil
			}
		}
		return "", fmt.Errorf("no version found <= %s for package %s", constraintVersion, pkgName)
	case "==", "=":
		if cmp == 0 {
			return constraintVersion, nil
		}
		return "", fmt.Errorf("exact version %s not found for package %s", constraintVersion, pkgName)
	case ">":
		if cmp > 0 {
			return latestVersion, nil
		}
		return "", fmt.Errorf("no version found > %s for package %s", constraintVersion, pkgName)
	case "<":
		// Find the highest version that is < constraint
		for _, v := range versions {
			if CompareVersions(v, constraintVersion) < 0 {
				return v, nil
			}
		}
		return "", fmt.Errorf("no version found < %s for package %s", constraintVersion, pkgName)
	}

	return latestVersion, nil
}

// ResolvePythonDependencies resolves Python package versions
func ResolvePythonDependencies(packages []string) (map[string]string, error) {
	resolved := make(map[string]string)

	for _, pkg := range packages {
		// Extract package name (without extras like [extra])
		pkgName := pkg
		for _, op := range []string{">=", "<=", "==", ">", "<", "="} {
			if idx := strings.Index(pkgName, op); idx != -1 {
				pkgName = pkgName[:idx]
				break
			}
		}
		pkgName = strings.TrimSpace(pkgName)

		// Handle extras
		baseName := pkgName
		if idx := strings.Index(pkgName, "["); idx != -1 {
			baseName = pkgName[:idx]
		}

		// Fetch version from PyPI with full constraint
		version, err := GetPythonPackageVersion(pkg)
		if err != nil {
			// If we can't fetch, use the constraint as-is
			resolved[baseName] = pkg
			continue
		}

		resolved[baseName] = version
	}

	return resolved, nil
}
