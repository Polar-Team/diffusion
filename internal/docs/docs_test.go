package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsBuiltinVariable(t *testing.T) {
	tests := []struct {
		name     string
		varName  string
		expected bool
	}{
		{"jinja2 loop item", "item", true},
		{"jinja2 loop var", "loop", true},
		{"jinja2 index", "index", true},
		{"ansible hostname", "ansible_hostname", true},
		{"ansible managed", "ansible_managed", true},
		{"ansible facts", "ansible_facts", true},
		{"inventory hostname", "inventory_hostname", true},
		{"hostvars", "hostvars", true},
		{"omit", "omit", true},
		{"true builtin", "true", true},
		{"false builtin", "false", true},
		{"None builtin", "None", true},
		{"custom variable", "app_name", false},
		{"custom port", "http_port", false},
		{"custom list", "packages", false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isBuiltinVariable(tt.varName)
			if got != tt.expected {
				t.Errorf("isBuiltinVariable(%q) = %v, want %v", tt.varName, got, tt.expected)
			}
		})
	}
}

func TestFormatMultiLineDefault(t *testing.T) {
	tests := []struct {
		name     string
		items    []string
		expected string
	}{
		{
			"empty list",
			[]string{},
			"",
		},
		{
			"yaml list items",
			[]string{"- nginx", "- curl", "- wget"},
			"[nginx, curl, wget]",
		},
		{
			"single list item",
			[]string{"- nginx"},
			"[nginx]",
		},
		{
			"map items",
			[]string{"key1: value1", "key2: value2"},
			"{key1: value1, key2: value2}",
		},
		{
			"single map item",
			[]string{"timeout: 30"},
			"{timeout: 30}",
		},
		{
			"list with quoted values",
			[]string{"- \"hello\"", "- \"world\""},
			"[\"hello\", \"world\"]",
		},
		{
			"map with complex values",
			[]string{"log_level: info", "max_retries: 3", "timeout: 30"},
			"{log_level: info, max_retries: 3, timeout: 30}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMultiLineDefault(tt.items)
			if got != tt.expected {
				t.Errorf("formatMultiLineDefault(%v) = %q, want %q", tt.items, got, tt.expected)
			}
		})
	}
}

func TestScanYAMLFile_SimpleVariables(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.yml")

	content := `---
#—| string
app_name: "my-app"
#—? The application name

#—| int
app_port: 8080
#—? TCP port for the application

#—| bool
debug_mode: false
#—? Enable debug mode
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile(filePath, "defaults/main.yml", varMap)
	if err != nil {
		t.Fatalf("scanYAMLFile failed: %v", err)
	}

	if len(varMap) != 3 {
		t.Fatalf("expected 3 variables, got %d", len(varMap))
	}

	// Check app_name
	v, ok := varMap["app_name"]
	if !ok {
		t.Fatal("expected app_name variable")
	}
	if v.Type != "string" {
		t.Errorf("app_name.Type = %q, want %q", v.Type, "string")
	}
	if v.Default != `"my-app"` {
		t.Errorf("app_name.Default = %q, want %q", v.Default, `"my-app"`)
	}
	if v.Description != "The application name" {
		t.Errorf("app_name.Description = %q, want %q", v.Description, "The application name")
	}

	// Check app_port
	v, ok = varMap["app_port"]
	if !ok {
		t.Fatal("expected app_port variable")
	}
	if v.Type != "int" {
		t.Errorf("app_port.Type = %q, want %q", v.Type, "int")
	}
	if v.Default != "8080" {
		t.Errorf("app_port.Default = %q, want %q", v.Default, "8080")
	}
	if v.Description != "TCP port for the application" {
		t.Errorf("app_port.Description = %q, want %q", v.Description, "TCP port for the application")
	}

	// Check debug_mode
	v, ok = varMap["debug_mode"]
	if !ok {
		t.Fatal("expected debug_mode variable")
	}
	if v.Type != "bool" {
		t.Errorf("debug_mode.Type = %q, want %q", v.Type, "bool")
	}
	if v.Default != "false" {
		t.Errorf("debug_mode.Default = %q, want %q", v.Default, "false")
	}
	if v.Description != "Enable debug mode" {
		t.Errorf("debug_mode.Description = %q, want %q", v.Description, "Enable debug mode")
	}
}

func TestScanYAMLFile_MultiLineValues(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.yml")

	content := `---
#—| list
packages:
  - nginx
  - curl
  - wget
#—? System packages to install

#—| map
config:
  log_level: info
  max_retries: 3
#—? Application configuration
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile(filePath, "defaults/main.yml", varMap)
	if err != nil {
		t.Fatalf("scanYAMLFile failed: %v", err)
	}

	if len(varMap) != 2 {
		t.Fatalf("expected 2 variables, got %d", len(varMap))
	}

	// Check packages (list)
	v, ok := varMap["packages"]
	if !ok {
		t.Fatal("expected packages variable")
	}
	if v.Type != "list" {
		t.Errorf("packages.Type = %q, want %q", v.Type, "list")
	}
	if v.Default != "[nginx, curl, wget]" {
		t.Errorf("packages.Default = %q, want %q", v.Default, "[nginx, curl, wget]")
	}
	if v.Description != "System packages to install" {
		t.Errorf("packages.Description = %q, want %q", v.Description, "System packages to install")
	}

	// Check config (map)
	v, ok = varMap["config"]
	if !ok {
		t.Fatal("expected config variable")
	}
	if v.Type != "map" {
		t.Errorf("config.Type = %q, want %q", v.Type, "map")
	}
	if v.Default != "{log_level: info, max_retries: 3}" {
		t.Errorf("config.Default = %q, want %q", v.Default, "{log_level: info, max_retries: 3}")
	}
	if v.Description != "Application configuration" {
		t.Errorf("config.Description = %q, want %q", v.Description, "Application configuration")
	}
}

