package main

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
	"gopkg.in/yaml.v3"
)

var (
	RoleFlag        string
	OrgFlag         string
	TagFlag         string
	VerifyFlag      bool
	LintFlag        bool
	IdempotenceFlag bool
	WipeFlag        bool
	CompactWSLFlag  bool
)

func main() {

	rootCmd := &cobra.Command{
		Use:   "diffusion",
		Short: "Molecule workflow helper (cross-platform) with Windows-only WSL compact features",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Ensure some env defaults and prompt when needed

			reader := bufio.NewReader(os.Stdin)

			config, err := LoadConfig() // ignore error for now
			if err != nil {
				log.Printf("\033[33mwarning loading config: %v\033[0m", err)
				log.Printf("\033[38;2;127;255;212mNew config file will be created...\033[0m")
				YamlLintRulesDefault := &YamlLintRules{
					Braces:   map[string]any{"max-spaces-inside": 1, "level": "warning"},
					Brackets: map[string]any{"max-spaces-inside": 1, "level": "warning"},
					NewLines: map[string]any{"type": "platform"},
				}
				YamlLintDefault := &YamlLint{
					Extends: "default",
					Ignore:  []string{".git/*", "molecule/**", "vars/*", "files/*", ".yamllint", ".ansible-lint"},
					Rules:   YamlLintRulesDefault,
				}

				AnsibleLintDefault := &AnsibleLint{
					ExcludedPaths: []string{"molecule/default/tests/*.yml", "molecule/default/tests/*/*/*.yml", "tests/test.yml"},
					WarnList:      []string{"meta-no-info", "yaml[line-length]"},
					SkipList:      []string{"meta-incorrect", "role-name[path]"},
				}

				fmt.Print("Enter RegistryServer: ")
				registryServer, _ := reader.ReadString('\n')
				registryServer = strings.TrimSpace(registryServer)

				fmt.Print("Enter RegistryProvider: ")
				registryProvider, _ := reader.ReadString('\n')
				registryProvider = strings.TrimSpace(registryProvider)

				if registryProvider != "YC" && registryProvider != "AWS" && registryProvider != "GCP" && registryProvider != "Public" {
					fmt.Fprintln(os.Stderr, "\033[31mInvalid RegistryProvider. Allowed values are: YC, AWS, GCP. \nIf you're using public registry, then choose Public - or choose it, if you want to authenticate externally.\033[0m")
					os.Exit(1)
				}

				fmt.Print("Enter MoleculeContainerName: ")
				moleculeContainerName, _ := reader.ReadString('\n')
				moleculeContainerName = strings.TrimSpace(moleculeContainerName)

				fmt.Print("Enter MoleculeContainerTag: ")
				moleculeContainerTag, _ := reader.ReadString('\n')
				moleculeContainerTag = strings.TrimSpace(moleculeContainerTag)

				ContainerRegistry := &ContainerRegistry{
					RegistryServer:        registryServer,
					RegistryProvider:      registryProvider,
					MoleculeContainerName: moleculeContainerName,
					MoleculeContainerTag:  moleculeContainerTag,
				}

				fmt.Print("Enable Vault Integration? (Y/n): ")
				vaultEnabledStr, _ := reader.ReadString('\n')
				vaultEnabledStr = strings.TrimSpace(vaultEnabledStr)
				if vaultEnabledStr == "" {
					vaultEnabledStr = "n"
				}
				vaultEnabled := strings.ToLower(vaultEnabledStr) == "y"

				HashicorpVaultSet := VaultConfigHelper(vaultEnabled)

				config = &Config{
					ContainerRegistry: ContainerRegistry,
					HashicorpVault:    HashicorpVaultSet,
					ArtifactUrl:       "https://example.com/repo",
					YamlLintConfig:    YamlLintDefault,
					AnsibleLintConfig: AnsibleLintDefault,
				}

				if err := SaveConfig(config); err != nil {
					log.Printf("\033[33mwarning saving new config: %v\033[0m", err)
				}

			}

			if err := os.Setenv("GIT_URL", config.ArtifactUrl); err != nil {
				log.Printf("Failed to set GIT_URL: %v", err)
			}
		},
	}

	// molecule command
	molCmd := &cobra.Command{
		Use:   "molecule",
		Short: "run molecule workflow (create/converge/verify/lint/idempotence/wipe)",
		RunE:  runMolecule,
	}

	molCmd.Flags().StringVarP(&RoleFlag, "role", "r", "sdl_collector", "role name")
	molCmd.Flags().StringVarP(&OrgFlag, "org", "o", "linru", "organization prefix")
	molCmd.Flags().StringVarP(&TagFlag, "tag", "t", "", "ANSIBLE_RUN_TAGS value (optional)")
	molCmd.Flags().BoolVar(&VerifyFlag, "verify", false, "run molecule verify")
	molCmd.Flags().BoolVar(&LintFlag, "lint", false, "run linting (yamllint / ansible-lint)")
	molCmd.Flags().BoolVar(&IdempotenceFlag, "idempotence", false, "run molecule idempotence")
	molCmd.Flags().BoolVar(&WipeFlag, "wipe", false, "remove container and molecule role folder")

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
	compactCmd.Flags().BoolVar(&CompactWSLFlag, "confirm", false, "confirm running Optimize-VHD (requires admin)")
	rootCmd.AddCommand(compactCmd)

	// Provide a top-level flag to run compact before molecule (Windows-only)
	rootCmd.PersistentFlags().BoolVar(&CompactWSLFlag, "compact-wsl", false, "on Windows: compact Docker Desktop WSL2 vhdx (runs before molecule actions)")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func VaultConfigHelper(intergration bool) *HashicorpVault {
	reader := bufio.NewReader(os.Stdin)

	if !intergration {
		return &HashicorpVault{
			HashicorpVaultIntegration: false,
		}
	}
	fmt.Print("Enter SecretKV2Path (e.g., secret/data/diffusion): ")
	secretKV2Path, _ := reader.ReadString('\n')
	secretKV2Path = strings.TrimSpace(secretKV2Path)

	fmt.Print("Enter Git Username Field in Vault (default: git_username): ")
	gitUsernameField, _ := reader.ReadString('\n')
	gitUsernameField = strings.TrimSpace(gitUsernameField)

	fmt.Print("Enter Git Token Field in Vault (default: git_token): ")
	gitTokenField, _ := reader.ReadString('\n')
	gitTokenField = strings.TrimSpace(gitTokenField)

	HashicorpVaultSet := &HashicorpVault{
		HashicorpVaultIntegration: true,
		SecretKV2Path:             secretKV2Path,
		UserNameField:             gitUsernameField,
		TokenField:                gitTokenField,
	}

	return HashicorpVaultSet
}
func PromptInput(prompt string) string {
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

// runCommandHide runs command and discards stdout/stderr
func runCommandHide(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

// runCommandCapture returns stdout (trimmed) and error
func runCommandCapture(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// ycCliInit runs yc commands and sets env variables YC_TOKEN, YC_CLOUD_ID, YC_FOLDER_ID
func YcCliInit() error {
	// yc iam create-token
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	token, err := runCommandCapture(ctx, "yc", "iam", "create-token")
	if err != nil {
		return fmt.Errorf("yc iam create-token failed: %v (%s)", err, token)
	}
	_ = os.Setenv("TOKEN", token)

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

	config, err := LoadConfig()
	if err != nil {
		log.Printf("\033[33mwarning loading config: %v\033[0m", err)
	}

	// prepare path
	path, err := os.Getwd()
	if err != nil {
		return err
	}
	// use forward slashes in mounts where required (docker on windows expects Windows paths but we keep raw)
	// Compose role path
	roleDirName := fmt.Sprintf("%s.%s", OrgFlag, RoleFlag)
	roleMoleculePath := filepath.Join(path, "molecule", roleDirName)

	// handle wipe
	if WipeFlag {
		log.Printf("\033[38;2;127;255;212mWiping: removing container molecule-%s and folder %s\n\033[0m", RoleFlag, roleMoleculePath)
		_ = runCommand("docker", "rm", fmt.Sprintf("molecule-%s", RoleFlag), "-f")
		if err := os.RemoveAll(roleMoleculePath); err != nil {
			log.Printf("\033[33mwarning: failed remove role path: %v\033[0m", err)
		}
		return nil
	}

	// handle lint/verify/idempotence by ensuring files are copied and running docker exec commands
	if LintFlag || VerifyFlag || IdempotenceFlag {
		if err := copyRoleData(path, roleMoleculePath); err != nil {
			log.Printf("\033[33mwarning copying data: %v\033[0m", err)
		}
		// ensure tests dir exists for verify/lint
		defaultTestsDir := filepath.Join(roleMoleculePath, "molecule", "default", "tests")
		if err := os.MkdirAll(defaultTestsDir, 0o755); err != nil {
			log.Printf("\033[33mwarning: cannot create tests dir: %v\033[0m", err)
		}
		if LintFlag {
			// run yamllint and ansible-lint inside container
			cmdStr := fmt.Sprintf(`cd ./%s && yamllint . -c .yamllint && ansible-lint -c .ansible-lint `, roleDirName)
			if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", cmdStr); err != nil {
				log.Printf("\033[31mLint failed: %v\033[0m", err)
				os.Exit(1)
			}
			log.Printf("\033[32mLint Done Successfully!\033[0m")
			return nil
		}
		if VerifyFlag {
			// copy tests/*
			testsSrc := filepath.Join(path, "tests")
			copyIfExists(testsSrc, defaultTestsDir)
			cmdStr := fmt.Sprintf("cd ./%s && molecule verify", roleDirName)
			if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", cmdStr); err != nil {
				log.Printf("\033[31mVerify failed: %v\033[0m", err)
				os.Exit(1)
			}
			log.Printf("\033[32mVerify Done Successfully!\033[0m")
			return nil
		}
		if IdempotenceFlag {
			tagEnv := ""
			if TagFlag != "" {
				tagEnv = fmt.Sprintf("ANSIBLE_RUN_TAGS=%s ", TagFlag)
			}
			cmdStr := fmt.Sprintf("cd ./%s && %smolecule idempotence", roleDirName, tagEnv)
			if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", cmdStr); err != nil {
				log.Printf("\033[31mIdempotence failed: %v\033[0m", err)
				os.Exit(1)
			}
			log.Printf("\033[32mIdempotence Done Successfully!\033[0m")
			return nil
		}
	}

	// default flow: create/run container if not exists, copy data, converge
	// check if container exists
	err = exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", RoleFlag)).Run()
	if err == nil {
		fmt.Printf("\033[38;2;127;255;212mContainer molecule-%s already exists. To purge use -wipe.\n\033[0m", RoleFlag)
	} else {

		if config.HashicorpVault.HashicorpVaultIntegration {

			git_raw := vault_client(context.Background(), config.HashicorpVault.SecretKV2Path, config.HashicorpVault.SecretKV2Name)

			gitUser := git_raw.Data.Data[config.HashicorpVault.UserNameField].(string)

			if err := os.Setenv("GIT_USER", gitUser); err != nil {
				log.Printf("Failed to set GIT_USER: %v", err)
			}

			gitToken := git_raw.Data.Data[config.HashicorpVault.TokenField].(string)

			if err := os.Setenv("GIT_PASSWORD", gitToken); err != nil {
				log.Printf("Failed to set GIT_PASSWORD: %v", err)
			}
		} else {
			log.Println("\033[35mHashiCorp Vault integration is disabled in config. Use public repositories.\033[0m")
		}

		// If user requested Windows compaction before running molecule
		if CompactWSLFlag && runtime.GOOS == "windows" {
			log.Println("Running Windows WSL compact prior to molecule (requested)...")
			if err := compactWSLAndOptimize(); err != nil {
				log.Printf("compact-wsl failed: %v", err)
			}
		}
		// create
		if err := YcCliInit(); err != nil {
			log.Printf("\033[32myc init warning: %v\033[0m", err)
		}

		if err := runCommandHide("docker", "login", config.ContainerRegistry.RegistryServer, "--username", "iam", "--password", os.Getenv("TOKEN")); err != nil {
			log.Printf("\033[33mdocker login to registry failed: %v\033[0m", err)
		}
		// run container
		// docker run --rm -d --name=molecule-$role -v "$path/molecule:/opt/molecule" -v /sys/fs/cgroup:/sys/fs/cgroup:rw -e ... --privileged --pull always cr.yandex/...
		image := fmt.Sprintf("%s/%s:%s", config.ContainerRegistry.RegistryServer, config.ContainerRegistry.MoleculeContainerName, config.ContainerRegistry.MoleculeContainerTag)
		args := []string{
			"run", "--rm", "-d", "--name=" + fmt.Sprintf("molecule-%s", RoleFlag),
			"-v", fmt.Sprintf("%s/molecule:/opt/molecule", path),
			"-v", "/sys/fs/cgroup:/sys/fs/cgroup:rw",
			"-e", "TOKEN=" + os.Getenv("TOKEN"),
			"-e", "VAULT_TOKEN=" + os.Getenv("VAULT_TOKEN"),
			"-e", "VAULT_ADDR=" + os.Getenv("VAULT_ADDR"),
			"-e", "GIT_USER=" + os.Getenv("GIT_USER"),
			"-e", "GIT_PASSWORD=" + os.Getenv("GIT_PASSWORD"),
			"-e", "GIT_URL=" + os.Getenv("GIT_URL"),
			"--cgroupns", "host",
			"--privileged", "--pull", "always",
			image,
		}
		if err := runCommand("docker", args...); err != nil {
			log.Printf("\033[33mdocker run failed: %v\033[0m", err)
		}
	}

	// ensure role exists
	if exists(roleMoleculePath) {
		fmt.Println("\033[35mThis role already exists in molecule\033[0m")
	} else {
		// docker exec -ti molecule-$role /bin/sh -c "ansible-galaxy role init $org.$role"
		if err := dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("ansible-galaxy role init %s.%s", OrgFlag, RoleFlag)); err != nil {
			log.Printf("\033[33mrole init warning: %v\033[0m", err)
		}
	}

	// docker exec login to cr.yandex inside container
	_ = dockerExecInteractiveHide(RoleFlag, "/bin/sh", "-c", `echo $TOKEN | docker login cr.yandex --username iam --password-stdin`)

	// copy files into molecule structure
	if err := copyRoleData(path, roleMoleculePath); err != nil {
		log.Printf("\033[33mcopy role data warning: %v\033[0m", err)
	}

	// finally create/converge
	err = exec.Command("docker", "inspect", fmt.Sprintf("molecule-%s", RoleFlag)).Run()
	if err == nil {
		// container exists
		_ = dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("cd ./%s && molecule converge", roleDirName))
	} else {
		_ = dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("cd ./%s && molecule create", roleDirName))
		_ = dockerExecInteractive(RoleFlag, "/bin/sh", "-c", fmt.Sprintf("cd ./%s && molecule converge", roleDirName))
	} // copy dotfiles

	return nil
}

