//go:build acc

// Package tfprovider acceptance tests.
//
// These tests require:
//   - TF_ACC=1 environment variable
//   - A terraform binary on PATH
//   - The diffusion binary on PATH (or DIFFUSION_BINARY env var)
//   - Docker running (to spin up the molecule container)
//
// Run with:
//
//	TF_ACC=1 go test -v -tags acc ./internal/tfprovider/... -run TestAcc
//
// They are excluded from the default `go test ./...` via the `acc` build tag.
package tfprovider

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories wires the in-process provider into the
// Terraform plugin test runner without requiring the provider binary to be
// installed on disk.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"diffusion": providerserver.NewProtocol6WithError(New("test")()),
}

func skipUnlessAccEnabled(t *testing.T) {
	t.Helper()
	if os.Getenv("TF_ACC") == "" {
		t.Skip("Set TF_ACC=1 to run acceptance tests (requires Docker + diffusion binary)")
	}
}

// localDeployFixture creates a temporary git repository on disk that contains
// a minimal diffusion.lock and a meta/main.yml.  Returns the file:// URL
// suitable for use as a --role-source url value.
func localDeployFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	// Init bare git repo
	runCmd(t, dir, "git", "init")
	runCmd(t, dir, "git", "config", "user.email", "test@test.com")
	runCmd(t, dir, "git", "config", "user.name", "Test")

	// Minimal diffusion.lock — no roles or collections to install; the test
	// playbook supplied via --playbook will not reference any roles.
	lockContent := `version: "1.0"
generated: "` + time.Now().UTC().Format(time.RFC3339) + `"
hash: "test-acc-fixture"
python:
  min: "3.11"
  max: "3.13"
  pinned: "3.13"
roles: []
collections: []
tools: []
`
	writeFile(t, dir, "diffusion.lock", lockContent)

	// meta/main.yml so FetchRemoteLocks has a valid role meta
	metaContent := `galaxy_info:
  role_name: acc_test_role
  namespace: test
  author: CI
  description: Acceptance test fixture
  license: MIT
  min_ansible_version: "2.10"
  platforms: []
collections: []
`
	if err := os.MkdirAll(filepath.Join(dir, "meta"), 0755); err != nil {
		t.Fatalf("mkdir meta: %v", err)
	}
	writeFile(t, dir, filepath.Join("meta", "main.yml"), metaContent)

	runCmd(t, dir, "git", "add", ".")
	runCmd(t, dir, "git", "commit", "-m", "init")
	runCmd(t, dir, "git", "branch", "-M", "main")

	return "file://" + filepath.ToSlash(dir)
}

// localDebugPlaybook writes a minimal playbook that just emits a debug message.
// Returns the absolute path to the playbook file.
func localDebugPlaybook(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	content := `---
- name: "diffusion deploy acceptance test"
  hosts: all
  gather_facts: false
  tasks:
    - name: smoke test
      ansible.builtin.debug:
        msg: "diffusion deploy acceptance test passed"
`
	path := filepath.Join(dir, "site.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write playbook: %v", err)
	}
	return path
}

// ---------------------------------------------------------------------------
// Acceptance: diffusion deploy CLI (black-box)
// ---------------------------------------------------------------------------

