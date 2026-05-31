package deploy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/secrets"
)

// DeployConfig is the top-level configuration for a deploy run.
type DeployConfig struct {
	// RoleSources are the remote role repos to fetch diffusion.lock from.
	// Each entry may carry Name and ApplyTo for playbook generation.
	RoleSources []RoleSource

	// Playbook is the optional path to a user-supplied ansible-playbook file on
	// the host. When empty, a playbook is auto-generated from RoleSources.
	Playbook string

	// Hosts, Groups, GlobalVars drive inventory generation.
	Hosts      []InventoryHost
	Groups     []InventoryGroup
	GlobalVars map[string]string

	// ExtraVars are passed to ansible-playbook as --extra-vars JSON.
	ExtraVars map[string]string

	// SkipIfSucceededFor: if > 0 and the last run for the same RunID succeeded
	// within this duration, remote hosts skip the deploy tasks.
	SkipIfSucceededFor time.Duration

	// ContainerRegistry configures the molecule container image.
	ContainerRegistry *config.ContainerRegistry

	// ArtifactSourcesCfg holds diffusion artifact source configurations used to
	// load private registry / Galaxy credentials.
	ArtifactSourcesCfg []config.ArtifactSource

	// VaultConfig is the HashiCorp Vault configuration for credential retrieval.
	VaultConfig *config.HashicorpVault

	// VaultToken / VaultAddr override the VAULT_TOKEN / VAULT_ADDR env vars.
	VaultToken string
	VaultAddr  string

	// DiffusionVersion is embedded into state files for diagnostics.
	DiffusionVersion string

	// Wait controls the pre-deploy host reachability wait phase.
	Wait WaitConfig
}

