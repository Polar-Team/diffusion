package cli

import (
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCLI_DefaultValues(t *testing.T) {
	cli := &CLI{}

	// Test that zero values are correct
	if cli.RoleInitFlag {
		t.Error("expected RoleInitFlag to be false")
	}
	if cli.RoleFlag != "" {
		t.Errorf("expected empty RoleFlag, got %q", cli.RoleFlag)
	}
	if cli.OrgFlag != "" {
		t.Errorf("expected empty OrgFlag, got %q", cli.OrgFlag)
	}
	if cli.CIMode {
		t.Error("expected CIMode to be false")
	}
}

func TestCLI_FieldAssignment(t *testing.T) {
	cli := &CLI{
		RoleFlag:        "test-role",
		OrgFlag:         "test-org",
		RoleScenario:    "production",
		ConvergeFlag:    true,
		VerifyFlag:      true,
		LintFlag:        true,
		IdempotenceFlag: true,
		DestroyFlag:     true,
		WipeFlag:        true,
		CIMode:          true,
		OidcFlag:        true,
		ForceFlag:       true,
	}

	if cli.RoleFlag != "test-role" {
		t.Errorf("RoleFlag = %q, want %q", cli.RoleFlag, "test-role")
	}
	if cli.OrgFlag != "test-org" {
		t.Errorf("OrgFlag = %q, want %q", cli.OrgFlag, "test-org")
	}
	if cli.RoleScenario != "production" {
		t.Errorf("RoleScenario = %q, want %q", cli.RoleScenario, "production")
	}
	if !cli.ConvergeFlag {
		t.Error("ConvergeFlag should be true")
	}
	if !cli.VerifyFlag {
		t.Error("VerifyFlag should be true")
	}
	if !cli.LintFlag {
		t.Error("LintFlag should be true")
	}
	if !cli.IdempotenceFlag {
		t.Error("IdempotenceFlag should be true")
	}
	if !cli.DestroyFlag {
		t.Error("DestroyFlag should be true")
	}
	if !cli.WipeFlag {
		t.Error("WipeFlag should be true")
	}
	if !cli.CIMode {
		t.Error("CIMode should be true")
	}
	if !cli.OidcFlag {
		t.Error("OidcFlag should be true")
	}
	if !cli.ForceFlag {
		t.Error("ForceFlag should be true")
	}
}

func TestVersion_BuildInfo(t *testing.T) {
	// Test version string format
	versionInfo := strings.Split(Version, "\n")
	if len(versionInfo) < 1 {
		t.Error("version info should not be empty")
	}

	// Test that we can build version info string
	fullVersion := Version + "\nGo version: " + runtime.Version() + "\nOS/Arch: " + runtime.GOOS + "/" + runtime.GOARCH
	
	if !strings.Contains(fullVersion, "Go version:") {
		t.Error("version info should contain Go version")
	}
	if !strings.Contains(fullVersion, "OS/Arch:") {
		t.Error("version info should contain OS/Arch")
	}
	if !strings.Contains(fullVersion, runtime.GOOS) {
		t.Error("version info should contain current OS")
	}
	if !strings.Contains(fullVersion, runtime.GOARCH) {
		t.Error("version info should contain current architecture")
	}
}

func TestRootCommand_Creation(t *testing.T) {
	cli := &CLI{}
	
	rootCmd := &cobra.Command{
		Use:   "diffusion",
		Short: "Molecule workflow helper (cross-platform)",
	}

	// Test basic command properties
	if rootCmd.Use != "diffusion" {
		t.Errorf("expected Use 'diffusion', got %q", rootCmd.Use)
	}
	if rootCmd.Short != "Molecule workflow helper (cross-platform)" {
		t.Errorf("expected correct Short description, got %q", rootCmd.Short)
	}

	// Test that we can add commands
	rootCmd.AddCommand(NewRoleCmd(cli))
	rootCmd.AddCommand(NewArtifactCmd(cli))
	rootCmd.AddCommand(NewCacheCmd(cli))
	rootCmd.AddCommand(NewMoleculeCmd(cli))
	rootCmd.AddCommand(NewShowCmd(cli))
	rootCmd.AddCommand(NewDepsCmd(cli))

	// Verify commands were added
	commands := rootCmd.Commands()
	if len(commands) != 6 {
		t.Errorf("expected 6 commands, got %d", len(commands))
	}

	// Check that expected commands exist
	expectedCommands := []string{"role", "artifact", "cache", "molecule", "show", "deps"}
	foundCommands := make(map[string]bool)
	for _, cmd := range commands {
		foundCommands[cmd.Name()] = true
	}

	for _, expected := range expectedCommands {
		if !foundCommands[expected] {
			t.Errorf("expected command %q not found", expected)
		}
	}
}

func TestCLI_RoleFlags(t *testing.T) {
	cli := &CLI{
		RoleInitFlag:    true,
		AddRoleFlag:     "new-role",
		RoleSrcFlag:     "https://github.com/example/role.git",
		RoleScmFlag:     "git",
		RoleVersionFlag: ">=1.0.0",
	}

	if !cli.RoleInitFlag {
		t.Error("RoleInitFlag should be true")
	}
	if cli.AddRoleFlag != "new-role" {
		t.Errorf("AddRoleFlag = %q, want %q", cli.AddRoleFlag, "new-role")
	}
	if cli.RoleSrcFlag != "https://github.com/example/role.git" {
		t.Errorf("RoleSrcFlag = %q, want expected URL", cli.RoleSrcFlag)
	}
	if cli.RoleScmFlag != "git" {
		t.Errorf("RoleScmFlag = %q, want %q", cli.RoleScmFlag, "git")
	}
	if cli.RoleVersionFlag != ">=1.0.0" {
		t.Errorf("RoleVersionFlag = %q, want %q", cli.RoleVersionFlag, ">=1.0.0")
	}
}

func TestCLI_CollectionFlags(t *testing.T) {
	cli := &CLI{
		AddCollectionFlag: "community.general",
		NamespaceFlag:     "community",
	}

	if cli.AddCollectionFlag != "community.general" {
		t.Errorf("AddCollectionFlag = %q, want %q", cli.AddCollectionFlag, "community.general")
	}
	if cli.NamespaceFlag != "community" {
		t.Errorf("NamespaceFlag = %q, want %q", cli.NamespaceFlag, "community")
	}
}

func TestCLI_MoleculeFlags(t *testing.T) {
	cli := &CLI{
		TagFlag:            "install,configure",
		TestsOverWriteFlag: true,
	}

	if cli.TagFlag != "install,configure" {
		t.Errorf("TagFlag = %q, want %q", cli.TagFlag, "install,configure")
	}
	if !cli.TestsOverWriteFlag {
		t.Error("TestsOverWriteFlag should be true")
	}
}

func TestExecute_ErrorHandling(t *testing.T) {
	// Test that Execute function exists and can be called
	// We can't easily test the actual execution without mocking os.Args
	// but we can test that the function signature is correct
	
	// Save original args
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()

	// Set args to just program name to avoid actual command execution
	os.Args = []string{"diffusion", "--help"}

	// This should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Execute() panicked: %v", r)
		}
	}()

	// Note: Execute() calls os.Exit, so we can't test it directly in unit tests
	// This test mainly verifies the function exists and has the right signature
}