func TestScanYAMLFile_NoAnnotations(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.yml")

	content := `---
simple_var: hello
another_var: 42
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile(filePath, "defaults/main.yml", varMap)
	if err != nil {
		t.Fatalf("scanYAMLFile failed: %v", err)
	}

	if len(varMap) != 2 {
		t.Fatalf("expected 2 variables, got %d", len(varMap))
	}

	v := varMap["simple_var"]
	if v.Type != "" {
		t.Errorf("simple_var.Type = %q, want empty", v.Type)
	}
	if v.Default != "hello" {
		t.Errorf("simple_var.Default = %q, want %q", v.Default, "hello")
	}
	if v.Description != "" {
		t.Errorf("simple_var.Description = %q, want empty", v.Description)
	}
}

func TestScanYAMLFile_SkipsIndentedKeys(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.yml")

	content := `---
#—| map
my_map:
  nested_key: nested_value
  another_key: another_value
#—? A map variable

top_level: works
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile(filePath, "defaults/main.yml", varMap)
	if err != nil {
		t.Fatalf("scanYAMLFile failed: %v", err)
	}

	// Should only find top-level variables, not nested keys
	if _, ok := varMap["nested_key"]; ok {
		t.Error("nested_key should NOT be captured as a variable")
	}
	if _, ok := varMap["another_key"]; ok {
		t.Error("another_key should NOT be captured as a variable")
	}
	if _, ok := varMap["my_map"]; !ok {
		t.Error("my_map should be captured as a variable")
	}
	if _, ok := varMap["top_level"]; !ok {
		t.Error("top_level should be captured as a variable")
	}
}

func TestScanYAMLFile_DashTypeComment(t *testing.T) {
	// Test that regular dash (-) also works for type comments, not just em-dash (—)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.yml")

	content := `---
#-| string
app_name: "test"
#-? The app name
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile(filePath, "defaults/main.yml", varMap)
	if err != nil {
		t.Fatalf("scanYAMLFile failed: %v", err)
	}

	v, ok := varMap["app_name"]
	if !ok {
		t.Fatal("expected app_name variable")
	}
	if v.Type != "string" {
		t.Errorf("app_name.Type = %q, want %q", v.Type, "string")
	}
	if v.Description != "The app name" {
		t.Errorf("app_name.Description = %q, want %q", v.Description, "The app name")
	}
}

func TestScanYAMLFile_NonexistentFile(t *testing.T) {
	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile("/nonexistent/path/main.yml", "defaults/main.yml", varMap)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestScanYAMLFile_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.yml")

	if err := os.WriteFile(filePath, []byte("---\n"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile(filePath, "defaults/main.yml", varMap)
	if err != nil {
		t.Fatalf("scanYAMLFile failed: %v", err)
	}

	if len(varMap) != 0 {
		t.Errorf("expected 0 variables, got %d", len(varMap))
	}
}

func TestScanFileForJinja2Vars(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "app.conf.j2")

	content := `server {
    listen {{ app_port }};
    server_name {{ app_name }};
    proxy_pass http://localhost:{{ backend_port }};
    timeout {{ http_timeout | default(30) }};
}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanFileForJinja2Vars(filePath, "templates", varMap)
	if err != nil {
		t.Fatalf("scanFileForJinja2Vars failed: %v", err)
	}

	expectedVars := []string{"app_port", "app_name", "backend_port", "http_timeout"}
	for _, name := range expectedVars {
		if _, ok := varMap[name]; !ok {
			t.Errorf("expected variable %q to be found", name)
		}
	}

	// All should have source "templates"
	for name, v := range varMap {
		if v.Source != "templates" {
			t.Errorf("variable %q source = %q, want %q", name, v.Source, "templates")
		}
	}
}

func TestScanFileForJinja2Vars_SkipsBuiltins(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "task.yml")

	content := `---
- name: Test task
  debug:
    msg: "{{ ansible_hostname }} {{ item }} {{ my_var }}"
  loop: "{{ my_list }}"
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanFileForJinja2Vars(filePath, "tasks", varMap)
	if err != nil {
		t.Fatalf("scanFileForJinja2Vars failed: %v", err)
	}

	// Should find my_var and my_list but NOT ansible_hostname or item
	if _, ok := varMap["my_var"]; !ok {
		t.Error("expected my_var to be found")
	}
	if _, ok := varMap["my_list"]; !ok {
		t.Error("expected my_list to be found")
	}
	if _, ok := varMap["ansible_hostname"]; ok {
		t.Error("ansible_hostname should be skipped as builtin")
	}
	if _, ok := varMap["item"]; ok {
		t.Error("item should be skipped as builtin")
	}
}

func TestScanFileForJinja2Vars_DoesNotOverwriteExisting(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "template.j2")

	content := `{{ app_name }}`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Pre-populate varMap with existing data
	varMap := map[string]*RoleVariable{
		"app_name": {
			Name:        "app_name",
			Type:        "string",
			Default:     "my-app",
			Description: "The app name",
			Source:      "defaults/main.yml",
		},
	}

	err := scanFileForJinja2Vars(filePath, "templates", varMap)
	if err != nil {
		t.Fatalf("scanFileForJinja2Vars failed: %v", err)
	}

	// Should NOT overwrite existing entry
	v := varMap["app_name"]
	if v.Source != "defaults/main.yml" {
		t.Errorf("app_name source should remain 'defaults/main.yml', got %q", v.Source)
	}
	if v.Type != "string" {
		t.Errorf("app_name type should remain 'string', got %q", v.Type)
	}
}

func TestScanRoleVariables_FullRole(t *testing.T) {
	roleDir := t.TempDir()

	// Create defaults/main.yml
	defaultsDir := filepath.Join(roleDir, "defaults")
	if err := os.MkdirAll(defaultsDir, 0755); err != nil {
		t.Fatalf("failed to create defaults dir: %v", err)
	}

	defaultsContent := `---
