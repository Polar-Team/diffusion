package cache

import (
	"strings"
	"testing"
)

func TestValidateCacheID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid hex", "a1b2c3d4", false},
		{"mixed case letters", "AbCdEf123", false},
		{"with underscore", "my_cache_id", false},
		{"with dash", "my-cache-id", false},
		{"single char", "a", false},
		{"64 chars exactly", strings.Repeat("a", 64), false},
		{"65 chars rejected", strings.Repeat("a", 65), true},
		{"empty string", "", true},
		{"path traversal", "../x", true},
		{"forward slash", "a/b", true},
		{"backslash", "a\\b", true},
		{"spaces", "a b", true},
		{"unicode", "café", true},
		{"dot dot alone", "..", true},
		{"just slash", "/", true},
		{"embedded null byte", "a\x00b", true},
		{"trailing newline", "abc\n", true},
		{"leading newline", "\nabc", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCacheID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateCacheID(%q) error = %v, wantErr %v", tt.id, err, tt.wantErr)
			}
		})
	}
}
