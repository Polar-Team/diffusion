package cli

import (
	"github.com/spf13/cobra"
)

// Version is set at build time via ldflags
var Version = "dev"

// CLI holds all command-line flags and state
type CLI struct {
	// Role flags
	RoleInitFlag    bool
	RoleFlag        string
	OrgFlag         string
	RoleScenario    string
	AddRoleFlag     string
	RoleSrcFlag     string
	RoleScmFlag     string
	RoleVersionFlag string

	// Collection flags
	AddCollectionFlag string

	// Molecule flags
	TagFlag            string
	ConvergeFlag       bool
	VerifyFlag         bool
	TestsOverWriteFlag bool
	LintFlag           bool
	IdempotenceFlag    bool
	DestroyFlag        bool
	WipeFlag           bool
	CIMode             bool
}

// Execute is the main entry point for the CLI
func Execute() {
	cli := &CLI{}

	rootCmd := &cobra.Command{
		Use:   "diffusion",
		Short: "Molecule workflow helper (cross-platform)",
	}

	// Add all commands using factory functions
	rootCmd.AddCommand(NewRoleCmd(cli))
	rootCmd.AddCommand(NewArtifactCmd(cli))
	rootCmd.AddCommand(NewCacheCmd(cli))
	rootCmd.AddCommand(NewMoleculeCmd(cli))
	rootCmd.AddCommand(NewShowCmd(cli))
	rootCmd.AddCommand(NewDepsCmd(cli))
	rootCmd.AddCommand(NewVersionCmd(cli))

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}