#—| string
app_name: "my-app"
#—? The application name

#—| int
app_port: 8080
#—? The port number

#—| list
packages:
  - nginx
  - curl
#—? Packages to install
`
	if err := os.WriteFile(filepath.Join(defaultsDir, "main.yml"), []byte(defaultsContent), 0644); err != nil {
		t.Fatalf("failed to write defaults: %v", err)
	}

	// Create templates directory with a Jinja2 file
	templatesDir := filepath.Join(roleDir, "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("failed to create templates dir: %v", err)
	}

	templateContent := `listen {{ app_port }};
server_name {{ app_name }};
backend {{ backend_url }};
`
	if err := os.WriteFile(filepath.Join(templatesDir, "app.conf.j2"), []byte(templateContent), 0644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	// Create tasks directory
	tasksDir := filepath.Join(roleDir, "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatalf("failed to create tasks dir: %v", err)
	}

	taskContent := `---
- name: Set log level
  lineinfile:
    line: "level={{ log_level }}"
`
	if err := os.WriteFile(filepath.Join(tasksDir, "main.yml"), []byte(taskContent), 0644); err != nil {
		t.Fatalf("failed to write task: %v", err)
	}

	// Run scan
	variables, err := ScanRoleVariables(roleDir)
	if err != nil {
		t.Fatalf("ScanRoleVariables failed: %v", err)
	}

	// Should find: app_name, app_port, packages (from defaults),
	// backend_url (from templates), log_level (from tasks)
	if len(variables) != 5 {
		t.Errorf("expected 5 variables, got %d", len(variables))
		for _, v := range variables {
			t.Logf("  found: %s (source: %s)", v.Name, v.Source)
		}
	}

	// Variables should be sorted by name
	for i := 1; i < len(variables); i++ {
		if variables[i].Name < variables[i-1].Name {
			t.Errorf("variables not sorted: %q comes after %q", variables[i].Name, variables[i-1].Name)
		}
	}

	// Check that defaults variables have full metadata
	varByName := make(map[string]RoleVariable)
	for _, v := range variables {
		varByName[v.Name] = v
	}

	if v, ok := varByName["app_name"]; ok {
		if v.Type != "string" {
			t.Errorf("app_name.Type = %q, want %q", v.Type, "string")
		}
		if v.Default != `"my-app"` {
			t.Errorf("app_name.Default = %q, want %q", v.Default, `"my-app"`)
		}
		if v.Source != "defaults/main.yml" {
			t.Errorf("app_name.Source = %q, want %q", v.Source, "defaults/main.yml")
		}
	} else {
		t.Error("expected app_name in results")
	}

	// Template-only vars should have no type/default
	if v, ok := varByName["backend_url"]; ok {
		if v.Type != "" {
			t.Errorf("backend_url.Type = %q, want empty", v.Type)
		}
		if v.Source != "templates/app.conf.j2" {
			t.Errorf("backend_url.Source = %q, want %q", v.Source, "templates/app.conf.j2")
		}
	} else {
		t.Error("expected backend_url in results")
	}

	// Task-only vars should have no type/default
	if v, ok := varByName["log_level"]; ok {
		if v.Type != "" {
			t.Errorf("log_level.Type = %q, want empty", v.Type)
		}
		if v.Source != "tasks/main.yml" {
			t.Errorf("log_level.Source = %q, want %q", v.Source, "tasks/main.yml")
		}
	} else {
		t.Error("expected log_level in results")
	}
}

func TestScanRoleVariables_EmptyRole(t *testing.T) {
	roleDir := t.TempDir()

	variables, err := ScanRoleVariables(roleDir)
	if err != nil {
		t.Fatalf("ScanRoleVariables failed: %v", err)
	}

	if len(variables) != 0 {
		t.Errorf("expected 0 variables for empty role, got %d", len(variables))
	}
}

func TestScanRoleVariables_OnlyDefaults(t *testing.T) {
	roleDir := t.TempDir()

	defaultsDir := filepath.Join(roleDir, "defaults")
	if err := os.MkdirAll(defaultsDir, 0755); err != nil {
		t.Fatalf("failed to create defaults dir: %v", err)
	}

	content := `---
#—| string
server_name: "localhost"
#—? The server hostname
`
	if err := os.WriteFile(filepath.Join(defaultsDir, "main.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write defaults: %v", err)
	}

	variables, err := ScanRoleVariables(roleDir)
	if err != nil {
		t.Fatalf("ScanRoleVariables failed: %v", err)
	}

	if len(variables) != 1 {
		t.Fatalf("expected 1 variable, got %d", len(variables))
	}

	if variables[0].Name != "server_name" {
		t.Errorf("expected variable 'server_name', got %q", variables[0].Name)
	}
}

func TestScanRoleVariables_VarsOverridesTemplates(t *testing.T) {
	roleDir := t.TempDir()

	// Create vars/main.yml with type info
	varsDir := filepath.Join(roleDir, "vars")
	if err := os.MkdirAll(varsDir, 0755); err != nil {
		t.Fatalf("failed to create vars dir: %v", err)
	}
	varsContent := `---
