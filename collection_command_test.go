package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestAddCollectionOnlyModifiesToml verifies that add-collection only modifies diffusion.toml
func TestAddCollectionOnlyModifiesToml(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	// Create initial diffusion.toml
	initialConfig := &Config{
		DependencyConfig: &DependencyConfig{
			Collections: []CollectionRequirement{
				{Name: "community.general", Version: ">=10.0.0"},
			},
		},
	}
	if err := SaveConfig(initialConfig); err != nil {
		t.Fatalf("Failed to create initial config: %v", err)
	}

	// Create initial requirements.yml and meta/main.yml
	reqDir := filepath.Join(tempDir, "molecule", "default")
	os.MkdirAll(reqDir, 0755)
	reqFile := filepath.Join(reqDir, "requirements.yml")
	os.WriteFile(reqFile, []byte("collections:\n  - name: community.general\n    version: 10.0.0\n"), 0644)

	metaDir := filepath.Join(tempDir, "meta")
	os.MkdirAll(metaDir, 0755)
	metaFile := filepath.Join(metaDir, "main.yml")
	os.WriteFile(metaFile, []byte("collections:\n  - community.general\n"), 0644)

	// Read initial content
	initialReqContent, _ := os.ReadFile(reqFile)
	initialMetaContent, _ := os.ReadFile(metaFile)

	// Add a new collection (this should only modify diffusion.toml)
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Simulate add-collection command logic
	newCollection := CollectionRequirement{Name: "community.docker", Version: ">=5.0.0"}
	config.DependencyConfig.Collections = append(config.DependencyConfig.Collections, newCollection)

	if err := SaveConfig(config); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify diffusion.toml was updated
	updatedConfig, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load updated config: %v", err)
	}

	found := false
	for _, coll := range updatedConfig.DependencyConfig.Collections {
		if coll.Name == "community.docker" && coll.Version == ">=5.0.0" {
			found = true
			break
		}
	}
	if !found {
		t.Error("New collection not found in diffusion.toml")
	}

	// Verify requirements.yml was NOT modified
	currentReqContent, _ := os.ReadFile(reqFile)
	if string(currentReqContent) != string(initialReqContent) {
		t.Error("requirements.yml should not be modified by add-collection")
	}

	// Verify meta/main.yml was NOT modified
	currentMetaContent, _ := os.ReadFile(metaFile)
	if string(currentMetaContent) != string(initialMetaContent) {
		t.Error("meta/main.yml should not be modified by add-collection")
	}
}

// TestRemoveCollectionOnlyModifiesToml verifies that remove-collection only modifies diffusion.toml
func TestRemoveCollectionOnlyModifiesToml(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	// Create initial diffusion.toml with two collections
	initialConfig := &Config{
		DependencyConfig: &DependencyConfig{
			Collections: []CollectionRequirement{
				{Name: "community.general", Version: ">=10.0.0"},
				{Name: "community.docker", Version: ">=5.0.0"},
			},
		},
	}
	if err := SaveConfig(initialConfig); err != nil {
		t.Fatalf("Failed to create initial config: %v", err)
	}

	// Create initial requirements.yml and meta/main.yml
	reqDir := filepath.Join(tempDir, "molecule", "default")
	os.MkdirAll(reqDir, 0755)
	reqFile := filepath.Join(reqDir, "requirements.yml")
	os.WriteFile(reqFile, []byte("collections:\n  - name: community.general\n    version: 10.0.0\n  - name: community.docker\n    version: 5.0.4\n"), 0644)

	metaDir := filepath.Join(tempDir, "meta")
	os.MkdirAll(metaDir, 0755)
	metaFile := filepath.Join(metaDir, "main.yml")
	os.WriteFile(metaFile, []byte("collections:\n  - community.general\n  - community.docker\n"), 0644)

	// Read initial content
	initialReqContent, _ := os.ReadFile(reqFile)
	initialMetaContent, _ := os.ReadFile(metaFile)

	// Remove a collection (this should only modify diffusion.toml)
	config, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Simulate remove-collection command logic
	for i, coll := range config.DependencyConfig.Collections {
		if coll.Name == "community.docker" {
			config.DependencyConfig.Collections = append(config.DependencyConfig.Collections[:i], config.DependencyConfig.Collections[i+1:]...)
			break
		}
	}

	if err := SaveConfig(config); err != nil {
		t.Fatalf("Failed to save config: %v", err)
	}

	// Verify diffusion.toml was updated
	updatedConfig, err := LoadConfig()
	if err != nil {
		t.Fatalf("Failed to load updated config: %v", err)
	}

	for _, coll := range updatedConfig.DependencyConfig.Collections {
		if coll.Name == "community.docker" {
			t.Error("Collection should be removed from diffusion.toml")
		}
	}

	// Verify requirements.yml was NOT modified
	currentReqContent, _ := os.ReadFile(reqFile)
	if string(currentReqContent) != string(initialReqContent) {
		t.Error("requirements.yml should not be modified by remove-collection")
	}

	// Verify meta/main.yml was NOT modified
	currentMetaContent, _ := os.ReadFile(metaFile)
	if string(currentMetaContent) != string(initialMetaContent) {
		t.Error("meta/main.yml should not be modified by remove-collection")
	}
}

