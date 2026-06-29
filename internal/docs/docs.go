package docs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// RoleVariable represents a documented Ansible role variable
type RoleVariable struct {
	Name        string // Variable name
	Type        string // Type from #—| comment (string, int, list, map, bool, etc.)
	Default     string // Default value from YAML declaration
	Description string // Description from #—? comment
	Source      string // File where variable was found (e.g., "defaults/main.yml")
}

// jinja2VarRegex matches {{ variable_name }} patterns in templates
var jinja2VarRegex = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*(?:\|[^}]*)?\}\}`)

// yamlVarDeclRegex matches YAML variable declarations like "variable_name: value"
var yamlVarDeclRegex = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*):\s*(.*)$`)

// typeCommentRegex matches type annotation comments: #—| type
var typeCommentRegex = regexp.MustCompile(`^#\s*[—\-]\|\s*(.+)$`)

// descCommentRegex matches description comments: #—? description
var descCommentRegex = regexp.MustCompile(`^#\s*[—\-]\?\s*(.+)$`)

// ScanRoleVariables scans an Ansible role directory and extracts all documented variables.
// It looks in defaults/main.yml, vars/main.yml, and template files for {{ }} interpolation.
func ScanRoleVariables(roleDir string) ([]RoleVariable, error) {
	varMap := make(map[string]*RoleVariable)

	// 1. Scan defaults/main.yml — primary source of role variables with defaults
	defaultsFile := filepath.Join(roleDir, "defaults", "main.yml")
	if err := scanYAMLFile(defaultsFile, "defaults/main.yml", varMap); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("scanning defaults/main.yml: %w", err)
	}

	// 2. Scan vars/main.yml — role variables (higher precedence, often without defaults)
	varsFile := filepath.Join(roleDir, "vars", "main.yml")
	if err := scanYAMLFile(varsFile, "vars/main.yml", varMap); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("scanning vars/main.yml: %w", err)
	}

	// 3. Scan template files for {{ variable }} references
	templatesDir := filepath.Join(roleDir, "templates")
	if info, err := os.Stat(templatesDir); err == nil && info.IsDir() {
		if err := scanTemplatesDir(templatesDir, varMap); err != nil {
			return nil, fmt.Errorf("scanning templates: %w", err)
		}
	}

	// 4. Scan tasks for {{ variable }} references
	tasksDir := filepath.Join(roleDir, "tasks")
	if info, err := os.Stat(tasksDir); err == nil && info.IsDir() {
		if err := scanDirForJinja2Vars(tasksDir, "tasks", varMap); err != nil {
			return nil, fmt.Errorf("scanning tasks: %w", err)
		}
	}

	// Convert map to sorted slice
	variables := make([]RoleVariable, 0, len(varMap))
	for _, v := range varMap {
		variables = append(variables, *v)
	}
	sort.Slice(variables, func(i, j int) bool {
		return variables[i].Name < variables[j].Name
	})

	return variables, nil
}

// scanYAMLFile parses a YAML variable file (defaults/main.yml or vars/main.yml)
// looking for variable declarations with optional type and description comments.
//
// Expected format:
//
//	#—| string
//	variable_name: "default_value"
//	#—? This is the description of the variable
//
// For multi-line values (lists/maps), the description comment is searched after
// the indented block ends.
func scanYAMLFile(filePath, sourceName string, varMap map[string]*RoleVariable) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	for i := 0; i < len(lines); i++ {
		rawLine := lines[i]
		line := strings.TrimSpace(rawLine)

		// Only match top-level declarations (no leading whitespace)
		if len(rawLine) > 0 && (rawLine[0] == ' ' || rawLine[0] == '\t') {
			continue
		}

		// Skip empty lines, comments, and document separators
		if line == "" || strings.HasPrefix(line, "#") || line == "---" || line == "..." {
			continue
		}

		// Match YAML variable declarations
		matches := yamlVarDeclRegex.FindStringSubmatch(line)
		if matches == nil {
			continue
		}

		varName := matches[1]
		defaultValue := strings.TrimSpace(matches[2])

		// Clean up default value (remove trailing comments)
		if idx := strings.Index(defaultValue, " #"); idx >= 0 {
			defaultValue = strings.TrimSpace(defaultValue[:idx])
		}

		// Determine if value is multi-line (empty value followed by indented lines)
		isMultiLine := defaultValue == "" || defaultValue == "|" || defaultValue == ">" || defaultValue == "|-" || defaultValue == ">-"

		// Look for type comment above the variable declaration (#—| type)
		varType := ""
		if i > 0 {
			prevLine := strings.TrimSpace(lines[i-1])
			typeMatch := typeCommentRegex.FindStringSubmatch(prevLine)
			if typeMatch != nil {
				varType = strings.TrimSpace(typeMatch[1])
			}
		}

		// For multi-line values, collect the indented block as the default value
		// and look for description comment after the block
		description := ""
		if isMultiLine {
			// Collect indented lines as the default value representation
			j := i + 1
			var multiLineItems []string
			for j < len(lines) {
				nextRaw := lines[j]
				// Stop at non-indented, non-empty lines
				if nextRaw != "" && len(nextRaw) > 0 && nextRaw[0] != ' ' && nextRaw[0] != '\t' {
					break
				}
				// Collect non-empty indented lines
				trimmed := strings.TrimSpace(nextRaw)
				if trimmed != "" {
					multiLineItems = append(multiLineItems, trimmed)
				}
				j++
			}

			// Build a compact representation of the multi-line default value
			if len(multiLineItems) > 0 {
				defaultValue = formatMultiLineDefault(multiLineItems)
			}

			// Check if the line after the block is a description comment
			if j < len(lines) {
				nextLine := strings.TrimSpace(lines[j])
				descMatch := descCommentRegex.FindStringSubmatch(nextLine)
				if descMatch != nil {
					description = strings.TrimSpace(descMatch[1])
				}
			}
		} else {
			// Single-line value: look immediately below
			if i+1 < len(lines) {
				nextLine := strings.TrimSpace(lines[i+1])
				descMatch := descCommentRegex.FindStringSubmatch(nextLine)
				if descMatch != nil {
					description = strings.TrimSpace(descMatch[1])
				}
			}
		}

		// Store or update variable
		if existing, ok := varMap[varName]; ok {
			// Update with more info if available
			if varType != "" && existing.Type == "" {
				existing.Type = varType
			}
			if description != "" && existing.Description == "" {
				existing.Description = description
			}
			if defaultValue != "" && existing.Default == "" {
				existing.Default = defaultValue
			}
		} else {
			varMap[varName] = &RoleVariable{
				Name:        varName,
				Type:        varType,
				Default:     defaultValue,
				Description: description,
				Source:      sourceName,
			}
		}
	}

	return nil
}

