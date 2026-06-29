package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateVariablesSection_Empty(t *testing.T) {
	section := GenerateVariablesSection(nil)

	if !strings.Contains(section, BeginMarker) {
		t.Error("section should contain begin marker")
	}
	if !strings.Contains(section, EndMarker) {
		t.Error("section should contain end marker")
	}
	if !strings.Contains(section, "_No variables found._") {
		t.Error("section should contain 'no variables' message")
	}
}

func TestGenerateVariablesSection_SingleVariable(t *testing.T) {
	vars := []RoleVariable{
		{
			Name:        "app_name",
			Type:        "string",
			Default:     `"my-app"`,
			Description: "The application name",
			Source:      "defaults/main.yml",
		},
	}

	section := GenerateVariablesSection(vars)

	if !strings.Contains(section, BeginMarker) {
		t.Error("section should contain begin marker")
	}
	if !strings.Contains(section, EndMarker) {
		t.Error("section should contain end marker")
	}
	if !strings.Contains(section, "## Role Variables") {
		t.Error("section should contain Role Variables heading")
	}
	if !strings.Contains(section, "| `app_name` |") {
		t.Error("section should contain app_name variable")
	}
	if !strings.Contains(section, "string") {
		t.Error("section should contain type 'string'")
	}
	if !strings.Contains(section, "The application name") {
		t.Error("section should contain description")
	}
}


func TestGenerateVariablesSection_MultipleVariables(t *testing.T) {
	vars := []RoleVariable{
		{Name: "app_name", Type: "string", Default: `"my-app"`, Description: "App name"},
		{Name: "app_port", Type: "int", Default: "8080", Description: "Port number"},
		{Name: "debug", Type: "bool", Default: "false", Description: "Debug mode"},
	}

	section := GenerateVariablesSection(vars)

	// Check table header
	if !strings.Contains(section, "| Variable | Type | Default | Description |") {
		t.Error("section should contain table header")
	}
	if !strings.Contains(section, "|----------|------|---------|-------------|") {
		t.Error("section should contain table separator")
	}

	// Check all variables present
	if !strings.Contains(section, "| `app_name` |") {
		t.Error("section should contain app_name")
	}
	if !strings.Contains(section, "| `app_port` |") {
		t.Error("section should contain app_port")
	}
	if !strings.Contains(section, "| `debug` |") {
		t.Error("section should contain debug")
	}
}

func TestGenerateVariablesSection_MissingFields(t *testing.T) {
	vars := []RoleVariable{
		{Name: "untyped_var", Default: "value"},
		{Name: "no_default_var", Type: "string"},
		{Name: "minimal_var"},
	}

	section := GenerateVariablesSection(vars)

	// Variables without type should show "-"
	lines := strings.Split(section, "\n")
	for _, line := range lines {
		if strings.Contains(line, "minimal_var") {
			// Should have "-" for type, default, and description
			parts := strings.Split(line, "|")
			// parts[2] is Type, parts[3] is Default, parts[4] is Description
			if len(parts) >= 5 {
				if strings.TrimSpace(parts[2]) != "-" {
					t.Errorf("minimal_var type should be '-', got %q", strings.TrimSpace(parts[2]))
				}
				if strings.TrimSpace(parts[3]) != "-" {
					t.Errorf("minimal_var default should be '-', got %q", strings.TrimSpace(parts[3]))
				}
				if strings.TrimSpace(parts[4]) != "-" {
					t.Errorf("minimal_var description should be '-', got %q", strings.TrimSpace(parts[4]))
				}
			}
		}
	}
}

func TestGenerateVariablesSection_PipeEscaping(t *testing.T) {
	vars := []RoleVariable{
		{Name: "filter_var", Type: "string", Default: "value|other"},
	}

	section := GenerateVariablesSection(vars)

	// Pipe in default value should be escaped
	if !strings.Contains(section, `value\|other`) {
		t.Error("pipe character in default should be escaped with backslash")
	}
}


func TestUpdateReadme_CreatesNewFile(t *testing.T) {
	roleDir := t.TempDir()

	vars := []RoleVariable{
		{Name: "test_var", Type: "string", Default: "hello", Description: "A test variable"},
	}

	err := UpdateReadme(roleDir, vars)
	if err != nil {
		t.Fatalf("UpdateReadme failed: %v", err)
	}

	// Verify file was created
	readmePath := filepath.Join(roleDir, "README.md")
	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README.md: %v", err)
	}

	contentStr := string(content)
	if !strings.Contains(contentStr, "# Role") {
		t.Error("new README should contain '# Role' heading")
	}
	if !strings.Contains(contentStr, BeginMarker) {
		t.Error("new README should contain begin marker")
	}
	if !strings.Contains(contentStr, EndMarker) {
		t.Error("new README should contain end marker")
	}
	if !strings.Contains(contentStr, "test_var") {
		t.Error("new README should contain test_var")
	}
}

