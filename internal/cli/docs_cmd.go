package cli

import (
	"fmt"
	"os"

	"diffusion/internal/docs"

	"github.com/spf13/cobra"
)

// NewDocsCmd creates the `diffusion docs` command for generating role documentation.
func NewDocsCmd(_ *CLI) *cobra.Command {
	var rolePath string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "docs",
		Short: "Generate role variable documentation in README.md",
		Long: `Scans Ansible role files for variables and generates a documentation
section in README.md.

The command scans:
  - defaults/main.yml — role default variables
  - vars/main.yml — role variables
  - templates/ — Jinja2 template files for {{ variable }} references
  - tasks/ — task files for {{ variable }} references

Variable annotations (placed in defaults/main.yml or vars/main.yml):
  #—| <type>         Type annotation (above the variable declaration)
  variable: value    The variable declaration itself
  #—? <description>  Description annotation (below the variable declaration)

Supported types: string, int, bool, list, map, float, dict, path, etc.

The generated documentation is placed between markers in README.md:
  <!-- begin role_variables -->
  ... generated table ...
  <!-- end role_variables -->

If the markers already exist, the content between them is replaced.
If no markers exist, the section is appended to the end of README.md.

EXAMPLES
  # Generate docs for role in current directory
  diffusion docs

  # Generate docs for a role in a specific path
  diffusion docs --path ./roles/my_role

  # Preview without writing (dry-run)
  diffusion docs --dry-run`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDocs(rolePath, dryRun)
		},
	}

	cmd.Flags().StringVarP(&rolePath, "path", "p", ".", "Path to the Ansible role directory")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print generated documentation without writing to README.md")

	return cmd
}

// runDocs executes the docs generation logic.
func runDocs(rolePath string, dryRun bool) error {
	// Resolve the role path
	if rolePath == "" || rolePath == "." {
		var err error
		rolePath, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Check that the path exists
	info, err := os.Stat(rolePath)
	if err != nil {
		return fmt.Errorf("role path %q does not exist: %w", rolePath, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("role path %q is not a directory", rolePath)
	}

	// Scan for variables
	fmt.Fprintf(os.Stderr, "Scanning role variables in: %s\n", rolePath)
	variables, err := docs.ScanRoleVariables(rolePath)
	if err != nil {
		return fmt.Errorf("failed to scan role variables: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Found %d variable(s)\n", len(variables))

	if len(variables) == 0 {
		fmt.Fprintln(os.Stderr, "No variables found. Ensure your role has defaults/main.yml, vars/main.yml, or template files.")
		return nil
	}

	// Print summary to stderr (informational output)
	for _, v := range variables {
		typeStr := v.Type
		if typeStr == "" {
			typeStr = "untyped"
		}
		fmt.Fprintf(os.Stderr, "  - %s (%s) [source: %s]\n", v.Name, typeStr, v.Source)
	}

	if dryRun {
		// Dry-run: output the generated section to stdout (data output)
		section := docs.GenerateVariablesSection(variables)
		fmt.Println(section)
		return nil
	}

	// Update README.md
	if err := docs.UpdateReadme(rolePath, variables); err != nil {
		return fmt.Errorf("failed to update README.md: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\nREADME.md updated successfully with %d variable(s)\n", len(variables))
	return nil
}
