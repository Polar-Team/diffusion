package cli

import (
	"diffusion/internal/config"
	"testing"
)

func TestValidatePythonVersion(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
		wantErr bool
	}{
		{
			name:    "valid 3.13",
			version: "3.13",
			want:    "3.13",
			wantErr: false,
		},
		{
			name:    "valid 3.12",
			version: "3.12",
			want:    "3.12",
			wantErr: false,
		},
		{
			name:    "valid 3.11",
			version: "3.11",
			want:    "3.11",
			wantErr: false,
		},
		{
			name:    "normalize 3.13.11 to 3.13",
			version: "3.13.11",
			want:    "3.13",
			wantErr: false,
		},
		{
			name:    "normalize 3.12.10 to 3.12",
			version: "3.12.10",
			want:    "3.12",
			wantErr: false,
		},
		{
			name:    "normalize 3.11.9 to 3.11",
			version: "3.11.9",
			want:    "3.11",
			wantErr: false,
		},
		{
			name:    "invalid 3.10",
			version: "3.10",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid 3.9",
			version: "3.9",
			want:    "",
			wantErr: true,
		},
		{
			name:    "invalid 3.14",
			version: "3.14",
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := config.ValidatePythonVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("config.ValidatePythonVersion(%q) error = %v, wantErr %v", tt.version, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("config.ValidatePythonVersion(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}

func TestExtractMajorMinor(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{
			name:    "full version 3.13.11",
			version: "3.13.11",
			want:    "3.13",
		},
		{
			name:    "major.minor 3.13",
			version: "3.13",
			want:    "3.13",
		},
		{
			name:    "with >= operator",
			version: ">=3.13.11",
			want:    "3.13",
		},
		{
			name:    "with == operator",
			version: "==3.12.10",
			want:    "3.12",
		},
		{
			name:    "single digit",
			version: "3",
			want:    "3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := config.ExtractMajorMinor(tt.version)
			if got != tt.want {
				t.Errorf("config.ExtractMajorMinor(%q) = %q, want %q", tt.version, got, tt.want)
			}
		})
	}
}
