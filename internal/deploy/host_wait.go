package deploy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"diffusion/internal/config"
	"diffusion/internal/utils"

	"gopkg.in/yaml.v3"
)

// WaitConfig holds the configuration for the host reachability wait phase.
type WaitConfig struct {
	// InitialDelay is the pause before the first probe attempt.
	// Useful for machines that need time to complete cloud-init.
	InitialDelay time.Duration
	// Interval is the time to wait between retry probes.
	Interval time.Duration
	// Timeout is the hard deadline. WaitForHosts returns an error if all hosts
	// are not reachable by this time.
	Timeout time.Duration
	// MaxAttempts is the maximum number of probe attempts. If all attempts are
	// exhausted without success, WaitForHosts returns an error. Set to 0 to
	// disable the attempt limit (rely on Timeout only).
	MaxAttempts int
}

// DefaultWaitConfig returns sensible defaults for the wait configuration.
func DefaultWaitConfig() WaitConfig {
	return WaitConfig{
		InitialDelay: 10 * time.Second,
		Interval:     15 * time.Second,
		Timeout:      10 * time.Minute,
		MaxAttempts:  20,
	}
}

// WaitForHosts blocks until all hosts in the inventory are reachable via
// ansible.builtin.ping, or until cfg.Timeout is exceeded.
//
// The probe runs inside the diffusion molecule container so that the same
// Python/SSH environment used by the actual deploy is also used here — this
// prevents false positives from mismatched SSH tooling on the host machine.
func WaitForHosts(ctx context.Context, inventoryContent []byte, containerCfg DeployContainerConfig, cfg WaitConfig) error {
	image := utils.GetImageURL(containerCfg.ContainerRegistry)

	log.Printf(config.ColorGreen+"Waiting for hosts to become reachable (timeout: %s, initial delay: %s, max attempts: %d)"+config.ColorReset,
		cfg.Timeout, cfg.InitialDelay, cfg.MaxAttempts)

	// Debug: show probe configuration details.
	if config.DebugEnabled() {
		log.Printf(config.ColorMagenta+"[debug] Probe image: %s"+config.ColorReset, image)
		log.Printf(config.ColorMagenta+"[debug] Inventory content (%d bytes):\n%s"+config.ColorReset,
			len(inventoryContent), string(inventoryContent))

		if len(containerCfg.SSHKeys) > 0 {
			for host, keyB64 := range containerCfg.SSHKeys {
				keyLen := len(keyB64)
				preview := keyB64
				if keyLen > 20 {
					preview = keyB64[:20] + "..."
				}
				log.Printf(config.ColorMagenta+"[debug] SSH key: host=%q, base64_length=%d, preview=%s"+config.ColorReset,
					host, keyLen, preview)
			}
		} else {
			log.Printf(config.ColorMagenta + "[debug] No SSH keys provided via env vars" + config.ColorReset)
		}

		if sshDir := sshKeyDir(); sshDir != "" {
			log.Printf(config.ColorMagenta+"[debug] Host ~/.ssh directory found: %s (will be mounted)"+config.ColorReset, sshDir)
		} else {
			log.Printf(config.ColorMagenta + "[debug] No host ~/.ssh directory found" + config.ColorReset)
		}

		extraDirs := extractSSHKeyDirsFromContent(inventoryContent)
		if len(extraDirs) > 0 {
			log.Printf(config.ColorMagenta+"[debug] Extra SSH key directories from inventory: %v"+config.ColorReset, extraDirs)
		}
	}

	// Initial delay — let cloud-init / boot settle.
	if cfg.InitialDelay > 0 {
		log.Printf(config.ColorYellow+"Initial delay: sleeping %s before first probe..."+config.ColorReset, cfg.InitialDelay)
		select {
		case <-time.After(cfg.InitialDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	deadline := time.Now().Add(cfg.Timeout)
	attempt := 0

	for {
		attempt++
		log.Printf(config.ColorYellow+"Host reachability probe #%d..."+config.ColorReset, attempt)

		err := runPingProbe(ctx, image, inventoryContent, containerCfg)
		if err == nil {
			log.Printf(config.ColorGreen+"All hosts reachable after %d probe(s)"+config.ColorReset, attempt)
			return nil
		}

		log.Printf(config.ColorYellow+"Probe #%d failed: %v"+config.ColorReset, attempt, err)

		// Check max attempts limit.
		if cfg.MaxAttempts > 0 && attempt >= cfg.MaxAttempts {
			return fmt.Errorf("hosts not reachable after %d attempt(s): %w", attempt, err)
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("hosts not reachable after %s (%d probe(s)): %w", cfg.Timeout, attempt, err)
		}

		select {
		case <-time.After(cfg.Interval):
			// continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// runPingProbe executes `ansible all -i <inventory> -m ansible.builtin.ping`
// inside a short-lived container with the same image and SSH env vars as the
// actual deploy container. The inventory is injected as a base64-encoded env
// var and decoded inside the container — no host-side file mount is needed.
func runPingProbe(ctx context.Context, image string, inventoryContent []byte, cfg DeployContainerConfig) error {
	if err := validateSSHKeyNames(cfg.SSHKeys); err != nil {
		return fmt.Errorf("invalid deploy config: %w", err)
	}

	// If per-host SSH keys are provided, inject ansible_ssh_private_key_file
	// into the inventory YAML in Go (proper YAML, no sed hacks).
	probeInventory := inventoryContent
	if len(cfg.SSHKeys) > 0 {
		modified, err := injectSSHKeyPaths(inventoryContent, cfg.SSHKeys)
		if err != nil {
			log.Printf(config.ColorYellow+"[debug] Failed to inject SSH key paths into inventory: %v — using original"+config.ColorReset, err)
		} else {
			probeInventory = modified
		}
	}

	inventoryB64 := base64.StdEncoding.EncodeToString(probeInventory)

	args := []string{
		"run", "--rm",
		"-e", "DIFFUSION_INVENTORY_B64=" + inventoryB64,
	}

	// Pass through SSH-related env vars from the deploy config.
	args = appendDeployEnvArgs(args, cfg)

	// Mount the user's ~/.ssh directory.
	if sshDir := sshKeyDir(); sshDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/root/.ssh:ro", sshDir))
	}

	// Pass base64 SSH keys as env vars (decoded inside container, no mount needed).
	for host, keyB64 := range cfg.SSHKeys {
		envName := "SSH_KEY_" + strings.ToUpper(sshKeyEnvSanitize(host))
		args = append(args, "-e", envName+"="+keyB64)
	}

	// Mount any additional SSH key directories referenced in the inventory.
	// This handles keys outside ~/.ssh (e.g. project-local generated keys from Terraform).
	extraDirs := extractSSHKeyDirsFromContent(inventoryContent)
	for i, dir := range extraDirs {
		containerPath := fmt.Sprintf("/probe/ssh-keys-%d", i)
		args = append(args, "-v", fmt.Sprintf("%s:%s:ro", dir, containerPath))
	}

	args = append(args, image)

	// Build the shell command that decodes inventory and runs ansible ping.
	decodeInv := "mkdir -p /probe && printenv DIFFUSION_INVENTORY_B64 | base64 -d > /probe/inventory.yml"

	// Build the ansible ping command — always disable host key checking for probes
	// since target hosts are typically ephemeral cloud instances not in known_hosts.
	sshArgs := "-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null"

	// Build the ansible ping command.
	if len(cfg.SSHKeys) > 0 {
		// Decode all keys inside the container.
		var decodeSteps []string
		decodeSteps = append(decodeSteps, "mkdir -p /tmp/ssh-keys")
		for host := range cfg.SSHKeys {
			envName := "SSH_KEY_" + strings.ToUpper(sshKeyEnvSanitize(host))
			// Use a safe filename — sanitize special chars for the filesystem.
			keyFile := sshKeyFileName(host)
			keyPath := "/tmp/ssh-keys/" + keyFile
			// Use printenv to output the raw env var value without any shell
			// interpretation — avoids corrupting base64 content that causes
			// "error in libcrypto" when SSH tries to load the decoded key.
			decodeSteps = append(decodeSteps,
				fmt.Sprintf("printenv %s | base64 -d > %s && chmod 600 %s", envName, keyPath, keyPath))
		}

		ansibleCmd := fmt.Sprintf("ANSIBLE_SSH_ARGS='%s' ansible all -i /probe/inventory.yml -m ansible.builtin.ping --timeout 5", sshArgs)

		// Wildcard: single key for all hosts — use --private-key flag.
		if _, hasWildcard := cfg.SSHKeys["*"]; hasWildcard && len(cfg.SSHKeys) == 1 {
			ansibleCmd += " --private-key /tmp/ssh-keys/_wildcard_"
		} else {
			// Per-host/group keys are already injected into inventory via injectSSHKeyPaths.
			// If there's also a wildcard, apply it as --private-key fallback.
			if _, hasWildcard := cfg.SSHKeys["*"]; hasWildcard {
				ansibleCmd += " --private-key /tmp/ssh-keys/_wildcard_"
			} else if len(cfg.SSHKeys) == 1 {
				// Single key — use it as --private-key for safety.
				// injectSSHKeyPaths already injects it into inventory, but this
				// provides an extra safety net.
				for keyName := range cfg.SSHKeys {
					ansibleCmd += " --private-key /tmp/ssh-keys/" + sshKeyFileName(keyName)
				}
			}
		}

		shellCmd := decodeInv + " && " + strings.Join(decodeSteps, " && ") + " && " + ansibleCmd
		args = append(args, "sh", "-c", shellCmd)
	} else if len(extraDirs) > 0 {
		// Use a shell wrapper to sed-replace host paths with container paths in the inventory.
		var builder strings.Builder
		for i, dir := range extraDirs {
			containerPath := fmt.Sprintf("/probe/ssh-keys-%d", i)
			hostEscaped := strings.ReplaceAll(dir, "/", "\\/")
			containerEscaped := strings.ReplaceAll(containerPath, "/", "\\/")
			fmt.Fprintf(&builder, "s/%s/%s/g;", hostEscaped, containerEscaped)
			hostWinEscaped := strings.ReplaceAll(strings.ReplaceAll(dir, "\\", "/"), "/", "\\/")
			if hostWinEscaped != hostEscaped {
				fmt.Fprintf(&builder, "s/%s/%s/g;", hostWinEscaped, containerEscaped)
			}
		}
		shellCmd := fmt.Sprintf(
			"%s && sed '%s' /probe/inventory.yml > /tmp/inventory.yml && ANSIBLE_SSH_ARGS='%s' ansible all -i /tmp/inventory.yml -m ansible.builtin.ping --timeout 5",
			decodeInv, builder.String(), sshArgs,
		)
		args = append(args, "sh", "-c", shellCmd)
	} else {
		// No extra mounts needed — decode inventory and run ansible ping directly.
		shellCmd := fmt.Sprintf(
			"%s && ANSIBLE_SSH_ARGS='%s' ansible all -i /probe/inventory.yml -m ansible.builtin.ping --timeout 5",
			decodeInv, sshArgs,
		)
		args = append(args, "sh", "-c", shellCmd)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)

	// Capture both stdout and stderr — ansible reports UNREACHABLE hosts in
	// stdout but may still exit 0 in some configurations.
	var stdoutBuf strings.Builder
	var stderrBuf strings.Builder
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if config.DebugEnabled() {
		log.Printf(config.ColorMagenta+"[debug] Probe docker command: docker %s"+config.ColorReset,
			redactProbeArgs(args))
	}

	runErr := cmd.Run()

	combinedOutput := stdoutBuf.String() + stderrBuf.String()

	if config.DebugEnabled() && combinedOutput != "" {
		log.Printf(config.ColorMagenta+"[debug] Probe container output:\n%s"+config.ColorReset, combinedOutput)
	}

	// Check for failure regardless of exit code — ansible ad-hoc can exit 0
	// while reporting hosts as UNREACHABLE in its output.
	if runErr != nil {
		return fmt.Errorf("ansible ping failed (exit error): %w", runErr)
	}

	// Even with exit 0, check output for unreachable/failed hosts.
	if strings.Contains(combinedOutput, "UNREACHABLE") || strings.Contains(combinedOutput, "FAILED") {
		return fmt.Errorf("ansible ping failed: hosts reported UNREACHABLE or FAILED")
	}

	// Verify at least one host reported SUCCESS — if there's no SUCCESS in the
	// output, the probe did not actually validate any host connectivity.
	if !strings.Contains(combinedOutput, "SUCCESS") {
		return fmt.Errorf("ansible ping failed: no hosts reported SUCCESS")
	}

	return nil
}

// injectSSHKeyPaths modifies the inventory YAML to add ansible_ssh_private_key_file
// for each SSH key entry. The key paths point to /tmp/ssh-keys/<name> which is
// where the container decodes the base64 keys at runtime.
// This produces valid YAML instead of relying on fragile sed manipulation.
//
// Key name matching (evaluated in priority order):
//   - "*" is a wildcard handled via --private-key flag (not injected here).
//   - "group:<name>" assigns the key to all hosts that belong to group <name>.
//   - If a key name matches a host in the inventory, it's assigned to that host only.
//   - If a key name does NOT match any host or group prefix (e.g. "default"),
//     it's treated as a fallback key and injected into ALL hosts that don't
//     already have a key assigned.
func injectSSHKeyPaths(inventoryContent []byte, sshKeys map[string]string) ([]byte, error) {
	if err := validateSSHKeyNames(sshKeys); err != nil {
		return nil, err
	}

	var inv map[string]any
	if err := yaml.Unmarshal(inventoryContent, &inv); err != nil {
		return nil, fmt.Errorf("failed to parse inventory: %w", err)
	}

	// Navigate to all.hosts
	allRaw, ok := inv["all"]
	if !ok {
		return nil, fmt.Errorf("inventory missing 'all' group")
	}
	allMap, ok := allRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("'all' is not a map")
	}
	hostsRaw, ok := allMap["hosts"]
	if !ok {
		return nil, fmt.Errorf("'all.hosts' not found")
	}
	hostsMap, ok := hostsRaw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("'all.hosts' is not a map")
	}

	// Build a group → hosts lookup from children.
	groupHosts := extractGroupHostsFromInventory(allMap)

	// Classify SSH key entries.
	var fallbackKeyName string
	for keyName := range sshKeys {
		if keyName == "*" {
			continue
		}
		if strings.HasPrefix(keyName, "group:") {
			// Group key — handled in second pass.
			continue
		}
		if _, isHost := hostsMap[keyName]; isHost {
			// Per-host key — inject directly.
			setHostKeyFile(hostsMap, keyName, "/tmp/ssh-keys/"+sshKeyFileName(keyName))
		} else {
			// Doesn't match any host — candidate for fallback.
			if fallbackKeyName == "" {
				fallbackKeyName = keyName
			}
		}
	}

	// Second pass: inject group-based keys.
	for keyName := range sshKeys {
		if !strings.HasPrefix(keyName, "group:") {
			continue
		}
		groupName := strings.TrimPrefix(keyName, "group:")
		members, exists := groupHosts[groupName]
		if !exists || len(members) == 0 {
			continue
		}
		keyPath := "/tmp/ssh-keys/" + sshKeyFileName(keyName)
		for _, hostName := range members {
			// Don't overwrite a per-host key that was already explicitly assigned.
			if hostHasKeyFile(hostsMap, hostName) {
				continue
			}
			setHostKeyFile(hostsMap, hostName, keyPath)
		}
	}

	// Third pass: apply fallback key to all hosts that don't already have a key.
	if fallbackKeyName != "" {
		fallbackKeyPath := "/tmp/ssh-keys/" + sshKeyFileName(fallbackKeyName)
		for hostName := range hostsMap {
			if hostHasKeyFile(hostsMap, hostName) {
				continue
			}
			setHostKeyFile(hostsMap, hostName, fallbackKeyPath)
		}
	}

	data, err := yaml.Marshal(inv)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal modified inventory: %w", err)
	}
	return data, nil
}

// setHostKeyFile sets ansible_ssh_private_key_file on a host in the inventory map.
func setHostKeyFile(hostsMap map[string]any, hostName, keyPath string) {
	hostVarsRaw := hostsMap[hostName]
	var hostVars map[string]any
	switch v := hostVarsRaw.(type) {
	case map[string]any:
		hostVars = v
	case nil:
		hostVars = make(map[string]any)
	default:
		return
	}
	hostVars["ansible_ssh_private_key_file"] = keyPath
	hostsMap[hostName] = hostVars
}

// hostHasKeyFile returns true if the host already has ansible_ssh_private_key_file set.
func hostHasKeyFile(hostsMap map[string]any, hostName string) bool {
	hostVarsRaw := hostsMap[hostName]
	hostVars, ok := hostVarsRaw.(map[string]any)
	if !ok {
		return false
	}
	_, has := hostVars["ansible_ssh_private_key_file"]
	return has
}

// extractGroupHostsFromInventory builds a map of group name → list of host names
// from the inventory's all.children structure.
func extractGroupHostsFromInventory(allMap map[string]any) map[string][]string {
	result := make(map[string][]string)

	childrenRaw, ok := allMap["children"]
	if !ok {
		return result
	}
	childrenMap, ok := childrenRaw.(map[string]any)
	if !ok {
		return result
	}

	for groupName, groupRaw := range childrenMap {
		groupMap, ok := groupRaw.(map[string]any)
		if !ok {
			continue
		}
		groupHostsRaw, ok := groupMap["hosts"]
		if !ok {
			continue
		}
		groupHostsMap, ok := groupHostsRaw.(map[string]any)
		if !ok {
			continue
		}
		hosts := make([]string, 0, len(groupHostsMap))
		for h := range groupHostsMap {
			hosts = append(hosts, h)
		}
		result[groupName] = hosts
	}

	return result
}

// sshKeyEnvSanitize converts an SSH key name into a string safe for use as an
// environment variable suffix. Replaces dashes, dots, colons, slashes, and the
// wildcard character with underscores/a fixed token — critical for "*" since
// an unmapped "*" would produce an env var name like "SSH_KEY_*" that gets
// glob-expanded when embedded unquoted in a shell command.
func sshKeyEnvSanitize(name string) string {
	if name == "*" {
		return "WILDCARD"
	}
	return strings.NewReplacer(
		"-", "_",
		".", "_",
		":", "_",
		"/", "_",
	).Replace(name)
}

// sshKeyFileName returns a filesystem-safe filename for the given SSH key name.
// Handles special cases like wildcard ("*") and group prefix ("group:").
func sshKeyFileName(name string) string {
	if name == "*" {
		return "_wildcard_"
	}
	// Replace characters that are problematic in filenames.
	return strings.NewReplacer(
		":", "_",
		"*", "_wildcard_",
	).Replace(name)
}

// redactProbeArgs returns a redacted version of the docker args for logging,
// truncating large env var values (inventory, SSH keys) to avoid log noise.
func redactProbeArgs(args []string) string {
	redacted := make([]string, len(args))
	for i, a := range args {
		// Redact DIFFUSION_INVENTORY_B64 value (can be large).
		if strings.HasPrefix(a, "DIFFUSION_INVENTORY_B64=") {
			redacted[i] = "DIFFUSION_INVENTORY_B64=<" + fmt.Sprintf("%d", len(a)-len("DIFFUSION_INVENTORY_B64=")) + " bytes>"
		} else if strings.HasPrefix(a, "SSH_KEY_") && strings.Contains(a, "=") {
			// Redact SSH key values.
			parts := strings.SplitN(a, "=", 2)
			redacted[i] = parts[0] + "=<redacted>"
		} else {
			redacted[i] = a
		}
	}
	return strings.Join(redacted, " ")
}

// appendDeployEnvArgs adds -e flags for Vault and TOKEN env vars, mirroring
// what runContainer does for the molecule container.
func appendDeployEnvArgs(args []string, cfg DeployContainerConfig) []string {
	args = append(args,
		"-e", "VAULT_TOKEN="+os.Getenv("VAULT_TOKEN"),
		"-e", "VAULT_ADDR="+os.Getenv("VAULT_ADDR"),
		"-e", "SSL_CERT_FILE=/etc/ssl/certs/ca-certificates.crt",
	)

	// Forward artifact credential env vars for private Galaxy repos.
	for _, cred := range cfg.ArtifactSources {
		key := sanitizeEnvKey(cred.Name)
		if cred.Username != "" {
			args = append(args, "-e", fmt.Sprintf("%s%s=%s", config.EnvGitUserPrefix, key, cred.Username))
		}
		if cred.Token != "" {
			args = append(args, "-e", fmt.Sprintf("TOKEN_%s=%s", key, cred.Token))
		}
	}

	return args
}

// sshKeyDir returns the user's ~/.ssh directory path if it exists, or "".
func sshKeyDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	sshDir := home + "/.ssh"
	if _, err := os.Stat(sshDir); err != nil {
		return ""
	}
	return sshDir
}

// ParseWaitDuration parses a duration string with helpful error messages.
// Accepts Go duration strings ("10s", "5m", "1h30m").
func ParseWaitDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "0" {
		return 0, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: use Go duration format (e.g. \"10s\", \"5m\", \"1h\"): %w", s, err)
	}
	return d, nil
}

