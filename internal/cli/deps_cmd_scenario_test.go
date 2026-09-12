package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestDepsSubcommandsHaveScenarioFlag(t *testing.T) {
	depsCmd := NewDepsCmd(&CLI{})

	tests := []string{"lock", "check", "sync"}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			var target = findSubcommand(depsCmd, name)
			if target == nil {
				t.Fatalf("subcommand %q not found", name)
			}

			flag := target.Flags().Lookup("scenario")
			if flag == nil {
				t.Fatalf("subcommand %q has no --scenario flag", name)
			}
			if flag.Shorthand != "s" {
				t.Errorf("subcommand %q --scenario shorthand = %q, want %q", name, flag.Shorthand, "s")
			}
			if flag.DefValue != "" {
				t.Errorf("subcommand %q --scenario default = %q, want empty", name, flag.DefValue)
			}
		})
	}
}

func findSubcommand(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}
