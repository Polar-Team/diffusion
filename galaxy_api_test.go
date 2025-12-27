package main

import (
	"testing"
)

func TestGetCollectionLatestVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping API test in short mode")
	}

	api := NewGalaxyAPI()

	tests := []struct {
		namespace string
		name      string
		wantErr   bool
	}{
		{"community", "general", false},
		{"community", "docker", false},
		{"kubernetes", "core", false},
		{"invalid", "notexist", true},
	}

	for _, tt := range tests {
		t.Run(tt.namespace+"."+tt.name, func(t *testing.T) {
			version, err := api.GetCollectionLatestVersion(tt.namespace, tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetCollectionLatestVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && version == "" {
				t.Errorf("GetCollectionLatestVersion() returned empty version")
			}
			if !tt.wantErr {
				t.Logf("Latest version of %s.%s: %s", tt.namespace, tt.name, version)
			}
		})
	}
}

func TestGetPythonPackageVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping API test in short mode")
	}

	tests := []struct {
		packageName string
		wantErr     bool
	}{
		{"ansible", false},
		{"molecule", false},
		{"psycopg2-binary", false},
		{"invalid-package-that-does-not-exist-12345", true},
	}

	for _, tt := range tests {
		t.Run(tt.packageName, func(t *testing.T) {
			version, err := GetPythonPackageVersion(tt.packageName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetPythonPackageVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && version == "" {
				t.Errorf("GetPythonPackageVersion() returned empty version")
			}
			if !tt.wantErr {
				t.Logf("Latest version of %s: %s", tt.packageName, version)
			}
		})
	}
}

func TestResolvePythonDependencies(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping API test in short mode")
	}

	packages := []string{
		"ansible>=10.0.0",
		"molecule>=24.0.0",
		"psycopg2-binary>=2.9.0",
	}

	resolved, err := ResolvePythonDependencies(packages)
	if err != nil {
		t.Fatalf("ResolvePythonDependencies() error = %v", err)
	}

	if len(resolved) == 0 {
		t.Error("ResolvePythonDependencies() returned empty map")
	}

	for pkg, version := range resolved {
		t.Logf("Resolved %s: %s", pkg, version)
		if version == "" {
			t.Errorf("Package %s has empty version", pkg)
		}
	}
}