// extractSSHKeyDirsFromContent parses the inventory YAML content and returns unique
// directory paths for any ansible_ssh_private_key_file values that are NOT under ~/.ssh.
// These directories need to be mounted into the container.
func extractSSHKeyDirsFromContent(inventoryContent []byte) []string {
	if len(inventoryContent) == 0 {
		return nil
	}

	// Parse inventory to extract host vars.
	var inv map[string]any
	if err := yaml.Unmarshal(inventoryContent, &inv); err != nil {
		return nil
	}

	sshHome := ""
	if home, err := os.UserHomeDir(); err == nil {
		sshHome = filepath.Join(home, ".ssh")
	}

	seen := make(map[string]bool)
	var dirs []string

	// Walk the inventory tree looking for ansible_ssh_private_key_file values.
	walkInventoryForKeyFiles(inv, sshHome, seen, &dirs)

	return dirs
}

// walkInventoryForKeyFiles recursively walks the inventory structure to find
// ansible_ssh_private_key_file values.
func walkInventoryForKeyFiles(obj any, sshHome string, seen map[string]bool, dirs *[]string) {
	switch v := obj.(type) {
	case map[string]any:
		for k, val := range v {
			if k == "ansible_ssh_private_key_file" {
				if s, ok := val.(string); ok && s != "" {
					dir := filepath.Dir(s)
					// Normalize path separators.
					dir = filepath.ToSlash(dir)
					sshHomeNorm := filepath.ToSlash(sshHome)
					// Skip if it's already under ~/.ssh (mounted separately).
					if sshHome != "" && (dir == sshHomeNorm || strings.HasPrefix(dir, sshHomeNorm+"/")) {
						continue
					}
					if !seen[dir] {
						seen[dir] = true
						*dirs = append(*dirs, dir)
					}
				}
			} else {
				walkInventoryForKeyFiles(val, sshHome, seen, dirs)
			}
		}
	case []any:
		for _, item := range v {
			walkInventoryForKeyFiles(item, sshHome, seen, dirs)
		}
	}
}
