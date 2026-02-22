package cli

import (
	"context"
	"runtime"
	"testing"

	"diffusion/internal/utils"
	"time"
)

// TestPromptInput tests the PromptInput function
func TestPromptInput(t *testing.T) {
	// This function reads from stdin, so we'll test it with a mock
	// In a real scenario, you'd want to refactor to accept an io.Reader
	t.Skip("Requires refactoring to accept io.Reader for testing")
}

// TestRunCommandCapture tests the runCommandCapture function
func TestRunCommandCapture(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test with a simple command that works on both Windows and Unix
	var cmd string
	var args []string
	if runtime.GOOS == "windows" {
		cmd = "cmd"
		args = []string{"/c", "echo hello"}
	} else {
		cmd = "echo"
		args = []string{"hello"}
	}

	output, err := utils.RunCommandCapture(ctx, cmd, args...)
	if err != nil {
		t.Fatalf("runCommandCapture failed: %v", err)
	}

	if output != "hello" {
		t.Errorf("unexpected output: got %q, want %q", output, "hello")
	}
}

// TestRunCommandCaptureTimeout tests timeout behavior
func TestRunCommandCaptureTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This command should timeout on most systems
	_, err := utils.RunCommandCapture(ctx, "sleep", "10")
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
}
