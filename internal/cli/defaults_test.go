package cli

import (
	"runtime"
	"strings"
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/utils"
)

// TestDefaultConstants verifies the default configuration values
func TestDefaultConstants(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{"config.DefaultRegistryServer", config.DefaultRegistryServer, "ghcr.io"},
		{"config.DefaultRegistryProvider", config.DefaultRegistryProvider, "Public"},
		{"config.DefaultMoleculeContainerName", config.DefaultMoleculeContainerName, "polar-team/diffusion-molecule-container"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != tt.expected {
				t.Errorf("%s: got %q, want %q", tt.name, tt.value, tt.expected)
			}
		})
	}
}

// TestGetDefaultMoleculeTagFormat verifies the tag format
func TestGetDefaultMoleculeTagFormat(t *testing.T) {
	tag := utils.GetDefaultMoleculeTag()

	// Should start with "latest-"
	if !strings.HasPrefix(tag, "latest-") {
		t.Errorf("tag should start with 'latest-', got %q", tag)
	}

	// Should end with architecture
	arch := runtime.GOARCH
	expectedTag := "latest-" + arch
	if arch != "amd64" && arch != "arm64" {
		expectedTag = "latest-amd64" // Default fallback
	}

	if tag != expectedTag {
		t.Errorf("expected tag %q, got %q", expectedTag, tag)
	}
}

// TestGetDefaultMoleculeTagArchitectures tests different architectures
func TestGetDefaultMoleculeTagArchitectures(t *testing.T) {
	// This test verifies the logic, actual runtime.GOARCH is system-dependent
	tag := utils.GetDefaultMoleculeTag()

	validTags := []string{"latest-amd64", "latest-arm64"}
	valid := false
	for _, validTag := range validTags {
		if tag == validTag {
			valid = true
			break
		}
	}

	if !valid {
		t.Errorf("tag %q is not one of the valid tags: %v", tag, validTags)
	}
}

// TestDefaultRegistryConfiguration tests the complete default configuration
func TestDefaultRegistryConfiguration(t *testing.T) {
	registry := &config.ContainerRegistry{
		RegistryServer:        config.DefaultRegistryServer,
		RegistryProvider:      config.DefaultRegistryProvider,
		MoleculeContainerName: config.DefaultMoleculeContainerName,
		MoleculeContainerTag:  utils.GetDefaultMoleculeTag(),
	}

	// Verify registry server
	if registry.RegistryServer != "ghcr.io" {
		t.Errorf("expected registry server 'ghcr.io', got %q", registry.RegistryServer)
	}

	// Verify registry provider
	if registry.RegistryProvider != "Public" {
		t.Errorf("expected registry provider 'Public', got %q", registry.RegistryProvider)
	}

	// Verify container name
	if registry.MoleculeContainerName != "polar-team/diffusion-molecule-container" {
		t.Errorf("expected container name 'polar-team/diffusion-molecule-container', got %q", registry.MoleculeContainerName)
	}

	// Verify tag format
	if !strings.HasPrefix(registry.MoleculeContainerTag, "latest-") {
		t.Errorf("expected tag to start with 'latest-', got %q", registry.MoleculeContainerTag)
	}

	// Verify complete image URL
	expectedImagePrefix := "ghcr.io/polar-team/diffusion-molecule-container:latest-"
	imageURL := utils.GetImageURL(registry)
	if !strings.HasPrefix(imageURL, expectedImagePrefix) {
		t.Errorf("expected image URL to start with %q, got %q", expectedImagePrefix, imageURL)
	}
}

// TestValidateDefaultRegistryProvider tests that the default provider is valid
func TestValidateDefaultRegistryProvider(t *testing.T) {
	err := utils.ValidateRegistryProvider(config.DefaultRegistryProvider)
	if err != nil {
		t.Errorf("default registry provider should be valid, got error: %v", err)
	}
}
