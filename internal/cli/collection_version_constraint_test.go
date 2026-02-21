package cli

import (
	"testing"
)

func TestVersionConstraintLogic(t *testing.T) {
	tests := []struct {
		name               string
		inputConstraint    string
		resolvedVersion    string
		expectedConstraint string
	}{
		{
			name:               "no constraint - should add >=",
			inputConstraint:    "",
			resolvedVersion:    "12.2.0",
			expectedConstraint: ">=12.2.0",
		},
		{
			name:               "latest - should add >=",
			inputConstraint:    "latest",
			resolvedVersion:    "12.2.0",
			expectedConstraint: ">=12.2.0",
		},
		{
			name:               "with >= constraint - keep as is",
			inputConstraint:    ">=7.4.0",
			resolvedVersion:    "12.2.0",
			expectedConstraint: ">=7.4.0",
		},
		{
			name:               "with == constraint - keep as is",
			inputConstraint:    "==7.4.0",
			resolvedVersion:    "7.4.0",
			expectedConstraint: "==7.4.0",
		},
		{
			name:               "with <= constraint - keep as is",
			inputConstraint:    "<=8.0.0",
			resolvedVersion:    "8.0.0",
			expectedConstraint: "<=8.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the logic from add-collection command
			configVersionConstraint := tt.inputConstraint
			if configVersionConstraint == "" || configVersionConstraint == "latest" {
				configVersionConstraint = ">=" + tt.resolvedVersion
			}

			if configVersionConstraint != tt.expectedConstraint {
				t.Errorf("Expected constraint %q, got %q", tt.expectedConstraint, configVersionConstraint)
			}
		})
	}
}