// copyRoleData copies tasks, handlers, templates, files, vars, defaults, meta, scenarios, .ansible-lint, .yamllint
func copyRoleData(basePath, roleMoleculePath string) error {
	config, err := LoadConfig()
	if err != nil {
		log.Printf("\033[33mwarning loading config: %v\033[0m", err)
	}
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

	yamlrules := YamlLintRulesExport{
		Braces:   config.YamlLintConfig.Rules.Braces,
		Brackets: config.YamlLintConfig.Rules.Brackets,
		NewLines: config.YamlLintConfig.Rules.NewLines,
	}

	exportYamlLint := YamlLintExport{
		Extends: config.YamlLintConfig.Extends,
		Ignore:  strings.Join(config.YamlLintConfig.Ignore, "\n"),
		Rules:   &yamlrules,
	}
	yamllint, err := yaml.Marshal(exportYamlLint)
	if err != nil {
		log.Printf("\033[33mwarning marshaling yamllint config: %v\033[0m", err)
	} else {
		yamllintPath := filepath.Join(roleMoleculePath, ".yamllint")
		if err := os.WriteFile(yamllintPath, yamllint, 0o644); err != nil {
			log.Printf("\033[33mwarning writing .yamllint: %v\033[0m", err)
		}
	}

	exportAnsibleLint := AnsibleLintExport{
		ExcludedPaths: config.AnsibleLintConfig.ExcludedPaths,
		WarnList:      config.AnsibleLintConfig.WarnList,
		SkipList:      config.AnsibleLintConfig.SkipList,
	}

	ansiblelint, err := yaml.Marshal(exportAnsibleLint)
	if err != nil {
		log.Printf("\033[33mwarning marshaling ansible-lint config: %v\033[0m", err)
	} else {
		ansiblelintPath := filepath.Join(roleMoleculePath, ".ansible-lint")
		if err := os.WriteFile(ansiblelintPath, ansiblelint, 0o644); err != nil {
			log.Printf("\033[33mwarning writing .ansible-lint: %v\033[0m", err)
		}
	}

	return nil
}

