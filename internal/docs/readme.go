package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Markers for the Role Variables section in README.md
const (
	BeginMarker = "<!-- begin role_variables -->"
	EndMarker   = "<!-- end role_variables -->"
)

// GenerateVariablesSection generates a markdown table of role variables
func GenerateVariablesSection(variables []RoleVariable) string {
	if len(variables) == 0 {
		return fmt.Sprintf("%s\n\n## Role Variables\n\n_No variables found._\n\n%s", BeginMarker, EndMarker)
	}

	var sb strings.Builder

	sb.WriteString(BeginMarker)
	sb.WriteString("\n\n## Role Variables\n\n")
	sb.WriteString("| Variable | Type | Default | Description |\n")
	sb.WriteString("|----------|------|---------|-------------|\n")

	for _, v := range variables {
		varType := v.Type
		if varType == "" {
			varType = "-"
		}

		defaultVal := v.Default
		if defaultVal == "" {
			defaultVal = "-"
		} else {
			// Escape pipe characters and backtick-wrap values
			defaultVal = strings.ReplaceAll(defaultVal, "|", "\\|")
			defaultVal = "`" + defaultVal + "`"
		}

		description := v.Description
		if description == "" {
			description = "-"
		}

		sb.WriteString(fmt.Sprintf("| `%s` | %s | %s | %s |\n", v.Name, varType, defaultVal, description))
	}

	sb.WriteString("\n")
	sb.WriteString(EndMarker)

	return sb.String()
}

// UpdateReadme updates the README.md file with the Role Variables section.
// If the markers already exist, it replaces the content between them.
// If the markers don't exist, it appends the section at the end of the file.
func UpdateReadme(roleDir string, variables []RoleVariable) error {
	readmePath := filepath.Join(roleDir, "README.md")

	section := GenerateVariablesSection(variables)

	// Check if README.md exists
	content, err := os.ReadFile(readmePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create a new README.md with just the variables section
			newContent := fmt.Sprintf("# Role\n\n%s\n", section)
			return os.WriteFile(readmePath, []byte(newContent), 0644)
		}
		return fmt.Errorf("reading README.md: %w", err)
	}

	existingContent := string(content)

	// Check if markers exist
	beginIdx := strings.Index(existingContent, BeginMarker)
	endIdx := strings.Index(existingContent, EndMarker)

	var newContent string
	if beginIdx >= 0 && endIdx >= 0 && endIdx > beginIdx {
		// Replace content between markers (inclusive of markers)
		markerEnd := endIdx + len(EndMarker)
		newContent = existingContent[:beginIdx] + section + existingContent[markerEnd:]
	} else {
		// Append at the end of the file
		newContent = existingContent
		if !strings.HasSuffix(newContent, "\n") {
			newContent += "\n"
		}
		newContent += "\n" + section + "\n"
	}

	return os.WriteFile(readmePath, []byte(newContent), 0644)
}
