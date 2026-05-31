package deploy

import (
	"strings"
	"testing"

	"diffusion/internal/dependency"
)

func lockWithGalaxyRole(ns, name, version, resolved string) dependency.LockFile {
	return dependency.LockFile{
		Version: dependency.LockFileVersion,
		Roles: []dependency.LockFileEntry{
			{Namespace: ns, Name: name, Version: version, ResolvedVersion: resolved, Type: "role"},
		},
	}
}

func lockWithCollection(ns, name, version, resolved string) dependency.LockFile {
	return dependency.LockFile{
		Version: dependency.LockFileVersion,
		Collections: []dependency.LockFileEntry{
			{Namespace: ns, Name: name, Version: version, ResolvedVersion: resolved, Type: "collection"},
		},
	}
}

func TestGenerateRequirements_NilLock(t *testing.T) {
	out, err := GenerateRequirements(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.TrimSpace(string(out)) != "---" {
		t.Errorf("expected '---' for nil lock, got: %q", string(out))
	}
}

func TestGenerateRequirements_GalaxyRole(t *testing.T) {
	lock := lockWithGalaxyRole("geerlingguy", "docker", ">=6.0.0", "6.3.0")
	out, err := GenerateRequirements(&lock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "geerlingguy.docker") {
		t.Errorf("expected 'geerlingguy.docker' in output, got:\n%s", s)
	}
	// resolved version should take precedence
	if !strings.Contains(s, "6.3.0") {
		t.Errorf("expected resolved version '6.3.0', got:\n%s", s)
	}
}

func TestGenerateRequirements_GalaxyRoleFallsBackToVersion(t *testing.T) {
	lock := lockWithGalaxyRole("myorg", "common", ">=1.0.0", "") // no resolved version
	out, err := GenerateRequirements(&lock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), ">=1.0.0") {
		t.Errorf("expected constraint '>=1.0.0' as fallback version, got:\n%s", string(out))
	}
}

func TestGenerateRequirements_GitRole(t *testing.T) {
	lock := dependency.LockFile{
		Version: dependency.LockFileVersion,
		Roles: []dependency.LockFileEntry{
			{
				Namespace:       "myorg",
				Name:            "myrole",
				Version:         "main",
				ResolvedVersion: "abc123",
				Source:          "git",
				Src:             "https://github.com/myorg/ansible-myrole.git",
				Type:            "role",
			},
		},
	}
	out, err := GenerateRequirements(&lock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "scm: git") {
		t.Errorf("expected 'scm: git' for git role, got:\n%s", s)
	}
	if !strings.Contains(s, "https://github.com/myorg/ansible-myrole.git") {
		t.Errorf("expected src URL in output, got:\n%s", s)
	}
}

func TestGenerateRequirements_Collection(t *testing.T) {
	lock := lockWithCollection("community", "general", ">=7.0.0", "7.5.0")
	out, err := GenerateRequirements(&lock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "community.general") {
		t.Errorf("expected 'community.general' in collections, got:\n%s", s)
	}
	if !strings.Contains(s, "7.5.0") {
		t.Errorf("expected resolved version '7.5.0', got:\n%s", s)
	}
}

func TestGenerateRequirements_CollectionWithCustomSource(t *testing.T) {
	lock := dependency.LockFile{
		Version: dependency.LockFileVersion,
		Collections: []dependency.LockFileEntry{
			{
				Namespace: "myorg",
				Name:      "private",
				Version:   "1.0.0",
				Src:       "https://my-galaxy.example.com",
				Type:      "collection",
			},
		},
	}
	out, err := GenerateRequirements(&lock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "https://my-galaxy.example.com") {
		t.Errorf("expected custom source URL in output, got:\n%s", string(out))
	}
}

func TestGenerateRequirements_Header(t *testing.T) {
	lock := lockWithCollection("ns", "col", ">=1.0.0", "1.2.0")
	out, err := GenerateRequirements(&lock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(string(out), "---") {
		t.Errorf("expected YAML header '---', got:\n%s", string(out))
	}
}

func TestGalaxyRoleName(t *testing.T) {
	cases := []struct{ ns, name, want string }{
		{"geerlingguy", "docker", "geerlingguy.docker"},
		{"", "docker", "docker"},
	}
	for _, c := range cases {
		got := galaxyRoleName(c.ns, c.name)
		if got != c.want {
			t.Errorf("galaxyRoleName(%q, %q) = %q, want %q", c.ns, c.name, got, c.want)
		}
	}
}

func TestCollectionFQCN(t *testing.T) {
	cases := []struct{ ns, name, want string }{
		{"community", "general", "community.general"},
		{"", "ns.col", "ns.col"},
		{"myorg", "default.mycol", "myorg.mycol"},
	}
	for _, c := range cases {
		got := collectionFQCN(c.ns, c.name)
		if got != c.want {
			t.Errorf("collectionFQCN(%q, %q) = %q, want %q", c.ns, c.name, got, c.want)
		}
	}
}
