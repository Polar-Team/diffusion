package role

import (
	"fmt"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestMetaYamlIndentation(t *testing.T) {
	meta := &Meta{
		GalaxyInfo: &GalaxyInfo{
			RoleName:          "test_role",
			Namespace:         "test_ns",
			Author:            "Test",
			Description:       "A test role",
			License:           "MIT",
			MinAnsibleVersion: "2.10",
			Platforms: []Platform{
				{OsName: "Ubuntu", Versions: []string{"focal", "jammy"}},
				{OsName: "Debian", Versions: []string{"bullseye", "bookworm"}},
			},
			GalaxyTags: []string{"docker", "container"},
		},
		Collections: []string{"community.general", "community.docker"},
	}

	// Use the production marshalYaml4Indent function
	data, err := marshalYaml4Indent(meta)
	if err != nil {
		t.Fatal(err)
	}
	result := string(data)

	fmt.Println("=== marshalYaml4Indent output ===")
	fmt.Println("---")
	fmt.Print(result)
	fmt.Println("=== END ===")

	// Verify round-trip: parse the fixed YAML back
	var meta2 Meta
	if err := yaml.Unmarshal(data, &meta2); err != nil {
		t.Fatalf("Round-trip parse failed: %v", err)
	}
	if meta2.GalaxyInfo.RoleName != "test_role" {
		t.Errorf("Round-trip: expected role_name 'test_role', got %q", meta2.GalaxyInfo.RoleName)
	}
	if len(meta2.GalaxyInfo.Platforms) != 2 {
		t.Errorf("Round-trip: expected 2 platforms, got %d", len(meta2.GalaxyInfo.Platforms))
	}
	if len(meta2.GalaxyInfo.Platforms[0].Versions) != 2 {
		t.Errorf("Round-trip: expected 2 versions for Ubuntu, got %d", len(meta2.GalaxyInfo.Platforms[0].Versions))
	}
	if meta2.GalaxyInfo.Platforms[0].Versions[0] != "focal" {
		t.Errorf("Round-trip: expected first version 'focal', got %q", meta2.GalaxyInfo.Platforms[0].Versions[0])
	}
	if len(meta2.Collections) != 2 {
		t.Errorf("Round-trip: expected 2 collections, got %d", len(meta2.Collections))
	}

	// Verify 4-space indentation after "versions:"
	lines := strings.Split(result, "\n")
	for i, line := range lines {
		if strings.Contains(line, "versions:") && i+1 < len(lines) {
			nextLine := lines[i+1]
			versionsIndent := len(line) - len(strings.TrimLeft(line, " "))
			itemIndent := len(nextLine) - len(strings.TrimLeft(nextLine, " "))
			diff := itemIndent - versionsIndent
			if diff != 4 {
				t.Errorf("Expected 4-space indent after 'versions:', got %d (line %d: %q -> line %d: %q)",
					diff, i, line, i+1, nextLine)
			}
		}
	}
	t.Log("Round-trip and indentation verification passed")
}
