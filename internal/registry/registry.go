package registry

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"diffusion/internal/config"
	"diffusion/internal/utils"
)

// YcCliInit runs yc commands and sets env variables YC_TOKEN, YC_CLOUD_ID, YC_FOLDER_ID
func YcCliInit() error {
	// yc iam create-token
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	token, err := utils.RunCommandCapture(ctx, "yc", "iam", "create-token")
	if err != nil {
		return fmt.Errorf("yc iam create-token failed: %v (%s)", err, token)
	}
	_ = os.Setenv("TOKEN", token)

	cloudID, _ := utils.RunCommandCapture(ctx, "yc", "config", "get", "cloud-id")
	if cloudID != "" {
		_ = os.Setenv("YC_CLOUD_ID", cloudID)
	}

	folderID, _ := utils.RunCommandCapture(ctx, "yc", "config", "get", "folder-id")
	if folderID != "" {
		_ = os.Setenv("YC_FOLDER_ID", folderID)
	}
	return nil
}

// AwsCliInit runs AWS CLI commands and retrieves ECR authorization token
// Sets TOKEN environment variable for Docker authentication
// Extracts region from registry server and sets AWS_REGION environment variable
func AwsCliInit(registryServer string) error {
	// Check if AWS CLI is installed
	if _, err := exec.LookPath("aws"); err != nil {
		return fmt.Errorf("AWS CLI is not installed or not in PATH. Please install AWS CLI to use AWS ECR authentication. Visit: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Extract region from registry server (e.g., "123456789.dkr.ecr.us-east-1.amazonaws.com")
	// Format: <account-id>.dkr.ecr.<region>.amazonaws.com
	parts := strings.Split(registryServer, ".")
	if len(parts) != 6 {
		return fmt.Errorf("invalid AWS ECR registry server format: %s (expected format: <account-id>.dkr.ecr.<region>.amazonaws.com)", registryServer)
	}

	// Validate ECR format: parts should be [account-id, dkr, ecr, region, amazonaws, com]
	if parts[1] != "dkr" || parts[2] != "ecr" || parts[4] != "amazonaws" || parts[5] != "com" {
		return fmt.Errorf("invalid AWS ECR registry server format: %s (expected format: <account-id>.dkr.ecr.<region>.amazonaws.com)", registryServer)
	}

	region := parts[3]

	// Get ECR authorization token using AWS CLI
	// This returns a base64-encoded authorization token
	// Note: utils.RunCommandCapture automatically trims whitespace from the output
	token, err := utils.RunCommandCapture(ctx, "aws", "ecr", "get-login-password", "--region", region)
	if err != nil {
		// Don't include AWS CLI error details in case they contain sensitive info
		return fmt.Errorf("aws ecr get-login-password failed for region %s. Ensure AWS CLI is configured with valid credentials", region)
	}

	// Set TOKEN environment variable for Docker authentication
	if err := os.Setenv("TOKEN", token); err != nil {
		return fmt.Errorf("failed to set TOKEN environment variable: %w", err)
	}

	// Set AWS region for reference (may be used by scripts or other tools)
	if err := os.Setenv("AWS_REGION", region); err != nil {
		return fmt.Errorf("failed to set AWS_REGION environment variable: %w", err)
	}

	return nil
}

// IsValidGcpRegistry validates if the registry server is a valid GCP registry format
// Supports:
// - Container Registry: gcr.io, us.gcr.io, eu.gcr.io, asia.gcr.io
// - Artifact Registry: <region>-docker.pkg.dev
func IsValidGcpRegistry(registryServer string) bool {
	// Container Registry formats
	// Examples: gcr.io, us.gcr.io, gcr.io/project, asia.gcr.io/project/image
	if registryServer == "gcr.io" || strings.HasPrefix(registryServer, "gcr.io/") {
		return true
	}
	if strings.HasSuffix(registryServer, ".gcr.io") || strings.Contains(registryServer, ".gcr.io/") {
		return true
	}

	// Artifact Registry formats
	// Examples: us-docker.pkg.dev, europe-west1-docker.pkg.dev, us-docker.pkg.dev/project/repo
	if strings.HasSuffix(registryServer, "-docker.pkg.dev") || strings.Contains(registryServer, "-docker.pkg.dev/") {
		return true
	}

	return false
}

// GcpCliInit runs gcloud CLI commands and retrieves access token
// Sets TOKEN environment variable for Docker authentication
// Supports both gcr.io and Artifact Registry (pkg.dev) formats
func GcpCliInit(registryServer string) error {
	// Check if gcloud CLI is installed
	if _, err := exec.LookPath("gcloud"); err != nil {
		return fmt.Errorf("gcloud CLI is not installed or not in PATH. Please install gcloud CLI to use GCP authentication. Visit: https://cloud.google.com/sdk/docs/install")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Validate GCP registry format
	if !IsValidGcpRegistry(registryServer) {
		return fmt.Errorf("invalid GCP registry server format: %s (expected format: gcr.io or <region>-docker.pkg.dev)", registryServer)
	}

	// Get GCP access token using gcloud CLI
	// This returns an OAuth2 access token that can be used for Docker authentication
	token, err := utils.RunCommandCapture(ctx, "gcloud", "auth", "print-access-token")
	if err != nil {
		// Don't include gcloud error details in case they contain sensitive info
		return fmt.Errorf("gcloud auth print-access-token failed. Ensure gcloud CLI is configured and authenticated (run 'gcloud auth login')")
	}

	// Set TOKEN environment variable for Docker authentication
	if err := os.Setenv("TOKEN", token); err != nil {
		return fmt.Errorf("failed to set TOKEN environment variable: %w", err)
	}

	// Try to get the project ID if available (optional, may fail if not set)
	// gcloud may return empty string or "(unset)" when project is not configured
	projectID, _ := utils.RunCommandCapture(ctx, "gcloud", "config", "get-value", "project")
	if projectID != "" && projectID != config.GcloudUnsetValue {
		if err := os.Setenv(config.EnvGCPProjectID, projectID); err != nil {
			// Non-fatal error, just log it
			log.Printf("warning: failed to set %s environment variable: %v", config.EnvGCPProjectID, err)
		}
	}

	return nil
}
