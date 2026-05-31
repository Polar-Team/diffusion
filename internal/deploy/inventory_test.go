package deploy

import (
	"strings"
	"testing"
)

func TestBuildInventory_SimpleHost(t *testing.T) {
	hosts := []InventoryHost{
		{Name: "web01", Variables: map[string]string{"ansible_host": "1.2.3.4", "ansible_user": "ubuntu"}},
	}
	out, err := BuildInventory(hosts, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "web01") {
		t.Errorf("expected host 'web01' in output, got:\n%s", s)
	}
	if !strings.Contains(s, "1.2.3.4") {
		t.Errorf("expected ansible_host value in output, got:\n%s", s)
	}
}

func TestBuildInventory_Groups(t *testing.T) {
	hosts := []InventoryHost{
		{Name: "web01", Variables: nil},
		{Name: "web02", Variables: nil},
	}
	groups := []InventoryGroup{
		{Name: "webservers", Hosts: []string{"web01", "web02"}},
	}
	out, err := BuildInventory(hosts, groups, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, "webservers") {
		t.Errorf("expected group 'webservers', got:\n%s", s)
	}
	if !strings.Contains(s, "children") {
		t.Errorf("expected 'children' key, got:\n%s", s)
	}
}

func TestBuildInventory_GlobalVars(t *testing.T) {
	hosts := []InventoryHost{{Name: "h1"}}
	out, err := BuildInventory(hosts, nil, map[string]string{"env": "production"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "env") || !strings.Contains(string(out), "production") {
		t.Errorf("expected global var in output, got:\n%s", string(out))
	}
}

func TestBuildInventory_EmptyHostName(t *testing.T) {
	hosts := []InventoryHost{{Name: ""}}
	_, err := BuildInventory(hosts, nil, nil)
	if err == nil {
		t.Fatal("expected error for empty host name, got nil")
	}
}

func TestBuildInventory_EmptyGroupName(t *testing.T) {
	hosts := []InventoryHost{{Name: "h1"}}
	groups := []InventoryGroup{{Name: "", Hosts: []string{"h1"}}}
	_, err := BuildInventory(hosts, groups, nil)
	if err == nil {
		t.Fatal("expected error for empty group name, got nil")
	}
}

func TestBuildInventory_NoHostsOrGroups(t *testing.T) {
	out, err := BuildInventory(nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should still produce valid YAML with "all" key
	if !strings.Contains(string(out), "all") {
		t.Errorf("expected 'all' in empty inventory, got:\n%s", string(out))
	}
}

func TestBuildInventory_DeterministicOutput(t *testing.T) {
	hosts := []InventoryHost{
		{Name: "h1", Variables: map[string]string{"z": "last", "a": "first"}},
	}
	out1, _ := BuildInventory(hosts, nil, nil)
	out2, _ := BuildInventory(hosts, nil, nil)
	if string(out1) != string(out2) {
		t.Errorf("expected deterministic output; got different results:\n%s\n---\n%s", out1, out2)
	}
}

func TestBuildInventory_NoChildrenKeyWhenNoGroups(t *testing.T) {
	hosts := []InventoryHost{{Name: "h1"}}
	out, err := BuildInventory(hosts, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(out), "children") {
		t.Errorf("unexpected 'children' key when no groups, got:\n%s", string(out))
	}
}
