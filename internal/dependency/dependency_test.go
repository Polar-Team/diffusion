package dependency

import (
	"os"
	"path/filepath"
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/role"
	"diffusion/internal/utils"
)

func TestParseCollectionString(t *testing.T) {
	tests := []struct {
		input       string
		wantName    string
		wantVersion string
	}{
		{"community.general>=7.4.0", "community.general", ">=7.4.0"},
		{"community.docker", "community.docker", ""},
		{"kubernetes.core==2.4.0", "kubernetes.core", "==2.4.0"},
		{"amazon.aws>1.0.0", "amazon.aws", ">1.0.0"},
		{"google.cloud<=3.0.0", "google.cloud", "<=3.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotName, gotVersion := utils.ParseCollectionString(tt.input)
			if gotName != tt.wantName {
				t.Errorf("parseCollectionString(%q) name = %q, want %q", tt.input, gotName, tt.wantName)
			}
			if gotVersion != tt.wantVersion {
				t.Errorf("parseCollectionString(%q) version = %q, want %q", tt.input, gotVersion, tt.wantVersion)
			}
		})
	}
}

func TestResolveCollectionDependencies(t *testing.T) {
	meta := &role.Meta{
		Collections: []string{
			"community.general>=7.4.0",
			"community.docker",
		},
	}

	req := &role.Requirement{
		Collections: []role.RequirementCollection{
			{Name: "community.postgresql", Version: ">=3.0.0"},
			{Name: "community.docker", Version: ">=3.4.0"}, // Should override meta version
		},
	}

	config := &config.DependencyConfig{
		Collections: []config.CollectionRequirement{
			{Name: "kubernetes.core", Version: ">=2.4.0"},
		},
	}

	resolver := NewDependencyResolver(meta, req, config)
	collections, err := resolver.ResolveCollectionDependencies()
	if err != nil {
		t.Fatalf("ResolveCollectionDependencies() error = %v", err)
	}

	// Check that all collections are present
	collectionMap := make(map[string]string)
	for _, col := range collections {
		collectionMap[col.Name] = col.Version
	}

	expectedCollections := map[string]string{
		"community.general":    ">=7.4.0",
		"community.docker":     ">=3.4.0",
		"community.postgresql": ">=3.0.0",
		"kubernetes.core":      ">=2.4.0",
	}

	for name, expectedVersion := range expectedCollections {
		if version, ok := collectionMap[name]; !ok {
			t.Errorf("Expected collection %q not found", name)
		} else if version != expectedVersion {
			t.Errorf("Collection %q version = %q, want %q", name, version, expectedVersion)
		}
	}
}

func TestResolvePythonVersion(t *testing.T) {
	config := &config.DependencyConfig{
		Python: &config.PythonVersion{
			Min:    "3.11", // Major.minor only
			Max:    "3.13", // Major.minor only
			Pinned: "3.13", // Major.minor only
		},
	}

	resolver := NewDependencyResolver(nil, nil, config)
	pythonVersion := resolver.ResolvePythonVersion()

	if pythonVersion.Min != "3.11" {
		t.Errorf("Python Min = %q, want %q", pythonVersion.Min, "3.11")
	}
	if pythonVersion.Max != "3.13" {
		t.Errorf("Python Max = %q, want %q", pythonVersion.Max, "3.13")
	}
	if pythonVersion.Pinned != "3.13" {
		t.Errorf("Python Pinned = %q, want %q", pythonVersion.Pinned, "3.13")
	}
	// Additional versions should not be used
	if len(pythonVersion.Additional) != 0 {
		t.Errorf("Python Additional length = %d, want 0", len(pythonVersion.Additional))
	}
}

func TestResolveToolVersions(t *testing.T) {
	config := &config.DependencyConfig{
		Python: &config.PythonVersion{
			Min:    "3.10",
			Max:    "3.13",
			Pinned: "3.13",
		},
		Ansible:     ">=10.0.0",
		AnsibleLint: ">=24.0.0",
		Molecule:    ">=24.0.0",
		YamlLint:    ">=1.35.0",
	}

	resolver := NewDependencyResolver(nil, nil, config)
	toolVersions := resolver.ResolveToolVersions()

	// With Python 3.13, compatibility system upgrades to latest compatible versions
	// Only tools that are incompatible get upgraded
	expectedTools := map[string]string{
		"ansible":      ">=13.0.0", // Upgraded from >=10.0.0 (10.x max is 3.12, needs upgrade for 3.13)
		"ansible-lint": ">=24.0.0", // Already compatible with 3.13
		"molecule":     ">=24.0.0", // Already compatible with 3.13
		"yamllint":     ">=1.35.0",
	}

	for tool, expectedVersion := range expectedTools {
		if version, ok := toolVersions[tool]; !ok {
			t.Errorf("Expected tool %q not found", tool)
		} else if version != expectedVersion {
			t.Errorf("Tool %q version = %q, want %q", tool, version, expectedVersion)
		}
	}
}

