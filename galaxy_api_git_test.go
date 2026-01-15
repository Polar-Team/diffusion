package main

import (
	"testing"
)

func TestNormalizeVersion(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "version with v prefix",
			input:    "v1.2.3",
			expected: "v1.2.3",
		},
		{
			name:     "version without v prefix",
			input:    "1.2.3",
			expected: "v1.2.3",
		},
		{
			name:     "version with two digits",
			input:    "1.2",
			expected: "v1.2",
		},
		{
			name:     "branch name",
			input:    "main",
			expected: "main",
		},
		{
			name:     "branch name develop",
			input:    "develop",
			expected: "develop",
		},
		{
			name:     "single digit",
			input:    "1",
			expected: "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeVersion(tt.input)
			if result != tt.expected {
				t.Errorf("NormalizeVersion(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetLatestGitTag(t *testing.T) {
	// Test with a real public repository
	tests := []struct {
		name    string
		gitURL  string
		wantErr bool
		skip    bool
	}{
		{
			name:    "valid github repo",
			gitURL:  "https://github.com/geerlingguy/ansible-role-docker.git",
			wantErr: false,
		},
		{
			name:    "invalid repo",
			gitURL:  "https://github.com/nonexistent/repo.git",
			wantErr: false, // Should fallback to "main"
			skip:    true,  // Skip on CI/Windows due to timeout issues
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skip {
				t.Skip("Skipping test that may timeout on Windows")
			}
			tag, err := GetLatestGitTag(tt.gitURL)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetLatestGitTag() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tag == "" {
				t.Error("GetLatestGitTag() returned empty tag")
			}
			t.Logf("Latest tag for %s: %s", tt.gitURL, tag)
		})
	}
}

func TestResolveVersionFromGit(t *testing.T) {
	tests := []struct {
		name              string
		gitURL            string
		versionConstraint string
		wantErr           bool
	}{
		{
			name:              "resolve latest from github",
			gitURL:            "https://github.com/geerlingguy/ansible-role-docker.git",
			versionConstraint: "latest",
			wantErr:           false,
		},
		{
			name:              "resolve main branch",
			gitURL:            "https://github.com/geerlingguy/ansible-role-docker.git",
			versionConstraint: "main",
			wantErr:           false,
		},
		{
			name:              "specific version",
			gitURL:            "https://github.com/geerlingguy/ansible-role-docker.git",
			versionConstraint: "6.0.0",
			wantErr:           false,
		},
		{
			name:              "version constraint >=6.0.0",
			gitURL:            "https://github.com/geerlingguy/ansible-role-docker.git",
			versionConstraint: ">=6.0.0",
			wantErr:           false,
		},
		{
			name:              "version constraint >=1.0.0",
			gitURL:            "https://github.com/geerlingguy/ansible-role-docker.git",
			versionConstraint: ">=1.0.0",
			wantErr:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := ResolveVersionFromGit(tt.gitURL, tt.versionConstraint)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveVersionFromGit() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if version == "" {
				t.Error("ResolveVersionFromGit() returned empty version")
			}
			t.Logf("Resolved version for %s with constraint %s: %s", tt.gitURL, tt.versionConstraint, version)
		})
	}
}

func TestCompareVersionConstraint(t *testing.T) {
	tests := []struct {
		name              string
		version           string
		operator          string
		constraintVersion string
		expected          bool
		wantErr           bool
	}{
		// >= operator
		{
			name:              "7.9.0 >= 6.0.0",
			version:           "7.9.0",
			operator:          ">=",
			constraintVersion: "6.0.0",
			expected:          true,
		},
		{
			name:              "6.0.0 >= 6.0.0",
			version:           "6.0.0",
			operator:          ">=",
			constraintVersion: "6.0.0",
			expected:          true,
		},
		{
			name:              "5.0.0 >= 6.0.0",
			version:           "5.0.0",
			operator:          ">=",
			constraintVersion: "6.0.0",
			expected:          false,
		},
		// <= operator
		{
			name:              "6.0.0 <= 7.0.0",
			version:           "6.0.0",
			operator:          "<=",
			constraintVersion: "7.0.0",
			expected:          true,
		},
		{
			name:              "8.0.0 <= 7.0.0",
			version:           "8.0.0",
			operator:          "<=",
			constraintVersion: "7.0.0",
			expected:          false,
		},
		// == operator
		{
			name:              "6.0.0 == 6.0.0",
			version:           "6.0.0",
			operator:          "==",
			constraintVersion: "6.0.0",
			expected:          true,
		},
		{
			name:              "6.0.1 == 6.0.0",
			version:           "6.0.1",
			operator:          "==",
			constraintVersion: "6.0.0",
			expected:          false,
		},
		// > operator
		{
			name:              "7.0.0 > 6.0.0",
			version:           "7.0.0",
			operator:          ">",
			constraintVersion: "6.0.0",
			expected:          true,
		},
		{
			name:              "6.0.0 > 6.0.0",
			version:           "6.0.0",
			operator:          ">",
			constraintVersion: "6.0.0",
			expected:          false,
		},
		// < operator
		{
			name:              "5.0.0 < 6.0.0",
			version:           "5.0.0",
			operator:          "<",
			constraintVersion: "6.0.0",
			expected:          true,
		},
		{
			name:              "6.0.0 < 6.0.0",
			version:           "6.0.0",
			operator:          "<",
			constraintVersion: "6.0.0",
			expected:          false,
		},
		// With 'v' prefix
		{
			name:              "v7.9.0 >= 6.0.0",
			version:           "v7.9.0",
			operator:          ">=",
			constraintVersion: "6.0.0",
			expected:          true,
		},
		{
			name:              "7.9.0 >= v6.0.0",
			version:           "7.9.0",
			operator:          ">=",
			constraintVersion: "v6.0.0",
			expected:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CompareVersionConstraint(tt.version, tt.operator, tt.constraintVersion)
			if (err != nil) != tt.wantErr {
				t.Errorf("CompareVersionConstraint() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if result != tt.expected {
				t.Errorf("CompareVersionConstraint(%q, %q, %q) = %v, want %v",
					tt.version, tt.operator, tt.constraintVersion, result, tt.expected)
			}
		})
	}
}

func TestParseSemanticVersion(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		expected []int
		wantErr  bool
	}{
		{
			name:     "standard version",
			version:  "1.2.3",
			expected: []int{1, 2, 3},
		},
		{
			name:     "version with v prefix",
			version:  "v1.2.3",
			expected: []int{1, 2, 3},
		},
		{
			name:     "version with two parts",
			version:  "1.2",
			expected: []int{1, 2, 0},
		},
		{
			name:     "version with suffix",
			version:  "1.2.3-beta",
			expected: []int{1, 2, 3},
		},
		{
			name:     "large version numbers",
			version:  "10.20.30",
			expected: []int{10, 20, 30},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseSemanticVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseSemanticVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(result) != len(tt.expected) {
					t.Errorf("parseSemanticVersion(%q) length = %d, want %d", tt.version, len(result), len(tt.expected))
					return
				}
				for i := range result {
					if result[i] != tt.expected[i] {
						t.Errorf("parseSemanticVersion(%q)[%d] = %d, want %d", tt.version, i, result[i], tt.expected[i])
					}
				}
			}
		})
	}
}
