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

	// Demo 1: Python 3.9 with incompatible tools
	fmt.Println("Demo 1: Python 3.9 with latest tools (should adjust)")
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

	adjusted, warnings := utils.AdjustToolVersionsForPython(toolVersions, "3.9")

	fmt.Println("\nAdjusted versions:")
	for tool, version := range adjusted {
		fmt.Printf("  %s: %s\n", tool, version)
	}

	fmt.Println("\nWarnings:")
	for _, warning := range warnings {
		fmt.Printf("  ⚠️  %s\n", warning)
	}

	// Verify adjustments were made
	if adjusted["ansible"] != ">=9.0.0" {
		t.Errorf("Expected ansible to be adjusted to >=9.0.0, got %s", adjusted["ansible"])
	}
	if adjusted["molecule"] != ">=6.0.0" {
		t.Errorf("Expected molecule to be adjusted to >=6.0.0, got %s", adjusted["molecule"])
	}
	if adjusted["ansible-lint"] != ">=6.0.0" {
		t.Errorf("Expected ansible-lint to be adjusted to >=6.0.0, got %s", adjusted["ansible-lint"])
	}

	// Demo 2: Python 3.10 with compatible tools
	fmt.Println("\n\nDemo 2: Python 3.10 with compatible tools (no adjustment)")
	fmt.Println("----------------------------------------------------------")
	toolVersions2 := map[string]string{
		"ansible":      ">=10.0.0",
		"molecule":     ">=24.0.0",
		"ansible-lint": ">=24.0.0",
		"yamllint":     ">=1.35.0",
	}

	fmt.Println("Original versions:")
	for tool, version := range toolVersions2 {
		fmt.Printf("  %s: %s\n", tool, version)
	}

	adjusted2, warnings2 := utils.AdjustToolVersionsForPython(toolVersions2, "3.10")

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

	// Demo 3: Recommended versions
	fmt.Println("\n\nDemo 3: Recommended versions for different Python versions")
	fmt.Println("-----------------------------------------------------------")
	for _, pyVer := range []string{"3.9", "3.10", "3.13"} {
		fmt.Printf("\nPython %s:\n", pyVer)
		recommended := utils.GetRecommendedVersions(pyVer)
		for tool, version := range recommended {
			fmt.Printf("  %s: %s\n", tool, version)
		}
	}

	fmt.Println("\n=== Demo completed successfully! ===")
	fmt.Println()
}