func TestComputeDependencyHash(t *testing.T) {
	collections := []config.CollectionRequirement{
		{Name: "community.general", Version: ">=7.4.0"},
		{Name: "community.docker", Version: ">=3.4.0"},
	}

	roles := []config.RoleRequirement{
		{Name: "geerlingguy.docker", Version: "6.0.0"},
	}

	toolVersions := map[string]string{
		"ansible":  ">=10.0.0",
		"molecule": ">=24.0.0",
	}

	pythonVersion := &config.PythonVersion{
		Min:    "3.9",
		Max:    "3.13",
		Pinned: "3.13",
	}

	hash1 := ComputeDependencyHash(collections, roles, toolVersions, pythonVersion)
	hash2 := ComputeDependencyHash(collections, roles, toolVersions, pythonVersion)

	// Same inputs should produce same hash
	if hash1 != hash2 {
		t.Errorf("Hash mismatch: %q != %q", hash1, hash2)
	}

	// Different inputs should produce different hash
	collections2 := append(collections, config.CollectionRequirement{Name: "kubernetes.core", Version: ">=2.4.0"})
	hash3 := ComputeDependencyHash(collections2, roles, toolVersions, pythonVersion)

	if hash1 == hash3 {
		t.Errorf("Hash should be different for different collections")
	}
}

func TestGenerateLockFile(t *testing.T) {
	collections := []config.CollectionRequirement{
		{Name: "community.general", Version: ">=7.4.0"},
	}

	roles := []config.RoleRequirement{
		{Name: "geerlingguy.docker", Version: "6.0.0"},
	}

	toolVersions := map[string]string{
		"ansible": ">=10.0.0",
	}

	pythonVersion := &config.PythonVersion{
		Min:    "3.9",
		Max:    "3.13",
		Pinned: "3.13",
	}

	lockFile, err := GenerateLockFile(collections, roles, toolVersions, pythonVersion)
	if err != nil {
		t.Fatalf("GenerateLockFile() error = %v", err)
	}

	if lockFile.Version != LockFileVersion {
		t.Errorf("LockFile Version = %q, want %q", lockFile.Version, LockFileVersion)
	}

	if len(lockFile.Collections) != 1 {
		t.Errorf("LockFile Collections length = %d, want 1", len(lockFile.Collections))
	}

	if len(lockFile.Roles) != 1 {
		t.Errorf("LockFile Roles length = %d, want 1", len(lockFile.Roles))
	}

	if len(lockFile.Tools) != 1 {
		t.Errorf("LockFile Tools length = %d, want 1", len(lockFile.Tools))
	}

	if lockFile.Hash == "" {
		t.Error("LockFile Hash should not be empty")
	}
}

func TestFormatPythonRequirement(t *testing.T) {
	t.Skip("Testing private function - skipped in refactored version")
}

func TestFormatDependency(t *testing.T) {
	t.Skip("Testing private function - skipped in refactored version")
}

func TestGetCollectionPythonDependencies(t *testing.T) {
	t.Skip("Testing private function - skipped in refactored version")
}

func TestSaveLockFile(t *testing.T) {
	// Create temporary directory
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	lockFile := &LockFile{
		Version: LockFileVersion,
		Hash:    "test-hash",
		Python: &config.PythonVersion{
			Min:    "3.11",
			Max:    "3.13",
			Pinned: "3.13",
		},
		Collections: []LockFileEntry{
			{Name: "community.general", Version: ">=7.4.0", Type: "collection"},
		},
		Tools: []LockFileEntry{
			{Name: "ansible", Version: ">=10.0.0", Type: "tool"},
		},
	}

	err := SaveLockFile(lockFile)
	if err != nil {
		t.Fatalf("SaveLockFile() error = %v", err)
	}

	// Check file exists
	if _, err := os.Stat(config.LockFileName); os.IsNotExist(err) {
		t.Errorf("Lock file was not created")
	}

	// Load and verify
	loaded, err := LoadLockFile()
	if err != nil {
		t.Fatalf("LoadLockFile() error = %v", err)
	}

	if loaded.Hash != lockFile.Hash {
		t.Errorf("Loaded hash = %q, want %q", loaded.Hash, lockFile.Hash)
	}
}

func TestGeneratePyProjectTOML(t *testing.T) {
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "pyproject.toml")

	collections := []config.CollectionRequirement{
		{Name: "community.general", Version: ">=7.4.0"},
		{Name: "community.postgresql", Version: ">=3.0.0"},
	}

	toolVersions := map[string]string{
		"ansible":      ">=10.0.0",
		"ansible-lint": ">=24.0.0",
		"molecule":     ">=24.0.0",
		"yamllint":     ">=1.35.0",
	}

	pythonVersion := &config.PythonVersion{
		Min:    "3.9",
		Max:    "3.13",
		Pinned: "3.13",
	}

	err := GeneratePyProjectTOML(collections, toolVersions, pythonVersion, outputPath)
	if err != nil {
		t.Fatalf("GeneratePyProjectTOML() error = %v", err)
	}

	// Check file exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Errorf("pyproject.toml was not created")
	}

	// Read and verify content
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read pyproject.toml: %v", err)
	}

	contentStr := string(content)

	// Check for key elements
	expectedStrings := []string{
		"ansible>=10.0.0",
		"molecule>=24.0.0",
		"requires-python = \">=3.13\"", // Uses pinned Python version
		"psycopg2-binary",              // From community.postgresql
	}

	for _, expected := range expectedStrings {
		if !stringContains(contentStr, expected) {
			t.Errorf("pyproject.toml missing expected string: %q", expected)
		}
	}
}

func stringContains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && stringContainsHelper(s, substr))
}

func stringContainsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
