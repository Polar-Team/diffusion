package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// TestExists tests the exists helper function
func TestExists(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T) string
		expected bool
	}{
		{
			name: "file exists",
			setup: func(t *testing.T) string {
				tmpFile, err := os.CreateTemp("", "test-*.txt")
				if err != nil {
					t.Fatal(err)
				}
				defer tmpFile.Close()
				return tmpFile.Name()
			},
			expected: true,
		},
		{
			name: "file does not exist",
			setup: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "nonexistent.txt")
			},
			expected: false,
		},
		{
			name: "directory exists",
			setup: func(t *testing.T) string {
				return t.TempDir()
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup(t)
			result := exists(path)
			if result != tt.expected {
				t.Errorf("exists(%s) = %v, want %v", path, result, tt.expected)
			}
		})
	}
}

// TestCopyFile tests the copyFile function
func TestCopyFile(t *testing.T) {
	// Create a temporary source file
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")
	dstPath := filepath.Join(tmpDir, "dest.txt")

	content := []byte("test content")
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		t.Fatal(err)
	}

	// Test copying
	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	// Verify destination file exists and has correct content
	dstContent, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}

	if string(dstContent) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", dstContent, content)
	}
}

// TestCopyDir tests the copyDir function
func TestCopyDir(t *testing.T) {
	tmpDir := t.TempDir()
	srcDir := filepath.Join(tmpDir, "source")
	dstDir := filepath.Join(tmpDir, "dest")

	// Create source directory structure
	if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"file1.txt":        "content1",
		"subdir/file2.txt": "content2",
	}

	for path, content := range files {
		fullPath := filepath.Join(srcDir, path)
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Test copying directory
	if err := copyDir(srcDir, dstDir); err != nil {
		t.Fatalf("copyDir failed: %v", err)
	}

	// Verify all files were copied
	for path, expectedContent := range files {
		dstPath := filepath.Join(dstDir, path)
		content, err := os.ReadFile(dstPath)
		if err != nil {
			t.Errorf("failed to read %s: %v", dstPath, err)
			continue
		}
		if string(content) != expectedContent {
			t.Errorf("content mismatch for %s: got %q, want %q", path, content, expectedContent)
		}
	}
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

// BenchmarkCopyFile benchmarks the copyFile function
func BenchmarkCopyFile(b *testing.B) {
	tmpDir := b.TempDir()
	srcPath := filepath.Join(tmpDir, "source.txt")

	// Create a 1MB test file
	content := make([]byte, 1024*1024)
	if err := os.WriteFile(srcPath, content, 0644); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dstPath := filepath.Join(tmpDir, fmt.Sprintf("dest%d.txt", i))
		if err := copyFile(srcPath, dstPath); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkExists benchmarks the exists function
func BenchmarkExists(b *testing.B) {
	tmpFile, err := os.CreateTemp("", "bench-*.txt")
	if err != nil {
		b.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = exists(tmpFile.Name())
	}
}
