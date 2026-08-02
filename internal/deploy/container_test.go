package deploy

import (
	"strings"
	"testing"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
)

func TestBuildContainerCommand_GitCredentials(t *testing.T) {
	cfg := DeployContainerConfig{
		SSHKeys:          nil,
		InventoryContent: []byte("all:\n  hosts:\n    h1: {}"),
	}
	cmd := buildContainerCommand(cfg)

	// Should contain git credential configuration loop.
	if !strings.Contains(cmd, "git config --global") {
		t.Error("expected git credential config in container command")
	}
	if !strings.Contains(cmd, "GIT_USER_") {
		t.Error("expected GIT_USER_ reference in credential loop")
	}
}

func TestBuildContainerCommand_DecodesInventoryWithPrintenv(t *testing.T) {
	cfg := DeployContainerConfig{
		InventoryContent: []byte("all:\n  hosts:\n    h1: {}"),
	}
	cmd := buildContainerCommand(cfg)

	if !strings.Contains(cmd, "printenv DIFFUSION_INVENTORY_B64") {
		t.Error("expected 'printenv DIFFUSION_INVENTORY_B64' for safe base64 decode")
	}
	if strings.Contains(cmd, "echo") {
		t.Error("unexpected 'echo' in container command — should use printenv")
	}
}

func TestBuildContainerCommand_SSHKeyWildcard(t *testing.T) {
	cfg := DeployContainerConfig{
		SSHKeys:          map[string]string{"*": "dGVzdA=="},
		InventoryContent: []byte("all:\n  hosts:\n    h1: {}"),
	}
	cmd := buildContainerCommand(cfg)

	// Should decode to _wildcard_ filename, not literal "*".
	if !strings.Contains(cmd, "/tmp/ssh-keys/_wildcard_") {
		t.Errorf("expected _wildcard_ filename, got:\n%s", cmd)
	}
	if strings.Contains(cmd, "/tmp/ssh-keys/*") {
		t.Error("unexpected glob pattern '/tmp/ssh-keys/*' in command")
	}
	// Should use --private-key flag.
	if !strings.Contains(cmd, "--private-key /tmp/ssh-keys/_wildcard_") {
		t.Error("expected --private-key flag with _wildcard_ path")
	}
}

func TestBuildContainerCommand_SSHKeyPerHost(t *testing.T) {
	cfg := DeployContainerConfig{
		SSHKeys:          map[string]string{"web01": "a2V5MQ==", "web02": "a2V5Mg=="},
		InventoryContent: []byte("all:\n  hosts:\n    web01: {}\n    web02: {}"),
	}
	cmd := buildContainerCommand(cfg)

	if !strings.Contains(cmd, "printenv SSH_KEY_WEB01") {
		t.Error("expected printenv for SSH_KEY_WEB01")
	}
	if !strings.Contains(cmd, "printenv SSH_KEY_WEB02") {
		t.Error("expected printenv for SSH_KEY_WEB02")
	}
	// No --private-key flag since no wildcard.
	if strings.Contains(cmd, "--private-key") {
		t.Error("unexpected --private-key flag with per-host keys only")
	}
}

func TestBuildContainerCommand_DeployVarsDir(t *testing.T) {
	cfg := DeployContainerConfig{
		InventoryContent: []byte("all:\n  hosts:\n    h1: {}"),
	}
	cmd := buildContainerCommand(cfg)

	// Should create /deploy/vars directory.
	if !strings.Contains(cmd, "/deploy/vars") {
		t.Error("expected /deploy/vars in mkdir command")
	}
}

func TestBuildContainerCommand_RolesInstallPath(t *testing.T) {
	cfg := DeployContainerConfig{
		InventoryContent: []byte("all:\n  hosts:\n    h1: {}"),
	}
	cmd := buildContainerCommand(cfg)

	// Roles should install to /deploy/roles.
	if !strings.Contains(cmd, "-p /deploy/roles") {
		t.Errorf("expected roles installed to /deploy/roles, got:\n%s", cmd)
	}
	// Collections should install to /deploy/collections.
	if !strings.Contains(cmd, "-p /deploy/collections") {
		t.Errorf("expected collections installed to /deploy/collections, got:\n%s", cmd)
	}
}

func TestBuildContainerCommand_ExtraVars(t *testing.T) {
	cfg := DeployContainerConfig{
		ExtraVarsFile:    "/tmp/extra.json",
		InventoryContent: []byte("all:\n  hosts:\n    h1: {}"),
	}
	cmd := buildContainerCommand(cfg)

	if !strings.Contains(cmd, "--extra-vars @/deploy/extra_vars.json") {
		t.Error("expected extra vars flag in playbook command")
	}
}

func TestBuildContainerCommand_NoExtraVars(t *testing.T) {
	cfg := DeployContainerConfig{
		InventoryContent: []byte("all:\n  hosts:\n    h1: {}"),
	}
	cmd := buildContainerCommand(cfg)

	if strings.Contains(cmd, "--extra-vars") {
		t.Error("unexpected --extra-vars when ExtraVarsFile is empty")
	}
}

func TestBuildContainerCommand_SetE(t *testing.T) {
	cfg := DeployContainerConfig{
		InventoryContent: []byte("all:\n  hosts:\n    h1: {}"),
	}
	cmd := buildContainerCommand(cfg)

	if !strings.HasPrefix(cmd, "set -e") {
		t.Error("expected command to start with 'set -e'")
	}
}

func TestBuildPyprojectFromLock(t *testing.T) {
	lock := &dependency.LockFile{
		Version: dependency.LockFileVersion,
		Python:  &config.PythonVersion{Pinned: "3.13"},
		Tools: []dependency.LockFileEntry{
			{Name: "ansible", ResolvedVersion: "10.0.0"},
		},
		Collections: []dependency.LockFileEntry{
			{Namespace: "community", Name: "general", ResolvedVersion: "7.0.0"},
		},
	}
	content, err := buildPyprojectFromLock(lock)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(content, "ansible") {
		t.Error("expected 'ansible' in pyproject content")
	}
}
