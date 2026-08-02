package deploy

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/utils"
)

// generateSessionID returns a random UUIDv4 string for unique container naming.
func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// Fallback: use timestamp-based ID if crypto/rand fails (should never happen).
		return fmt.Sprintf("%x", b)
	}
	// Set version (4) and variant (RFC 4122) bits.
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

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

	// InventoryContent is the generated YAML inventory passed into the container
	// as a base64-encoded env var. No host-side file is created or mounted.
	InventoryContent []byte

	// PlaybookDir is the host-side directory mounted read-only at /deploy/playbook/.
	// It must contain:
	//   wrapper.yml        — the generated wrapper playbook
	//   <playbook>.yml     — the user or auto-generated target playbook
	//   requirements.yml   — galaxy install manifest
	PlaybookDir string

	// ExtraVarsFile is an optional host-side JSON file mounted at /deploy/extra_vars.json.
	ExtraVarsFile string

	// SSHKeys is a map of hostname (or "*" for all) → base64-encoded SSH private
	// key. Passed as env vars into the container, decoded at runtime to
	// /tmp/ssh-keys/<host>, and set as ansible_ssh_private_key_file in the inventory.
	SSHKeys map[string]string

	// ContainerRegistry configures the molecule container image.
	ContainerRegistry *config.ContainerRegistry

	// ArtifactSources holds resolved credentials forwarded as env vars.
	ArtifactSources []ResolvedCredential

	// VaultToken and VaultAddr are forwarded when set (override env vars).
	VaultToken string
	VaultAddr  string

	// CacheDir is the host-side directory where cached roles and collections
	// are stored. When set, the directory is mounted read-write into the
	// container so that ansible-galaxy installs persist across runs with the
	// same RunID. Empty means caching is disabled.
	CacheDir string
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

	sessionID := generateSessionID()

	args, err := buildDeployDockerArgs(cfg, image)
	if err != nil {
		return fmt.Errorf("failed to build docker args: %w", err)
	}

	log.Printf(config.ColorGreen + "Starting deploy container (roles/collections will be installed inside)..." + config.ColorReset)

	return utils.DockerRunDeployContainer(sessionID, args)
}