func TestUpdateReadme_AppendsToExisting(t *testing.T) {
	roleDir := t.TempDir()
	readmePath := filepath.Join(roleDir, "README.md")

	// Create existing README without markers
	existing := "# My Role\n\nSome content here.\n"
	if err := os.WriteFile(readmePath, []byte(existing), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	vars := []RoleVariable{
		{Name: "my_var", Type: "int", Default: "42", Description: "A number"},
	}

	err := UpdateReadme(roleDir, vars)
	if err != nil {
		t.Fatalf("UpdateReadme failed: %v", err)
	}

	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README: %v", err)
	}

	contentStr := string(content)

	// Original content should be preserved
	if !strings.Contains(contentStr, "# My Role") {
		t.Error("original heading should be preserved")
	}
	if !strings.Contains(contentStr, "Some content here.") {
		t.Error("original content should be preserved")
	}

	// New section should be appended
	if !strings.Contains(contentStr, BeginMarker) {
		t.Error("appended section should contain begin marker")
	}
	if !strings.Contains(contentStr, "my_var") {
		t.Error("appended section should contain my_var")
	}
}


func TestUpdateReadme_ReplacesExistingMarkers(t *testing.T) {
	roleDir := t.TempDir()
	readmePath := filepath.Join(roleDir, "README.md")

	// Create existing README with markers and old content
	existing := `# My Role

Some intro.

<!-- begin role_variables -->

## Role Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| ` + "`old_var`" + ` | string | ` + "`old`" + ` | Old variable |

<!-- end role_variables -->

## License

MIT
`
	if err := os.WriteFile(readmePath, []byte(existing), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	vars := []RoleVariable{
		{Name: "new_var", Type: "bool", Default: "true", Description: "New variable"},
	}

	err := UpdateReadme(roleDir, vars)
	if err != nil {
		t.Fatalf("UpdateReadme failed: %v", err)
	}

	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README: %v", err)
	}

	contentStr := string(content)

	// Old variable should be gone
	if strings.Contains(contentStr, "old_var") {
		t.Error("old_var should be replaced")
	}

	// New variable should be present
	if !strings.Contains(contentStr, "new_var") {
		t.Error("new_var should be present")
	}

	// Content before markers should be preserved
	if !strings.Contains(contentStr, "# My Role") {
		t.Error("heading should be preserved")
	}
	if !strings.Contains(contentStr, "Some intro.") {
		t.Error("intro should be preserved")
	}

	// Content after markers should be preserved
	if !strings.Contains(contentStr, "## License") {
		t.Error("License section should be preserved")
	}
	if !strings.Contains(contentStr, "MIT") {
		t.Error("MIT text should be preserved")
	}
}

func TestUpdateReadme_Idempotent(t *testing.T) {
	roleDir := t.TempDir()
	readmePath := filepath.Join(roleDir, "README.md")

	existing := "# Role\n\n"
	if err := os.WriteFile(readmePath, []byte(existing), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	vars := []RoleVariable{
		{Name: "my_var", Type: "string", Default: "hello", Description: "Test"},
	}

	// Run twice
	if err := UpdateReadme(roleDir, vars); err != nil {
		t.Fatalf("first UpdateReadme failed: %v", err)
	}
	if err := UpdateReadme(roleDir, vars); err != nil {
		t.Fatalf("second UpdateReadme failed: %v", err)
	}

	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README: %v", err)
	}

	contentStr := string(content)

	// Should only have ONE set of markers
	beginCount := strings.Count(contentStr, BeginMarker)
	endCount := strings.Count(contentStr, EndMarker)

	if beginCount != 1 {
		t.Errorf("expected 1 begin marker, got %d", beginCount)
	}
	if endCount != 1 {
		t.Errorf("expected 1 end marker, got %d", endCount)
	}

	// Should only have ONE table header
	headerCount := strings.Count(contentStr, "| Variable | Type | Default | Description |")
	if headerCount != 1 {
		t.Errorf("expected 1 table header, got %d", headerCount)
	}
}

func TestUpdateReadme_EmptyVariables(t *testing.T) {
	roleDir := t.TempDir()
	readmePath := filepath.Join(roleDir, "README.md")

	existing := "# Role\n"
	if err := os.WriteFile(readmePath, []byte(existing), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	err := UpdateReadme(roleDir, nil)
	if err != nil {
		t.Fatalf("UpdateReadme failed: %v", err)
	}

	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README: %v", err)
	}

	if !strings.Contains(string(content), "_No variables found._") {
		t.Error("empty variables should produce 'no variables' message")
	}
}


func TestUpdateReadme_NoTrailingNewlineInExisting(t *testing.T) {
	roleDir := t.TempDir()
	readmePath := filepath.Join(roleDir, "README.md")

	// No trailing newline
	existing := "# Role\nContent without trailing newline"
	if err := os.WriteFile(readmePath, []byte(existing), 0644); err != nil {
		t.Fatalf("failed to write README: %v", err)
	}

	vars := []RoleVariable{
		{Name: "x", Type: "int", Default: "1"},
	}

	err := UpdateReadme(roleDir, vars)
	if err != nil {
		t.Fatalf("UpdateReadme failed: %v", err)
	}

	content, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("failed to read README: %v", err)
	}

	contentStr := string(content)

	// Should still have the original content
	if !strings.Contains(contentStr, "Content without trailing newline") {
		t.Error("original content should be preserved")
	}
	// Should have the variables section
	if !strings.Contains(contentStr, BeginMarker) {
		t.Error("section should be appended")
	}
}

func TestBeginEndMarkerConstants(t *testing.T) {
	if BeginMarker != "<!-- begin role_variables -->" {
		t.Errorf("BeginMarker = %q, want %q", BeginMarker, "<!-- begin role_variables -->")
	}
	if EndMarker != "<!-- end role_variables -->" {
		t.Errorf("EndMarker = %q, want %q", EndMarker, "<!-- end role_variables -->")
	}
}
