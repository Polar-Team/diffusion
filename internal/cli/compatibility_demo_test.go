package cli

import (
	"fmt"
	"testing"

	"diffusion/internal/utils"
)

// TestCompatibilityDemo demonstrates the compatibility checking system
func TestCompatibilityDemo(t *testing.T) {
	fmt.Println("\n=== Python Version Compatibility Demo ===")
	fmt.Println()

	// Demo 1: Python 3.11 with latest tools (should adjust some)
	fmt.Println("Demo 1: Python 3.11 with latest tools (should adjust)")
	fmt.Println("----------------------------------------------------")
	toolVersions := map[string]string{
		"ansible":      ">=13.0.0",
		"molecule":     ">=25.0.0",
		"ansible-lint": ">=24.0.0",
		"yamllint":     ">=1.35.0",
	}

	fmt.Println("Original versions:")
	for tool, version := range toolVersions {
		fmt.Printf("  %s: %s\n", tool, version)
	}

	adjusted, warnings := utils.AdjustToolVersionsForPython(toolVersions, "3.11")

	fmt.Println("\nAdjusted versions:")
	for tool, version := range adjusted {
		fmt.Printf("  %s: %s\n", tool, version)
	}

	fmt.Println("\nWarnings:")
	for _, warning := range warnings {
		fmt.Printf("  ⚠️  %s\n", warning)
	}

	// Demo 2: Python 3.13 with compatible tools (no adjustment needed)
	fmt.Println("\n\nDemo 2: Python 3.13 with compatible tools (no adjustment)")
	fmt.Println("----------------------------------------------------------")
	toolVersions2 := map[string]string{
		"ansible":      ">=13.0.0",
		"molecule":     ">=25.0.0",
		"ansible-lint": ">=24.0.0",
		"yamllint":     ">=1.35.0",
	}

	fmt.Println("Original versions:")
	for tool, version := range toolVersions2 {
		fmt.Printf("  %s: %s\n", tool, version)
	}

	adjusted2, warnings2 := utils.AdjustToolVersionsForPython(toolVersions2, "3.13")

	fmt.Println("\nAdjusted versions:")
	for tool, version := range adjusted2 {
		fmt.Printf("  %s: %s\n", tool, version)
	}

	if len(warnings2) > 0 {
		fmt.Println("\nWarnings:")
		for _, warning := range warnings2 {
			fmt.Printf("  ⚠️  %s\n", warning)
		}
	} else {
		fmt.Println("\n✅ No adjustments needed - all tools compatible!")
	}

	// Demo 3: Recommended versions for supported Python versions
	fmt.Println("\n\nDemo 3: Recommended versions for supported Python versions")
	fmt.Println("-----------------------------------------------------------")
	for _, pyVer := range []string{"3.11", "3.12", "3.13"} {
		fmt.Printf("\nPython %s:\n", pyVer)
		recommended := utils.GetRecommendedVersions(pyVer)
		for tool, version := range recommended {
			fmt.Printf("  %s: %s\n", tool, version)
		}
	}

	fmt.Println("\n=== Demo completed successfully! ===")
	fmt.Println()
}