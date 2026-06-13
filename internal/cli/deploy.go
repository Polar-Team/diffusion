package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"diffusion/internal/config"
	"diffusion/internal/deploy"

	"github.com/spf13/cobra"
)

// deployFlags holds all flags for the `diffusion deploy` command.
type deployFlags struct {
	// Role source flags — repeatable; each value is a comma-separated key=value set.
	// Required fields: scm, version.
	// Conditional:     url (when scm=git), galaxy (when scm=galaxy).
	// Optional:        name (role name override), apply_to (hosts pattern, default "all").
	roleSources []string

	// Playbook — optional. When omitted a playbook is auto-generated from role sources.
	playbook string

	// Inventory
	hosts  []string // "hostname=key=value,key=value"
	groups []string // "groupname=host1,host2"
	vars   []string // "key=value"

	// Extra vars passed to ansible-playbook
	extraVars []string // "key=value"

	// Skip / idempotence
	skipPeriod string // Go duration string, e.g. "24h"

	// Host reachability wait
	hostWaitInitialDelay string
	hostWaitInterval     string
	hostWaitTimeout      string
}

// NewDeployCmd creates the `diffusion deploy` Cobra command.
func NewDeployCmd(_ *CLI) *cobra.Command {
	f := &deployFlags{}

	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Deploy Ansible roles to remote hosts using the diffusion molecule container",
		Long: `diffusion deploy fetches diffusion.lock from each remote role repo,
merges all dependency constraints, and runs ansible-playbook inside the
diffusion molecule container.

Roles and collections are installed INSIDE the container from a generated
requirements.yml — nothing is downloaded to the host machine.

PLAYBOOK
  If --playbook is omitted, a playbook is auto-generated that applies each
  role to its configured hosts pattern (default: "all"). Use the apply_to
  field in --role-source to control which group each role targets.

ROLE SOURCE FORMAT
  Each --role-source flag accepts a comma-separated key=value string:

    scm=git,version=>=2.0.0,url=https://github.com/org/role.git
    scm=git,version=main,url=https://github.com/org/role.git,apply_to=webservers
    scm=galaxy,version=>=7.0.0,galaxy=geerlingguy.docker
    scm=galaxy,version=>=7.0.0,galaxy=geerlingguy.docker,apply_to=all,name=docker

  Fields:
    scm        required  "git" or "galaxy"
    version    required  version constraint or ref (e.g. ">=1.0.0", "main")
    url        required when scm=git
    galaxy     required when scm=galaxy  (format: namespace.role_name)
    name       optional  role name override used in the generated playbook
    apply_to   optional  Ansible hosts pattern (default: "all")

EXAMPLES
  # Auto-generated playbook, single role
  diffusion deploy \
    --role-source "scm=galaxy,version=>=7.0.0,galaxy=geerlingguy.docker" \
    --host "web01=ansible_host=1.2.3.4,ansible_user=ubuntu"

  # Two roles targeting different groups, with skip
  diffusion deploy \
    --role-source "scm=git,version=main,url=https://github.com/org/common.git,apply_to=all" \
    --role-source "scm=git,version=main,url=https://github.com/org/app.git,apply_to=webservers" \
    --host "web01=ansible_host=1.2.3.4,ansible_user=ubuntu" \
    --group "webservers=web01" \
    --skip-period 24h

  # User-supplied playbook
  diffusion deploy \
    --role-source "scm=galaxy,version=>=9.0.0,galaxy=community.general" \
    --playbook site.yml \
    --host "web01=ansible_host=1.2.3.4,ansible_user=ubuntu"`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(cmd.Context(), f)
		},
	}

	cmd.Flags().StringArrayVar(&f.roleSources, "role-source", nil,
		`Remote role source (repeatable). Required fields: scm, version. See help for full format.`)
	cmd.Flags().StringVarP(&f.playbook, "playbook", "p", "",
		"Path to an Ansible playbook. When omitted, a playbook is auto-generated from role sources.")
	cmd.Flags().StringArrayVar(&f.hosts, "host", nil,
		`Inventory host (repeatable). Format: "hostname=key=value,key=value"`)
	cmd.Flags().StringArrayVar(&f.groups, "group", nil,
		`Inventory group (repeatable). Format: "groupname=host1,host2"`)
	cmd.Flags().StringArrayVar(&f.vars, "var", nil,
		`Global inventory variable (repeatable). Format: "key=value"`)
	cmd.Flags().StringArrayVar(&f.extraVars, "extra-var", nil,
		`Extra variable for ansible-playbook --extra-vars (repeatable). Format: "key=value"`)
	cmd.Flags().StringVar(&f.skipPeriod, "skip-period", "",
		`Skip re-deploy if last run succeeded within this period (e.g. "24h"). Default: always deploy.`)
	cmd.Flags().StringVar(&f.hostWaitInitialDelay, "host-wait-initial-delay", "10s",
		`Pause before the first host reachability probe (e.g. "30s").`)
	cmd.Flags().StringVar(&f.hostWaitInterval, "host-wait-interval", "15s",
		`Interval between host reachability probes (e.g. "20s").`)
	cmd.Flags().StringVar(&f.hostWaitTimeout, "host-wait-timeout", "10m",
		`Hard deadline for host reachability (e.g. "15m").`)

	_ = cmd.MarkFlagRequired("role-source")

	return cmd
}

