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
	Name        string   // Variable name
	Type        string   // Type from #—| comment (string, int, list, map, bool, etc.)
	Default     string   // Default value from YAML declaration
	Description string   // Description from #—? comment
	Source      string   // Primary source file (e.g., "defaults/main.yml")
	AllSources  []string // All sources where this variable was declared (for duplicate detection)
	IsDuplicate bool     // True if variable is declared (non-Jinja2) in multiple YAML sources of the SAME type (e.g., two defaults files)
	HasVarsRef  bool     // True if variable exists in both defaults/ and vars/ (informational, not a critical issue)
	Required    bool     // True if variable is marked with #—! (required, no default)
	Optional    bool     // True if variable is marked with #—& (optional, default set in Jinja2 logic)
}

// jinja2VarRegex matches {{ variable_name }} patterns in templates
var jinja2VarRegex = regexp.MustCompile(`\{\{\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*(?:\|[^}]*)?\}\}`)

// yamlVarDeclRegex matches YAML variable declarations like "variable_name: value"
var yamlVarDeclRegex = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*):\s*(.*)$`)

// typeCommentRegex matches type annotation comments: #—| type
var typeCommentRegex = regexp.MustCompile(`^#\s*[—\-]\|\s*(.+)$`)

// descCommentRegex matches description comments: #—? description
var descCommentRegex = regexp.MustCompile(`^#\s*[—\-]\?\s*(.+)$`)

// requiredCommentRegex matches required variable markers: #—! variable_name
var requiredCommentRegex = regexp.MustCompile(`^#\s*[—\-]!\s*([a-zA-Z_][a-zA-Z0-9_]*)$`)

// optionalCommentRegex matches optional variable markers: #—& variable_name
var optionalCommentRegex = regexp.MustCompile(`^#\s*[—\-]&\s*([a-zA-Z_][a-zA-Z0-9_]*)$`)

// forLoopRegex matches Jinja2 for-loops: {% for <iterators> in <expression> %}
// Captures: group 1 = iterator(s), group 2 = source expression
var forLoopRegex = regexp.MustCompile(`\{%[-\s]*for\s+(.+?)\s+in\s+(.+?)\s*[-]?%\}`)

// setVarRegex matches Jinja2 set statements: {% set var = expression %}
// Captures: group 1 = variable name being set
var setVarRegex = regexp.MustCompile(`\{%[-\s]*set\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*=`)

