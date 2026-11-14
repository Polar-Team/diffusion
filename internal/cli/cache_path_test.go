package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// setHome overrides HOME (and USERPROFILE on Windows) to point at dir for
// the duration of the test.
func setHome(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
}

const minimalDiffusionTOML = `url = "https://example.com"

[container_registry]
registry_server = "cr.example.com"
registry_provider = "Public"
molecule_container_name = "molecule"
molecule_container_tag = "latest"

[vault]
enabled = false

[yaml_lint]
extends = "default"

[ansible_lint]

[tests]
type = "diffusion"
`

func writeMinimalConfig(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "diffusion.toml")
	if err := os.WriteFile(path, []byte(minimalDiffusionTOML), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCacheEnable_CacheIDFlag_NewConfig(t *testing.T) {
	dir := chdirTemp(t)
	setHome(t, dir)
	writeMinimalConfig(t, dir)

	cmd := NewCacheCmd(&CLI{})
	cmd.SetArgs([]string{"enable", "--cache-id", "my-fixed-id"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cache enable failed: %v\noutput: %s", err, out.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, "diffusion.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `cache_id = "my-fixed-id"`) {
		t.Errorf("expected cache_id 'my-fixed-id' to be persisted, got:\n%s", string(data))
	}
}

func TestCacheEnable_CacheIDFlag_OverridesExisting(t *testing.T) {
	dir := chdirTemp(t)
	setHome(t, dir)
	writeMinimalConfig(t, dir)

	// First enable with one ID.
	cmd1 := NewCacheCmd(&CLI{})
	cmd1.SetArgs([]string{"enable", "--cache-id", "old-id"})
	cmd1.SetOut(&bytes.Buffer{})
	if err := cmd1.Execute(); err != nil {
		t.Fatalf("first enable failed: %v", err)
	}

	// Override with a new ID.
	cmd2 := NewCacheCmd(&CLI{})
	cmd2.SetArgs([]string{"enable", "--cache-id", "new-id"})
	var out bytes.Buffer
	cmd2.SetOut(&out)
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("second enable failed: %v\noutput: %s", err, out.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, "diffusion.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `cache_id = "new-id"`) {
		t.Errorf("expected cache_id to be overridden to 'new-id', got:\n%s", string(data))
	}
	if strings.Contains(string(data), `cache_id = "old-id"`) {
		t.Errorf("old cache_id should not remain in config:\n%s", string(data))
	}

	if !strings.Contains(out.String(), "Cache ID overridden: old-id -> new-id") {
		t.Errorf("expected override notice in output, got:\n%s", out.String())
	}
}

func TestCacheEnable_InvalidCacheID_LeavesConfigUntouched(t *testing.T) {
	dir := chdirTemp(t)
	setHome(t, dir)
	writeMinimalConfig(t, dir)

	before, err := os.ReadFile(filepath.Join(dir, "diffusion.toml"))
	if err != nil {
		t.Fatal(err)
	}

	cmd := NewCacheCmd(&CLI{})
	cmd.SetArgs([]string{"enable", "--cache-id", "../../etc/passwd"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	err = cmd.Execute()
	if err == nil {
		t.Fatal("expected error for invalid cache ID, got nil")
	}

	after, err := os.ReadFile(filepath.Join(dir, "diffusion.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("config should be unchanged on validation failure.\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestCachePath_DefaultPath(t *testing.T) {
	dir := chdirTemp(t)
	setHome(t, dir)
	writeMinimalConfig(t, dir)

	enableCmd := NewCacheCmd(&CLI{})
	enableCmd.SetArgs([]string{"enable", "--cache-id", "abc123"})
	enableCmd.SetOut(&bytes.Buffer{})
	if err := enableCmd.Execute(); err != nil {
		t.Fatalf("enable failed: %v", err)
	}

	pathCmd := NewCacheCmd(&CLI{})
	var out bytes.Buffer
	pathCmd.SetArgs([]string{"path"})
	pathCmd.SetOut(&out)
	if err := pathCmd.Execute(); err != nil {
		t.Fatalf("cache path failed: %v", err)
	}

	got := strings.TrimSpace(out.String())
	want := filepath.Join(dir, ".diffusion", "cache", "role_abc123")
	if got != want {
		t.Errorf("cache path = %q, want %q", got, want)
	}
}

func TestCachePath_CustomPathExists(t *testing.T) {
	dir := chdirTemp(t)
	setHome(t, dir)
	writeMinimalConfig(t, dir)

	customDir := filepath.Join(dir, "custom-cache-location")
	if err := os.MkdirAll(customDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Enable then manually set cache_path by re-writing config with the custom path.
	enableCmd := NewCacheCmd(&CLI{})
	enableCmd.SetArgs([]string{"enable", "--cache-id", "xyz789"})
	enableCmd.SetOut(&bytes.Buffer{})
	if err := enableCmd.Execute(); err != nil {
		t.Fatalf("enable failed: %v", err)
	}

	appendCachePath(t, dir, customDir)

	pathCmd := NewCacheCmd(&CLI{})
	var out bytes.Buffer
	pathCmd.SetArgs([]string{"path"})
	pathCmd.SetOut(&out)
	if err := pathCmd.Execute(); err != nil {
		t.Fatalf("cache path failed: %v", err)
	}

	got := strings.TrimSpace(out.String())
	want := filepath.Join(customDir, "cache", "role_xyz789")
	if got != want {
		t.Errorf("cache path = %q, want %q", got, want)
	}
}

func TestCachePath_CustomPathNonExistentFallsBack(t *testing.T) {
	dir := chdirTemp(t)
	setHome(t, dir)
	writeMinimalConfig(t, dir)

	enableCmd := NewCacheCmd(&CLI{})
	enableCmd.SetArgs([]string{"enable", "--cache-id", "fallback1"})
	enableCmd.SetOut(&bytes.Buffer{})
	if err := enableCmd.Execute(); err != nil {
		t.Fatalf("enable failed: %v", err)
	}

	nonExistent := filepath.Join(dir, "does-not-exist")
	appendCachePath(t, dir, nonExistent)

	pathCmd := NewCacheCmd(&CLI{})
	var out bytes.Buffer
	pathCmd.SetArgs([]string{"path"})
	pathCmd.SetOut(&out)
	if err := pathCmd.Execute(); err != nil {
		t.Fatalf("cache path failed: %v", err)
	}

	got := strings.TrimSpace(out.String())
	want := filepath.Join(dir, ".diffusion", "cache", "role_fallback1")
	if got != want {
		t.Errorf("cache path = %q, want fallback %q", got, want)
	}
}

func TestCachePath_NotConfigured_Errors(t *testing.T) {
	dir := chdirTemp(t)
	setHome(t, dir)
	writeMinimalConfig(t, dir)

	pathCmd := NewCacheCmd(&CLI{})
	var out bytes.Buffer
	pathCmd.SetArgs([]string{"path"})
	pathCmd.SetOut(&out)
	pathCmd.SetErr(&out)
	if err := pathCmd.Execute(); err == nil {
		t.Fatal("expected error when cache is not configured, got nil")
	}
}

func TestCachePath_BaseFlag(t *testing.T) {
	dir := chdirTemp(t)
	setHome(t, dir)
	writeMinimalConfig(t, dir)

	enableCmd := NewCacheCmd(&CLI{})
	enableCmd.SetArgs([]string{"enable", "--cache-id", "basetest"})
	enableCmd.SetOut(&bytes.Buffer{})
	if err := enableCmd.Execute(); err != nil {
		t.Fatalf("enable failed: %v", err)
	}

	pathCmd := NewCacheCmd(&CLI{})
	var out bytes.Buffer
	pathCmd.SetArgs([]string{"path", "--base"})
	pathCmd.SetOut(&out)
	if err := pathCmd.Execute(); err != nil {
		t.Fatalf("cache path --base failed: %v", err)
	}

	got := strings.TrimSpace(out.String())
	want := filepath.Join(dir, ".diffusion", "cache")
	if got != want {
		t.Errorf("cache path --base = %q, want %q", got, want)
	}
}

// appendCachePath appends a cache_path entry under [cache] in the
// diffusion.toml located in dir, for tests exercising custom cache paths.
func appendCachePath(t *testing.T, dir, cachePath string) {
	t.Helper()
	configPath := filepath.Join(dir, "diffusion.toml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	idx := strings.Index(content, "[cache]")
	if idx == -1 {
		t.Fatalf("expected [cache] section in config, got:\n%s", content)
	}
	lineEnd := strings.Index(content[idx:], "\n")
	insertAt := idx + lineEnd + 1
	escaped := strings.ReplaceAll(cachePath, `\`, `\\`)
	newContent := content[:insertAt] + "cache_path = \"" + escaped + "\"\n" + content[insertAt:]
	if err := os.WriteFile(configPath, []byte(newContent), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCachePath_RelativeCustomPathIsAbsolute(t *testing.T) {
	dir := chdirTemp(t)
	setHome(t, dir)
	writeMinimalConfig(t, dir)

	// Create the relative cache_path target in the cwd so the "exists" branch
	// of GetCacheDir is taken.
	if err := os.MkdirAll(filepath.Join(dir, ".cache"), 0755); err != nil {
		t.Fatal(err)
	}

	enableCmd := NewCacheCmd(&CLI{})
	enableCmd.SetArgs([]string{"enable", "--cache-id", "relcache1"})
	enableCmd.SetOut(&bytes.Buffer{})
	if err := enableCmd.Execute(); err != nil {
		t.Fatalf("enable failed: %v", err)
	}

	appendCachePath(t, dir, ".cache")

	pathCmd := NewCacheCmd(&CLI{})
	var out bytes.Buffer
	pathCmd.SetArgs([]string{"path"})
	pathCmd.SetOut(&out)
	if err := pathCmd.Execute(); err != nil {
		t.Fatalf("cache path failed: %v", err)
	}

	got := strings.TrimSpace(out.String())
	if !filepath.IsAbs(got) {
		t.Fatalf("cache path should be absolute for a relative cache_path, got %q", got)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cwd, ".cache", "cache", "role_relcache1")

	// Resolve symlinks on both sides using the existing grandparent directory
	// (`.cache`; the `cache/role_<id>` subdirectories are never created by
	// `cache path`): on macOS, temp dirs live under /var, which is a symlink
	// to /private/var, so a naive string comparison can spuriously fail.
	gotBaseResolved, err := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(got)))
	if err != nil {
		t.Fatalf("failed to resolve symlinks for got path %q: %v", got, err)
	}
	wantBaseResolved, err := filepath.EvalSymlinks(filepath.Dir(filepath.Dir(want)))
	if err != nil {
		t.Fatalf("failed to resolve symlinks for want path %q: %v", want, err)
	}
	gotResolved := filepath.Join(gotBaseResolved, "cache", "role_relcache1")
	wantResolved := filepath.Join(wantBaseResolved, "cache", "role_relcache1")

	if gotResolved != wantResolved {
		t.Errorf("cache path = %q (resolved %q), want %q (resolved %q)", got, gotResolved, want, wantResolved)
	}
}

func TestCacheEnable_NoFlagNoCommitted_GeneratesRandomID(t *testing.T) {
	dir := chdirTemp(t)
	setHome(t, dir)
	writeMinimalConfig(t, dir)

	cmd := NewCacheCmd(&CLI{})
	cmd.SetArgs([]string{"enable"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cache enable failed: %v\noutput: %s", err, out.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, "diffusion.toml"))
	if err != nil {
		t.Fatal(err)
	}

	re := regexp.MustCompile(`cache_id = "([0-9a-f]{16})"`)
	match := re.FindStringSubmatch(string(data))
	if match == nil {
		t.Fatalf("expected a generated 16-hex cache_id in config, got:\n%s", string(data))
	}
}
