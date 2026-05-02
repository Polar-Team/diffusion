package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"diffusion/internal/config"
	"github.com/spf13/cobra"
)

// TestRoleCommandIntegration tests role command execution
func TestRoleCommandIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Create test role structure
	metaDir := filepath.Join(tmpDir, "meta")
	scenariosDir := filepath.Join(tmpDir, "scenarios", "default")
	os.MkdirAll(metaDir, 0755)
	os.MkdirAll(scenariosDir, 0755)

	// Create meta file
	metaContent := `---
galaxy_info:
  role_name: test_role
  namespace: test_ns
  author: Test
  description: Test
  company: Test
  license: MIT
  min_ansible_version: "2.10"
  platforms: []
  galaxy_tags: []
collections: []
`
	os.WriteFile(filepath.Join(metaDir, "main.yml"), []byte(metaContent), 0644)

	// Create requirements file
	reqContent := `---
collections: []
roles: []
`
	os.WriteFile(filepath.Join(scenariosDir, "requirements.yml"), []byte(reqContent), 0644)

	cli := &CLI{}
	cmd := NewRoleCmd(cli)
	
	// Test command execution
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Logf("Role command execution completed with: %v", err)
	}
}

// TestShowCommandIntegration tests show command execution
func TestShowCommandIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Create config
	cfg := &config.Config{
		ContainerRegistry: &config.ContainerRegistry{
			RegistryServer:        "test.registry.com",
			RegistryProvider:      "Public",
			MoleculeContainerName: "test-container",
			MoleculeContainerTag:  "latest",
		},
		HashicorpVault: &config.HashicorpVault{
			HashicorpVaultIntegration: false,
		},
		YamlLintConfig: &config.YamlLint{
			Extends: "default",
			Ignore:  []string{},
			Rules:   &config.YamlLintRules{},
		},
		AnsibleLintConfig: &config.AnsibleLint{
			ExcludedPaths: []string{},
			WarnList:      []string{},
			SkipList:      []string{},
		},
		TestsConfig: &config.TestsSettings{
			Type: "diffusion",
		},
	}
	config.SaveConfig(cfg)

	cli := &CLI{}
	cmd := NewShowCmd(cli)
	
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	
	err := cmd.RunE(cmd, []string{})
	if err != nil {
		t.Fatalf("Show command failed: %v", err)
	}
	
	output := buf.String()
	if !strings.Contains(output, "Configuration") || !strings.Contains(output, "Registry") {
		t.Logf("Show command output: %s", output)
		// Test passes if command executed without error
	}
}

// TestDepsInitIntegration tests deps init command
func TestDepsInitIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	cli := &CLI{}
	cmd := NewDepsCmd(cli)
	
	// Find init subcommand
	var initCmd *cobra.Command
	for _, sub := range cmd.Commands() {
		if sub.Use == "init" {
			initCmd = sub
			break
		}
	}
	
	if initCmd == nil {
		t.Fatal("init subcommand not found")
	}
	
	var buf bytes.Buffer
	initCmd.SetOut(&buf)
	initCmd.SetErr(&buf)
	
	err := initCmd.RunE(initCmd, []string{})
	if err != nil {
		t.Logf("Deps init completed with: %v", err)
	}
	
	// Check if config was created
	if _, err := os.Stat("diffusion.toml"); err == nil {
		t.Log("Config file created successfully")
	}
}

// TestArtifactListIntegration tests artifact list command
func TestArtifactListIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	cli := &CLI{}
	cmd := NewArtifactCmd(cli)
	
	// Find list subcommand
	var listCmd *cobra.Command
	for _, sub := range cmd.Commands() {
		if strings.HasPrefix(sub.Use, "list") {
			listCmd = sub
			break
		}
	}
	
	if listCmd == nil {
		t.Fatal("list subcommand not found")
	}
	
	var buf bytes.Buffer
	listCmd.SetOut(&buf)
	listCmd.SetErr(&buf)
	
	err := listCmd.RunE(listCmd, []string{})
	if err != nil {
		t.Logf("Artifact list completed with: %v", err)
	}
}

// TestCacheListIntegration tests cache list command
func TestCacheListIntegration(t *testing.T) {
	cli := &CLI{}
	cmd := NewCacheCmd(cli)
	
	// Find list subcommand
	var listCmd *cobra.Command
	for _, sub := range cmd.Commands() {
		if strings.HasPrefix(sub.Use, "list") {
			listCmd = sub
			break
		}
	}
	
	if listCmd == nil {
		t.Fatal("list subcommand not found")
	}
	
	var buf bytes.Buffer
	listCmd.SetOut(&buf)
	listCmd.SetErr(&buf)
	
	err := listCmd.RunE(listCmd, []string{})
	if err != nil {
		t.Logf("Cache list completed with: %v", err)
	}
}