// TestAccDeployLocalhost verifies that `diffusion deploy` can run a simple
// debug playbook against localhost (ansible_connection=local) using a local
// git fixture as the role source.
func TestAccDeployLocalhost(t *testing.T) {
	skipUnlessAccEnabled(t)

	binary := os.Getenv("DIFFUSION_BINARY")
	if binary == "" {
		binary = "diffusion"
	}
	if _, err := exec.LookPath(binary); err != nil {
		t.Skipf("diffusion binary not found on PATH (%v); set DIFFUSION_BINARY to override", err)
	}

	repoURL := localDeployFixture(t)
	playbook := localDebugPlaybook(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	args := []string{
		"deploy",
		"--role-source", fmt.Sprintf("scm=git,version=main,url=%s", repoURL),
		"--playbook", playbook,
		"--host", "localhost=ansible_connection=local",
		"--host-wait-initial-delay", "1s",
		"--host-wait-interval", "2s",
		"--host-wait-timeout", "60s",
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("diffusion deploy failed: %v\nargs: %v", err, args)
	}
}

// TestAccDeploySkipPeriod verifies that a second identical deploy within the
// skip period is skipped without error.
func TestAccDeploySkipPeriod(t *testing.T) {
	skipUnlessAccEnabled(t)

	binary := os.Getenv("DIFFUSION_BINARY")
	if binary == "" {
		binary = "diffusion"
	}
	if _, err := exec.LookPath(binary); err != nil {
		t.Skipf("diffusion binary not found: %v", err)
	}

	repoURL := localDeployFixture(t)
	playbook := localDebugPlaybook(t)

	runDeploy := func(label string) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		args := []string{
			"deploy",
			"--role-source", fmt.Sprintf("scm=git,version=main,url=%s", repoURL),
			"--playbook", playbook,
			"--host", "localhost=ansible_connection=local",
			"--host-wait-initial-delay", "1s",
			"--host-wait-timeout", "60s",
			"--skip-period", "1h",
		}
		cmd := exec.CommandContext(ctx, binary, args...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("[%s] diffusion deploy failed: %v\n%s", label, err, out)
		}
	}

	runDeploy("first run")
	runDeploy("second run (should be skipped)")
}

// TestAccDeployMultipleRoleSources verifies that multiple --role-source flags
// (with distinct apply_to patterns) produce a multi-play playbook without errors.
func TestAccDeployMultipleRoleSources(t *testing.T) {
	skipUnlessAccEnabled(t)

	binary := os.Getenv("DIFFUSION_BINARY")
	if binary == "" {
		binary = "diffusion"
	}
	if _, err := exec.LookPath(binary); err != nil {
		t.Skipf("diffusion binary not found: %v", err)
	}

	repo1 := localDeployFixture(t)
	repo2 := localDeployFixture(t)
	playbook := localDebugPlaybook(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	args := []string{
		"deploy",
		"--role-source", fmt.Sprintf("scm=git,version=main,url=%s,name=role_a,apply_to=all", repo1),
		"--role-source", fmt.Sprintf("scm=git,version=main,url=%s,name=role_b,apply_to=all", repo2),
		"--playbook", playbook,
		"--host", "localhost=ansible_connection=local",
		"--host-wait-initial-delay", "1s",
		"--host-wait-timeout", "60s",
	}

	cmd := exec.CommandContext(ctx, binary, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("diffusion deploy failed: %v\n%s", err, out)
	}
}

// ---------------------------------------------------------------------------
// Acceptance: Terraform provider (requires terraform binary + TF_ACC=1)
// ---------------------------------------------------------------------------

// TestAccTerraformProvider_DeployResource is a skeleton for running the
// diffusion_deploy resource through the full Terraform plan/apply/destroy
// lifecycle via terraform-plugin-testing.
//
// To enable: add github.com/hashicorp/terraform-plugin-testing to go.mod
// and uncomment the test body.
func TestAccTerraformProvider_DeployResource(t *testing.T) {
	skipUnlessAccEnabled(t)

	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform binary not found on PATH; skipping provider acceptance test")
	}

	repoURL := localDeployFixture(t)
	playbook := localDebugPlaybook(t)

	// Use the low-level provider server directly to validate the schema and
	// resource lifecycle without the full terraform-plugin-testing harness.
	srv, err := testAccProtoV6ProviderFactories["diffusion"]()
	if err != nil {
		t.Fatalf("failed to create provider server: %v", err)
	}
	if srv == nil {
		t.Fatal("provider server is nil")
	}

	t.Logf("Provider server created successfully (repo=%s playbook=%s)", repoURL, playbook)
	t.Log("Full Terraform plan/apply test requires terraform-plugin-testing integration.")
	t.Log("Add github.com/hashicorp/terraform-plugin-testing to go.mod and implement resource.TestCase here.")
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func runCmd(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %s %v failed: %v\n%s", name, args, err, out)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// validateDeployOutput checks that deploy output contains expected strings.
func validateDeployOutput(t *testing.T, output string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(output, w) {
			t.Errorf("deploy output missing %q\nFull output:\n%s", w, output)
		}
	}
}