// runDeploy parses flags into a deploy.DeployConfig and calls deploy.Deploy.
func runDeploy(ctx context.Context, f *deployFlags) error {
	roleSources, err := parseRoleSources(f.roleSources)
	if err != nil {
		return err
	}

	hosts, err := parseHosts(f.hosts)
	if err != nil {
		return err
	}

	groups, err := parseGroups(f.groups)
	if err != nil {
		return err
	}

	globalVars, err := parseKeyValues(f.vars, "var")
	if err != nil {
		return err
	}

	extraVars, err := parseKeyValues(f.extraVars, "extra-var")
	if err != nil {
		return err
	}

	skipPeriod, err := deploy.ParseWaitDuration(f.skipPeriod)
	if err != nil {
		return fmt.Errorf("--skip-period: %w", err)
	}

	waitCfg, err := parseWaitConfig(f)
	if err != nil {
		return err
	}

	cfg, err := config.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, config.ColorYellow+"warning: could not load diffusion.toml: %v — using defaults\n"+config.ColorReset, err)
		cfg = &config.Config{}
	}

	if cfg.ContainerRegistry == nil {
		cfg.ContainerRegistry = &config.ContainerRegistry{
			RegistryServer:        config.DefaultRegistryServer,
			RegistryProvider:      config.DefaultRegistryProvider,
			MoleculeContainerName: config.DefaultMoleculeContainerName,
			MoleculeContainerTag:  config.DefaultMoleculeTag,
		}
	}

	deployCfg := deploy.DeployConfig{
		RoleSources:        roleSources,
		Playbook:           f.playbook,
		Hosts:              hosts,
		Groups:             groups,
		GlobalVars:         globalVars,
		ExtraVars:          extraVars,
		SkipIfSucceededFor: skipPeriod,
		ContainerRegistry:  cfg.ContainerRegistry,
		ArtifactSourcesCfg: cfg.ArtifactSources,
		VaultConfig:        cfg.HashicorpVault,
		VaultToken:         os.Getenv("VAULT_TOKEN"),
		VaultAddr:          os.Getenv("VAULT_ADDR"),
		DiffusionVersion:   Version,
		Wait:               waitCfg,
	}

	return deploy.Deploy(ctx, deployCfg)
}

