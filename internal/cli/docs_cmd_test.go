package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewDocsCmd(t *testing.T) {
	cli := &CLI{}
	cmd := NewDocsCmd(cli)

	if cmd.Use != "docs" {
		t.Errorf("expected Use 'docs', got %q", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("expected non-empty Short description")
	}
	if cmd.Long == "" {
		t.Error("expected non-empty Long description")
	}
	if cmd.RunE == nil {
		t.Error("expected RunE function to be set")
	}
}

func TestNewDocsCmd_Flags(t *testing.T) {
	cli := &CLI{}
	cmd := NewDocsCmd(cli)

	// Check --path flag exists
	pathFlag := cmd.Flags().Lookup("path")
	if pathFlag == nil {
		t.Fatal("expected --path flag to exist")
	}
	if pathFlag.Shorthand != "p" {
		t.Errorf("--path shorthand = %q, want %q", pathFlag.Shorthand, "p")
	}
	if pathFlag.DefValue != "." {
		t.Errorf("--path default = %q, want %q", pathFlag.DefValue, ".")
	}

	// Check --dry-run flag exists
	dryRunFlag := cmd.Flags().Lookup("dry-run")
	if dryRunFlag == nil {
		t.Fatal("expected --dry-run flag to exist")
	}
	if dryRunFlag.DefValue != "false" {
		t.Errorf("--dry-run default = %q, want %q", dryRunFlag.DefValue, "false")
	}
}


func TestRunDocs_ValidRole(t *testing.T) {
	roleDir := t.TempDir()

	// Create defaults/main.yml
	defaultsDir := filepath.Join(roleDir, "defaults")
	if err := os.MkdirAll(defaultsDir, 0755); err != nil {
		t.Fatalf("failed to create defaults dir: %v", err)
	}
	content := `---
#—| string
app_name: "test-app"
#—? The application name

#—| int
app_port: 9090
#—? The port number
`
	if err := os.WriteFile(filepath.Join(defaultsDir, "main.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write defaults: %v", err)
	}

	// Run docs (not dry-run)
	err := runDocs(roleDir, false)
	if err != nil {
		t.Fatalf("runDocs failed: %v", err)
	}

	// Verify README.md was created
	readmePath := filepath.Join(roleDir, "README.md")
	readmeContent, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	readme := string(readmeContent)
	if !strings.Contains(readme, "app_name") {
		t.Error("README should contain app_name")
	}
	if !strings.Contains(readme, "app_port") {
		t.Error("README should contain app_port")
	}
	if !strings.Contains(readme, "<!-- begin role_variables -->") {
		t.Error("README should contain begin marker")
	}
	if !strings.Contains(readme, "<!-- end role_variables -->") {
		t.Error("README should contain end marker")
	}
}

func TestRunDocs_DryRun(t *testing.T) {
	roleDir := t.TempDir()

	// Create defaults/main.yml
	defaultsDir := filepath.Join(roleDir, "defaults")
	if err := os.MkdirAll(defaultsDir, 0755); err != nil {
		t.Fatalf("failed to create defaults dir: %v", err)
	}
	content := `---
my_var: hello
`
	if err := os.WriteFile(filepath.Join(defaultsDir, "main.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write defaults: %v", err)
	}

	// Run in dry-run mode
	err := runDocs(roleDir, true)
	if err != nil {
		t.Fatalf("runDocs dry-run failed: %v", err)
	}

	// README.md should NOT be created in dry-run mode
	readmePath := filepath.Join(roleDir, "README.md")
	if _, err := os.Stat(readmePath); !os.IsNotExist(err) {
		t.Error("README.md should not be created in dry-run mode")
	}
}


func TestRunDocs_NonexistentPath(t *testing.T) {
	err := runDocs("/nonexistent/path/role", false)
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should mention 'does not exist', got: %v", err)
	}
}