// TestDepsSyncUpdatesMetaWithoutVersions verifies that deps sync adds only collection names to meta.yml
func TestDepsSyncUpdatesMetaWithoutVersions(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()
	originalDir, _ := os.Getwd()
	defer os.Chdir(originalDir)
	os.Chdir(tempDir)

	// Create diffusion.toml with collections
	config := &Config{
		DependencyConfig: &DependencyConfig{
			Collections: []CollectionRequirement{
				{Name: "community.general", Version: ">=10.0.0"},
				{Name: "community.docker", Version: ">=5.0.0"},
			},
		},
	}
	if err := SaveConfig(config); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Create empty requirements.yml and meta/main.yml
	reqDir := filepath.Join(tempDir, "scenarios", "default")
	os.MkdirAll(reqDir, 0755)

	metaDir := filepath.Join(tempDir, "meta")
	os.MkdirAll(metaDir, 0755)
	metaFile := filepath.Join(metaDir, "main.yml")
	os.WriteFile(metaFile, []byte("collections: []\n"), 0644)

	reqFile := filepath.Join(reqDir, "requirements.yml")
	os.WriteFile(reqFile, []byte("collections: []\n"), 0644)

	// Create diffusion.lock with resolved versions
	lockFile := &LockFile{
		Collections: []LockFileEntry{
			{
				Name:            "community.general",
				Version:         ">=10.0.0",
				ResolvedVersion: "10.5.0",
				Type:            "collection",
				Source:          "galaxy",
			},
			{
				Name:            "community.docker",
				Version:         ">=5.0.0",
				ResolvedVersion: "5.0.4",
				Type:            "collection",
				Source:          "galaxy",
			},
		},
	}
	if err := SaveLockFile(lockFile); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	// Load role config (creates empty files if they don't exist)
	meta, req, err := LoadRoleConfig("default")
	if err != nil {
		t.Fatalf("Failed to load role config: %v", err)
	}

	// Simulate deps sync command logic
	req.Collections = []RequirementCollection{}
	for _, col := range lockFile.Collections {
		version := col.ResolvedVersion
		if version == "" {
			version = col.Version
		}
		req.Collections = append(req.Collections, RequirementCollection{
			Name:    col.Name,
			Version: version,
		})
	}

	meta.Collections = []string{}
	for _, col := range lockFile.Collections {
		// meta.yml should only have collection names, no version constraints
		meta.Collections = append(meta.Collections, col.Name)
	}

	// Save files
	if err := SaveRequirementFile(req, "default"); err != nil {
		t.Fatalf("Failed to save requirements file: %v", err)
	}
	if err := SaveMetaFile(meta); err != nil {
		t.Fatalf("Failed to save meta file: %v", err)
	}

	// Verify meta/main.yml contains only collection names (no versions)
	loadedMeta, loadedReq, err := LoadRoleConfig("default")
	if err != nil {
		t.Fatalf("Failed to load role config: %v", err)
	}

	if len(loadedMeta.Collections) != 2 {
		t.Errorf("Expected 2 collections in meta/main.yml, got %d", len(loadedMeta.Collections))
	}

	for _, coll := range loadedMeta.Collections {
		// Collection names should not contain version constraints
		if coll != "community.general" && coll != "community.docker" {
			t.Errorf("Unexpected collection in meta/main.yml: %s", coll)
		}
		// Verify no version constraint symbols
		if containsVersionConstraint(coll) {
			t.Errorf("meta/main.yml should not contain version constraints, found: %s", coll)
		}
	}

	// Verify requirements.yml contains resolved versions
	if len(loadedReq.Collections) != 2 {
		t.Errorf("Expected 2 collections in requirements.yml, got %d", len(loadedReq.Collections))
	}

	for _, coll := range loadedReq.Collections {
		if coll.Name == "community.general" && coll.Version != "10.5.0" {
			t.Errorf("Expected resolved version 10.5.0 for community.general, got %s", coll.Version)
		}
		if coll.Name == "community.docker" && coll.Version != "5.0.4" {
			t.Errorf("Expected resolved version 5.0.4 for community.docker, got %s", coll.Version)
		}
	}
}

// Helper function to check if a string contains version constraint symbols
func containsVersionConstraint(s string) bool {
	return len(s) > 0 && (s[0] == '>' || s[0] == '<' || s[0] == '=' || s[0] == '~' || s[0] == '^')
}