// Deploy is the top-level orchestrator. It:
//  1. Resolves credentials for role source repos.
//  2. Fetches diffusion.lock from each remote role source.
//  3. Merges all lock files into one.
//  4. Generates requirements.yml (roles + collections) from the merged lock.
//  5. Generates or copies the target playbook.
//  6. Builds the Ansible inventory.
//  7. Computes a stable RunID.
//  8. Waits for all target hosts to become reachable.
//  9. Generates the wrapper playbook.
// 10. Runs the container — roles/collections install INSIDE it from requirements.yml.
func Deploy(ctx context.Context, cfg DeployConfig) error {
	// --- 1. Resolve credentials ---
	creds, err := resolveCredentials(cfg)
	if err != nil {
		return fmt.Errorf("credential resolution failed: %w", err)
	}

	// --- 2. Fetch remote lock files ---
	log.Printf(config.ColorGreen+"Fetching diffusion.lock from %d role source(s)..."+config.ColorReset,
		len(cfg.RoleSources))
	locks, err := FetchRemoteLocks(cfg.RoleSources, creds)
	if err != nil {
		return fmt.Errorf("fetching remote locks failed: %w", err)
	}
	log.Printf(config.ColorGreen+"Fetched %d lock file(s)"+config.ColorReset, len(locks))

	// --- 3. Merge lock files ---
	mergedLock, err := MergeLocks(locks)
	if err != nil {
		return fmt.Errorf("lock merge failed: %w", err)
	}
	log.Printf(config.ColorGreen+"Lock files merged — python: %s, collections: %d, roles: %d"+config.ColorReset,
		mergedLock.Python.Pinned, len(mergedLock.Collections), len(mergedLock.Roles))

	// --- 4. Generate requirements.yml from merged lock ---
	requirementsBytes, err := GenerateRequirements(mergedLock)
	if err != nil {
		return fmt.Errorf("requirements.yml generation failed: %w", err)
	}

	// --- 5. Prepare playbook ---
	var playbookBytes []byte
	var playbookFilename string

	if cfg.Playbook != "" {
		// User supplied a playbook — read it from disk.
		playbookBytes, err = os.ReadFile(cfg.Playbook)
		if err != nil {
			return fmt.Errorf("failed to read user playbook %q: %w", cfg.Playbook, err)
		}
		playbookFilename = filepath.Base(cfg.Playbook)
		log.Printf(config.ColorGreen+"Using user-supplied playbook: %s"+config.ColorReset, cfg.Playbook)
	} else {
		// Auto-generate playbook from role sources.
		playbookBytes, err = GeneratePlaybook(cfg.RoleSources)
		if err != nil {
			return fmt.Errorf("auto-playbook generation failed: %w", err)
		}
		playbookFilename = "site.yml"
		log.Printf(config.ColorGreen + "Auto-generated playbook from role sources" + config.ColorReset)
	}

	// --- 6. Build inventory ---
	inventoryBytes, err := BuildInventory(cfg.Hosts, cfg.Groups, cfg.GlobalVars)
	if err != nil {
		return fmt.Errorf("inventory generation failed: %w", err)
	}

	// Create a temp working directory for all generated files.
	tmpDir, err := os.MkdirTemp("", "diffusion-deploy-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			log.Printf(config.ColorYellow+"warning: failed to remove temp dir %s: %v"+config.ColorReset, tmpDir, err)
		}
	}()

	// Write inventory
	inventoryPath := filepath.Join(tmpDir, "inventory.yml")
	if err := os.WriteFile(inventoryPath, inventoryBytes, 0o600); err != nil {
		return fmt.Errorf("failed to write inventory: %w", err)
	}

	// --- 7. Compute RunID ---
	runID := computeRunID(mergedLock, inventoryBytes, playbookFilename, cfg.ExtraVars)
	log.Printf(config.ColorGreen+"RunID: %s..."+config.ColorReset, runID[:16])

	// --- 8. Wait for hosts ---
	containerCfg := buildContainerConfig(cfg, mergedLock, creds, inventoryPath, "", "")
	if err := WaitForHosts(ctx, inventoryPath, containerCfg, cfg.Wait); err != nil {
		return fmt.Errorf("host reachability check failed: %w", err)
	}

	// --- 9. Generate wrapper playbook ---
	wrapperBytes, err := GenerateWrapperPlaybook(WrapperConfig{
		UserPlaybook:       "/deploy/playbook/" + playbookFilename,
		SkipIfSucceededFor: cfg.SkipIfSucceededFor,
		RunID:              runID,
		DiffusionVersion:   cfg.DiffusionVersion,
	})
	if err != nil {
		return fmt.Errorf("wrapper playbook generation failed: %w", err)
	}

	// Assemble the playbook directory:
	//   /deploy/playbook/wrapper.yml        — wrapper + state logic
	//   /deploy/playbook/<playbook>.yml     — user or auto-generated target
	//   /deploy/playbook/requirements.yml  — galaxy install manifest
	playbookDir := filepath.Join(tmpDir, "playbook")
	if err := os.MkdirAll(playbookDir, 0o755); err != nil {
		return fmt.Errorf("failed to create playbook dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(playbookDir, "wrapper.yml"), wrapperBytes, 0o644); err != nil {
		return fmt.Errorf("failed to write wrapper.yml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(playbookDir, playbookFilename), playbookBytes, 0o644); err != nil {
		return fmt.Errorf("failed to write target playbook: %w", err)
	}
	if err := os.WriteFile(filepath.Join(playbookDir, "requirements.yml"), requirementsBytes, 0o644); err != nil {
		return fmt.Errorf("failed to write requirements.yml: %w", err)
	}

	// Write extra vars file if any.
	var extraVarsFile string
	if len(cfg.ExtraVars) > 0 {
		evData, err := json.Marshal(cfg.ExtraVars)
		if err != nil {
			return fmt.Errorf("failed to marshal extra vars: %w", err)
		}
		extraVarsFile = filepath.Join(tmpDir, "extra_vars.json")
		if err := os.WriteFile(extraVarsFile, evData, 0o600); err != nil {
			return fmt.Errorf("failed to write extra vars: %w", err)
		}
	}

	// --- 10. Run deploy container ---
	containerCfg = buildContainerConfig(cfg, mergedLock, creds, inventoryPath, playbookDir, extraVarsFile)

	log.Printf(config.ColorGreen + "Starting deploy container..." + config.ColorReset)
	if err := RunDeployContainer(containerCfg); err != nil {
		writeFailureState(inventoryPath, runID, containerCfg)
		return fmt.Errorf("deploy failed: %w", err)
	}

	log.Printf(config.ColorGreen + "Deploy completed successfully." + config.ColorReset)
	return nil
}

// buildContainerConfig assembles a DeployContainerConfig from DeployConfig.
func buildContainerConfig(
	cfg DeployConfig,
	mergedLock *dependency.LockFile,
	creds []config.ArtifactCredentials,
	inventoryPath, playbookDir, extraVarsFile string,
) DeployContainerConfig {
	resolved := make([]ResolvedCredential, 0, len(creds))
	for _, c := range creds {
		resolved = append(resolved, ResolvedCredential{
			Name:     c.Name,
			URL:      c.URL,
			Username: c.Username,
			Password: c.Password,
			Token:    c.Token,
		})
	}
	return DeployContainerConfig{
		MergedLock:        mergedLock,
		InventoryPath:     inventoryPath,
		PlaybookDir:       playbookDir,
		ExtraVarsFile:     extraVarsFile,
		ContainerRegistry: cfg.ContainerRegistry,
		ArtifactSources:   resolved,
		VaultToken:        cfg.VaultToken,
		VaultAddr:         cfg.VaultAddr,
	}
}

// resolveCredentials loads ArtifactCredentials for all configured ArtifactSources.
func resolveCredentials(cfg DeployConfig) ([]config.ArtifactCredentials, error) {
	var creds []config.ArtifactCredentials
	for _, src := range cfg.ArtifactSourcesCfg {
		cred, err := secrets.GetArtifactCredentials(&src, cfg.VaultConfig)
		if err != nil {
			log.Printf(config.ColorYellow+"warning: could not load credentials for artifact source %q: %v"+config.ColorReset,
				src.Name, err)
			continue
		}
		creds = append(creds, *cred)
	}
	return creds, nil
}

// computeRunID produces a stable SHA-256 hash of all deploy inputs.
func computeRunID(lock *dependency.LockFile, inventory []byte, playbookName string, extraVars map[string]string) string {
	h := sha256.New()
	if lock != nil {
		fmt.Fprintf(h, "lock:%s\n", lock.Hash)
	}
	h.Write(inventory)
	fmt.Fprintf(h, "playbook:%s\n", playbookName)
	if len(extraVars) > 0 {
		keys := make([]string, 0, len(extraVars))
		for k := range extraVars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(h, "var:%s=%s\n", k, extraVars[k])
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// writeFailureState writes a failure marker to all remote hosts.
func writeFailureState(inventoryPath, runID string, cfg DeployContainerConfig) {
	image := ""
	if cfg.ContainerRegistry != nil {
		image = cfg.ContainerRegistry.RegistryServer + "/" +
			cfg.ContainerRegistry.MoleculeContainerName + ":" +
			cfg.ContainerRegistry.MoleculeContainerTag
	}
	if image == "" || strings.HasPrefix(image, "/") {
		image = "ghcr.io/" + config.DefaultMoleculeContainerName + ":latest"
	}

	stateContent := fmt.Sprintf(`{"status":"failed","run_id":"%s"}`, runID)
	dockerArgs := []string{
		"run", "--rm",
		"-v", fmt.Sprintf("%s:/deploy/inventory.yml:ro", inventoryPath),
	}
	if sshDir := sshKeyDir(); sshDir != "" {
		dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s:/root/.ssh:ro", sshDir))
	}
	dockerArgs = append(dockerArgs,
		image,
		"ansible", "all",
		"-i", "/deploy/inventory.yml",
		"-m", "ansible.builtin.copy",
		"-a", fmt.Sprintf("dest=~/.diffusion/state content='%s' mode=0600", stateContent),
	)

	cmd := buildExecCommand("docker", dockerArgs...)
	if err := cmd.Run(); err != nil {
		log.Printf(config.ColorYellow+"warning: could not write failure state to remote hosts: %v"+config.ColorReset, err)
	}
}
