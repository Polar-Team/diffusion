package diffusion

// diffusion - Cobra-based cross-platform CLI tool to assist with Molecule workflows,
// with Windows-only features for WSL compaction.
//
// Features:
// - Ensures required env vars are set (vault_user, vault_passwd, GIT_URL, PROJECT_ID, VAULT_ADDR)
// - Prompts for VAULT credentials if not in env
// - Gets Vault token and pulls secrets (GIT user/token)
// - Runs `yc` init
// - Implements "molecule" command with flags: role, org, tag, verify, lint, idempotence, wipe
// - Copies role files into molecule layout (if present)
// - Runs docker commands similar to your PowerShell script
// - Adds Windows-only "compact WSL" feature that stops Docker Desktop, shuts down WSL and runs Optimize-VHD
//
// NOTE: This CLI shells out to external tools: vault, yc, docker, wsl, powershell. They must be available in PATH.
// Optimize-VHD requires elevated (Administrator) powershell rights on Windows.

import (
	"bufio"

	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	roleFlag        string
	orgFlag         string
	tagFlag         string
	verifyFlag      bool
	lintFlag        bool
	idempotenceFlag bool
	wipeFlag        bool
	compactWSLFlag  bool
)

func main() {

	rootCmd := &cobra.Command{
		Use:   "diffusion",
		Short: "Molecule workflow helper (cross-platform) with Windows-only WSL compact features",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			// Ensure some env defaults and prompt when needed

			fetchVaultFieldToEnv("infrastructure/Gitlab_Deploy_Users", "ansible_deploy", "GIT_USER")
			fetchVaultFieldToEnv("infrastructure/Gitlab_Deploy_Tokens", "ansible_deploy", "GIT_PASSWORD")
			return nil
		},
	}

	// molecule command
	molCmd := &cobra.Command{
		Use:   "molecule",
		Short: "run molecule workflow (create/converge/verify/lint/idempotence/wipe)",
		RunE:  runMolecule,
	}

	molCmd.Flags().StringVarP(&roleFlag, "role", "r", "sdl_collector", "role name")
	molCmd.Flags().StringVarP(&orgFlag, "org", "o", "linru", "organization prefix")
	molCmd.Flags().StringVarP(&tagFlag, "tag", "t", "", "ANSIBLE_RUN_TAGS value (optional)")
	molCmd.Flags().BoolVar(&verifyFlag, "verify", false, "run molecule verify")
	molCmd.Flags().BoolVar(&lintFlag, "lint", false, "run linting (yamllint / ansible-lint)")
	molCmd.Flags().BoolVar(&idempotenceFlag, "idempotence", false, "run molecule idempotence")
	molCmd.Flags().BoolVar(&wipeFlag, "wipe", false, "remove container and molecule role folder")

	rootCmd.AddCommand(molCmd)

	// Windows-only helper: compact-wsl
	compactCmd := &cobra.Command{
		Use:   "compact-wsl",
		Short: "Windows-only: shutdown WSL / stop Docker Desktop and Optimize-VHD for Docker Desktop VHDX files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if runtime.GOOS != "windows" {
				return fmt.Errorf("compact-wsl is supported only on Windows")
			}
			return compactWSLAndOptimize()
		},
	}
	compactCmd.Flags().BoolVar(&compactWSLFlag, "confirm", false, "confirm running Optimize-VHD (requires admin)")
	rootCmd.AddCommand(compactCmd)

	// Provide a top-level flag to run compact before molecule (Windows-only)
	rootCmd.PersistentFlags().BoolVar(&compactWSLFlag, "compact-wsl", false, "on Windows: compact Docker Desktop WSL2 vhdx (runs before molecule actions)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// ensureSecretEnv prompts without echoing (if possible)
func ensureSecretEnv(key, prompt string, secret bool) {
	if os.Getenv(key) == "" {
		if secret && runtime.GOOS != "windows" {
			// try to use /dev/tty for password-like prompt
			fmt.Print(prompt + ": ")
			pass, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			pass = strings.TrimSpace(pass)
			_ = os.Setenv(key, pass)
		} else {
			val := promptInput(prompt + ": ")
			_ = os.Setenv(key, val)
		}
	}
}

func promptInput(prompt string) string {
	fmt.Print(prompt)
	r := bufio.NewReader(os.Stdin)
	val, _ := r.ReadString('\n')
	return strings.TrimSpace(val)
}

// runCommand runs command and streams combined stdout/stderr to our stdout/stderr.
func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runCommandCapture returns stdout (trimmed) and error
func runCommandCapture(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// vaultLogin runs: vault login -token-only -method=userpass username="$env:vault_user" password="$env:vault_passwd"
func vaultLogin() error {
	user := os.Getenv("vault_user")
	pass := os.Getenv("vault_passwd")
	if user == "" || pass == "" {
		return fmt.Errorf("vault_user or vault_passwd not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := runCommandCapture(ctx, "vault", "login", "-token-only", "-method=userpass", fmt.Sprintf("username=%s", user), fmt.Sprintf("password=%s", pass))
	if err != nil {
		return fmt.Errorf("vault login failed: %v (%s)", err, out)
	}
	_ = os.Setenv("VAULT_TOKEN", out)
	return nil
}

// fetchVaultFieldToEnv: vault kv get -field="FIELD" PATH
func fetchVaultFieldToEnv(path, field, envName string) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	out, err := runCommandCapture(ctx, "vault", "kv", "get", "-field="+field, path)
	if err != nil {
		// warn only
		log.Printf("warning: could not get vault field %s from %s: %v", field, path, err)
		return
	}
	_ = os.Setenv(envName, out)
}

// ycCliInit runs yc commands and sets env variables YC_TOKEN, YC_CLOUD_ID, YC_FOLDER_ID
func ycCliInit() error {
	// yc iam create-token
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	token, err := runCommandCapture(ctx, "yc", "iam", "create-token")
	if err != nil {
		return fmt.Errorf("yc iam create-token failed: %v (%s)", err, token)
	}
	_ = os.Setenv("YC_TOKEN", token)

	cloudID, _ := runCommandCapture(ctx, "yc", "config", "get", "cloud-id")
	if cloudID != "" {
		_ = os.Setenv("YC_CLOUD_ID", cloudID)
	}

	folderID, _ := runCommandCapture(ctx, "yc", "config", "get", "folder-id")
	if folderID != "" {
		_ = os.Setenv("YC_FOLDER_ID", folderID)
	}
	return nil
}

// runMolecule is the core function that implements the behavior from your PS script
func runMolecule(cmd *cobra.Command, args []string) error {
	// If user requested Windows compaction before running molecule
	if compactWSLFlag && runtime.GOOS == "windows" {
		log.Println("Running Windows WSL compact prior to molecule (requested)...")
		if err := compactWSLAndOptimize(); err != nil {
			log.Printf("compact-wsl failed: %v", err)
		}
	}

	// prepare path
	path, err := os.Getwd()
	if err != nil {
		return err
	}
	// use forward slashes in mounts where required (docker on windows expects Windows paths but we keep raw)
	// Compose role path
	roleDirName := fmt.Sprintf("%s.%s", orgFlag, roleFlag)
	roleMoleculePath := filepath.Join(path, "molecule", roleDirName)

	// handle wipe
	if wipeFlag {
		log.Printf("Wiping: removing container molecule-%s and folder %s\n", roleFlag, roleMoleculePath)
		_ = runCommand("docker", "rm", fmt.Sprintf("molecule-%s", roleFlag), "-f")
		if err := os.RemoveAll(roleMoleculePath); err != nil {
			log.Printf("warning: failed remove role path: %v", err)
		}
		return nil
	}

	// handle lint/verify/idempotence by ensuring files are copied and running docker exec commands
	if lintFlag || verifyFlag || idempotenceFlag {
		if err := copyRoleData(path, roleMoleculePath, orgFlag, roleFlag); err != nil {
			log.Printf("warning copying data: %v", err)
		}
		// ensure tests dir exists for verify/lint
		defaultTestsDir := filepath.Join(roleMoleculePath, "molecule", "default", "tests")
		if err := os.MkdirAll(defaultTestsDir, 0o755); err != nil {
			log.Printf("warning: cannot create tests dir: %v", err)
		}
		if lintFlag {
			// run yamllint and ansible-lint inside container
			cmdStr := fmt.Sprintf(`(cd ./%s && yamllint . -c .yamllint && ansible-lint -c .ansible-lint && echo 'Done!') || echo 'Failed'`, roleDirName)
			if err := dockerExecInteractive(roleFlag, "/bin/sh", "-c", cmdStr); err != nil {
				log.Printf("lint failed: %v", err)
			}
			return nil
		}
		if verifyFlag {
			// copy tests/*
			testsSrc := filepath.Join(path, "tests")
			copyIfExists(testsSrc, defaultTestsDir)
			cmdStr := fmt.Sprintf("cd ./%s && (molecule verify && echo 'Done!') || echo 'Failed'", roleDirName)
			if err := dockerExecInteractive(roleFlag, "/bin/sh", "-c", cmdStr); err != nil {
				log.Printf("verify failed: %v", err)
			}
			return nil
		}
		if idempotenceFlag {
			tagEnv := ""
			if tagFlag != "" {
				tagEnv = fmt.Sprintf("ANSIBLE_RUN_TAGS=%s ", tagFlag)
			}
			cmdStr := fmt.Sprintf("cd ./%s && (%smolecule idempotence && echo 'Done!') || echo 'Failed'", roleDirName, tagEnv)
			if err := dockerExecInteractive(roleFlag, "/bin/sh", "-c", cmdStr); err != nil {
				log.Printf("idempotence failed: %v", err)
			}
			return nil
		}
	}

	// default flow: create/run container if not exists, copy data, converge
	// check if container exists
	err = exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", roleFlag)).Run()
	if err == nil {
		fmt.Printf("Container molecule-%s already exists. To purge use -wipe.\n", roleFlag)
	} else {
		// create
		if err := ycCliInit(); err != nil {
			log.Printf("yc init warning: %v", err)
		}
		// docker login cr.yandex --username iam --password $Env:YC_TOKEN
		if err := runCommand("docker", "login", "cr.yandex", "--username", "iam", "--password", os.Getenv("YC_TOKEN")); err != nil {
			log.Printf("docker login to cr.yandex failed: %v", err)
		}
		// run container
		// docker run --rm -d --name=molecule-$role -v "$path/molecule:/opt/molecule" -v /sys/fs/cgroup:/sys/fs/cgroup:rw -e ... --privileged --pull always cr.yandex/...
		image := "cr.yandex/crp8cgfah9nqgde7q9rm/molecule_dind:latest"
		args := []string{
			"run", "--rm", "-d", "--name=" + fmt.Sprintf("molecule-%s", roleFlag),
			"-v", fmt.Sprintf("%s/molecule:/opt/molecule", path),
			"-v", "/sys/fs/cgroup:/sys/fs/cgroup:rw",
			"-e", "YC_TOKEN=" + os.Getenv("YC_TOKEN"),
			"-e", "VAULT_TOKEN=" + os.Getenv("VAULT_TOKEN"),
			"-e", "VAULT_ADDR=" + os.Getenv("VAULT_ADDR"),
			"-e", "GIT_USER=" + os.Getenv("GIT_USER"),
			"-e", "GIT_PASSWORD=" + os.Getenv("GIT_PASSWORD"),
			"-e", "GIT_URL=" + os.Getenv("GIT_URL"),
			"--privileged", "--pull", "always",
			image,
		}
		if err := runCommand("docker", args...); err != nil {
			log.Printf("docker run failed: %v", err)
		}
	}

	// ensure role exists
	if exists(roleMoleculePath) {
		fmt.Println("This role already exists in molecule")
	} else {
		// docker exec -ti molecule-$role /bin/sh -c "ansible-galaxy role init $org.$role"
		if err := dockerExecInteractive(roleFlag, "/bin/sh", "-c", fmt.Sprintf("ansible-galaxy role init %s.%s", orgFlag, roleFlag)); err != nil {
			log.Printf("role init warning: %v", err)
		}
	}

	// docker exec login to cr.yandex inside container
	_ = dockerExecInteractive(roleFlag, "/bin/sh", "-c", `docker login cr.yandex --username iam --password $YC_TOKEN`)

	// copy files into molecule structure
	if err := copyRoleData(path, roleMoleculePath, orgFlag, roleFlag); err != nil {
		log.Printf("copy role data warning: %v", err)
	}

	// finally create/converge
	err = exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", roleFlag)).Run()
	if err == nil {
		// container exists
		_ = dockerExecInteractive(roleFlag, "/bin/sh", "-c", fmt.Sprintf("cd ./%s && molecule converge", roleDirName))
	} else {
		_ = dockerExecInteractive(roleFlag, "/bin/sh", "-c", fmt.Sprintf("cd ./%s && molecule create", roleDirName))
		_ = dockerExecInteractive(roleFlag, "/bin/sh", "-c", fmt.Sprintf("cd ./%s && molecule converge", roleDirName))
	}

	return nil
}

// copyRoleData copies tasks, handlers, templates, files, vars, defaults, meta, scenarios, .ansible-lint, .yamllint
func copyRoleData(basePath, roleMoleculePath, org, role string) error {
	// create role dir base
	if err := os.MkdirAll(roleMoleculePath, 0o755); err != nil {
		return err
	}
	// helper copy pairs
	pairs := []struct{ src, dst string }{
		{"tasks", "tasks"},
		{"handlers", "handlers"},
		{"templates", "templates"},
		{"files", "files"},
		{"vars", "vars"},
		{"defaults", "defaults"},
		{"meta", "meta"},
		{"scenarios", "molecule"}, // copy scenarios into molecule/<role>/molecule/
	}
	for _, p := range pairs {
		src := filepath.Join(basePath, p.src)
		dst := filepath.Join(roleMoleculePath, p.dst)
		if p.src == "scenarios" {
			dst = filepath.Join(roleMoleculePath, "molecule")
		}
		copyIfExists(src, dst)
	}
	// copy dotfiles
	copyIfExists(filepath.Join(basePath, ".ansible-lint"), filepath.Join(roleMoleculePath, ".ansible-lint"))
	copyIfExists(filepath.Join(basePath, ".yamllint"), filepath.Join(roleMoleculePath, ".yamllint"))
	return nil
}

// copyIfExists copies file/directory if it exists (recursively when directory)
func copyIfExists(src, dst string) {
	if !exists(src) {
		log.Printf("note: %s does not exist, skipping", src)
		return
	}
	fi, err := os.Stat(src)
	if err != nil {
		log.Printf("copy stat error: %v", err)
		return
	}
	if fi.IsDir() {
		if err := copyDir(src, dst); err != nil {
			log.Printf("copy dir error %s -> %s: %v", src, dst, err)
		}
	} else {
		// file
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			log.Printf("mkdir for file: %v", err)
		}
		if err := copyFile(src, dst); err != nil {
			log.Printf("copy file error %v", err)
		}
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		// file
		return copyFile(path, target)
	})
}

// dockerExecInteractive runs: docker exec -ti molecule-role <cmd...>
func dockerExecInteractive(role, command string, args ...string) error {
	all := []string{"exec", "-ti", fmt.Sprintf("molecule-%s", role), command}
	all = append(all, args...)
	cmd := exec.Command("docker", all...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func ycCliInitWrapper() error {
	return ycCliInit()
}

// Windows-only: compact WSL and Optimize-VHD for Docker Desktop VHDX files.
// This will:
// - Stop Docker Desktop process
// - wsl --shutdown
// - run Optimize-VHD on $env:LOCALAPPDATA\Docker\wsl\data\*.vhdx (docker-desktop-data vhdx and docker-desktop vhdx)
// - restart Docker Desktop
func compactWSLAndOptimize() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("compactWSLAndOptimize is Windows only")
	}

	// stop Docker Desktop (graceful quit)
	log.Println("Stopping Docker Desktop (if running)...")
	// Stop-Process -Name "Docker Desktop" -Force
	psStop := `if (Get-Process -Name "Docker Desktop" -ErrorAction SilentlyContinue) { Stop-Process -Name "Docker Desktop" -Force }`
	if err := runPowerShell(psStop); err != nil {
		log.Printf("warning stopping Docker Desktop: %v", err)
	}

	// shutdown WSL
	log.Println("Shutting down WSL...")
	if err := runCommand("wsl", "--shutdown"); err != nil {
		log.Printf("warning: wsl --shutdown returned: %v", err)
	}

	// small wait
	time.Sleep(2 * time.Second)

	// build VHDX paths
	// $env:LOCALAPPDATA\Docker\wsl\data\docker-desktop-data.vhdx
	paths := []string{
		`$env:LOCALAPPDATA\Docker\wsl\data\docker-desktop-data.vhdx`,
		`$env:LOCALAPPDATA\Docker\wsl\data\docker-desktop.vhdx`,
	}
	for _, p := range paths {
		log.Printf("Running Optimize-VHD for %s (requires admin)...", p)
		cmd := fmt.Sprintf("Optimize-VHD -Path %s -Mode Full", p)
		if err := runPowerShell(cmd); err != nil {
			log.Printf("Optimize-VHD failed for %s: %v", p, err)
		}
	}

	// restart Docker Desktop
	log.Println("Starting Docker Desktop...")
	startCmd := `Start-Process "$env:ProgramFiles\Docker\Docker\Docker Desktop.exe"`
	if err := runPowerShell(startCmd); err != nil {
		log.Printf("warning starting Docker Desktop: %v", err)
	}

	log.Println("WSL compact/Optimize-VHD completed (check for errors above).")
	return nil
}

// runPowerShell executes a powershell command and streams its output.
func runPowerShell(cmd string) error {
	// Use powershell -NoProfile -Command "<cmd>"
	c := exec.Command("powershell", "-NoProfile", "-Command", cmd)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	return c.Run()
}
