package docs

import (
	"os"
	"path/filepath"
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
		if v.Source != "templates" {
			t.Errorf("backend_url.Source = %q, want %q", v.Source, "templates")
		}
	} else {
		t.Error("expected backend_url in results")
	}

	// Task-only vars should have no type/default
	if v, ok := varByName["log_level"]; ok {
		if v.Type != "" {
			t.Errorf("log_level.Type = %q, want empty", v.Type)
		}
		if v.Source != "tasks" {
			t.Errorf("log_level.Source = %q, want %q", v.Source, "tasks")
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