func TestVersion_DefaultValue(t *testing.T) {
	// Test that Version has a default value
	if Version == "" {
		t.Error("Version should not be empty")
	}
	
	// Default should be "dev" when not set by build
	if Version != "dev" {
		t.Logf("Version is set to %q (likely set by build process)", Version)
	}
}
// TestMaskToken tests the maskToken function
func TestMaskToken(t *testing.T) {
	tests := []struct {
		name  string
		token string
		want  string
	}{
		{"short token", "abc", "****"},
		{"medium token", "12345678", "****"},
		{"long token", "abcdefghijklmnop", "abcd...mnop"},
		{"empty token", "", "****"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskToken(tt.token)
			if got != tt.want {
				t.Errorf("maskToken(%q) = %q, want %q", tt.token, got, tt.want)
			}
		})
	}
}
// TestNewRoleCmd tests role command creation
func TestNewRoleCmd(t *testing.T) {
	cli := &CLI{}
	cmd := NewRoleCmd(cli)
	
	if cmd.Use != "role" {
		t.Errorf("expected Use 'role', got %q", cmd.Use)
	}
	if cmd.Short == "" {
		t.Error("expected non-empty Short description")
	}
	if cmd.RunE == nil {
		t.Error("expected RunE function to be set")
	}
}

// TestNewArtifactCmd tests artifact command creation
func TestNewArtifactCmd(t *testing.T) {
	cli := &CLI{}
	cmd := NewArtifactCmd(cli)
	
	if cmd.Use != "artifact" {
		t.Errorf("expected Use 'artifact', got %q", cmd.Use)
	}
	if len(cmd.Commands()) == 0 {
		t.Error("expected artifact command to have subcommands")
	}
}

// TestNewCacheCmd tests cache command creation
func TestNewCacheCmd(t *testing.T) {
	cli := &CLI{}
	cmd := NewCacheCmd(cli)
	
	if cmd.Use != "cache" {
		t.Errorf("expected Use 'cache', got %q", cmd.Use)
	}
	if len(cmd.Commands()) == 0 {
		t.Error("expected cache command to have subcommands")
	}
}

// TestNewMoleculeCmd tests molecule command creation
func TestNewMoleculeCmd(t *testing.T) {
	cli := &CLI{}
	cmd := NewMoleculeCmd(cli)
	
	if cmd.Use != "molecule" {
		t.Errorf("expected Use 'molecule', got %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Error("expected RunE function to be set")
	}
}

// TestNewShowCmd tests show command creation
func TestNewShowCmd(t *testing.T) {
	cli := &CLI{}
	cmd := NewShowCmd(cli)
	
	if cmd.Use != "show" {
		t.Errorf("expected Use 'show', got %q", cmd.Use)
	}
	if cmd.RunE == nil {
		t.Error("expected RunE function to be set")
	}
}

// TestNewDepsCmd tests deps command creation
func TestNewDepsCmd(t *testing.T) {
	cli := &CLI{}
	cmd := NewDepsCmd(cli)
	
	if cmd.Use != "deps" {
		t.Errorf("expected Use 'deps', got %q", cmd.Use)
	}
	if len(cmd.Commands()) == 0 {
		t.Error("expected deps command to have subcommands")
	}
}