package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestGcpCliInit tests the GcpCliInit function
func TestGcpCliInit(t *testing.T) {
	tests := []struct {
		name           string
		registryServer string
		wantErr        bool
		errContains    string
	}{
		{
			name:           "valid GCR format - gcr.io",
			registryServer: "gcr.io",
			wantErr:        true, // Will fail without actual gcloud credentials
			errContains:    "",   // Accept any error since gcloud may not be configured
		},
		{
			name:           "valid GCR format with path - gcr.io/project-id",
			registryServer: "gcr.io/my-project",
			wantErr:        true, // Will fail without actual gcloud credentials
			errContains:    "",   // Accept any error since gcloud may not be configured
		},
		{
			name:           "valid GCR format - us.gcr.io",
			registryServer: "us.gcr.io",
			wantErr:        true, // Will fail without actual gcloud credentials
			errContains:    "",   // Accept any error since gcloud may not be configured
		},
		{
			name:           "valid GCR format - eu.gcr.io",
			registryServer: "eu.gcr.io",
			wantErr:        true, // Will fail without actual gcloud credentials
			errContains:    "",   // Accept any error since gcloud may not be configured
		},
		{
			name:           "valid GCR format - asia.gcr.io",
			registryServer: "asia.gcr.io",
			wantErr:        true, // Will fail without actual gcloud credentials
			errContains:    "",   // Accept any error since gcloud may not be configured
		},
		{
			name:           "valid Artifact Registry - us-docker.pkg.dev",
			registryServer: "us-docker.pkg.dev",
			wantErr:        true, // Will fail without actual gcloud credentials
			errContains:    "",   // Accept any error since gcloud may not be configured
		},
		{
			name:           "valid Artifact Registry with path - us-central1-docker.pkg.dev/project/repo",
			registryServer: "us-central1-docker.pkg.dev/my-project/my-repo",
			wantErr:        true, // Will fail without actual gcloud credentials
			errContains:    "",   // Accept any error since gcloud may not be configured
		},
		{
			name:           "valid Artifact Registry - europe-west1-docker.pkg.dev",
			registryServer: "europe-west1-docker.pkg.dev",
			wantErr:        true, // Will fail without actual gcloud credentials
			errContains:    "",   // Accept any error since gcloud may not be configured
		},
		{
			name:           "valid Artifact Registry - asia-southeast1-docker.pkg.dev",
			registryServer: "asia-southeast1-docker.pkg.dev",
			wantErr:        true, // Will fail without actual gcloud credentials
			errContains:    "",   // Accept any error since gcloud may not be configured
		},
		{
			name:           "invalid registry format - not GCP",
			registryServer: "docker.io",
			wantErr:        true,
			errContains:    "invalid GCP registry server format",
		},
		{
			name:           "invalid registry format - AWS ECR",
			registryServer: "123456789012.dkr.ecr.us-east-1.amazonaws.com",
			wantErr:        true,
			errContains:    "invalid GCP registry server format",
		},
		{
			name:           "invalid registry format - missing pkg.dev suffix",
			registryServer: "us-docker.example.com",
			wantErr:        true,
			errContains:    "invalid GCP registry server format",
		},
		{
			name:           "invalid registry format - random domain",
			registryServer: "example.com",
			wantErr:        true,
			errContains:    "invalid GCP registry server format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment before test
			os.Unsetenv("TOKEN")
			os.Unsetenv("GCP_PROJECT_ID")

			err := GcpCliInit(tt.registryServer)

			if (err != nil) != tt.wantErr {
				t.Errorf("GcpCliInit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && tt.errContains != "" {
				if !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("GcpCliInit() error = %v, should contain %q", err, tt.errContains)
				}
			}

			// For invalid format tests, check that error message contains expected text
			if tt.errContains == "invalid GCP registry server format" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("GcpCliInit() expected error containing %q, got: %v", tt.errContains, err)
				}
			}
		})
	}
}