// ScanRoleVariables scans an Ansible role directory and extracts all documented variables.
// It looks in defaults/main.yml, vars/main.yml, and template files for {{ }} interpolation.
//
// Priority: defaults/ is the authoritative source. If a variable is declared in defaults/,
// its type, default value, description, and source are preserved regardless of other sources.
// Variables declared in multiple YAML sources (not Jinja2 references) are flagged as duplicates.
func ScanRoleVariables(roleDir string) ([]RoleVariable, error) {
	varMap := make(map[string]*RoleVariable)

	// 1. Scan all YAML files under defaults/ — HIGHEST PRIORITY source for role variables
	defaultsDir := filepath.Join(roleDir, "defaults")
	if info, err := os.Stat(defaultsDir); err == nil && info.IsDir() {
		if err := scanYAMLDir(defaultsDir, "defaults", varMap); err != nil {
			return nil, fmt.Errorf("scanning defaults: %w", err)
		}
	}

	// 2. Scan all YAML files under vars/ — supplements defaults but does NOT override
	varsDir := filepath.Join(roleDir, "vars")
	if info, err := os.Stat(varsDir); err == nil && info.IsDir() {
		if err := scanYAMLDir(varsDir, "vars", varMap); err != nil {
			return nil, fmt.Errorf("scanning vars: %w", err)
		}
	}

	// 3. Scan template files for {{ variable }} references (Jinja2 — not treated as duplicates)
	templatesDir := filepath.Join(roleDir, "templates")
	if info, err := os.Stat(templatesDir); err == nil && info.IsDir() {
		if err := scanTemplatesDir(templatesDir, varMap); err != nil {
			return nil, fmt.Errorf("scanning templates: %w", err)
		}
	}

	// 4. Scan tasks for {{ variable }} references (Jinja2 — not treated as duplicates)
	tasksDir := filepath.Join(roleDir, "tasks")
	if info, err := os.Stat(tasksDir); err == nil && info.IsDir() {
		if err := scanDirForJinja2Vars(tasksDir, "tasks", varMap); err != nil {
			return nil, fmt.Errorf("scanning tasks: %w", err)
		}
	}

	// 5. Mark duplicates — variables declared (non-Jinja2) in multiple YAML sources
	// Distinguish between true duplicates (multiple sources of the same type) and
	// vars/defaults cross-references (acceptable for static role bindings).
	for _, v := range varMap {
		if len(v.AllSources) > 1 {
			hasDefaults := false
			hasVars := false
			defaultsCount := 0
			varsCount := 0
			for _, src := range v.AllSources {
				if strings.HasPrefix(src, "defaults") {
					hasDefaults = true
					defaultsCount++
				} else if strings.HasPrefix(src, "vars") {
					hasVars = true
					varsCount++
				}
			}
			// Cross-reference between defaults/ and vars/ is informational, not a critical duplicate
			if hasDefaults && hasVars && defaultsCount <= 1 && varsCount <= 1 {
				v.HasVarsRef = true
			} else {
				// True duplicate: same var in multiple files of the same source type
				v.IsDuplicate = true
			}
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

// scanYAMLDir walks a directory and scans all .yml/.yaml files for variable declarations.
func scanYAMLDir(dir, sourceName string, varMap map[string]*RoleVariable) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".yml" || ext == ".yaml" {
			// Use relative path from the dir as the source name
			rel, _ := filepath.Rel(dir, path)
			return scanYAMLFile(path, sourceName+"/"+rel, varMap)
		}
		return nil
	})
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

		// Check for required variable marker: #—! variable_name
		if strings.HasPrefix(line, "#") {
			reqMatch := requiredCommentRegex.FindStringSubmatch(line)
			if reqMatch != nil {
				varName := reqMatch[1]

				// Look for type comment above (#—| type)
				varType := ""
				if i > 0 {
					prevLine := strings.TrimSpace(lines[i-1])
					typeMatch := typeCommentRegex.FindStringSubmatch(prevLine)
					if typeMatch != nil {
						varType = strings.TrimSpace(typeMatch[1])
					}
				}

				// Look for description comment below (#—? description)
				description := ""
				if i+1 < len(lines) {
					nextLine := strings.TrimSpace(lines[i+1])
					descMatch := descCommentRegex.FindStringSubmatch(nextLine)
					if descMatch != nil {
						description = strings.TrimSpace(descMatch[1])
					}
				}

				// Store required variable
				if existing, ok := varMap[varName]; ok {
					existing.Required = true
					existing.AllSources = append(existing.AllSources, sourceName)
					if varType != "" && existing.Type == "" {
						existing.Type = varType
					}
					if description != "" && existing.Description == "" {
						existing.Description = description
					}
				} else {
					varMap[varName] = &RoleVariable{
						Name:        varName,
						Type:        varType,
						Description: description,
						Source:      sourceName,
						AllSources:  []string{sourceName},
						Required:    true,
					}
				}
				continue
			}

			// Check for optional variable marker: #—& variable_name
			optMatch := optionalCommentRegex.FindStringSubmatch(line)
			if optMatch != nil {
				varName := optMatch[1]

				// Look for type comment above (#—| type)
				varType := ""
				if i > 0 {
					prevLine := strings.TrimSpace(lines[i-1])
					typeMatch := typeCommentRegex.FindStringSubmatch(prevLine)
					if typeMatch != nil {
						varType = strings.TrimSpace(typeMatch[1])
					}
				}

				// Look for description comment below (#—? description)
				description := ""
				if i+1 < len(lines) {
					nextLine := strings.TrimSpace(lines[i+1])
					descMatch := descCommentRegex.FindStringSubmatch(nextLine)
					if descMatch != nil {
						description = strings.TrimSpace(descMatch[1])
					}
				}

				// Store optional variable
				if existing, ok := varMap[varName]; ok {
					existing.Optional = true
					existing.AllSources = append(existing.AllSources, sourceName)
					if varType != "" && existing.Type == "" {
						existing.Type = varType
					}
					if description != "" && existing.Description == "" {
						existing.Description = description
					}
				} else {
					varMap[varName] = &RoleVariable{
						Name:        varName,
						Type:        varType,
						Description: description,
						Source:      sourceName,
						AllSources:  []string{sourceName},
						Optional:    true,
					}
				}
				continue
			}
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

		// Store or update variable — defaults/ has highest priority
		if existing, ok := varMap[varName]; ok {
			// Track this as an additional source (duplicate YAML declaration)
			existing.AllSources = append(existing.AllSources, sourceName)

			// Only fill in MISSING metadata — never overwrite data from defaults/
			if strings.HasPrefix(existing.Source, "defaults") {
				// defaults/ is authoritative: only fill empty fields, keep everything else
				if varType != "" && existing.Type == "" {
					existing.Type = varType
				}
				if description != "" && existing.Description == "" {
					existing.Description = description
				}
				// Do NOT overwrite Default from defaults — it is the canonical value
			} else {
				// Neither source is defaults: fill missing info from later source
				if varType != "" && existing.Type == "" {
					existing.Type = varType
				}
				if description != "" && existing.Description == "" {
					existing.Description = description
				}
				if defaultValue != "" && existing.Default == "" {
					existing.Default = defaultValue
				}
			}
		} else {
			varMap[varName] = &RoleVariable{
				Name:        varName,
				Type:        varType,
				Default:     defaultValue,
				Description: description,
				Source:      sourceName,
				AllSources:  []string{sourceName},
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
			rel, _ := filepath.Rel(templatesDir, path)
			return scanFileForJinja2Vars(path, "templates/"+rel, varMap)
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
			rel, _ := filepath.Rel(dir, path)
			return scanFileForJinja2Vars(path, sourceName+"/"+rel, varMap)
		}
		return nil
	})
}

