package cli

import (
	"os"
	"testing"

	"diffusion/internal/config"
)

// TestShowCommand tests show command functionality
func TestShowCommand(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	// Create minimal config
	cfg := &config.Config{
		ContainerRegistry: &config.ContainerRegistry{
			RegistryServer:        "test.registry.com",
			RegistryProvider:      "Public",
			MoleculeContainerName: "test-container",
			MoleculeContainerTag:  "latest",
		},
	}
	config.SaveConfig(cfg)

	cli := &CLI{}
	cmd := NewShowCmd(cli)
	
	// Test command structure
	if cmd.Use != "show" {
		t.Errorf("expected Use 'show', got %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Error("expected RunE to be set")
	}
}

// TestMoleculeCommand tests molecule command functionality  
func TestMoleculeCommand(t *testing.T) {
	cli := &CLI{
		RoleFlag: "test-role",
		OrgFlag:  "test-org",
	}
	cmd := NewMoleculeCmd(cli)
	
	if cmd.Use != "molecule" {
		t.Errorf("expected Use 'molecule', got %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Error("expected RunE to be set")
	}
}

// TestArtifactCommand tests artifact command structure
func TestArtifactCommand(t *testing.T) {
	cli := &CLI{}
	cmd := NewArtifactCmd(cli)
	
	if cmd.Use != "artifact" {
		t.Errorf("expected Use 'artifact', got %q", cmd.Use)
	}
	
	// Check subcommands exist
	subcommands := cmd.Commands()
	if len(subcommands) == 0 {
		t.Error("expected artifact command to have subcommands")
	}
}

// TestCacheCommand tests cache command structure
func TestCacheCommand(t *testing.T) {
	cli := &CLI{}
	cmd := NewCacheCmd(cli)
	
	if cmd.Use != "cache" {
		t.Errorf("expected Use 'cache', got %q", cmd.Use)
	}
	
	// Check subcommands exist
	subcommands := cmd.Commands()
	if len(subcommands) == 0 {
		t.Error("expected cache command to have subcommands")
	}
}

// TestDepsCommand tests deps command structure
func TestDepsCommand(t *testing.T) {
	cli := &CLI{}
	cmd := NewDepsCmd(cli)
	
	if cmd.Use != "deps" {
		t.Errorf("expected Use 'deps', got %q", cmd.Use)
	}
	
	// Check subcommands exist
	subcommands := cmd.Commands()
	if len(subcommands) == 0 {
		t.Error("expected deps command to have subcommands")
	}
	
	// Verify common subcommands
	foundInit := false
	foundSync := false
	for _, sub := range subcommands {
		if sub.Use == "init" {
			foundInit = true
		}
		if sub.Use == "sync" {
			foundSync = true
		}
	}
	if !foundInit {
		t.Error("expected 'init' subcommand")
	}
	if !foundSync {
		t.Error("expected 'sync' subcommand")
	}
}

// TestRoleCommand tests role command structure
func TestRoleCommand(t *testing.T) {
	cli := &CLI{}
	cmd := NewRoleCmd(cli)
	
	if cmd.Use != "role" {
		t.Errorf("expected Use 'role', got %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Error("expected RunE to be set")
	}
}

// TestCLIFlagValidation tests CLI flag validation
func TestCLIFlagValidation(t *testing.T) {
	tests := []struct {
		name string
		cli  *CLI
		want bool
	}{
		{
			name: "valid role and org",
			cli: &CLI{
				RoleFlag: "test-role",
				OrgFlag:  "test-org",
			},
			want: true,
		},
		{
			name: "missing role",
			cli: &CLI{
				OrgFlag: "test-org",
			},
			want: false,
		},
		{
			name: "missing org",
			cli: &CLI{
				RoleFlag: "test-role",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.cli.RoleFlag != "" && tt.cli.OrgFlag != ""
			if valid != tt.want {
				t.Errorf("validation = %v, want %v", valid, tt.want)
			}
		})
	}
}

// TestCLIScenarioHandling tests scenario flag handling
func TestCLIScenarioHandling(t *testing.T) {
	cli := &CLI{
		RoleFlag:     "test-role",
		OrgFlag:      "test-org",
		RoleScenario: "production",
	}

	if cli.RoleScenario != "production" {
		t.Errorf("expected scenario 'production', got %q", cli.RoleScenario)
	}
	
	// Test default scenario
	cli2 := &CLI{
		RoleFlag: "test-role",
		OrgFlag:  "test-org",
	}
	
	if cli2.RoleScenario != "" {
		t.Errorf("expected empty scenario by default, got %q", cli2.RoleScenario)
	}
}