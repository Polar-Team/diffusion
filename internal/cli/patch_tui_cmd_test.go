package cli

import (
	"strings"
	"testing"
)

func TestShouldUseTUI(t *testing.T) {
	cases := []struct {
		name               string
		format             string
		forceTUI           bool
		noTUI              bool
		consoleInteractive bool
		autoDisabled       bool
		want               bool
		wantErr            bool
	}{
		{"both flags error", "tree", true, true, true, false, false, true},
		{"force with yaml errors", "yaml", true, false, true, false, false, true},
		{"force non-interactive errors", "tree", true, false, false, false, false, true},
		{"force tree interactive ok", "tree", true, false, true, false, true, false},
		{"force overrides env auto-disable", "tree", true, false, true, true, true, false},
		{"no-tui wins over interactive", "tree", false, true, true, false, false, false},
		{"no-tui wins over yaml", "yaml", false, true, true, false, false, false},
		{"auto tree interactive", "tree", false, false, true, false, true, false},
		{"auto tree non-interactive", "tree", false, false, false, false, false, false},
		{"auto tree interactive but env disabled", "tree", false, false, true, true, false, false},
		{"auto yaml interactive", "yaml", false, false, true, false, false, false},
		{"auto yaml non-interactive", "yaml", false, false, false, false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := shouldUseTUI(tc.format, tc.forceTUI, tc.noTUI, tc.consoleInteractive, tc.autoDisabled)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTruthyEnvValue(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"0":     false,
		"false": false,
		"False": false,
		"1":     true,
		"true":  true,
		"True":  true,
		"yes":   true,
		"YES":   true,
		"on":    true,
		"On":    true,
		"maybe": false,
	}
	for in, want := range cases {
		if got := truthyEnvValue(in); got != want {
			t.Errorf("truthyEnvValue(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestAutoTUIDisabledByEnv(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"nothing set", map[string]string{}, false},
		{"TERM dumb", map[string]string{"TERM": "dumb"}, true},
		{"TERM DUMB (case-insensitive)", map[string]string{"TERM": "DUMB"}, true},
		{"TERM dumb with whitespace", map[string]string{"TERM": "  dumb  "}, true},
		{"TERM xterm", map[string]string{"TERM": "xterm-256color"}, false},
		{"NO_COLOR set", map[string]string{"NO_COLOR": "1"}, true},
		{"NO_COLOR empty", map[string]string{"NO_COLOR": ""}, false},
		{"DIFFUSION_NO_TUI true", map[string]string{"DIFFUSION_NO_TUI": "true"}, true},
		{"DIFFUSION_NO_TUI yes", map[string]string{"DIFFUSION_NO_TUI": "yes"}, true},
		{"DIFFUSION_NO_TUI false", map[string]string{"DIFFUSION_NO_TUI": "false"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookup := func(k string) string { return tc.env[k] }
			if got := autoTUIDisabledByEnv(lookup); got != tc.want {
				t.Errorf("autoTUIDisabledByEnv(%v) = %v, want %v", tc.env, got, tc.want)
			}
		})
	}
}

// TestPatchAnalyzeFlagValidationBeforeAnalysis verifies that conflicting
// flag combinations fail fast, before any role analysis is attempted. Any
// role name works here (including a nonexistent one) because validation
// must return an error before AnalyzeExternalRole is ever called.
func TestPatchAnalyzeFlagValidationBeforeAnalysis(t *testing.T) {
	t.Run("--tui with --format yaml", func(t *testing.T) {
		cmd := newPatchAnalyzeCmd()
		cmd.SetArgs([]string{"does-not-exist.role", "--format", "yaml", "--tui"})
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "--format tree") {
			t.Fatalf("expected format-mismatch error, got: %v", err)
		}
	})

	t.Run("--tui and --no-tui together", func(t *testing.T) {
		cmd := newPatchAnalyzeCmd()
		cmd.SetArgs([]string{"does-not-exist.role", "--tui", "--no-tui"})
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "none of the others can be") {
			t.Fatalf("expected mutually-exclusive error, got: %v", err)
		}
	})

	t.Run("unknown format", func(t *testing.T) {
		cmd := newPatchAnalyzeCmd()
		cmd.SetArgs([]string{"does-not-exist.role", "--format", "xml"})
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		err := cmd.Execute()
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "unknown format") {
			t.Fatalf("expected unknown-format error, got: %v", err)
		}
	})
}

// TestPatchAnalyzeTUIRequiresConsole overrides canRunInteractiveTUI (no
// t.Parallel, so the override can't race with other tests) to simulate a
// non-interactive console (e.g. Windows Git Bash/mintty) and verifies
// --tui fails fast, before any role analysis, with a message mentioning
// the console/mintty requirement.
func TestPatchAnalyzeTUIRequiresConsole(t *testing.T) {
	orig := canRunInteractiveTUI
	canRunInteractiveTUI = func() bool { return false }
	t.Cleanup(func() { canRunInteractiveTUI = orig })

	cmd := newPatchAnalyzeCmd()
	cmd.SetArgs([]string{"does-not-exist.role", "--tui"})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "interactive console") || !strings.Contains(err.Error(), "mintty") {
		t.Fatalf("expected a console/mintty error, got: %v", err)
	}
}
