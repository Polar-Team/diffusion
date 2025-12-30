package main

import (
	"testing"
)

func TestResolveCollectionVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping API test in short mode")
	}

	api := NewGalaxyAPI()

	tests := []struct {
		name              string
		collectionName    string
		versionConstraint string
		wantErr           bool
	}{
		{
			name:              "latest version",
			collectionName:    "community.general",
			versionConstraint: "latest",
			wantErr:           false,
		},
		{
			name:              "version with >= constraint",
			collectionName:    "community.general",
			versionConstraint: ">=7.4.0",
			wantErr:           false,
		},
		{
			name:              "specific version",
			collectionName:    "community.docker",
			versionConstraint: "3.0.0",
			wantErr:           false,
		},
		{
			name:              "empty constraint (should get latest)",
			collectionName:    "kubernetes.core",
			versionConstraint: "",
			wantErr:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := api.ResolveVersion(tt.collectionName, "collection", tt.versionConstraint)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if version == "" {
					t.Errorf("ResolveCollectionVersion() returned empty version")
				} else {
					t.Logf("Resolved %s %s to version: %s", tt.collectionName, tt.versionConstraint, version)
				}
			}
		})
	}
}