// parseRoleSources converts "--role-source" flag strings into deploy.RoleSource.
// Accepted fields: scm, version, url, galaxy, name, apply_to.
func parseRoleSources(raw []string) ([]deploy.RoleSource, error) {
	var sources []deploy.RoleSource
	for i, s := range raw {
		kv, err := parseKVPairs(s)
		if err != nil {
			return nil, fmt.Errorf("--role-source[%d] %q: %w", i, s, err)
		}

		src := deploy.RoleSource{
			SCM:     kv["scm"],
			Version: kv["version"],
			URL:     kv["url"],
			Galaxy:  kv["galaxy"],
			Name:    kv["name"],
			ApplyTo: kv["apply_to"],
		}

		if src.SCM == "" {
			return nil, fmt.Errorf("--role-source[%d]: missing required field 'scm' (must be 'git' or 'galaxy')", i)
		}
		if src.Version == "" {
			return nil, fmt.Errorf("--role-source[%d]: missing required field 'version'", i)
		}
		switch strings.ToLower(src.SCM) {
		case "git":
			if src.URL == "" {
				return nil, fmt.Errorf("--role-source[%d]: 'url' is required when scm=git", i)
			}
		case "galaxy":
			if src.Galaxy == "" {
				return nil, fmt.Errorf("--role-source[%d]: 'galaxy' is required when scm=galaxy (format: namespace.role_name)", i)
			}
		default:
			return nil, fmt.Errorf("--role-source[%d]: unsupported scm %q (must be 'git' or 'galaxy')", i, src.SCM)
		}

		sources = append(sources, src)
	}
	return sources, nil
}

// parseHosts converts "hostname=key=value,key=value" strings into InventoryHost.
func parseHosts(raw []string) ([]deploy.InventoryHost, error) {
	var hosts []deploy.InventoryHost
	for _, s := range raw {
		idx := strings.Index(s, "=")
		if idx < 0 {
			return nil, fmt.Errorf("--host %q: expected format \"hostname=key=value,...\"", s)
		}
		name := s[:idx]
		rest := s[idx+1:]
		vars := make(map[string]string)
		for _, pair := range strings.Split(rest, ",") {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}
			k, v, ok := strings.Cut(pair, "=")
			if !ok {
				return nil, fmt.Errorf("--host %q: invalid variable pair %q (expected key=value)", s, pair)
			}
			vars[k] = v
		}
		hosts = append(hosts, deploy.InventoryHost{Name: name, Variables: vars})
	}
	return hosts, nil
}

// parseGroups converts "groupname=host1,host2" strings into InventoryGroup.
func parseGroups(raw []string) ([]deploy.InventoryGroup, error) {
	var groups []deploy.InventoryGroup
	for _, s := range raw {
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			return nil, fmt.Errorf("--group %q: expected format \"groupname=host1,host2\"", s)
		}
		var hostNames []string
		for _, h := range strings.Split(v, ",") {
			if h = strings.TrimSpace(h); h != "" {
				hostNames = append(hostNames, h)
			}
		}
		groups = append(groups, deploy.InventoryGroup{Name: k, Hosts: hostNames})
	}
	return groups, nil
}

// parseKeyValues converts "key=value" strings into a map.
func parseKeyValues(raw []string, flagName string) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	m := make(map[string]string)
	for _, s := range raw {
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			return nil, fmt.Errorf("--%s %q: expected format \"key=value\"", flagName, s)
		}
		m[k] = v
	}
	return m, nil
}

// parseKVPairs parses a comma-separated "key=value" string into a map.
// Splits only on the first '=' per token so URLs are preserved intact.
func parseKVPairs(s string) (map[string]string, error) {
	result := make(map[string]string)
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			return nil, fmt.Errorf("invalid key=value pair %q", part)
		}
		result[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return result, nil
}

// parseWaitConfig converts duration flag strings into a deploy.WaitConfig.
func parseWaitConfig(f *deployFlags) (deploy.WaitConfig, error) {
	defaults := deploy.DefaultWaitConfig()

	initialDelay, err := parseDurationWithDefault(f.hostWaitInitialDelay, defaults.InitialDelay, "--host-wait-initial-delay")
	if err != nil {
		return deploy.WaitConfig{}, err
	}
	interval, err := parseDurationWithDefault(f.hostWaitInterval, defaults.Interval, "--host-wait-interval")
	if err != nil {
		return deploy.WaitConfig{}, err
	}
	timeout, err := parseDurationWithDefault(f.hostWaitTimeout, defaults.Timeout, "--host-wait-timeout")
	if err != nil {
		return deploy.WaitConfig{}, err
	}

	return deploy.WaitConfig{
		InitialDelay: initialDelay,
		Interval:     interval,
		Timeout:      timeout,
	}, nil
}

func parseDurationWithDefault(s string, def time.Duration, flag string) (time.Duration, error) {
	if s == "" {
		return def, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("%s %q: %w", flag, s, err)
	}
	return d, nil
}
