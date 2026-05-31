package deploy

import (
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/utils"
)

// ResolvedCredential is a flattened artifact credential ready to be injected
// into the container environment.
type ResolvedCredential struct {
	Name     string
	URL      string
	Username string
	Password string
	Token    string
}

// DeployContainerConfig holds everything needed to spin up the molecule
// container in "deploy mode". Roles and collections are installed INSIDE the
// container from a requirements.yml — there are no host-side download paths.
type DeployContainerConfig struct {
	// MergedLock drives the Python version, Ansible version, and pyproject.toml.
	MergedLock *dependency.LockFile

	// InventoryPath is the absolute host-side path to the generated YAML inventory.
	InventoryPath string

	// PlaybookDir is the host-side directory mounted read-only at /deploy/playbook/.
	// It must contain:
	//   wrapper.yml        — the generated wrapper playbook
	//   <playbook>.yml     — the user or auto-generated target playbook
	//   requirements.yml   — galaxy install manifest
	PlaybookDir string

	// ExtraVarsFile is an optional host-side JSON file mounted at /deploy/extra_vars.json.
	ExtraVarsFile string

	// ContainerRegistry configures the molecule container image.
	ContainerRegistry *config.ContainerRegistry

	// ArtifactSources holds resolved credentials forwarded as env vars.
	ArtifactSources []ResolvedCredential

	// VaultToken and VaultAddr are forwarded when set (override env vars).
	VaultToken string
	VaultAddr  string
}

// containerRolesPath and containerCollectionsPath are the install targets used
// inside the container. ansible-galaxy writes here; ansible-playbook reads here.
const (
	containerRolesPath       = "/tmp/diffusion/roles"
	containerCollectionsPath = "/tmp/diffusion/collections"
)

// RunDeployContainer starts the molecule container in deploy mode.
// The container:
//  1. Installs roles + collections from /deploy/playbook/requirements.yml
//     into in-container /tmp/diffusion/{roles,collections}.
//  2. Runs ansible-playbook with the wrapper playbook.
//
// Nothing is written to the host file system for dependencies — the container
// is ephemeral and discarded on exit (--rm).
func RunDeployContainer(cfg DeployContainerConfig) error {
	image := utils.GetImageURL(cfg.ContainerRegistry)
	log.Printf(config.ColorGreen+"Using container image: %s"+config.ColorReset, image)

	args, err := buildDeployDockerArgs(cfg, image)
	if err != nil {
		return fmt.Errorf("failed to build docker args: %w", err)
	}

	log.Printf(config.ColorGreen + "Starting deploy container (roles/collections will be installed inside)..." + config.ColorReset)

	cmd := exec.Command("docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("deploy container exited with error: %w", err)
	}
	return nil
}

