package deploy

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// InventoryHost represents a single host with its Ansible connection variables.
type InventoryHost struct {
	// Name is the Ansible inventory hostname (alias).
	Name string
	// Variables holds Ansible connection variables (ansible_host, ansible_user, etc.).
	Variables map[string]string
}

// InventoryGroup represents an Ansible host group.
type InventoryGroup struct {
	// Name is the group name.
	Name string
	// Hosts lists the inventory host names that belong to this group.
	Hosts []string
}

// BuildInventory generates a valid Ansible YAML inventory from the provided
// hosts, groups, and global variables. Returns the raw YAML bytes suitable for
// writing to a file or passing directly to ansible-playbook via -i.
//
// Output structure:
//
//	all:
//	  hosts:
//	    web01:
//	      ansible_host: "1.2.3.4"
//	  children:
//	    webservers:
//	      hosts:
//	        web01: {}
//	  vars:
//	    app_version: "1.0"
func BuildInventory(hosts []InventoryHost, groups []InventoryGroup, globalVars map[string]string) ([]byte, error) {
	// Build hosts map.
	hostsMap := make(map[string]interface{})
	for _, h := range hosts {
		if h.Name == "" {
			return nil, fmt.Errorf("inventory host has an empty name")
		}
		vars := make(map[string]interface{})
		// Sort keys for deterministic output.
		keys := make([]string, 0, len(h.Variables))
		for k := range h.Variables {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			vars[k] = h.Variables[k]
		}
		hostsMap[h.Name] = vars
	}

	// Build children map.
	childrenMap := make(map[string]interface{})
	for _, g := range groups {
		if g.Name == "" {
			return nil, fmt.Errorf("inventory group has an empty name")
		}
		groupHosts := make(map[string]interface{})
		for _, hName := range g.Hosts {
			groupHosts[hName] = map[string]interface{}{}
		}
		childrenMap[g.Name] = map[string]interface{}{
			"hosts": groupHosts,
		}
	}

	// Build top-level all group.
	allGroup := map[string]interface{}{
		"hosts": hostsMap,
	}
	if len(childrenMap) > 0 {
		allGroup["children"] = childrenMap
	}
	if len(globalVars) > 0 {
		sortedVars := make(map[string]interface{})
		for k, v := range globalVars {
			sortedVars[k] = v
		}
		allGroup["vars"] = sortedVars
	}

	inventory := map[string]interface{}{
		"all": allGroup,
	}

	data, err := yaml.Marshal(inventory)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal inventory: %w", err)
	}

	return data, nil
}
