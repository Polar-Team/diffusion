package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
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

	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("failed to close response body: %v\n", err)
		}
	}()

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

	maxLen := max(len(parts1), len(parts2))

	for i := range maxLen {
		n1 := CalcVersion(i, parts1)
		n2 := CalcVersion(i, parts2)
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
	url := fmt.Sprintf("https://galaxy.ansible.com/api/v3/roles/?owner__username=%s&name=%s",
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

	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("failed to close response body: %v\n", err)
		}
	}()

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
func (g *GalaxyAPI) ResolveVersion(objectName, objectType, versionConstraint string) (string, error) {
	// Parse collection name (namespace.name)
	parts := strings.Split(objectName, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid collection name format: %s (expected namespace.name)", objectName)
	}

	namespace, name := parts[0], parts[1]

	// If version is "latest" or empty, fetch latest
	if versionConstraint == "" || versionConstraint == "latest" {
		return g.GetCollectionLatestVersion(namespace, name)
	}

	// If version has operators (>=, <=, etc.), fetch latest and validate
	if strings.ContainsAny(versionConstraint, ">=<") {
		var operand string
		var constraintVersion string

		for _, op := range []string{">=", "<=", "==", ">", "<", "="} {
			if idx := strings.Index(versionConstraint, op); idx != -1 {
				operand = op
				constraintVersion = strings.TrimSpace(versionConstraint[idx+len(op):])
				break
			}
		}

		if constraintVersion == "" {
			return "", fmt.Errorf("invalid version constraint: %s", versionConstraint)
		}

		url := ""
		switch objectType {
		case "collection":
			url = fmt.Sprintf("https://galaxy.ansible.com/api/v3/plugin/ansible/content/published/collections/index/%s/%s/versions/", namespace, name)
		case "role":
			url = fmt.Sprintf("https://galaxy.ansible.com/api/v1/roles/?owner__username=%s&name=%s", namespace, name)
		default:
			return "", fmt.Errorf("unknown object type: %s", objectType)
		}

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(url)
		if err != nil {
			return "", fmt.Errorf("failed to fetch collection info: %w", err)
		}

		defer func() {
			if err := resp.Body.Close(); err != nil {
				fmt.Printf("failed to close response body: %v\n", err)
			}
		}()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("galaxy return status %d for collection %s", resp.StatusCode, name)
		}

		var result struct {
			Data []struct {
				Version         string `json:"version"`
				RequiresAnsible string `json:"requires_ansible"`
			} `json:"data"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return "", fmt.Errorf("failed to decode response: %w", err)
		}

		// Get all versions strings from data array
		versions := make([]string, 0, len(result.Data))
		for _, versionData := range result.Data {
			versions = append(versions, versionData.Version)
		}

		if len(versions) == 0 {
			return "", fmt.Errorf("no releases found for collection %s", name)
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
		cmp := CompareVersions(latestVersion, objectName)

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
			return "", fmt.Errorf("no version found <= %s for package %s", constraintVersion, name)
		case "==", "=":
			if cmp == 0 {
				return constraintVersion, nil
			}
			return "", fmt.Errorf("exact version %s not found for package %s", constraintVersion, name)
		case ">":
			if cmp > 0 {
				return latestVersion, nil
			}
			return "", fmt.Errorf("no version found > %s for package %s", constraintVersion, name)
		case "<":
			// Find the highest version that is < constraint
			for _, v := range versions {
				if CompareVersions(v, constraintVersion) < 0 {
					return v, nil
				}
			}
			return "", fmt.Errorf("no version found < %s for package %s", constraintVersion, name)
		}

		return latestVersion, nil
	}

	// If it's a specific version, return as-is
	return versionConstraint, nil
}

// ResolveRoleVersion resolves a role version constraint to an actual version
func (g *GalaxyAPI) ResolveRoleVersion(namespace, name, versionConstraint string) (string, error) {
	// If version is "latest", "main", or empty, fetch latest
	if versionConstraint == "" || versionConstraint == "latest" || versionConstraint == "main" || versionConstraint == "master" {
		return g.GetRoleLatestVersion(namespace, name)
	}

	// If it's a specific version or branch, return as-is
	return versionConstraint, nil
}

// ResolveVersionFromGit resolves a role version from a git repository
// It fetches tags from the git repo and returns the latest version or resolves a constraint
func ResolveVersionFromGit(gitURL, versionConstraint string) (string, error) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// If version is "latest", "main", or empty, fetch latest tag
	if versionConstraint == "" || versionConstraint == "latest" || versionConstraint == "main" || versionConstraint == "master" {

		attempts := 0
		maxAttempts := 3
		var tag string
		var err error

		for attempts < maxAttempts {
			tag, err = GetLatestGitTag(gitURL)
			fmt.Printf("Fetched latest git tag: %s\n", tag)

			if err != nil {
				// Error occurred, retry
				attempts++
				time.Sleep(2 * time.Second)
				continue
			}

			// Success - check if we got a valid tag
			if tag != "main" && tag != "master" && tag != "" {
				return NormalizeVersion(tag), nil
			}

			fmt.Println("No valid tags found, retrying...")
			attempts++
			time.Sleep(2 * time.Second)
		}

		fmt.Println("No tags found after all attempts, defaulting to 'not-defined'")
	} else {
		// If version has operators (>=, <=, etc.), resolve from git tags
		if strings.ContainsAny(versionConstraint, ">=<") {
			var operand string
			var constraintVersion string

			for _, op := range []string{">=", "<=", "==", ">", "<", "="} {
				if idx := strings.Index(versionConstraint, op); idx != -1 {
					operand = op
					constraintVersion = strings.TrimSpace(versionConstraint[idx+len(op):])
					break
				}
			}

			if constraintVersion == "" {
				return "", fmt.Errorf("invalid version constraint: %s", versionConstraint)
			}

			// Fetch all tags from git
			cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "--sort=-v:refname", gitURL)
			output, err := cmd.Output()

			if err != nil {
				return "", fmt.Errorf("failed to fetch tags from git: %w", err)
			}

			// Parse all tags and find the latest that satisfies the constraint
			lines := strings.Split(string(output), "\n")
			var latestMatchingTag string

			for _, line := range lines {
				if line == "" {
					continue
				}
				// Format: <hash>\trefs/tags/<tag>
				parts := strings.Split(line, "\t")
				if len(parts) != 2 {
					continue
				}
				tagRef := parts[1]
				// Extract tag name from refs/tags/<tag>
				if strings.HasPrefix(tagRef, "refs/tags/") {
					tag := strings.TrimPrefix(tagRef, "refs/tags/")
					// Skip tags ending with ^{} (annotated tag references)
					if strings.HasSuffix(tag, "^{}") {
						continue
					}

					// Normalize tag for comparison (strip 'v' prefix)
					normalizedTag := strings.TrimPrefix(tag, "v")

					// Check if this tag satisfies the constraint
					satisfies, err := CompareVersionConstraint(normalizedTag, operand, constraintVersion)
					if err != nil {
						continue // Skip invalid version tags
					}

					if satisfies {
						// Return the first (latest) matching tag
						return NormalizeVersion(tag), nil
					}
				}
			}

			// If no matching tag found, return error
			if latestMatchingTag == "" {
				return "", fmt.Errorf("no git tag found satisfying constraint %s", versionConstraint)
			}

			return NormalizeVersion(latestMatchingTag), nil
		}
	}

	// If it's a specific version or branch, return as-is
	return versionConstraint, nil
}

// GetLatestGitTag fetches the latest tag from a git repository
func GetLatestGitTag(gitURL string) (string, error) {
	// Use git ls-remote to fetch tags with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "ls-remote", "--tags", "--sort=-v:refname", gitURL)
	output, err := cmd.Output()
	if err != nil {
		return "main", nil // Fallback to main if git command fails
	}

	// Parse output to find the latest tag
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		// Format: <hash>\trefs/tags/<tag>
		parts := strings.Split(line, "\t")
		if len(parts) != 2 {
			continue
		}
		tagRef := parts[1]
		// Extract tag name from refs/tags/<tag>
		if strings.HasPrefix(tagRef, "refs/tags/") {
			tag := strings.TrimPrefix(tagRef, "refs/tags/")
			// Skip tags ending with ^{} (annotated tag references)
			if strings.HasSuffix(tag, "^{}") {
				continue
			}
			// Return the tag (with or without 'v' prefix)
			return tag, nil
		}
	}

	return "main", nil // Fallback to main if no tags found
}

// NormalizeVersion removes or adds 'v' prefix as needed
func NormalizeVersion(version string) string {
	// If version starts with 'v' followed by a digit, keep it
	if len(version) > 1 && version[0] == 'v' && version[1] >= '0' && version[1] <= '9' {
		return version
	}
	// If version is a semantic version without 'v', add it
	if len(version) > 0 && version[0] >= '0' && version[0] <= '9' {
		// Check if it looks like a semantic version (x.y.z)
		parts := strings.Split(version, ".")
		if len(parts) >= 2 {
			return "v" + version
		}
	}
	return version
}

// CompareVersionConstraint compares a version against a constraint
// Returns true if the version satisfies the constraint
func CompareVersionConstraint(version, operator, constraintVersion string) (bool, error) {
	// Parse versions into comparable format
	v1, err := parseSemanticVersion(version)
	if err != nil {
		return false, err
	}

	v2, err := parseSemanticVersion(constraintVersion)
	if err != nil {
		return false, err
	}

	// Compare versions
	cmp := compareVersions(v1, v2)

	switch operator {
	case ">=":
		return cmp >= 0, nil
	case "<=":
		return cmp <= 0, nil
	case "==", "=":
		return cmp == 0, nil
	case ">":
		return cmp > 0, nil
	case "<":
		return cmp < 0, nil
	default:
		return false, fmt.Errorf("unknown operator: %s", operator)
	}
}

// parseSemanticVersion parses a semantic version string into [major, minor, patch]
func parseSemanticVersion(version string) ([]int, error) {
	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	// Split by '.'
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid version format: %s", version)
	}

	// Parse up to 3 parts (major, minor, patch)
	result := make([]int, 3)
	for i := 0; i < 3 && i < len(parts); i++ {
		// Remove any non-numeric suffix (e.g., "1.2.3-beta" -> "1.2.3")
		numStr := parts[i]
		for j, c := range numStr {
			if c < '0' || c > '9' {
				numStr = numStr[:j]
				break
			}
		}

		if numStr == "" {
			result[i] = 0
			continue
		}

		num, err := strconv.Atoi(numStr)
		if err != nil {
			return nil, fmt.Errorf("invalid version number: %s", parts[i])
		}
		result[i] = num
	}

	return result, nil
}

// compareVersions compares two version arrays
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 []int) int {
	for i := 0; i < 3; i++ {
		if v1[i] < v2[i] {
			return -1
		}
		if v1[i] > v2[i] {
			return 1
		}
	}
	return 0
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

	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Printf("failed to close response body: %v\n", err)
		}
	}()

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

// GetCollectionVersion is a convenience function to resolve collection version
func GetCollectionVersion(collectionName, versionConstraint string) (string, error) {
	api := NewGalaxyAPI()
	return api.ResolveVersion(collectionName, "collection", versionConstraint)
}

// CaclcVersion extracts the numeric part of a version string at a given index
func CalcVersion(index int, parts []string) int {
	n := 0
	if index < len(parts) {
		// Parse number, ignoring any non-numeric suffix (like -alpha, -beta)
		numStr := parts[index]
		for j, c := range numStr {
			if c < '0' || c > '9' {
				numStr = numStr[:j]
				break
			}
		}
		if numStr != "" {
			n, _ = strconv.Atoi(numStr)
		}
	}
	return n
}

// GetRoleVersion is a convenience function to resolve role version
func GetRoleVersion(roleName, versionConstraint string) (string, error) {
	api := NewGalaxyAPI()
	return api.ResolveVersion(roleName, "role", versionConstraint)
}