func TestRunDocs_FileNotDirectory(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "not-a-dir.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	err := runDocs(filePath, false)
	if err == nil {
		t.Error("expected error for file path (not directory)")
	}
	if !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("error should mention 'is not a directory', got: %v", err)
	}
}

func TestRunDocs_EmptyRole(t *testing.T) {
	roleDir := t.TempDir()

	// Empty role should not error, just report no variables
	err := runDocs(roleDir, false)
	if err != nil {
		t.Fatalf("runDocs should not fail for empty role: %v", err)
	}

	// README should NOT be created when no variables found
	readmePath := filepath.Join(roleDir, "README.md")
	if _, err := os.Stat(readmePath); !os.IsNotExist(err) {
		t.Error("README.md should not be created when no variables found")
	}
}

func TestRunDocs_CurrentDirectory(t *testing.T) {
	// Test that "." or empty path resolves to cwd
	roleDir := t.TempDir()
	defaultsDir := filepath.Join(roleDir, "defaults")
	if err := os.MkdirAll(defaultsDir, 0755); err != nil {
		t.Fatalf("failed to create defaults dir: %v", err)
	}
	content := `---
test_var: value
`
	if err := os.WriteFile(filepath.Join(defaultsDir, "main.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write defaults: %v", err)
	}

	// Change to role directory
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(roleDir)

	err := runDocs(".", false)
	if err != nil {
		t.Fatalf("runDocs with '.' path failed: %v", err)
	}

	// Verify README was created
	readmePath := filepath.Join(roleDir, "README.md")
	if _, err := os.Stat(readmePath); os.IsNotExist(err) {
		t.Error("README.md should be created")
	}
}

func TestRunDocs_UpdatesExistingReadme(t *testing.T) {
	roleDir := t.TempDir()

	// Create existing README
	readmePath := filepath.Join(roleDir, "README.md")
	existing := "# My Role\n\nExisting content.\n"
	if err := os.WriteFile(readmePath, []byte(existing), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	// Create defaults
	defaultsDir := filepath.Join(roleDir, "defaults")
	if err := os.MkdirAll(defaultsDir, 0755); err != nil {
		t.Fatalf("failed to create defaults dir: %v", err)
	}
	content := `---
#—| string
my_var: "value"
#—? My variable description
`
	if err := os.WriteFile(filepath.Join(defaultsDir, "main.yml"), []byte(content), 0644); err != nil {
		t.Fatalf("failed to write defaults: %v", err)
	}

	err := runDocs(roleDir, false)
	if err != nil {
		t.Fatalf("runDocs failed: %v", err)
	}

	readmeContent, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README: %v", err)
	}

	readme := string(readmeContent)

	// Existing content preserved
	if !strings.Contains(readme, "# My Role") {
		t.Error("existing heading should be preserved")
	}
	if !strings.Contains(readme, "Existing content.") {
		t.Error("existing content should be preserved")
	}

	// New content added
	if !strings.Contains(readme, "my_var") {
		t.Error("variable should be in README")
	}
	if !strings.Contains(readme, "My variable description") {
		t.Error("description should be in README")
	}
}


func TestNewDocsCmd_HelpContainsExamples(t *testing.T) {
	cli := &CLI{}
	cmd := NewDocsCmd(cli)

	if !strings.Contains(cmd.Long, "defaults/main.yml") {
		t.Error("help should mention defaults/main.yml")
	}
	if !strings.Contains(cmd.Long, "templates/") {
		t.Error("help should mention templates/")
	}
	if !strings.Contains(cmd.Long, "tasks/") {
		t.Error("help should mention tasks/")
	}
	if !strings.Contains(cmd.Long, "<!-- begin role_variables -->") {
		t.Error("help should mention begin marker")
	}
	if !strings.Contains(cmd.Long, "<!-- end role_variables -->") {
		t.Error("help should mention end marker")
	}
	if !strings.Contains(cmd.Long, "--dry-run") {
		t.Error("help should mention --dry-run")
	}
}

