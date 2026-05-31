package deploy

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"diffusion/internal/config"
	"diffusion/internal/utils"
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

	// Mount SSH keys directory if the inventory references key files.
	if sshDir := sshKeyDir(); sshDir != "" {
		args = append(args, "-v", fmt.Sprintf("%s:/root/.ssh:ro", sshDir))
	}

	args = append(args, image)

	// The container entrypoint: ansible ping with a short per-host timeout.
	args = append(args,
		"ansible", "all",
		"-i", "/probe/inventory.yml",
		"-m", "ansible.builtin.ping",
		"--timeout", "5",
	)

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