// formatMultiLineDefault converts collected indented YAML lines into a compact
// single-line representation for display in the documentation table.
// Lists become: [item1, item2, ...], Maps become: {key1: val1, key2: val2, ...}
func formatMultiLineDefault(items []string) string {
	if len(items) == 0 {
		return ""
	}

	// Detect if it's a list (items start with "- ")
	isList := true
	for _, item := range items {
		if !strings.HasPrefix(item, "- ") && !strings.HasPrefix(item, "-\t") {
			isList = false
			break
		}
	}

	if isList {
		var listItems []string
		for _, item := range items {
			val := strings.TrimPrefix(item, "- ")
			val = strings.TrimPrefix(val, "-\t")
			val = strings.TrimSpace(val)
			listItems = append(listItems, val)
		}
		return "[" + strings.Join(listItems, ", ") + "]"
	}

	// Otherwise treat as map/dict (key: value pairs)
	var mapItems []string
	for _, item := range items {
		mapItems = append(mapItems, strings.TrimSpace(item))
	}
	return "{" + strings.Join(mapItems, ", ") + "}"
}

// scanTemplatesDir scans Jinja2 template files for {{ variable }} references
func scanTemplatesDir(templatesDir string, varMap map[string]*RoleVariable) error {
	return filepath.Walk(templatesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Process .j2, .jinja2, .yml, .yaml, .conf, .cfg and other template files
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".j2" || ext == ".jinja2" || ext == ".yml" || ext == ".yaml" ||
			ext == ".conf" || ext == ".cfg" || ext == ".ini" || ext == ".toml" ||
			ext == ".sh" || ext == ".txt" || ext == "" {
			return scanFileForJinja2Vars(path, "templates", varMap)
		}
		return nil
	})
}

// scanDirForJinja2Vars scans all YAML files in a directory for {{ variable }} references
func scanDirForJinja2Vars(dir, sourceName string, varMap map[string]*RoleVariable) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".yml" || ext == ".yaml" || ext == ".j2" || ext == ".jinja2" {
			return scanFileForJinja2Vars(path, sourceName, varMap)
		}
		return nil
	})
}

// scanFileForJinja2Vars extracts variable names from {{ variable }} patterns in a file.
// It does NOT extract type/description from templates — those come from defaults/vars YAML files.
func scanFileForJinja2Vars(filePath, sourceName string, varMap map[string]*RoleVariable) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	content := string(data)
	matches := jinja2VarRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		varName := strings.TrimSpace(match[1])

		// Skip Jinja2 built-in variables and common loop vars
		if isBuiltinVariable(varName) {
			continue
		}

		// Only add if not already known from YAML declarations
		if _, exists := varMap[varName]; !exists {
			varMap[varName] = &RoleVariable{
				Name:   varName,
				Source: sourceName,
			}
		}
	}

	return nil
}

// isBuiltinVariable returns true for Jinja2/Ansible built-in variables that
// should not be documented as role variables
func isBuiltinVariable(name string) bool {
	builtins := map[string]bool{
		// Jinja2 loop variables
		"loop":  true,
		"item":  true,
		"index": true,
		// Ansible built-in variables
		"ansible_hostname":           true,
		"ansible_os_family":          true,
		"ansible_distribution":       true,
		"ansible_architecture":       true,
		"ansible_managed":            true,
		"ansible_facts":              true,
		"ansible_env":                true,
		"ansible_user_id":            true,
		"ansible_default_ipv4":       true,
		"ansible_all_ipv4_addresses": true,
		"inventory_hostname":         true,
		"inventory_hostname_short":   true,
		"group_names":                true,
		"groups":                     true,
		"hostvars":                   true,
		"play_hosts":                 true,
		"ansible_play_hosts":         true,
		"ansible_play_batch":         true,
		"ansible_check_mode":         true,
		"ansible_version":            true,
		"role_path":                  true,
		"ansible_role_name":          true,
		"omit":                       true,
		"undefined":                  true,
		"none":                       true,
		"true":                       true,
		"false":                      true,
		"True":                       true,
		"False":                      true,
		"None":                       true,
	}
	return builtins[name]
}