#—| string
my_var: "from_vars"
#—? Defined in vars
`
	if err := os.WriteFile(filepath.Join(varsDir, "main.yml"), []byte(varsContent), 0644); err != nil {
		t.Fatalf("failed to write vars: %v", err)
	}

	// Create template that references the same var
	templatesDir := filepath.Join(roleDir, "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("failed to create templates dir: %v", err)
	}
	templateContent := `value={{ my_var }}`
	if err := os.WriteFile(filepath.Join(templatesDir, "t.j2"), []byte(templateContent), 0644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	variables, err := ScanRoleVariables(roleDir)
	if err != nil {
		t.Fatalf("ScanRoleVariables failed: %v", err)
	}

	if len(variables) != 1 {
		t.Fatalf("expected 1 variable, got %d", len(variables))
	}

	// Should retain metadata from vars/main.yml, not overwritten by template scan
	v := variables[0]
	if v.Name != "my_var" {
		t.Errorf("expected 'my_var', got %q", v.Name)
	}
	if v.Type != "string" {
		t.Errorf("my_var.Type = %q, want %q", v.Type, "string")
	}
	if v.Source != "vars/main.yml" {
		t.Errorf("my_var.Source = %q, want %q", v.Source, "vars/main.yml")
	}
}

func TestScanYAMLFile_TrailingComment(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.yml")

	content := `---
timeout: 30 # seconds
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile(filePath, "defaults/main.yml", varMap)
	if err != nil {
		t.Fatalf("scanYAMLFile failed: %v", err)
	}

	v, ok := varMap["timeout"]
	if !ok {
		t.Fatal("expected timeout variable")
	}
	if v.Default != "30" {
		t.Errorf("timeout.Default = %q, want %q", v.Default, "30")
	}
}

func TestScanFileForJinja2Vars_WithFilters(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "template.j2")

	content := `{{ my_var | default("hello") }}
{{ another_var | upper }}
{{ plain_var }}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanFileForJinja2Vars(filePath, "templates", varMap)
	if err != nil {
		t.Fatalf("scanFileForJinja2Vars failed: %v", err)
	}

	expectedVars := []string{"my_var", "another_var", "plain_var"}
	for _, name := range expectedVars {
		if _, ok := varMap[name]; !ok {
			t.Errorf("expected variable %q to be found", name)
		}
	}
}

func TestScanRoleVariables_NonexistentDir(t *testing.T) {
	variables, err := ScanRoleVariables("/nonexistent/role/path")
	if err != nil {
		t.Fatalf("ScanRoleVariables should not fail for nonexistent dir, got: %v", err)
	}
	if len(variables) != 0 {
		t.Errorf("expected 0 variables, got %d", len(variables))
	}
}

func TestJinja2VarRegex(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{"simple var", "{{ my_var }}", []string{"my_var"}},
		{"var with filter", "{{ my_var | default('x') }}", []string{"my_var"}},
		{"no spaces", "{{my_var}}", []string{"my_var"}},
		{"extra spaces", "{{  my_var  }}", []string{"my_var"}},
		{"multiple vars", "{{ a }} and {{ b }}", []string{"a", "b"}},
		{"underscore start", "{{ _private }}", []string{"_private"}},
		{"with numbers", "{{ var123 }}", []string{"var123"}},
		{"not a var (number start)", "{{ 123abc }}", nil},
		{"empty braces", "{{ }}", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matches := jinja2VarRegex.FindAllStringSubmatch(tt.input, -1)
			var got []string
			for _, m := range matches {
				got = append(got, m[1])
			}
			if len(got) != len(tt.expected) {
				t.Errorf("got %v, want %v", got, tt.expected)
				return
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("match[%d] = %q, want %q", i, got[i], tt.expected[i])
				}
			}
		})
	}
}

func TestScanYAMLFile_RequiredMarker(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.yml")

	content := `---
#—| string
app_name: "my-app"
#—? The application name

#—| string
#—! postgres_password
#—? Database password (required, no default)

#—| int
#—! postgres_port
#—? PostgreSQL port number
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile(filePath, "defaults/main.yml", varMap)
	if err != nil {
		t.Fatalf("scanYAMLFile failed: %v", err)
	}

	if len(varMap) != 3 {
		t.Fatalf("expected 3 variables, got %d", len(varMap))
	}

	// Check regular variable
	v, ok := varMap["app_name"]
	if !ok {
		t.Fatal("expected app_name variable")
	}
	if v.Required {
		t.Error("app_name should NOT be required")
	}
	if v.Default != `"my-app"` {
		t.Errorf("app_name.Default = %q, want %q", v.Default, `"my-app"`)
	}

	// Check required variable: postgres_password
	v, ok = varMap["postgres_password"]
	if !ok {
		t.Fatal("expected postgres_password variable")
	}
	if !v.Required {
		t.Error("postgres_password should be required")
	}
	if v.Type != "string" {
		t.Errorf("postgres_password.Type = %q, want %q", v.Type, "string")
	}
	if v.Description != "Database password (required, no default)" {
		t.Errorf("postgres_password.Description = %q, want %q", v.Description, "Database password (required, no default)")
	}
	if v.Default != "" {
		t.Errorf("postgres_password.Default should be empty, got %q", v.Default)
	}

	// Check required variable: postgres_port
	v, ok = varMap["postgres_port"]
	if !ok {
		t.Fatal("expected postgres_port variable")
	}
	if !v.Required {
		t.Error("postgres_port should be required")
	}
	if v.Type != "int" {
		t.Errorf("postgres_port.Type = %q, want %q", v.Type, "int")
	}
	if v.Description != "PostgreSQL port number" {
		t.Errorf("postgres_port.Description = %q, want %q", v.Description, "PostgreSQL port number")
	}
}

