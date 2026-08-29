package deploy

import "testing"

func TestValidateSSHKeyName(t *testing.T) {
	validCases := []string{
		"web01",
		"192.168.1.1",
		"group:webservers",
		"*",
		"my-host.example.com",
		"host_01",
	}
	for _, name := range validCases {
		t.Run("valid_"+name, func(t *testing.T) {
			if err := validateSSHKeyName(name); err != nil {
				t.Errorf("expected %q to be valid, got error: %v", name, err)
			}
			if err := ValidateSSHKeyName(name); err != nil {
				t.Errorf("ValidateSSHKeyName: expected %q to be valid, got error: %v", name, err)
			}
		})
	}

	invalidCases := []string{
		"",
		"; rm -rf / #",
		"`id`",
		"$(id)",
		"host name",
		"host\"name",
		"host'name",
		"host\nname",
		"host|name",
		"host&&name",
		"host;name",
		".",
		"..",
		"group:.",
		"group:..",
	}
	for _, name := range invalidCases {
		t.Run("invalid_"+name, func(t *testing.T) {
			if err := validateSSHKeyName(name); err == nil {
				t.Errorf("expected %q to be invalid, got no error", name)
			}
			if err := ValidateSSHKeyName(name); err == nil {
				t.Errorf("ValidateSSHKeyName: expected %q to be invalid, got no error", name)
			}
		})
	}
}

func TestValidateSSHKeyName_WildcardSentinelCollision(t *testing.T) {
	// The literal "*" wildcard must remain valid.
	if err := validateSSHKeyName("*"); err != nil {
		t.Errorf("expected %q to be valid, got error: %v", "*", err)
	}

	// Names that sanitize to the wildcard's env var suffix (SSH_KEY_WILDCARD)
	// must be rejected. The env var side is upper-cased, so the collision is
	// case-insensitive.
	envCollisions := []string{"wildcard", "WILDCARD", "Wildcard", "WiLdCaRd"}
	for _, name := range envCollisions {
		t.Run("env_collision_"+name, func(t *testing.T) {
			if err := validateSSHKeyName(name); err == nil {
				t.Errorf("expected %q to be rejected (env var collision with wildcard sentinel), got no error", name)
			}
		})
	}

	// A name that sanitizes to the wildcard's filename (_wildcard_) must be
	// rejected. The filename side is case-sensitive.
	if err := validateSSHKeyName("_wildcard_"); err == nil {
		t.Errorf("expected %q to be rejected (filename collision with wildcard sentinel), got no error", "_wildcard_")
	}
}

func TestValidateSSHKeyNames(t *testing.T) {
	t.Run("valid map", func(t *testing.T) {
		m := map[string]string{
			"web01":            "base64key1",
			"group:webservers": "base64key2",
			"*":                "base64key3",
		}
		if err := validateSSHKeyNames(m); err != nil {
			t.Errorf("expected valid map, got error: %v", err)
		}
	})

	t.Run("invalid map", func(t *testing.T) {
		m := map[string]string{
			"web01":        "base64key1",
			"; rm -rf / #": "base64key2",
		}
		if err := validateSSHKeyNames(m); err == nil {
			t.Error("expected error for invalid map, got nil")
		}
	})

	t.Run("nil map", func(t *testing.T) {
		if err := validateSSHKeyNames(nil); err != nil {
			t.Errorf("expected nil map to be valid, got error: %v", err)
		}
	})
}
