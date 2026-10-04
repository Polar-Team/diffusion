package cli

import (
	"fmt"
	"strings"
	"testing"
)

// TestResolvePatchContainer covers container selection for the patch
// subcommands: an explicit --role must map to molecule-<role> and verify
// the container is actually running, while an empty --role falls back to
// auto-detection.
func TestResolvePatchContainer(t *testing.T) {
	orig := dockerContainerRunning
	defer func() { dockerContainerRunning = orig }()

	t.Run("running container resolves", func(t *testing.T) {
		var inspected string
		dockerContainerRunning = func(c string) error {
			inspected = c
			return nil
		}
		got, err := resolvePatchContainer("myrole")
		if err != nil {
			t.Fatalf("resolvePatchContainer: %v", err)
		}
		if got != "molecule-myrole" {
			t.Errorf("container = %q, want molecule-myrole", got)
		}
		if inspected != "molecule-myrole" {
			t.Errorf("inspected %q, want molecule-myrole", inspected)
		}
	})

	t.Run("stopped or missing container gives actionable error", func(t *testing.T) {
		dockerContainerRunning = func(c string) error {
			return fmt.Errorf("No such object: %s", c)
		}
		_, err := resolvePatchContainer("myrole")
		if err == nil {
			t.Fatal("expected an error for a container that is not running")
		}
		for _, want := range []string{
			"container molecule-myrole is not running",
			"diffusion molecule -r myrole --converge",
		} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q missing %q", err, want)
			}
		}
	})

	t.Run("whitespace-only role falls back to auto-detection", func(t *testing.T) {
		dockerContainerRunning = func(c string) error {
			t.Fatal("must not inspect a container when --role is empty")
			return nil
		}
		// No running molecule container in the test environment, so the
		// auto-detect path is expected to fail — the point is that it
		// does not go through the --role branch.
		if _, err := resolvePatchContainer("   "); err == nil {
			t.Skip("a molecule container happens to be running on this host")
		}
	})
}