func TestRunDocs_FullIntegration(t *testing.T) {
	roleDir := t.TempDir()

	// Create a complete role structure
	dirs := []string{"defaults", "vars", "templates", "tasks"}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(roleDir, d), 0755); err != nil {
			t.Fatalf("failed to create dir %s: %v", d, err)
		}
	}

	// defaults/main.yml
	defaults := `---
#—| string
app_name: "integration-test"
#—? Application name for integration test

#—| list
app_ports:
  - 8080
  - 8443
#—? Ports the app listens on

#—| map
app_settings:
  env: production
  workers: 4
#—? Application settings dictionary
`
	if err := os.WriteFile(filepath.Join(roleDir, "defaults", "main.yml"), []byte(defaults), 0644); err != nil {
		t.Fatalf("failed to write defaults: %v", err)
	}

	// vars/main.yml
	vars := `---
#—| string
internal_var: "secret"
#—? Internal variable from vars
`
	if err := os.WriteFile(filepath.Join(roleDir, "vars", "main.yml"), []byte(vars), 0644); err != nil {
		t.Fatalf("failed to write vars: %v", err)
	}

	// templates/app.conf.j2
	template := `server {{ app_name }} {
    listen {{ app_ports }};
    backend {{ backend_host }}:{{ backend_port }};
}
`
	if err := os.WriteFile(filepath.Join(roleDir, "templates", "app.conf.j2"), []byte(template), 0644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	// tasks/main.yml
	task := `---
- name: Deploy
  command: "deploy {{ deploy_target }}"
`
	if err := os.WriteFile(filepath.Join(roleDir, "tasks", "main.yml"), []byte(task), 0644); err != nil {
		t.Fatalf("failed to write task: %v", err)
	}

	// Create existing README
	readme := "# Integration Test Role\n\nThis tests the full workflow.\n"
	if err := os.WriteFile(filepath.Join(roleDir, "README.md"), []byte(readme), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	// Run docs
	err := runDocs(roleDir, false)
	if err != nil {
		t.Fatalf("runDocs integration test failed: %v", err)
	}

	// Read result
	content, err := os.ReadFile(filepath.Join(roleDir, "README.md"))
	if err != nil {
		t.Fatalf("failed to read README: %v", err)
	}
	result := string(content)

	// Verify all expected variables are present
	expectedVars := []string{
		"app_name", "app_ports", "app_settings",
		"internal_var", "backend_host", "backend_port", "deploy_target",
	}
	for _, varName := range expectedVars {
		if !strings.Contains(result, varName) {
			t.Errorf("expected variable %q in README output", varName)
		}
	}

	// Verify typed variables have their types
	if !strings.Contains(result, "string") {
		t.Error("README should contain 'string' type")
	}
	if !strings.Contains(result, "list") {
		t.Error("README should contain 'list' type")
	}
	if !strings.Contains(result, "map") {
		t.Error("README should contain 'map' type")
	}

	// Verify list default is shown
	if !strings.Contains(result, "[8080, 8443]") {
		t.Error("README should contain list default '[8080, 8443]'")
	}

	// Verify map default is shown
	if !strings.Contains(result, "{env: production, workers: 4}") {
		t.Error("README should contain map default '{env: production, workers: 4}'")
	}

	// Verify descriptions
	if !strings.Contains(result, "Application name for integration test") {
		t.Error("README should contain app_name description")
	}
	if !strings.Contains(result, "Internal variable from vars") {
		t.Error("README should contain internal_var description")
	}

	// Verify original content preserved
	if !strings.Contains(result, "# Integration Test Role") {
		t.Error("original heading should be preserved")
	}
	if !strings.Contains(result, "This tests the full workflow.") {
		t.Error("original content should be preserved")
	}

	// Verify markers
	if !strings.Contains(result, "<!-- begin role_variables -->") {
		t.Error("README should contain begin marker")
	}
	if !strings.Contains(result, "<!-- end role_variables -->") {
		t.Error("README should contain end marker")
	}
}