// copyIfExists copies file/directory if it exists (recursively when directory)
func copyIfExists(src, dst string) {
	if !exists(src) {
		log.Printf("\033[38;2;127;255;212mnote: %s does not exist, skipping\033[0m", src)
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
	defer func() {
		if cerr := in.Close(); cerr != nil {
			log.Printf("Failed to close source file: %v", cerr)
		}
	}()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := out.Close(); cerr != nil {
			log.Printf("Failed to close destination file: %v", cerr)
		}
	}()
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

// dockerExecInteractiveHide runs: docker exec -ti molecule-role <cmd...>
func dockerExecInteractiveHide(role, command string, args ...string) error {
	all := []string{"exec", "-ti", fmt.Sprintf("molecule-%s", role), command}
	all = append(all, args...)
	cmd := exec.Command("docker", all...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Stdin = os.Stdin
	return cmd.Run()
}
func YcCliInitWrapper() error {
	return YcCliInit()
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
		log.Printf("\033[33mwarning stopping Docker Desktop: %v\033[0m", err)
	}

	// shutdown WSL
	log.Println("Shutting down WSL...")
	if err := runCommand("wsl", "--shutdown"); err != nil {
		log.Printf("\033[33mwarning: wsl --shutdown returned: %v\033[0m", err)
	}

	// small wait
	time.Sleep(2 * time.Second)

	// build VHDX paths
	// $env:LOCALAPPDATA\Docker\wsl\data\docker-desktop-data.vhdx
	paths := []string{
		`$env:LOCALAPPDATA\Docker\wsl\disk\docker_data.vhdx`,
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
		log.Printf("\033[33mwarning starting Docker Desktop: %v\033[0m", err)
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
