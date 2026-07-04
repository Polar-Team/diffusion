package cli

import (
	"fmt"
	"os"
	"strings"

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
  #—| <type>         Type annotation (above the variable or #—! marker)
  variable: value    The variable declaration itself
  #—? <description>  Description annotation (below the variable or #—! marker)

Required variable marker (no YAML declaration needed):
  #—| <type>         Type annotation (optional, above)
  #—! variable_name  Marks variable as required — no default value
  #—? <description>  Description annotation (optional, below)

Optional variable marker (default set in Jinja2 logic, no YAML declaration):
  #—| <type>         Type annotation (optional, above)
  #—& variable_name  Marks variable as optional — default handled in templates
  #—? <description>  Description annotation (optional, below)

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
		if v.IsDuplicate {
			fmt.Fprintf(os.Stderr, "  \033[31m⚠ %s (%s) [source: %s] — DUPLICATE in: %s\033[0m\n",
				v.Name, typeStr, v.Source, joinSources(v.AllSources))
		} else if v.Required {
			fmt.Fprintf(os.Stderr, "  \033[33m* %s (%s) [source: %s] — REQUIRED\033[0m\n",
				v.Name, typeStr, v.Source)
		} else if v.Optional {
			fmt.Fprintf(os.Stderr, "  \033[36m~ %s (%s) [source: %s] — OPTIONAL\033[0m\n",
				v.Name, typeStr, v.Source)
		} else {
			fmt.Fprintf(os.Stderr, "  - %s (%s) [source: %s]\n", v.Name, typeStr, v.Source)
		}
	}

	// Print duplicate summary warning
	duplicateCount := 0
	requiredCount := 0
	optionalCount := 0
	for _, v := range variables {
		if v.IsDuplicate {
			duplicateCount++
		}
		if v.Required {
			requiredCount++
		}
		if v.Optional {
			optionalCount++
		}
	}
	if requiredCount > 0 || optionalCount > 0 {
		fmt.Fprintf(os.Stderr, "\n")
		if requiredCount > 0 {
			fmt.Fprintf(os.Stderr, "\033[33m* %d required variable(s) — must be provided by the user (no default).\033[0m\n", requiredCount)
		}
		if optionalCount > 0 {
			fmt.Fprintf(os.Stderr, "\033[36m~ %d optional variable(s) — default handled in Jinja2 logic.\033[0m\n", optionalCount)
		}
	}
	if duplicateCount > 0 {
		fmt.Fprintf(os.Stderr, "\n\033[31m⚠ WARNING: %d variable(s) declared in multiple YAML sources (duplicates).\033[0m\n", duplicateCount)
		fmt.Fprintf(os.Stderr, "\033[31m  Values from defaults/ take priority. Consider removing duplicate declarations.\033[0m\n")
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

// joinSources joins multiple source names into a comma-separated string.
func joinSources(sources []string) string {
	return strings.Join(sources, ", ")
}
