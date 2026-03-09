package galaxy

import (
	"strings"
	"testing"
)

func TestResolveCollectionVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping API test in short mode")
	}

	api := NewGalaxyAPI()

	tests := []struct {
		name              string
		namespace         string
		collectionName    string
		versionConstraint string
		wantErr           bool
	}{
		{
			name:              "latest version",
			namespace:         "community",
			collectionName:    "general",
			versionConstraint: "latest",
			wantErr:           false,
		},
		{
			name:              "version with >= constraint",
			namespace:         "community",
			collectionName:    "general",
			versionConstraint: ">=7.4.0",
			wantErr:           false,
		},
		{
			name:              "specific version",
			namespace:         "community",
			collectionName:    "docker",
			versionConstraint: "3.0.0",
			wantErr:           false,
		},
		{
			name:              "empty constraint (should get latest)",
			namespace:         "kubernetes",
			collectionName:    "core",
			versionConstraint: "",
			wantErr:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			version, err := api.ResolveVersion(tt.namespace, tt.collectionName, "collection", tt.versionConstraint)
			if (err != nil) != tt.wantErr {
				t.Errorf("ResolveVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if version == "" {
					t.Errorf("ResolveCollectionVersion() returned empty version")
				} else {
					displayName := strings.Join([]string{tt.namespace, tt.collectionName}, ".")
					t.Logf("Resolved %s %s to version: %s", displayName, tt.versionConstraint, version)
				}
			}
		})
	}
}