func TestScanYAMLFile_RequiredMarkerDash(t *testing.T) {
	// Test with regular dash (-) instead of em-dash (—)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.yml")

	content := `---
#-| string
#-! api_key
#-? API key for authentication
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile(filePath, "defaults/main.yml", varMap)
	if err != nil {
		t.Fatalf("scanYAMLFile failed: %v", err)
	}

	v, ok := varMap["api_key"]
	if !ok {
		t.Fatal("expected api_key variable")
	}
	if !v.Required {
		t.Error("api_key should be required")
	}
	if v.Type != "string" {
		t.Errorf("api_key.Type = %q, want %q", v.Type, "string")
	}
	if v.Description != "API key for authentication" {
		t.Errorf("api_key.Description = %q, want %q", v.Description, "API key for authentication")
	}
}

func TestScanYAMLFile_RequiredMarkerNoAnnotations(t *testing.T) {
	// Required marker with no type or description
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.yml")

	content := `---
#-! bare_required_var
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile(filePath, "defaults/main.yml", varMap)
	if err != nil {
		t.Fatalf("scanYAMLFile failed: %v", err)
	}

	v, ok := varMap["bare_required_var"]
	if !ok {
		t.Fatal("expected bare_required_var variable")
	}
	if !v.Required {
		t.Error("bare_required_var should be required")
	}
	if v.Type != "" {
		t.Errorf("bare_required_var.Type should be empty, got %q", v.Type)
	}
	if v.Description != "" {
		t.Errorf("bare_required_var.Description should be empty, got %q", v.Description)
	}
	if v.Source != "defaults/main.yml" {
		t.Errorf("bare_required_var.Source = %q, want %q", v.Source, "defaults/main.yml")
	}
}

func TestScanRoleVariables_RequiredWithFullRole(t *testing.T) {
	roleDir := t.TempDir()

	// Create defaults/main.yml with both regular and required vars
	defaultsDir := filepath.Join(roleDir, "defaults")
	if err := os.MkdirAll(defaultsDir, 0755); err != nil {
		t.Fatalf("failed to create defaults dir: %v", err)
	}

	defaultsContent := `---
#—| string
app_name: "my-app"
#—? The application name

#—| string
#—! db_password
#—? Database password (must be provided)

#—| int
app_port: 8080
#—? The port number
`
	if err := os.WriteFile(filepath.Join(defaultsDir, "main.yml"), []byte(defaultsContent), 0644); err != nil {
		t.Fatalf("failed to write defaults: %v", err)
	}

	// Create tasks that reference the required var
	tasksDir := filepath.Join(roleDir, "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatalf("failed to create tasks dir: %v", err)
	}

	taskContent := `---
- name: Configure DB
  template:
    content: "password={{ db_password }}"
`
	if err := os.WriteFile(filepath.Join(tasksDir, "main.yml"), []byte(taskContent), 0644); err != nil {
		t.Fatalf("failed to write task: %v", err)
	}

	variables, err := ScanRoleVariables(roleDir)
	if err != nil {
		t.Fatalf("ScanRoleVariables failed: %v", err)
	}

	if len(variables) != 3 {
		t.Fatalf("expected 3 variables, got %d", len(variables))
		for _, v := range variables {
			t.Logf("  found: %s (source: %s, required: %v)", v.Name, v.Source, v.Required)
		}
	}

	// Check that db_password is required and retains metadata from defaults
	varByName := make(map[string]RoleVariable)
	for _, v := range variables {
		varByName[v.Name] = v
	}

	dbPass, ok := varByName["db_password"]
	if !ok {
		t.Fatal("expected db_password in results")
	}
	if !dbPass.Required {
		t.Error("db_password should be required")
	}
	if dbPass.Type != "string" {
		t.Errorf("db_password.Type = %q, want %q", dbPass.Type, "string")
	}
	if dbPass.Description != "Database password (must be provided)" {
		t.Errorf("db_password.Description = %q, want %q", dbPass.Description, "Database password (must be provided)")
	}
	if dbPass.Source != "defaults/main.yml" {
		t.Errorf("db_password.Source = %q, want %q", dbPass.Source, "defaults/main.yml")
	}

	// Regular vars should not be required
	if varByName["app_name"].Required {
		t.Error("app_name should NOT be required")
	}
	if varByName["app_port"].Required {
		t.Error("app_port should NOT be required")
	}
}

func TestGenerateVariablesSection_RequiredVariable(t *testing.T) {
	vars := []RoleVariable{
		{Name: "app_name", Type: "string", Default: `"my-app"`, Description: "App name", Source: "defaults/main.yml"},
		{Name: "db_password", Type: "string", Description: "Database password", Source: "defaults/main.yml", Required: true},
	}

	section := GenerateVariablesSection(vars)

	// Required variable should show **required** instead of a default value
	if !strings.Contains(section, "**required**") {
		t.Error("section should contain '**required**' for required variables")
	}

	// Regular variable should still have its default
	if !strings.Contains(section, "`\"my-app\"`") {
		t.Error("section should contain the default value for app_name")
	}

	// db_password should not have a backtick-wrapped default
	if strings.Contains(section, "`db_password`") && strings.Contains(section, "` `") {
		t.Error("db_password should not have an empty backtick-wrapped default")
	}
}

