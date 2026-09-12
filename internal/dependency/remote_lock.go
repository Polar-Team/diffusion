package dependency

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"diffusion/internal/config"

	"gopkg.in/yaml.v3"
)

// RemoteManifest is what a remote diffusion project exposes to us: its lock
// file (authoritative, with resolved versions) and/or its diffusion.toml
// dependency configuration (declared constraints only).
//
// Both fields may be nil — not every Ansible role is a diffusion project.
type RemoteManifest struct {
	// Lock is the parsed diffusion.lock from the remote repo root, or nil.
	Lock *LockFile
	// Config is the parsed [dependencies] section of the remote diffusion.toml,
	// or nil when the file is absent or has no dependency section.
	Config *config.DependencyConfig
	// Dir is the directory the manifest was read from. It is already removed
	// by the time FetchLockFromGit returns and is kept for diagnostics only.
	Dir string
}

// IsEmpty reports whether the remote repo carried no diffusion metadata at all.
func (rm *RemoteManifest) IsEmpty() bool {
	return rm == nil || (rm.Lock == nil && rm.Config == nil)
}

// FetchLockFromGit shallow-clones a git repository at the given version and
// reads its diffusion.lock and diffusion.toml. The clone is removed before
// returning — only parsed data is kept.
//
// Credentials for private repos are injected via the GIT_USER_* /
// GIT_PASSWORD_* / GIT_URL_* environment convention (see BuildGitEnv).
func FetchLockFromGit(url, version string, creds []config.ArtifactCredentials) (*RemoteManifest, error) {
	if url == "" {
		return nil, fmt.Errorf("git URL is required")
	}

	// URLs and refs at depth > 0 come from third-party lock files. A value
	// starting with "-" would be parsed by git as an option (e.g.
	// "--upload-pack=..." executes an arbitrary command), so reject it before
	// building the argument vector.
	ref := ResolveGitRef(version)
	if strings.HasPrefix(url, "-") {
		return nil, fmt.Errorf("refusing to clone %q: argument looks like a git option", url)
	}
	if strings.HasPrefix(ref, "-") {
		return nil, fmt.Errorf("refusing to clone %q: argument looks like a git option", ref)
	}

	tmpDir, err := os.MkdirTemp("", "diffusion-remote-lock-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			log.Printf(config.ColorYellow+"warning: failed to clean temp dir %s: %v"+config.ColorReset, tmpDir, err)
		}
	}()

	cloneArgs := []string{"clone", "--depth", "1", "--no-tags"}

	// Resolve the ref: Version doubles as the branch/tag/commit ref.
	if ref != "" {
		cloneArgs = append(cloneArgs, "--branch", ref)
	}

	// "--" terminates option parsing so the URL can never be read as a flag.
	cloneArgs = append(cloneArgs, "--", url, tmpDir)

	cmd := exec.Command("git", cloneArgs...)
	cmd.Env = BuildGitEnv(url, creds)

	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("git clone failed: %w\n%s", err, string(out))
	}

	lock, err := ReadLockFromDir(tmpDir)
	if err != nil {
		return nil, err
	}

	depCfg, err := readDependencyConfigFromDir(tmpDir)
	if err != nil {
		return nil, err
	}

	return &RemoteManifest{Lock: lock, Config: depCfg, Dir: tmpDir}, nil
}

// ReadLockFromDir reads and parses diffusion.lock from the given directory.
// Returns (nil, nil) if the file does not exist (the repo has no lock).
func ReadLockFromDir(dir string) (*LockFile, error) {
	lockPath := filepath.Join(dir, config.LockFileName)

	data, err := os.ReadFile(lockPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", config.LockFileName, err)
	}

	var lf LockFile
	if err := yaml.Unmarshal(data, &lf); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", config.LockFileName, err)
	}

	return &lf, nil
}

// readDependencyConfigFromDir reads the [dependencies] section of a
// diffusion.toml located in dir. Returns (nil, nil) when the file is absent or
// carries no dependency configuration. A malformed file is reported as an
// error so that a typo in a remote repo is visible rather than silently
// dropping its dependencies.
func readDependencyConfigFromDir(dir string) (*config.DependencyConfig, error) {
	cfgPath := filepath.Join(dir, "diffusion.toml")
	if _, err := os.Stat(cfgPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to stat diffusion.toml: %w", err)
	}

	cfg, err := config.LoadConfigFrom(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load remote diffusion.toml: %w", err)
	}
	if cfg == nil {
		return nil, nil
	}

	return cfg.DependencyConfig, nil
}

// ResolveGitRef converts a version constraint or ref name into a usable git ref.
// Plain semver constraints like ">=1.0.0" cannot be used as a branch; in that
// case we return "" to clone the default branch and let the merger handle
// version resolution later.
func ResolveGitRef(version string) string {
	if version == "" || version == "latest" {
		return ""
	}
	// If it looks like a constraint operator, clone the default branch.
	if strings.HasPrefix(version, ">=") || strings.HasPrefix(version, "<=") ||
		strings.HasPrefix(version, ">") || strings.HasPrefix(version, "<") ||
		strings.HasPrefix(version, "==") {
		return ""
	}
	return version
}

// BuildGitEnv constructs an os.Environ slice injecting credential env vars for
// the matching artifact credential, following the GIT_USER_* / GIT_PASSWORD_* /
// GIT_URL_* convention used in molecule.go.
func BuildGitEnv(repoURL string, creds []config.ArtifactCredentials) []string {
	env := os.Environ()

	for _, cred := range creds {
		if cred.URL == "" || !strings.Contains(repoURL, StripScheme(cred.URL)) {
			continue
		}
		// Use a sanitised key suffix derived from the credential name.
		key := SanitizeEnvKey(cred.Name)
		env = append(env,
			fmt.Sprintf("%s%s=%s", config.EnvGitUserPrefix, key, cred.Username),
			fmt.Sprintf("%s%s=%s", config.EnvGitPassPrefix, key, cred.Password),
			fmt.Sprintf("%s%s=%s", config.EnvGitURLPrefix, key, cred.URL),
		)

		// Configure git credential helper inline so git uses our env vars.
		// GIT_CONFIG_COUNT / GIT_CONFIG_KEY_n / GIT_CONFIG_VALUE_n is the
		// portable way to inject git config without touching ~/.gitconfig.
		env = append(env,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=credential.helper",
			fmt.Sprintf("GIT_CONFIG_VALUE_0=!f(){ echo username=$%s%s; echo password=$%s%s; }; f",
				config.EnvGitUserPrefix, key,
				config.EnvGitPassPrefix, key),
		)
		break
	}

	return env
}

// StripScheme removes a URL scheme / scp-style user prefix from u.
// The set of prefixes is kept identical to the original deploy implementation
// so that credential matching behaviour is unchanged; identity normalisation
// for the transitive resolver handles a wider set (see normalizeIdentity).
func StripScheme(u string) string {
	for _, pfx := range []string{"https://", "http://", "git@"} {
		u = strings.TrimPrefix(u, pfx)
	}
	return u
}

// SanitizeEnvKey converts an artifact source name into an env-var-safe suffix.
func SanitizeEnvKey(name string) string {
	return strings.ToUpper(strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(name))
}
