package tfprovider

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"diffusion/internal/deploy"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// RoleSourceEntry is the provider-internal representation of a role source.
type RoleSourceEntry struct {
	SCM     string
	Version string
	URL     string
	Galaxy  string
	// Name overrides the role name in the auto-generated playbook.
	Name string
	// ApplyTo is the Ansible hosts pattern for the auto-generated play (default: "all").
	ApplyTo string
}

// DiffusionRunConfig holds the complete configuration for one diffusion deploy
// invocation, assembled from provider config + resource config.
type DiffusionRunConfig struct {
	// Provider-level
	DiffusionBinary  string
	RegistryServer   string
	RegistryProvider string
	ContainerName    string
	ContainerTag     string
	VaultAddr        string
	VaultToken       string
	ArtifactSources  []ArtifactSourceModel

	// Host wait (provider defaults, overridable per resource)
	HostWaitInitialDelay string
	HostWaitInterval     string
	HostWaitTimeout      string

	// Resource-level
	RoleSources           []RoleSourceEntry
	Playbook              string // empty = auto-generate
	Hosts                 []deploy.InventoryHost
	Groups                []deploy.InventoryGroup
	GlobalVars            map[string]string
	ExtraVars             map[string]string
	SkipIfSucceededWithin string

	// Pre-rendered inventory (for the computed attribute)
	InventoryRendered string
}

// DeployResult holds the computed values returned after a successful deploy.
type DeployResult struct {
	RunID             string
	LastDeployed      string
	MergedLockHash    string
	InventoryRendered string
}

// RunDiffusionDeploy builds the `diffusion deploy` CLI argument list and
// executes it, streaming output via tflog.
func RunDiffusionDeploy(ctx context.Context, cfg DiffusionRunConfig) (DeployResult, error) {
	args := buildArgs(cfg)

	binary := cfg.DiffusionBinary
	if binary == "" {
		binary = "diffusion"
	}

	tflog.Info(ctx, "Executing diffusion deploy", map[string]interface{}{
		"binary": binary,
		"args":   redactArgs(args),
	})

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = &tflogWriter{ctx: ctx, level: "info"}
	cmd.Stderr = &tflogWriter{ctx: ctx, level: "warn"}

	// Build env: start from current process env and inject Vault vars.
	env := os.Environ()
	if cfg.VaultToken != "" {
		env = append(env, "VAULT_TOKEN="+cfg.VaultToken)
	}
	if cfg.VaultAddr != "" {
		env = append(env, "VAULT_ADDR="+cfg.VaultAddr)
	}
	cmd.Env = env

	if err := cmd.Run(); err != nil {
		return DeployResult{}, fmt.Errorf("diffusion deploy failed: %w", err)
	}

	return DeployResult{
		LastDeployed:      time.Now().UTC().Format(time.RFC3339),
		InventoryRendered: cfg.InventoryRendered,
		MergedLockHash:    "", // exposed by the binary in a future --output json flag
		RunID:             computeResultRunID(cfg),
	}, nil
}

// buildArgs constructs the full `diffusion deploy` argument list.
func buildArgs(cfg DiffusionRunConfig) []string {
	args := []string{"deploy"}

	// Role sources — each emits one --role-source flag
	for _, rs := range cfg.RoleSources {
		spec := fmt.Sprintf("scm=%s,version=%s", rs.SCM, rs.Version)
		if rs.URL != "" {
			spec += ",url=" + rs.URL
		}
		if rs.Galaxy != "" {
			spec += ",galaxy=" + rs.Galaxy
		}
		if rs.Name != "" {
			spec += ",name=" + rs.Name
		}
		if rs.ApplyTo != "" {
			spec += ",apply_to=" + rs.ApplyTo
		}
		args = append(args, "--role-source", spec)
	}

	// Playbook (optional — omit entirely when empty so CLI auto-generates)
	if cfg.Playbook != "" {
		args = append(args, "--playbook", cfg.Playbook)
	}

	// Hosts
	for _, h := range cfg.Hosts {
		var parts []string
		for k, v := range h.Variables {
			parts = append(parts, k+"="+v)
		}
		args = append(args, "--host", h.Name+"="+strings.Join(parts, ","))
	}

	// Groups
	for _, g := range cfg.Groups {
		args = append(args, "--group", g.Name+"="+strings.Join(g.Hosts, ","))
	}

	// Global vars
	for k, v := range cfg.GlobalVars {
		args = append(args, "--var", k+"="+v)
	}

	// Extra vars
	for k, v := range cfg.ExtraVars {
		args = append(args, "--extra-var", k+"="+v)
	}

	// Skip period
	if cfg.SkipIfSucceededWithin != "" {
		args = append(args, "--skip-period", cfg.SkipIfSucceededWithin)
	}

	// Host wait settings
	if cfg.HostWaitInitialDelay != "" {
		args = append(args, "--host-wait-initial-delay", cfg.HostWaitInitialDelay)
	}
	if cfg.HostWaitInterval != "" {
		args = append(args, "--host-wait-interval", cfg.HostWaitInterval)
	}
	if cfg.HostWaitTimeout != "" {
		args = append(args, "--host-wait-timeout", cfg.HostWaitTimeout)
	}

	return args
}

// computeResultRunID derives a stable run ID from the config.
func computeResultRunID(cfg DiffusionRunConfig) string {
	h := sha256.New()
	fmt.Fprintf(h, "playbook:%s\n", cfg.Playbook)
	fmt.Fprintf(h, "skip:%s\n", cfg.SkipIfSucceededWithin)
	for _, rs := range cfg.RoleSources {
		fmt.Fprintf(h, "role:%s:%s:%s:%s:%s:%s\n",
			rs.SCM, rs.Version, rs.URL, rs.Galaxy, rs.Name, rs.ApplyTo)
	}
	for k, v := range cfg.GlobalVars {
		fmt.Fprintf(h, "var:%s=%s\n", k, v)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// redactArgs masks sensitive flag values in log output.
func redactArgs(args []string) []string {
	redacted := make([]string, len(args))
	copy(redacted, args)
	for i, a := range redacted {
		lower := strings.ToLower(a)
		if strings.Contains(lower, "password") || strings.Contains(lower, "token") {
			if parts := strings.SplitN(a, "=", 2); len(parts) == 2 {
				redacted[i] = parts[0] + "=***"
			}
		}
	}
	return redacted
}

// tflogWriter implements io.Writer routing output line-by-line to tflog.
type tflogWriter struct {
	ctx   context.Context
	level string
	buf   strings.Builder
}

func (w *tflogWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	s := w.buf.String()
	for {
		idx := strings.IndexByte(s, '\n')
		if idx < 0 {
			break
		}
		line := s[:idx]
		s = s[idx+1:]
		if w.level == "warn" {
			tflog.Warn(w.ctx, line)
		} else {
			tflog.Info(w.ctx, line)
		}
	}
	w.buf.Reset()
	w.buf.WriteString(s)
	return len(p), nil
}