func TestIsExcludedVariable(t *testing.T) {
	tests := []struct {
		name     string
		varName  string
		expected bool
	}{
		{"underscore prefix", "_sd", true},
		{"underscore prefix 2", "_sp", true},
		{"underscore prefix longer", "_internal_var", true},
		{"single underscore", "_", true},
		{"normal variable", "app_name", false},
		{"contains underscore middle", "my_var", false},
		{"ends with underscore", "var_", false},
		{"empty string", "", false},
		{"double underscore", "__private", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExcludedVariable(tt.varName)
			if got != tt.expected {
				t.Errorf("isExcludedVariable(%q) = %v, want %v", tt.varName, got, tt.expected)
			}
		})
	}
}

func TestScanFileForJinja2Vars_SkipsExcludedPrefixes(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "task.yml")

	content := `---
- name: Test with internal vars
  debug:
    msg: "{{ app_name }} {{ _sd_internal }} {{ _sp_private }} {{ normal_var }}"
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanFileForJinja2Vars(filePath, "tasks/task.yml", varMap)
	if err != nil {
		t.Fatalf("scanFileForJinja2Vars failed: %v", err)
	}

	// Should find app_name and normal_var
	if _, ok := varMap["app_name"]; !ok {
		t.Error("expected app_name to be found")
	}
	if _, ok := varMap["normal_var"]; !ok {
		t.Error("expected normal_var to be found")
	}

	// Should NOT find _sd_internal or _sp_private
	if _, ok := varMap["_sd_internal"]; ok {
		t.Error("_sd_internal should be excluded (underscore prefix)")
	}
	if _, ok := varMap["_sp_private"]; ok {
		t.Error("_sp_private should be excluded (underscore prefix)")
	}
}

func TestScanFileForJinja2Vars_ExcludedVarStillKeptIfDeclared(t *testing.T) {
	// If a variable with excluded prefix is EXPLICITLY declared in defaults,
	// it should still be kept (the exclusion only applies to Jinja2 auto-discovery)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "template.j2")

	content := `{{ _special_var }}`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	// Pre-populate varMap with explicit declaration from defaults
	varMap := map[string]*RoleVariable{
		"_special_var": {
			Name:        "_special_var",
			Type:        "string",
			Default:     "declared",
			Description: "Explicitly declared underscore var",
			Source:      "defaults/main.yml",
			AllSources:  []string{"defaults/main.yml"},
		},
	}

	err := scanFileForJinja2Vars(filePath, "templates/template.j2", varMap)
	if err != nil {
		t.Fatalf("scanFileForJinja2Vars failed: %v", err)
	}

	// Should still exist — the exclusion doesn't remove already-declared vars
	v, ok := varMap["_special_var"]
	if !ok {
		t.Fatal("_special_var should still be in varMap (was explicitly declared)")
	}
	if v.Source != "defaults/main.yml" {
		t.Errorf("_special_var.Source should remain 'defaults/main.yml', got %q", v.Source)
	}
}

func TestScanRoleVariables_SourceIncludesFileName(t *testing.T) {
	roleDir := t.TempDir()

	// Create defaults/main.yml
	defaultsDir := filepath.Join(roleDir, "defaults")
	if err := os.MkdirAll(defaultsDir, 0755); err != nil {
		t.Fatalf("failed to create defaults dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(defaultsDir, "main.yml"), []byte("---\napp_name: test\n"), 0644); err != nil {
		t.Fatalf("failed to write defaults: %v", err)
	}

	// Create tasks/deploy.yml with a Jinja2 reference
	tasksDir := filepath.Join(roleDir, "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatalf("failed to create tasks dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tasksDir, "deploy.yml"), []byte("---\n- debug: msg=\"{{ deploy_host }}\"\n"), 0644); err != nil {
		t.Fatalf("failed to write task: %v", err)
	}

	// Create templates/nginx.conf.j2
	templatesDir := filepath.Join(roleDir, "templates")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		t.Fatalf("failed to create templates dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templatesDir, "nginx.conf.j2"), []byte("server_name {{ server_host }};\n"), 0644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	variables, err := ScanRoleVariables(roleDir)
	if err != nil {
		t.Fatalf("ScanRoleVariables failed: %v", err)
	}

	varByName := make(map[string]RoleVariable)
	for _, v := range variables {
		varByName[v.Name] = v
	}

	// Check that sources include the file name
	if v, ok := varByName["app_name"]; ok {
		if v.Source != "defaults/main.yml" {
			t.Errorf("app_name.Source = %q, want %q", v.Source, "defaults/main.yml")
		}
	} else {
		t.Error("expected app_name")
	}

	if v, ok := varByName["deploy_host"]; ok {
		if v.Source != "tasks/deploy.yml" {
			t.Errorf("deploy_host.Source = %q, want %q", v.Source, "tasks/deploy.yml")
		}
	} else {
		t.Error("expected deploy_host")
	}

	if v, ok := varByName["server_host"]; ok {
		if v.Source != "templates/nginx.conf.j2" {
			t.Errorf("server_host.Source = %q, want %q", v.Source, "templates/nginx.conf.j2")
		}
	} else {
		t.Error("expected server_host")
	}
}

func TestScanFileForJinja2Vars_ForLoopIteratorExcluded(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "template.j2")

	content := `{% for item in my_packages %}
  install {{ item }}
{% endfor %}

{% for key, value in my_config.items() %}
  {{ key }} = {{ value }}
{% endfor %}

