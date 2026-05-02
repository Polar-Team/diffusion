package utils

import (
	"testing"
)

func TestValidateToolCompatibility(t *testing.T) {
	tests := []struct {
		name          string
		tool          string
		toolVersion   string
		pythonVersion string
		wantOK        bool
	}{
		// Ansible tests - using supported Python versions 3.11, 3.12, 3.13
		{"Ansible 13 with Python 3.13", "ansible", "13.1.0", "3.13", true},
		{"Ansible 13 with Python 3.11", "ansible", "13.1.0", "3.11", true},
		{"Ansible 10 with Python 3.12", "ansible", "10.0.0", "3.12", true},
		{"Ansible 9 with Python 3.11", "ansible", "9.0.0", "3.11", true},

		// Molecule tests - using supported Python versions
		{"Molecule 25 with Python 3.13", "molecule", "25.12.0", "3.13", true},
		{"Molecule 25 with Python 3.11", "molecule", "25.12.0", "3.11", true},
		{"Molecule 24 with Python 3.12", "molecule", "24.0.0", "3.12", true},
		{"Molecule 6 with Python 3.11", "molecule", "6.0.0", "3.11", true},

		// ansible-lint tests - using supported Python versions
		{"ansible-lint 24 with Python 3.13", "ansible-lint", "24.0.0", "3.13", true},
		{"ansible-lint 24 with Python 3.11", "ansible-lint", "24.0.0", "3.11", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOK, message := ValidateToolCompatibility(tt.tool, tt.toolVersion, tt.pythonVersion)
			if gotOK != tt.wantOK {
				t.Errorf("ValidateToolCompatibility() = %v, want %v. Message: %s", gotOK, tt.wantOK, message)
			}
			if !gotOK && message == "" {
				t.Error("Expected error message for incompatible versions")
			}
		})
	}
}

func TestGetCompatibleVersion(t *testing.T) {
	tests := []struct {
		name          string
		tool          string
		pythonVersion string
		wantVersion   string
		wantErr       bool
	}{
		{"Ansible for Python 3.11", "ansible", "3.11", ">=13.0.0", false},
		{"Ansible for Python 3.13", "ansible", "3.13", ">=13.0.0", false},
		{"Molecule for Python 3.11", "molecule", "3.11", ">=25.0.0", false},
		{"Molecule for Python 3.13", "molecule", "3.13", ">=25.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetCompatibleVersion(tt.tool, tt.pythonVersion)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCompatibleVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.wantVersion {
				t.Errorf("GetCompatibleVersion() = %v, want %v", got, tt.wantVersion)
			}
		})
	}
}

func TestAdjustToolVersionsForPython(t *testing.T) {
	tests := []struct {
		name          string
		toolVersions  map[string]string
		pythonVersion string
		wantAdjusted  bool
	}{
		{
			name: "Python 3.11 with incompatible tools",
			toolVersions: map[string]string{
				"ansible":      ">=13.0.0",
				"molecule":     ">=25.0.0",
				"ansible-lint": ">=24.0.0",
			},
			pythonVersion: "3.11",
			wantAdjusted:  false,
		},
		{
			name: "Python 3.13 with compatible tools",
			toolVersions: map[string]string{
				"ansible":      ">=13.0.0",
				"molecule":     ">=25.0.0",
				"ansible-lint": ">=24.0.0",
			},
			pythonVersion: "3.13",
			wantAdjusted:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adjusted, warnings := AdjustToolVersionsForPython(tt.toolVersions, tt.pythonVersion)

			hasWarnings := len(warnings) > 0
			if hasWarnings != tt.wantAdjusted {
				t.Errorf("AdjustToolVersionsForPython() warnings = %v, wantAdjusted %v", warnings, tt.wantAdjusted)
			}

			if len(adjusted) != len(tt.toolVersions) {
				t.Errorf("AdjustToolVersionsForPython() returned %d tools, want %d", len(adjusted), len(tt.toolVersions))
			}

			// Log warnings for inspection
			for _, warning := range warnings {
				t.Logf("Warning: %s", warning)
			}
		})
	}
}

func TestGetRecommendedVersions(t *testing.T) {
	tests := []struct {
		name          string
		pythonVersion string
		wantAnsible   string
		wantMolecule  string
	}{
		{
			name:          "Python 3.11",
			pythonVersion: "3.11",
			wantAnsible:   ">=10.0.0",
			wantMolecule:  ">=24.0.0",
		},
		{
			name:          "Python 3.12",
			pythonVersion: "3.12",
			wantAnsible:   ">=10.0.0",
			wantMolecule:  ">=24.0.0",
		},
		{
			name:          "Python 3.13",
			pythonVersion: "3.13",
			wantAnsible:   ">=10.0.0",
			wantMolecule:  ">=24.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetRecommendedVersions(tt.pythonVersion)

			if got["ansible"] != tt.wantAnsible {
				t.Errorf("GetRecommendedVersions() ansible = %v, want %v", got["ansible"], tt.wantAnsible)
			}
			if got["molecule"] != tt.wantMolecule {
				t.Errorf("GetRecommendedVersions() molecule = %v, want %v", got["molecule"], tt.wantMolecule)
			}
		})
	}
}