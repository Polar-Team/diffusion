package registry

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestYcCliInit tests the YcCliInit function
func TestYcCliInit(t *testing.T) {
	// Clear environment before test
	os.Unsetenv("TOKEN")
	os.Unsetenv("YC_CLOUD_ID")
	os.Unsetenv("YC_FOLDER_ID")

	err := YcCliInit()

	if err != nil {
		// yc CLI may not be installed or configured in this environment
		errMsg := err.Error()
		if strings.Contains(errMsg, "yc iam create-token failed") {
			t.Logf("YcCliInit() failed as expected without yc credentials: %s", errMsg)
		} else {
			t.Logf("YcCliInit() returned error: %s", errMsg)
		}
		return
	}

	// If no error, TOKEN must be set
	token := os.Getenv("TOKEN")
	if token == "" {
		t.Error("YcCliInit() succeeded but TOKEN env var is empty")
	}
}

// TestYcCliInitSetsEnvVars tests that YcCliInit sets the expected environment variables
func TestYcCliInitSetsEnvVars(t *testing.T) {
	// Clear environment before test
	os.Unsetenv("TOKEN")
	os.Unsetenv("YC_CLOUD_ID")
	os.Unsetenv("YC_FOLDER_ID")

	err := YcCliInit()
	if err != nil {
		t.Skipf("Skipping env var test: yc CLI not configured (%v)", err)
	}

	// TOKEN must always be set on success
	if os.Getenv("TOKEN") == "" {
		t.Error("YcCliInit() did not set TOKEN environment variable")
	}

	// YC_CLOUD_ID and YC_FOLDER_ID are optional (set only when configured)
	t.Logf("YC_CLOUD_ID=%q, YC_FOLDER_ID=%q", os.Getenv("YC_CLOUD_ID"), os.Getenv("YC_FOLDER_ID"))
}

// TestYcCliNotInstalled verifies the error when yc CLI is not in PATH
func TestYcCliNotInstalled(t *testing.T) {
	_, err := exec.LookPath("yc")
	if err == nil {
		t.Skip("yc CLI is installed; skipping not-installed error test")
	}

	// yc is not installed — YcCliInit should return an error
	initErr := YcCliInit()
	if initErr == nil {
		t.Fatal("YcCliInit() expected an error when yc is not installed, got nil")
	}

	t.Logf("Got expected error when yc not installed: %s", initErr.Error())
}

// TestYcCliInstalled checks that the yc CLI binary is present and executable
func TestYcCliInstalled(t *testing.T) {
	path, err := exec.LookPath("yc")
	if err != nil {
		t.Skipf("yc CLI is not installed (not in PATH): %v", err)
	}
	t.Logf("yc CLI found at: %s", path)
}

// TestYcRegistryServerFormat validates expected Yandex Container Registry server values
func TestYcRegistryServerFormat(t *testing.T) {
	tests := []struct {
		name           string
		registryServer string
		valid          bool
	}{
		{
			name:           "standard YC container registry",
			registryServer: "cr.yandex",
			valid:          true,
		},
		{
			name:           "YC registry with image path",
			registryServer: "cr.yandex/crp1234567890abcdef01",
			valid:          true,
		},
		{
			name:           "empty registry server",
			registryServer: "",
			valid:          false,
		},
		{
			name:           "non-YC registry",
			registryServer: "docker.io",
			valid:          false,
		},
		{
			name:           "AWS ECR registry",
			registryServer: "123456789012.dkr.ecr.us-east-1.amazonaws.com",
			valid:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidYcRegistry(tt.registryServer)
			if got != tt.valid {
				t.Errorf("IsValidYcRegistry(%q) = %v, want %v", tt.registryServer, got, tt.valid)
			}
		})
	}
}
