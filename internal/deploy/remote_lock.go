package deploy

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/dependency"
	"diffusion/internal/utils"
)

// RoleSource describes a remote Ansible role repo to fetch a diffusion.lock from.
type RoleSource struct {
	// SCM is the source control type: "git" or "galaxy".
	SCM string
	// Version is a version constraint string (e.g. ">=1.0.0", "main", "v2.3.1").
	Version string
	// URL is the git repository URL. Required when SCM == "git".
	URL string
	// Galaxy is the Galaxy role name in "namespace.role_name" format.
	// Required when SCM == "galaxy".
	Galaxy string
	// Name overrides the role name used in the auto-generated playbook.
	// Defaults to the Galaxy name (last segment) or the repo basename for git.
	Name string
	// ApplyTo is the Ansible hosts pattern for the auto-generated playbook play
	// that applies this role. Defaults to "all".
	ApplyTo string
}

// EffectiveName returns the resolved role name for playbook / requirements use.
func (rs RoleSource) EffectiveName() string {
	if rs.Name != "" {
		return rs.Name
	}
	if rs.Galaxy != "" {
		return rs.Galaxy // full "namespace.role_name"
	}
	// Derive from git URL basename, strip .git suffix.
	base := rs.URL
	if idx := strings.LastIndexAny(base, "/:\\"); idx >= 0 {
		base = base[idx+1:]
	}
	base = strings.TrimSuffix(base, ".git")
	return base
}

// EffectiveApplyTo returns the hosts pattern, defaulting to "all".
func (rs RoleSource) EffectiveApplyTo() string {
	if rs.ApplyTo != "" {
		return rs.ApplyTo
	}
	return "all"
}

// FetchRemoteLocks clones / downloads each role source, reads its diffusion.lock,
// and returns the parsed lock files. Credentials for private git repos are
// injected via GIT_USER_* / GIT_PASSWORD_* / GIT_URL_* environment variables,
// following the existing diffusion convention from config/constants.go.
func FetchRemoteLocks(sources []RoleSource, creds []config.ArtifactCredentials) ([]dependency.LockFile, error) {
	var locks []dependency.LockFile

	for i, src := range sources {
		log.Printf(config.ColorGreen+"Fetching diffusion.lock from role source %d/%d (%s)"+config.ColorReset,
			i+1, len(sources), sourceLabel(src))

		var lockFile *dependency.LockFile
		var err error

		switch strings.ToLower(src.SCM) {
		case "git":
			lockFile, err = fetchLockFromGit(src, creds)
		case "galaxy":
			lockFile, err = fetchLockFromGalaxy(src)
		default:
			return nil, fmt.Errorf("role source %d: unsupported SCM %q (must be \"git\" or \"galaxy\")", i+1, src.SCM)
		}

		if err != nil {
			return nil, fmt.Errorf("role source %d (%s): %w", i+1, sourceLabel(src), err)
		}

		if lockFile == nil {
			log.Printf(config.ColorYellow+"role source %d (%s): no diffusion.lock found — skipping"+config.ColorReset,
				i+1, sourceLabel(src))
			continue
		}

		locks = append(locks, *lockFile)
	}

	return locks, nil
}

// fetchLockFromGit shallow-clones the git repo and reads diffusion.lock from
// its root. Supports authenticated repos via ArtifactCredentials.
// The clone/parse logic lives in internal/dependency so the transitive
// resolver can share it.
func fetchLockFromGit(src RoleSource, creds []config.ArtifactCredentials) (*dependency.LockFile, error) {
	if src.URL == "" {
		return nil, fmt.Errorf("URL is required for SCM=git")
	}

	manifest, err := dependency.FetchLockFromGit(src.URL, src.Version, creds)
	if err != nil {
		return nil, err
	}
	if manifest == nil {
		return nil, nil
	}
	return manifest.Lock, nil
}

// fetchLockFromGalaxy downloads the role tarball from Ansible Galaxy at the
// resolved version and extracts diffusion.lock from it.
func fetchLockFromGalaxy(src RoleSource) (*dependency.LockFile, error) {
	if src.Galaxy == "" {
		return nil, fmt.Errorf("galaxy name is required for SCM=galaxy (format: namespace.role_name)")
	}

	parts := strings.SplitN(src.Galaxy, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid galaxy name %q: expected \"namespace.role_name\" format", src.Galaxy)
	}
	namespace, roleName := parts[0], parts[1]

	// The galaxy name and version end up as a positional argument;
	// ansible-galaxy has no "--" terminator, so reject option-like values.
	if err := utils.ValidateCLIArgument("galaxy name", src.Galaxy); err != nil {
		return nil, err
	}
	if err := utils.ValidateCLIArgument("version", src.Version); err != nil {
		return nil, err
	}

	tmpDir, err := os.MkdirTemp("", "diffusion-galaxy-lock-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			log.Printf(config.ColorYellow+"warning: failed to clean temp dir %s: %v"+config.ColorReset, tmpDir, err)
		}
	}()

	// Build ansible-galaxy install command to download into tmpDir.
	// ansible-galaxy writes roles into <destpath>/<namespace>/<rolename>/.
	roleSpec := fmt.Sprintf("%s.%s", namespace, roleName)
	if src.Version != "" && src.Version != "latest" {
		roleSpec = fmt.Sprintf("%s,%s", roleSpec, src.Version)
	}

	cmd := exec.Command("ansible-galaxy", "role", "install",
		"--roles-path", tmpDir,
		"--no-deps",
		roleSpec,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("ansible-galaxy role install failed: %w\n%s", err, string(out))
	}

	// ansible-galaxy installs into <tmpDir>/<namespace>.<rolename>/
	roleDir := filepath.Join(tmpDir, fmt.Sprintf("%s.%s", namespace, roleName))
	return readLockFromDir(roleDir)
}

func sourceLabel(src RoleSource) string {
	if src.SCM == "galaxy" {
		return fmt.Sprintf("galaxy:%s", src.Galaxy)
	}
	return fmt.Sprintf("git:%s", src.URL)
}