// buildDeployDockerArgs constructs the full `docker run` argument list.
//
// The container entrypoint is overridden to a shell one-liner that:
//   a) installs roles + collections from requirements.yml into /tmp/diffusion/
//   b) runs ansible-playbook with those paths set via ANSIBLE_* env vars
func buildDeployDockerArgs(cfg DeployContainerConfig, image string) ([]string, error) {
	args := []string{
		"run", "--rm",
		"--name", "diffusion-deploy",
	}

	// --- Volume mounts (read-only; host paths → container paths) ---

	// Inventory
	args = append(args, "-v", fmt.Sprintf("%s:/deploy/inventory.yml:ro", cfg.InventoryPath))

	// Playbook directory: wrapper.yml + target playbook + requirements.yml
	args = append(args, "-v", fmt.Sprintf("%s:/deploy/playbook:ro", cfg.PlaybookDir))

	// SSH keys (needed for Ansible SSH connections to target hosts)
	if sshDir := sshKeyDir(); sshDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/root/.ssh:ro", sshDir))
	}

	// Extra vars (optional)
	if cfg.ExtraVarsFile != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/deploy/extra_vars.json:ro", cfg.ExtraVarsFile))
	}

	// --- Environment variables ---

	args = append(args,
		"-e", "UV_VENV_CLEAR=1",
		"-e", "SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt",
		// Tell Ansible where the in-container installs will land.
		"-e", fmt.Sprintf("ANSIBLE_ROLES_PATH=%s", containerRolesPath),
		"-e", fmt.Sprintf("ANSIBLE_COLLECTIONS_PATH=%s", containerCollectionsPath),
	)

	// Vault
	vaultToken := cfg.VaultToken
	if vaultToken == "" {
		vaultToken = os.Getenv("VAULT_TOKEN")
	}
	vaultAddr := cfg.VaultAddr
	if vaultAddr == "" {
		vaultAddr = os.Getenv("VAULT_ADDR")
	}
	args = append(args,
		"-e", "VAULT_TOKEN="+vaultToken,
		"-e", "VAULT_ADDR="+vaultAddr,
	)

	// Python version from merged lock
	pythonVer := config.PinnedPythonVersion
	if cfg.MergedLock != nil && cfg.MergedLock.Python != nil && cfg.MergedLock.Python.Pinned != "" {
		pythonVer = cfg.MergedLock.Python.Pinned
	}
	args = append(args, "-e", "PYTHON_PINNED_VERSION="+pythonVer)

	// pyproject.toml from merged lock (Ansible + tool versions)
	if cfg.MergedLock != nil {
		if content, err := buildPyprojectFromLock(cfg.MergedLock); err == nil {
			args = append(args, "-e",
				"PYPROJECT_TOML_CONTENT="+base64.StdEncoding.EncodeToString([]byte(content)))
		} else {
			log.Printf(config.ColorYellow+"warning: could not generate pyproject.toml from merged lock: %v"+config.ColorReset, err)
		}
	}

	// Artifact source credentials (private Galaxy / git repos inside container)
	for _, cred := range cfg.ArtifactSources {
		key := sanitizeEnvKey(cred.Name)
		if cred.Username != "" {
			args = append(args,
				"-e", fmt.Sprintf("%s%s=%s", config.EnvGitUserPrefix, key, cred.Username),
				"-e", fmt.Sprintf("%s%s=%s", config.EnvGitPassPrefix, key, cred.Password),
			)
		}
		if cred.URL != "" {
			args = append(args, "-e", fmt.Sprintf("%s%s=%s", config.EnvGitURLPrefix, key, cred.URL))
		}
		if cred.Token != "" {
			args = append(args, "-e", fmt.Sprintf("TOKEN_%s=%s", key, cred.Token))
		}
	}

	// --- Image ---
	args = append(args, image)

	// --- Container command ---
	// A shell one-liner that installs deps then runs the playbook.
	// All paths are container-internal; nothing touches the host file system.
	playbookCmd := buildContainerCommand(cfg)
	args = append(args, "sh", "-c", playbookCmd)

	return args, nil
}

// buildContainerCommand returns the shell command executed inside the container.
// It runs ansible-galaxy to install roles+collections into /tmp/diffusion/,
// then runs ansible-playbook from that isolated environment.
func buildContainerCommand(cfg DeployContainerConfig) string {
	rolesPath := containerRolesPath
	colsPath := containerCollectionsPath

	// Step 1: create install directories
	mkdirs := fmt.Sprintf("mkdir -p %s %s", rolesPath, colsPath)

	// Step 2: install roles from requirements.yml (if any roles present)
	installRoles := fmt.Sprintf(
		"ansible-galaxy role install -r /deploy/playbook/requirements.yml -p %s --no-deps 2>/dev/null || true",
		rolesPath,
	)

	// Step 3: install collections from requirements.yml (if any collections present)
	installCols := fmt.Sprintf(
		"ansible-galaxy collection install -r /deploy/playbook/requirements.yml -p %s 2>/dev/null || true",
		colsPath,
	)

	// Step 4: run ansible-playbook with the wrapper
	playbookArgs := fmt.Sprintf(
		"ANSIBLE_ROLES_PATH=%s ANSIBLE_COLLECTIONS_PATH=%s ansible-playbook -i /deploy/inventory.yml /deploy/playbook/wrapper.yml",
		rolesPath, colsPath,
	)
	if cfg.ExtraVarsFile != "" {
		playbookArgs += " --extra-vars @/deploy/extra_vars.json"
	}

	return fmt.Sprintf("set -e && %s && %s && %s && %s",
		mkdirs, installRoles, installCols, playbookArgs)
}

// buildPyprojectFromLock generates pyproject.toml content from a merged lock.
// Delegates to the existing dependency.GeneratePyProjectContent.
func buildPyprojectFromLock(lf *dependency.LockFile) (string, error) {
	collections := make([]config.CollectionRequirement, 0, len(lf.Collections))
	for _, c := range lf.Collections {
		collections = append(collections, config.CollectionRequirement{
			Name:      c.Name,
			Namespace: c.Namespace,
			Version:   c.ResolvedVersion,
		})
	}
	toolVersions := make(map[string]string)
	for _, t := range lf.Tools {
		toolVersions[t.Name] = t.ResolvedVersion
	}
	return dependency.GeneratePyProjectContent(collections, toolVersions, lf.Python)
}

func stripScenarioPrefix(name string) string {
	if parts := splitDot(name); len(parts) == 2 {
		return parts[1]
	}
	return name
}

func splitDot(s string) []string {
	for i, c := range s {
		if c == '.' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}