{{ app_name }}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanFileForJinja2Vars(filePath, "templates/template.j2", varMap)
	if err != nil {
		t.Fatalf("scanFileForJinja2Vars failed: %v", err)
	}

	// "item", "key", "value" are iterators — should NOT be in varMap
	if _, ok := varMap["item"]; ok {
		t.Error("'item' is a for-loop iterator, should NOT be a role variable")
	}
	if _, ok := varMap["key"]; ok {
		t.Error("'key' is a for-loop iterator, should NOT be a role variable")
	}
	if _, ok := varMap["value"]; ok {
		t.Error("'value' is a for-loop iterator, should NOT be a role variable")
	}

	// "my_packages" and "my_config" are the source variables — SHOULD be in varMap
	if _, ok := varMap["my_packages"]; !ok {
		t.Error("'my_packages' is a for-loop source variable, should be a role variable")
	}
	if _, ok := varMap["my_config"]; !ok {
		t.Error("'my_config' is a for-loop source variable, should be a role variable")
	}

	// "app_name" is a regular {{ }} reference — SHOULD be in varMap
	if _, ok := varMap["app_name"]; !ok {
		t.Error("'app_name' should be a role variable")
	}
}

func TestScanFileForJinja2Vars_ForLoopWithFilters(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "template.j2")

	content := `{% for entry in my_list | sort %}
  {{ entry.name }}
{% endfor %}

{% for host in server_hosts | unique %}
  server {{ host }};
{% endfor %}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanFileForJinja2Vars(filePath, "templates/template.j2", varMap)
	if err != nil {
		t.Fatalf("scanFileForJinja2Vars failed: %v", err)
	}

	// Iterators should be excluded
	if _, ok := varMap["entry"]; ok {
		t.Error("'entry' is a for-loop iterator, should NOT be a role variable")
	}
	if _, ok := varMap["host"]; ok {
		t.Error("'host' is a for-loop iterator, should NOT be a role variable")
	}

	// Source variables (before filter) should be included
	if _, ok := varMap["my_list"]; !ok {
		t.Error("'my_list' should be a role variable (for-loop source)")
	}
	if _, ok := varMap["server_hosts"]; !ok {
		t.Error("'server_hosts' should be a role variable (for-loop source)")
	}
}

func TestScanFileForJinja2Vars_SetVarExcluded(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "template.j2")

	content := `{% set local_prefix = "myapp" %}
server_name {{ local_prefix }}.example.com;
port {{ app_port }};
env {{ env_name }};
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanFileForJinja2Vars(filePath, "templates/template.j2", varMap)
	if err != nil {
		t.Fatalf("scanFileForJinja2Vars failed: %v", err)
	}

	// "local_prefix" is set locally — should NOT be a role variable
	if _, ok := varMap["local_prefix"]; ok {
		t.Error("'local_prefix' is a {% set %} local variable, should NOT be a role variable")
	}

	// "app_port" and "env_name" are proper role variables (referenced in {{ }})
	if _, ok := varMap["app_port"]; !ok {
		t.Error("'app_port' should be a role variable")
	}
	if _, ok := varMap["env_name"]; !ok {
		t.Error("'env_name' should be a role variable")
	}
}

func TestScanFileForJinja2Vars_NestedForLoops(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "template.j2")

	content := `{% for server in servers %}
  {% for port in server.ports %}
    listen {{ port }};
  {% endfor %}
  name {{ server.name }};
{% endfor %}
{{ global_var }}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanFileForJinja2Vars(filePath, "templates/template.j2", varMap)
	if err != nil {
		t.Fatalf("scanFileForJinja2Vars failed: %v", err)
	}

	// "server" and "port" are iterators — excluded
	if _, ok := varMap["server"]; ok {
		t.Error("'server' is a for-loop iterator, should NOT be a role variable")
	}
	if _, ok := varMap["port"]; ok {
		t.Error("'port' is a for-loop iterator, should NOT be a role variable")
	}

	// "servers" is the source variable — included
	if _, ok := varMap["servers"]; !ok {
		t.Error("'servers' should be a role variable (for-loop source)")
	}

	// "global_var" is a regular reference — included
	if _, ok := varMap["global_var"]; !ok {
		t.Error("'global_var' should be a role variable")
	}
}

func TestScanFileForJinja2Vars_ForLoopSourceDeclaredLocally(t *testing.T) {
	// If the for-loop source is a {% set %} variable, it should NOT be treated as a role var
	dir := t.TempDir()
	filePath := filepath.Join(dir, "template.j2")

	content := `{% set local_list = [1, 2, 3] %}
{% for num in local_list %}
  {{ num }}
{% endfor %}
{{ external_var }}
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanFileForJinja2Vars(filePath, "templates/template.j2", varMap)
	if err != nil {
		t.Fatalf("scanFileForJinja2Vars failed: %v", err)
	}

	// "local_list" is set locally AND used as for-loop source — NOT a role variable
	if _, ok := varMap["local_list"]; ok {
		t.Error("'local_list' is a locally-set variable, should NOT be a role variable")
	}

	// "num" is an iterator — NOT a role variable
	if _, ok := varMap["num"]; ok {
		t.Error("'num' is a for-loop iterator, should NOT be a role variable")
	}

	// "external_var" is a proper role variable
	if _, ok := varMap["external_var"]; !ok {
		t.Error("'external_var' should be a role variable")
	}
}