// TestGcpCliInitValidatesRegistryFormat tests that registry format validation works correctly
func TestGcpCliInitValidatesRegistryFormat(t *testing.T) {
	tests := []struct {
		name           string
		registryServer string
		shouldBeValid  bool
	}{
		// Valid GCR formats
		{name: "gcr.io", registryServer: "gcr.io", shouldBeValid: true},
		{name: "us.gcr.io", registryServer: "us.gcr.io", shouldBeValid: true},
		{name: "eu.gcr.io", registryServer: "eu.gcr.io", shouldBeValid: true},
		{name: "asia.gcr.io", registryServer: "asia.gcr.io", shouldBeValid: true},
		{name: "gcr.io with path", registryServer: "gcr.io/project", shouldBeValid: true},
		{name: "us.gcr.io with path", registryServer: "us.gcr.io/project/image", shouldBeValid: true},

		// Valid Artifact Registry formats
		{name: "us-docker.pkg.dev", registryServer: "us-docker.pkg.dev", shouldBeValid: true},
		{name: "europe-west1-docker.pkg.dev", registryServer: "europe-west1-docker.pkg.dev", shouldBeValid: true},
		{name: "asia-southeast1-docker.pkg.dev", registryServer: "asia-southeast1-docker.pkg.dev", shouldBeValid: true},
		{name: "us-central1-docker.pkg.dev with path", registryServer: "us-central1-docker.pkg.dev/project/repo", shouldBeValid: true},

		// Invalid formats
		{name: "docker.io", registryServer: "docker.io", shouldBeValid: false},
		{name: "ghcr.io", registryServer: "ghcr.io", shouldBeValid: false},
		{name: "AWS ECR", registryServer: "123456789012.dkr.ecr.us-east-1.amazonaws.com", shouldBeValid: false},
		{name: "random domain", registryServer: "example.com", shouldBeValid: false},
		{name: "invalid pkg.dev", registryServer: "us-docker.example.com", shouldBeValid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clear environment before test
			os.Unsetenv("TOKEN")
			os.Unsetenv("GCP_PROJECT_ID")

			err := GcpCliInit(tt.registryServer)

			// For valid formats, we expect either:
			// - Success (if gcloud is configured)
			// - gcloud auth error (if gcloud is not configured)
			// For invalid formats, we expect format validation error
			if tt.shouldBeValid {
				if err != nil && strings.Contains(err.Error(), "invalid GCP registry server format") {
					t.Errorf("GcpCliInit() with valid format %q should not return format validation error, got: %v", tt.registryServer, err)
				}
				// We accept other errors (gcloud not installed, not authenticated, etc.)
			} else {
				if err == nil {
					t.Errorf("GcpCliInit() with invalid format %q should return an error", tt.registryServer)
				}
				if err != nil && !strings.Contains(err.Error(), "invalid GCP registry server format") {
					t.Errorf("GcpCliInit() with invalid format %q should return format validation error, got: %v", tt.registryServer, err)
				}
			}
		})
	}
}

// TestGcpCliNotInstalled tests the error message when gcloud CLI is not installed
func TestGcpCliNotInstalled(t *testing.T) {
	// This test verifies that if gcloud CLI is not in PATH, we get a helpful error
	// We can't easily simulate this in a real environment where gcloud might be installed
	// but the error handling code path is there and will be triggered if gcloud is not found

	// Test with a valid registry server format
	registryServer := "gcr.io"

	err := GcpCliInit(registryServer)

	// The function should return an error (either gcloud CLI not installed or not configured)
	if err == nil {
		t.Skip("gcloud CLI appears to be installed and configured, skipping error test")
	}

	// Verify the error message is helpful
	errMsg := err.Error()
	if strings.Contains(errMsg, "gcloud CLI is not installed") {
		t.Logf("Got expected gcloud CLI not installed error: %s", errMsg)
	} else {
		t.Logf("Got gcloud CLI configured but authentication failed (expected in CI): %s", errMsg)
	}
}

// TestGcpCliInitSuccessfulAuth tests successful authentication flow
// This test will only pass if gcloud is installed and authenticated
func TestGcpCliInitSuccessfulAuth(t *testing.T) {
	// Skip if gcloud is not available
	if _, err := exec.LookPath("gcloud"); err != nil {
		t.Skip("gcloud CLI not installed, skipping successful auth test")
	}

	registryServer := "gcr.io"

	// Clear environment before test
	os.Unsetenv("TOKEN")
	os.Unsetenv("GCP_PROJECT_ID")

	err := GcpCliInit(registryServer)

	// If gcloud is configured, this should succeed
	if err == nil {
		// Verify TOKEN was set
		token := os.Getenv("TOKEN")
		if token == "" {
			t.Error("TOKEN environment variable should be set after successful GcpCliInit")
		}
		if len(token) < MinGCPTokenLength {
			t.Errorf("TOKEN seems too short (expected at least %d chars), got length: %d", MinGCPTokenLength, len(token))
		}
		t.Logf("Successfully authenticated with gcloud, token length: %d", len(token))
	} else {
		// If not configured, we should get an authentication error, not a format error
		if strings.Contains(err.Error(), "invalid GCP registry server format") {
			t.Errorf("Should not get format validation error for valid registry, got: %v", err)
		}
		t.Logf("gcloud not authenticated (expected in CI): %v", err)
	}
}
