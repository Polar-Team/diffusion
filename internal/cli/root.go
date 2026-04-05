package cli

import (
	"fmt"
	"runtime"

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

	// Namespace flag (shared by role and collection commands)
	NamespaceFlag string

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
	OidcFlag           bool
	ForceFlag          bool
}

// Execute is the main entry point for the CLI
func Execute() {
	cli := &CLI{}

	versionInfo := fmt.Sprintf("%s\nGo version: %s\nOS/Arch: %s/%s", Version, runtime.Version(), runtime.GOOS, runtime.GOARCH)

	rootCmd := &cobra.Command{
		Use:     "diffusion",
		Short:   "Molecule workflow helper (cross-platform)",
		Version: versionInfo,
	}

	// Add all commands using factory functions
	rootCmd.AddCommand(NewRoleCmd(cli))
	rootCmd.AddCommand(NewArtifactCmd(cli))
	rootCmd.AddCommand(NewCacheCmd(cli))
	rootCmd.AddCommand(NewMoleculeCmd(cli))
	rootCmd.AddCommand(NewShowCmd(cli))
	rootCmd.AddCommand(NewDepsCmd(cli))

	if err := rootCmd.Execute(); err != nil {
		panic(err)
	}
}