// scanFileForJinja2Vars extracts variable names from {{ variable }} patterns in a file.
// It does NOT extract type/description from templates — those come from defaults/vars YAML files.
// Variables with excluded prefixes (e.g., _sd, _sp) are skipped unless already declared
// with explicit annotations in defaults/vars.
//
// For-loop handling:
//   - {% for item in my_list %} → "item" is an iterator (excluded), "my_list" is a proper variable
//   - {% set local_var = ... %} → "local_var" is a local variable (excluded)
func scanFileForJinja2Vars(filePath, sourceName string, varMap map[string]*RoleVariable) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}

	content := string(data)

	// 1. Extract for-loop iterators and their source variables
	//    Pattern: {% for <iter> in <variable> %}
	//    Also handles: {% for <key>, <value> in <variable>.items() %}
	localVars := make(map[string]bool)
	forLoopVars := make(map[string]bool) // variables used after "in" — these are proper role vars

	forMatches := forLoopRegex.FindAllStringSubmatch(content, -1)
	for _, m := range forMatches {
		iteratorsPart := strings.TrimSpace(m[1]) // e.g., "item" or "key, value"
		sourcePart := strings.TrimSpace(m[2])    // e.g., "my_list" or "my_dict.items()"

		// Mark all iterators as local (not role variables)
		for _, iter := range strings.Split(iteratorsPart, ",") {
			iter = strings.TrimSpace(iter)
			if iter != "" {
				localVars[iter] = true
			}
		}

		// Extract the base variable name from the source part
		// Handles: my_list, my_dict.items(), my_dict | dict2items, etc.
		sourceVar := extractBaseVariable(sourcePart)
		if sourceVar != "" {
			forLoopVars[sourceVar] = true
		}
	}

	// 2. Extract {% set var = ... %} local variable assignments
	setMatches := setVarRegex.FindAllStringSubmatch(content, -1)
	for _, m := range setMatches {
		localVarName := strings.TrimSpace(m[1])
		if localVarName != "" {
			localVars[localVarName] = true
		}
	}

	// 3. Add for-loop source variables as proper role variables
	for varName := range forLoopVars {
		if isBuiltinVariable(varName) || isExcludedVariable(varName) {
			continue
		}
		// Don't add if it's also a local var (set in same file)
		if localVars[varName] {
			continue
		}
		if _, exists := varMap[varName]; !exists {
			varMap[varName] = &RoleVariable{
				Name:       varName,
				Source:     sourceName,
				AllSources: []string{},
			}
		}
	}

	// 4. Extract {{ variable }} references, skipping iterators and local vars
	matches := jinja2VarRegex.FindAllStringSubmatch(content, -1)

	for _, match := range matches {
		varName := strings.TrimSpace(match[1])

		// Skip Jinja2 built-in variables and common loop vars
		if isBuiltinVariable(varName) {
			continue
		}

		// Skip variables with excluded prefixes (internal/private vars like _sd, _sp)
		if isExcludedVariable(varName) {
			continue
		}

		// Skip for-loop iterators and locally-set variables
		if localVars[varName] {
			continue
		}

		// Only add if not already known from YAML declarations
		if _, exists := varMap[varName]; !exists {
			varMap[varName] = &RoleVariable{
				Name:       varName,
				Source:     sourceName,
				AllSources: []string{}, // Jinja2 references don't count as YAML declarations
			}
		}
	}

	return nil
}

