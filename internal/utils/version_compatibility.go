package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// PythonCompatibility defines Python version compatibility for tools
type PythonCompatibility struct {
	MinPython string
	MaxPython string
}

// ToolCompatibility maps tool versions to Python compatibility
var ToolCompatibility = map[string]map[string]PythonCompatibility{
	"ansible": {
		"13.x": {MinPython: "3.10", MaxPython: "3.13"}, // Ansible 13.x requires Python 3.10+
		"12.x": {MinPython: "3.10", MaxPython: "3.13"}, // Ansible 12.x requires Python 3.10+
		"11.x": {MinPython: "3.10", MaxPython: "3.13"}, // Ansible 11.x requires Python 3.10+
		"10.x": {MinPython: "3.10", MaxPython: "3.12"}, // Ansible 10.x requires Python 3.10+
		"9.x":  {MinPython: "3.9", MaxPython: "3.12"},  // Ansible 9.x supports Python 3.9+
		"8.x":  {MinPython: "3.9", MaxPython: "3.11"},  // Ansible 8.x supports Python 3.9+
	},
	"molecule": {
		"25.x": {MinPython: "3.10", MaxPython: "3.13"}, // Molecule 25.x requires Python 3.10+
		"24.x": {MinPython: "3.10", MaxPython: "3.13"}, // Molecule 24.x requires Python 3.10+
		"6.x":  {MinPython: "3.9", MaxPython: "3.12"},  // Molecule 6.x supports Python 3.9+
		"5.x":  {MinPython: "3.9", MaxPython: "3.11"},  // Molecule 5.x supports Python 3.9+
	},
	"ansible-lint": {
		"24.x": {MinPython: "3.10", MaxPython: "3.13"}, // ansible-lint 24.x requires Python 3.10+
		"6.x":  {MinPython: "3.9", MaxPython: "3.12"},  // ansible-lint 6.x supports Python 3.9+
	},
}

// comparePythonVersions compares two Python versions (major.minor format)
func comparePythonVersions(v1, v2 string) int {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	for i := 0; i < 2; i++ {
		var n1, n2 int

		if i < len(parts1) && parts1[i] != "x" {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) && parts2[i] != "x" {
			n2, _ = strconv.Atoi(parts2[i])
		}

		if n1 > n2 {
			return 1
		} else if n1 < n2 {
			return -1
		}
	}

	return 0
}

// ValidateToolCompatibility validates if a tool version is compatible with Python version
func ValidateToolCompatibility(tool, toolVersion, pythonVersion string) (bool, string) {
	// Get Python version as major.minor (e.g., "3.10")
	pythonParts := strings.Split(strings.TrimPrefix(pythonVersion, "v"), ".")
	pythonMajorMinor := pythonVersion
	if len(pythonParts) >= 2 {
		pythonMajorMinor = pythonParts[0] + "." + pythonParts[1]
	}

	// Get tool version as major.x (e.g., "13.x")
	toolParts := strings.Split(strings.TrimPrefix(toolVersion, "v"), ".")
	toolKey := "0.x"
	if len(toolParts) >= 1 {
		// Remove any operators
		major := toolParts[0]
		for _, op := range []string{">=", "<=", "==", ">", "<", "="} {
			major = strings.TrimPrefix(major, op)
		}
		major = strings.TrimSpace(major)
		toolKey = major + ".x"
	}

	compatibility, ok := ToolCompatibility[tool]
	if !ok {
		return true, "" // No compatibility info, assume compatible
	}

	compat, ok := compatibility[toolKey]
	if !ok {
		return true, "" // No compatibility info for this version, assume compatible
	}

	// Check if Python version is in range
	if comparePythonVersions(pythonMajorMinor, compat.MinPython) < 0 {
		return false, fmt.Sprintf("%s %s requires Python %s or higher (you have %s)",
			tool, toolVersion, compat.MinPython, pythonVersion)
	}

	if comparePythonVersions(pythonMajorMinor, compat.MaxPython) > 0 {
		return false, fmt.Sprintf("%s %s supports Python up to %s (you have %s)",
			tool, toolVersion, compat.MaxPython, pythonVersion)
	}

	return true, ""
}

// GetCompatibleVersion returns a compatible version for the given Python version
func GetCompatibleVersion(tool, pythonVersion string) (string, error) {
	// Get Python version as major.minor
	pythonParts := strings.Split(strings.TrimPrefix(pythonVersion, "v"), ".")
	pythonMajorMinor := pythonVersion
	if len(pythonParts) >= 2 {
		pythonMajorMinor = pythonParts[0] + "." + pythonParts[1]
	}

	compatibility, ok := ToolCompatibility[tool]
	if !ok {
		return "", fmt.Errorf("no compatibility info for tool: %s", tool)
	}

	// Try to find a compatible version
	// Check in order: latest to oldest
	versionOrder := []string{"25.x", "24.x", "13.x", "12.x", "11.x", "10.x", "9.x", "8.x", "6.x", "5.x"}

	for _, toolVersion := range versionOrder {
		compat, ok := compatibility[toolVersion]
		if !ok {
			continue
		}

		// Check if Python version is compatible
		if comparePythonVersions(pythonMajorMinor, compat.MinPython) >= 0 &&
			comparePythonVersions(pythonMajorMinor, compat.MaxPython) <= 0 {
			// Return the version constraint
			major := strings.Split(toolVersion, ".")[0]
			return ">=" + major + ".0.0", nil
		}
	}

	return "", fmt.Errorf("no compatible %s version found for Python %s", tool, pythonVersion)
}

// AdjustToolVersionsForPython adjusts tool versions to be compatible with Python version
func AdjustToolVersionsForPython(toolVersions map[string]string, pythonVersion string) (map[string]string, []string) {
	adjusted := make(map[string]string)
	warnings := []string{}

	for tool, version := range toolVersions {
		// Check compatibility
		compatible, message := ValidateToolCompatibility(tool, version, pythonVersion)

		if !compatible {
			// Try to find a compatible version
			compatVersion, err := GetCompatibleVersion(tool, pythonVersion)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Warning: %s. Could not find compatible version.", message))
				adjusted[tool] = version // Keep original
			} else {
				warnings = append(warnings, fmt.Sprintf("Adjusted %s from %s to %s for Python %s compatibility",
					tool, version, compatVersion, pythonVersion))
				adjusted[tool] = compatVersion
			}
		} else {
			adjusted[tool] = version
		}
	}

	return adjusted, warnings
}

// GetRecommendedVersions returns recommended tool versions for a Python version
func GetRecommendedVersions(pythonVersion string) map[string]string {
	// Get Python version as major.minor
	pythonParts := strings.Split(strings.TrimPrefix(pythonVersion, "v"), ".")
	pythonMajorMinor := pythonVersion
	if len(pythonParts) >= 2 {
		pythonMajorMinor = pythonParts[0] + "." + pythonParts[1]
	}

	recommended := make(map[string]string)

	// Python 3.9 - use older versions
	if comparePythonVersions(pythonMajorMinor, "3.10") < 0 {
		recommended["ansible"] = ">=9.0.0"
		recommended["molecule"] = ">=6.0.0"
		recommended["ansible-lint"] = ">=6.0.0"
		recommended["yamllint"] = ">=1.35.0"
	} else {
		// Python 3.10+ - use latest versions
		recommended["ansible"] = ">=10.0.0"
		recommended["molecule"] = ">=24.0.0"
		recommended["ansible-lint"] = ">=24.0.0"
		recommended["yamllint"] = ">=1.35.0"
	}

	return recommended
}