func TestExtractBaseVariable(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"simple variable", "my_list", "my_list"},
		{"with method call", "my_dict.items()", "my_dict"},
		{"with attribute", "my_obj.attr", "my_obj"},
		{"with filter", "my_list | sort", "my_list"},
		{"with filter and method", "my_dict.items() | sort", "my_dict"},
		{"range function", "range(10)", "range"},
		{"empty string", "", ""},
		{"number literal", "123", ""},
		{"string literal", "'hello'", ""},
		{"complex filter chain", "my_var | default([]) | sort", "my_var"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBaseVariable(tt.input)
			if got != tt.expected {
				t.Errorf("extractBaseVariable(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestScanYAMLFile_OptionalMarker(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.yml")

	content := `---
#—| string
app_name: "my-app"
#—? The application name

#—| int
#—& http_timeout
#—? HTTP timeout (default set via Jinja2 default filter)

#—| bool
#—& debug_enabled
#—? Debug mode (defaults to false in template logic)
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile(filePath, "defaults/main.yml", varMap)
	if err != nil {
		t.Fatalf("scanYAMLFile failed: %v", err)
	}

	if len(varMap) != 3 {
		t.Fatalf("expected 3 variables, got %d", len(varMap))
	}

	// Check regular variable
	v, ok := varMap["app_name"]
	if !ok {
		t.Fatal("expected app_name variable")
	}
	if v.Required || v.Optional {
		t.Error("app_name should NOT be required or optional (it has a default)")
	}

	// Check optional variable: http_timeout
	v, ok = varMap["http_timeout"]
	if !ok {
		t.Fatal("expected http_timeout variable")
	}
	if !v.Optional {
		t.Error("http_timeout should be optional")
	}
	if v.Required {
		t.Error("http_timeout should NOT be required")
	}
	if v.Type != "int" {
		t.Errorf("http_timeout.Type = %q, want %q", v.Type, "int")
	}
	if v.Description != "HTTP timeout (default set via Jinja2 default filter)" {
		t.Errorf("http_timeout.Description = %q", v.Description)
	}

	// Check optional variable: debug_enabled
	v, ok = varMap["debug_enabled"]
	if !ok {
		t.Fatal("expected debug_enabled variable")
	}
	if !v.Optional {
		t.Error("debug_enabled should be optional")
	}
	if v.Type != "bool" {
		t.Errorf("debug_enabled.Type = %q, want %q", v.Type, "bool")
	}
}

func TestScanYAMLFile_OptionalMarkerDash(t *testing.T) {
	// Test with regular dash (-) instead of em-dash (—)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "main.yml")

	content := `---
#-| string
#-& fallback_url
#-? Fallback URL (uses default in template)
`
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	varMap := make(map[string]*RoleVariable)
	err := scanYAMLFile(filePath, "defaults/main.yml", varMap)
	if err != nil {
		t.Fatalf("scanYAMLFile failed: %v", err)
	}

	v, ok := varMap["fallback_url"]
	if !ok {
		t.Fatal("expected fallback_url variable")
	}
	if !v.Optional {
		t.Error("fallback_url should be optional")
	}
	if v.Type != "string" {
		t.Errorf("fallback_url.Type = %q, want %q", v.Type, "string")
	}
	if v.Description != "Fallback URL (uses default in template)" {
		t.Errorf("fallback_url.Description = %q", v.Description)
	}
}

func TestGenerateVariablesSection_OptionalVariable(t *testing.T) {
	vars := []RoleVariable{
		{Name: "app_name", Type: "string", Default: `"my-app"`, Description: "App name", Source: "defaults/main.yml"},
		{Name: "db_password", Type: "string", Description: "Database password", Source: "defaults/main.yml", Required: true},
		{Name: "http_timeout", Type: "int", Description: "Timeout", Source: "defaults/main.yml", Optional: true},
	}

	section := GenerateVariablesSection(vars)

	// Required variable should show **required**
	if !strings.Contains(section, "**required**") {
		t.Error("section should contain '**required**' for required variables")
	}

	// Optional variable should show *optional*
	if !strings.Contains(section, "*optional*") {
		t.Error("section should contain '*optional*' for optional variables")
	}

	// Regular variable should still have its default
	if !strings.Contains(section, "`\"my-app\"`") {
		t.Error("section should contain the default value for app_name")
	}
}

func TestScanRoleVariables_MixedRequiredOptional(t *testing.T) {
	roleDir := t.TempDir()

	defaultsDir := filepath.Join(roleDir, "defaults")
	if err := os.MkdirAll(defaultsDir, 0755); err != nil {
		t.Fatalf("failed to create defaults dir: %v", err)
	}

	content := `---
#—| string
app_name: "my-app"
#—? App name

#—| string
#—! db_password
#—? Database password (required)

#—| int
#—& http_timeout
#—? HTTP timeout (optional, Jinja2 default)

#—| int
app_port: 8080
#—? Port
`
	if err := os.WriteFile(filepath.Join(defaultsDir, "main.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write defaults: %v", err)
	}

	variables, err := ScanRoleVariables(roleDir)
	if err != nil {
		t.Fatalf("ScanRoleVariables failed: %v", err)
	}

	if len(variables) != 4 {
		t.Fatalf("expected 4 variables, got %d", len(variables))
	}

	varByName := make(map[string]RoleVariable)
	for _, v := range variables {
		varByName[v.Name] = v
	}

	// app_name: regular (has default)
	if varByName["app_name"].Required || varByName["app_name"].Optional {
		t.Error("app_name should be regular (not required, not optional)")
	}

	// db_password: required
	if !varByName["db_password"].Required {
		t.Error("db_password should be required")
	}
	if varByName["db_password"].Optional {
		t.Error("db_password should NOT be optional")
	}

	// http_timeout: optional
	if !varByName["http_timeout"].Optional {
		t.Error("http_timeout should be optional")
	}
	if varByName["http_timeout"].Required {
		t.Error("http_timeout should NOT be required")
	}

	// app_port: regular (has default)
	if varByName["app_port"].Required || varByName["app_port"].Optional {
		t.Error("app_port should be regular (not required, not optional)")
	}
}
