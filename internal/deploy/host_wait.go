package deploy

import (
	"context"
	"fmt"
	"io"
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
}

// DefaultWaitConfig returns sensible defaults for the wait configuration.
func DefaultWaitConfig() WaitConfig {
	return WaitConfig{
		InitialDelay: 10 * time.Second,
		Interval:     15 * time.Second,
		Timeout:      10 * time.Minute,
	}
}

// WaitForHosts blocks until all hosts in the inventory are reachable via
// ansible.builtin.ping, or until cfg.Timeout is exceeded.
//
// The probe runs inside the diffusion molecule container so that the same
// Python/SSH environment used by the actual deploy is also used here — this
// prevents false positives from mismatched SSH tooling on the host machine.
func WaitForHosts(ctx context.Context, inventoryPath string, containerCfg DeployContainerConfig, cfg WaitConfig) error {
	image := utils.GetImageURL(containerCfg.ContainerRegistry)

	log.Printf(config.ColorGreen+"Waiting for hosts to become reachable (timeout: %s, initial delay: %s)"+config.ColorReset,
		cfg.Timeout, cfg.InitialDelay)

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

		err := runPingProbe(ctx, image, inventoryPath, containerCfg)
		if err == nil {
			log.Printf(config.ColorGreen+"All hosts reachable after %d probe(s)"+config.ColorReset, attempt)
			return nil
		}

		log.Printf(config.ColorYellow+"Probe #%d failed: %v"+config.ColorReset, attempt, err)

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
// actual deploy container.
func runPingProbe(ctx context.Context, image, inventoryPath string, cfg DeployContainerConfig) error {
	args := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/probe/inventory.yml:ro", inventoryPath),
	}

	// Pass through SSH-related env vars from the deploy config.
	args = appendDeployEnvArgs(args, cfg)

	// Mount the user's ~/.ssh directory.
	if sshDir := sshKeyDir(); sshDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/root/.ssh:ro", sshDir))
	}

	// Mount any additional SSH key directories referenced in the inventory.
	// This handles keys outside ~/.ssh (e.g. project-local generated keys from Terraform).
	extraDirs := extractSSHKeyDirs(inventoryPath)
	for i, dir := range extraDirs {
		containerPath := fmt.Sprintf("/probe/ssh-keys-%d", i)
		args = append(args, "-v", fmt.Sprintf("%s:%s:ro", dir, containerPath))
	}

	args = append(args, image)

	// Build the container command: rewrite inventory key paths, then run ansible ping.
	if len(extraDirs) > 0 {
		// Use a shell wrapper to sed-replace host paths with container paths in the inventory.
		sedExpr := ""
		for i, dir := range extraDirs {
			containerPath := fmt.Sprintf("/probe/ssh-keys-%d", i)
			// Escape slashes for sed (handle both forward and backslash paths).
			hostEscaped := strings.ReplaceAll(dir, "/", "\\/")
			containerEscaped := strings.ReplaceAll(containerPath, "/", "\\/")
			sedExpr += fmt.Sprintf("s/%s/%s/g;", hostEscaped, containerEscaped)
			// Also handle Windows backslash paths that might end up in YAML.
			hostWinEscaped := strings.ReplaceAll(strings.ReplaceAll(dir, "\\", "/"), "/", "\\/")
			if hostWinEscaped != hostEscaped {
				sedExpr += fmt.Sprintf("s/%s/%s/g;", hostWinEscaped, containerEscaped)
			}
		}
		shellCmd := fmt.Sprintf(
			"cp /probe/inventory.yml /tmp/inventory.yml && sed -i '%s' /tmp/inventory.yml && ansible all -i /tmp/inventory.yml -m ansible.builtin.ping --timeout 5",
			sedExpr,
		)
		args = append(args, "sh", "-c", shellCmd)
	} else {
		// No extra mounts needed — run ansible ping directly.
		args = append(args,
			"ansible", "all",
			"-i", "/probe/inventory.yml",
			"-m", "ansible.builtin.ping",
			"--timeout", "5",
		)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ansible ping failed: %w", err)
	}
	return nil
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

// extractSSHKeyDirs parses the inventory YAML and returns unique directory paths
// for any ansible_ssh_private_key_file values that are NOT under ~/.ssh.
// These directories need to be mounted into the container.
func extractSSHKeyDirs(inventoryPath string) []string {
	data, err := os.ReadFile(inventoryPath)
	if err != nil {
		return nil
	}

	// Parse inventory to extract host vars.
	var inv map[string]interface{}
	if err := yaml.Unmarshal(data, &inv); err != nil {
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
func walkInventoryForKeyFiles(obj interface{}, sshHome string, seen map[string]bool, dirs *[]string) {
	switch v := obj.(type) {
	case map[string]interface{}:
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
	case []interface{}:
		for _, item := range v {
			walkInventoryForKeyFiles(item, sshHome, seen, dirs)
		}
	}
}