// buildDeployDockerArgs constructs the full `docker run` argument list.
//
// The container entrypoint is overridden to a shell one-liner that:
//
//	a) installs roles + collections from requirements.yml into /tmp/diffusion/
//	b) runs ansible-playbook with those paths set via ANSIBLE_* env vars
func buildDeployDockerArgs(cfg DeployContainerConfig, image string) ([]string, error) {
	var args []string

	// --- Volume mounts (read-only; host paths → container paths) ---

	// Inventory: inject per-host SSH key paths into the YAML in Go (proper YAML,
	// no sed hacks), then pass as base64-encoded env var.
	deployInventory := cfg.InventoryContent
	if len(cfg.SSHKeys) > 0 {
		modified, err := injectSSHKeyPaths(deployInventory, cfg.SSHKeys)
		if err != nil {
			log.Printf(config.ColorYellow+"warning: could not inject SSH key paths into inventory: %v — using original"+config.ColorReset, err)
		} else {
			deployInventory = modified
		}
	}
	inventoryB64 := base64.StdEncoding.EncodeToString(deployInventory)
	args = append(args, "-e", "DIFFUSION_INVENTORY_B64="+inventoryB64)

	// Playbook directory: wrapper.yml + target playbook + requirements.yml
	args = append(args, "-v", fmt.Sprintf("%s:/deploy/playbook:ro", cfg.PlaybookDir))

	// SSH keys (needed for Ansible SSH connections to target hosts)
	if sshDir := sshKeyDir(); sshDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/root/.ssh:ro", sshDir))
	}

	// Pass base64 SSH keys as env vars (decoded inside container, no mount needed).
	for host, keyB64 := range cfg.SSHKeys {
		envName := "SSH_KEY_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(host))
		args = append(args, "-e", envName+"="+keyB64)
	}

	// Mount additional SSH key directories referenced in the inventory
	// (handles keys outside ~/.ssh, e.g. project-local keys generated by Terraform).
	extraDirs := extractSSHKeyDirsFromContent(cfg.InventoryContent)
	for i, dir := range extraDirs {
		containerPath := fmt.Sprintf("/deploy/ssh-keys-%d", i)
		args = append(args, "-v", fmt.Sprintf("%s:%s:ro", dir, containerPath))
	}

	// Extra vars (optional)
	if cfg.ExtraVarsFile != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/deploy/extra_vars.json:ro", cfg.ExtraVarsFile))
	}

	// Cache: mount host-side roles/collections directories into the container.
	// This allows ansible-galaxy install results to persist across runs.
	if cfg.CacheDir != "" {
		rolesCacheDir := filepath.Join(cfg.CacheDir, "roles")
		collectionsCacheDir := filepath.Join(cfg.CacheDir, "collections")
		args = append(args,
			"-v", fmt.Sprintf("%s:/deploy/roles", rolesCacheDir),
			"-v", fmt.Sprintf("%s:/deploy/collections", collectionsCacheDir),
		)
	}

	// --- Environment variables ---

	args = append(args,
		"-e", "UV_VENV_CLEAR=1",
		"-e", "SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt",
		// Tell Ansible where the in-container installs will land.
		"-e", "ANSIBLE_ROLES_PATH=/deploy/roles",
		"-e", "ANSIBLE_COLLECTIONS_PATH=/deploy/collections",
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
	// Install roles and collections directly under /deploy/ so that role-internal
	// relative paths (defaults/, vars/, tasks/) resolve correctly when the
	// wrapper playbook at /deploy/playbook/wrapper.yml imports the user playbook.
	rolesPath := "/deploy/roles"
	colsPath := "/deploy/collections"

	// Step 0: configure git credentials from indexed GIT_USER_*/GIT_PASSWORD_*/GIT_URL_* env vars.
	// This replicates what dockerd-entrypoint.sh does, since we bypass the entrypoint with sh -c.
	configureGit := `i=1; while [ "$i" -le 100 ]; do ` +
		`gu=$(printenv "GIT_USER_${i}" 2>/dev/null || true); ` +
		`gp=$(printenv "GIT_PASSWORD_${i}" 2>/dev/null || true); ` +
		`gurl=$(printenv "GIT_URL_${i}" 2>/dev/null || true); ` +
		`[ -z "$gu" ] && [ -z "$gp" ] && [ -z "$gurl" ] && break; ` +
		`if [ -n "$gu" ] && [ -n "$gp" ] && [ -n "$gurl" ]; then ` +
		`git config --global url."https://${gu}:${gp}@${gurl}".insteadOf "https://${gurl}"; fi; ` +
		`i=$((i + 1)); done`

	// Step 1: create install directories and decode inventory from env var
	mkdirs := fmt.Sprintf("mkdir -p %s %s /deploy/vars", rolesPath, colsPath)
	decodeInventory := "printenv DIFFUSION_INVENTORY_B64 | base64 -d > /deploy/inventory.yml"

	// Step 2: install roles from requirements.yml (if any roles present)
	installRoles := fmt.Sprintf(
		"ansible-galaxy role install -r /deploy/playbook/requirements.yml -p %s --force 2>&1 || true",
		rolesPath,
	)

	// Step 3: install collections from requirements.yml (if any collections present)
	installCols := fmt.Sprintf(
		"ansible-galaxy collection install -r /deploy/playbook/requirements.yml -p %s 2>&1 || true",
		colsPath,
	)

	// Step 4: decode base64 SSH keys inside container (if provided via env vars).
	decodeKeyCmd := ""
	privateKeyFlag := ""
	if len(cfg.SSHKeys) > 0 {
		var decodeSteps []string
		decodeSteps = append(decodeSteps, "mkdir -p /tmp/ssh-keys")
		for host := range cfg.SSHKeys {
			envName := "SSH_KEY_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(host))
			// Use a safe filename for the wildcard key to avoid shell glob issues.
			keyFile := host
			if host == "*" {
				keyFile = "_wildcard_"
			}
			keyPath := "/tmp/ssh-keys/" + keyFile
			// Use printenv to output the raw env var value without any shell
			// interpretation — avoids corrupting base64 content.
			decodeSteps = append(decodeSteps,
				fmt.Sprintf("printenv %s | base64 -d > %s && chmod 600 %s", envName, keyPath, keyPath))
		}
		decodeKeyCmd = strings.Join(decodeSteps, " && ") + " && "

		// If there's a wildcard key "*", use --private-key globally.
		if _, hasWildcard := cfg.SSHKeys["*"]; hasWildcard && len(cfg.SSHKeys) == 1 {
			privateKeyFlag = " --private-key /tmp/ssh-keys/_wildcard_"
		} else if _, hasWildcard := cfg.SSHKeys["*"]; hasWildcard {
			// Mixed mode: per-host keys are in inventory, wildcard as fallback.
			privateKeyFlag = " --private-key /tmp/ssh-keys/_wildcard_"
		} else if len(cfg.SSHKeys) == 1 {
			// Single named key that doesn't match any host (e.g. "default") —
			// use it as --private-key for all hosts. injectSSHKeyPaths already
			// injects it into inventory, but this provides an extra safety net.
			for keyName := range cfg.SSHKeys {
				privateKeyFlag = " --private-key /tmp/ssh-keys/" + keyName
			}
		}
	}

	// Step 5: rewrite SSH key paths in the inventory if extra key dirs are mounted.
	// Per-host SSH key paths are already injected into InventoryContent via Go
	// (injectSSHKeyPathsForDeploy in buildDeployDockerArgs), so no sed needed for those.
	inventoryFile := "/deploy/inventory.yml"
	extraDirs := extractSSHKeyDirsFromContent(cfg.InventoryContent)
	rewriteCmd := ""

	if len(extraDirs) > 0 {
		inventoryFile = "/tmp/inventory-rewritten.yml"
		var builder strings.Builder

		// Rewrite mounted extra SSH key dirs.
		for i, dir := range extraDirs {
			containerPath := fmt.Sprintf("/deploy/ssh-keys-%d", i)
			hostEscaped := strings.ReplaceAll(dir, "/", "\\/")
			containerEscaped := strings.ReplaceAll(containerPath, "/", "\\/")
			fmt.Fprintf(&builder, "s/%s/%s/g;", hostEscaped, containerEscaped)
			hostWinEscaped := strings.ReplaceAll(strings.ReplaceAll(dir, "\\", "/"), "/", "\\/")
			if hostWinEscaped != hostEscaped {
				fmt.Fprintf(&builder, "s/%s/%s/g;", hostWinEscaped, containerEscaped)
			}
		}

		sedExpr := builder.String()
		if sedExpr != "" {
			rewriteCmd = fmt.Sprintf("sed '%s' /deploy/inventory.yml > %s && ", sedExpr, inventoryFile)
		} else {
			rewriteCmd = fmt.Sprintf("cp /deploy/inventory.yml %s && ", inventoryFile)
		}
	}

	// Step 6: run ansible-playbook with the wrapper
	playbookArgs := fmt.Sprintf(
		"ANSIBLE_ROLES_PATH=%s ANSIBLE_COLLECTIONS_PATH=%s ansible-playbook -i %s /deploy/playbook/wrapper.yml",
		rolesPath, colsPath, inventoryFile,
	)
	if cfg.ExtraVarsFile != "" {
		playbookArgs += " --extra-vars @/deploy/extra_vars.json"
	}
	playbookArgs += privateKeyFlag

	return fmt.Sprintf("set -e && %s && %s && %s && %s && %s && %s%s%s",
		configureGit, mkdirs, decodeInventory, installRoles, installCols, decodeKeyCmd, rewriteCmd, playbookArgs)
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

// stripScenarioPrefix removes the "scenario." prefix from a name string.
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