// extractBaseVariable extracts the root variable name from a Jinja2 expression.
// Examples:
//
//	"my_list"              → "my_list"
//	"my_dict.items()"      → "my_dict"
//	"my_list | sort"       → "my_list"
//	"range(10)"            → "" (built-in function, not a variable)
func extractBaseVariable(expr string) string {
	expr = strings.TrimSpace(expr)

	// Strip filter expressions (everything after |)
	if idx := strings.Index(expr, "|"); idx >= 0 {
		expr = strings.TrimSpace(expr[:idx])
	}

	// Strip method calls and attribute access (take only the first identifier)
	if idx := strings.IndexAny(expr, ".("); idx >= 0 {
		expr = expr[:idx]
	}

	expr = strings.TrimSpace(expr)

	// Validate it's a proper identifier
	if expr == "" {
		return ""
	}
	if !isValidIdentifier(expr) {
		return ""
	}

	return expr
}

// isValidIdentifier checks if a string is a valid Python/Jinja2 identifier
func isValidIdentifier(s string) bool {
	if len(s) == 0 {
		return false
	}
	// First char must be letter or underscore
	if s[0] != '_' && !(s[0] >= 'a' && s[0] <= 'z') && !(s[0] >= 'A' && s[0] <= 'Z') {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if c != '_' && !(c >= 'a' && c <= 'z') && !(c >= 'A' && c <= 'Z') && !(c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

// DefaultExcludePrefixes defines variable name prefixes that are excluded from documentation
// by default. Variables starting with these prefixes are considered internal/private and
// will only appear in docs if explicitly annotated with #-| or #-! markers.
var DefaultExcludePrefixes = []string{"_"}

// isExcludedVariable returns true if a variable name matches an excluded prefix pattern.
// Variables starting with underscore or other configured prefixes are considered internal.
func isExcludedVariable(name string) bool {
	for _, prefix := range DefaultExcludePrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
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
