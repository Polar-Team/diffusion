package cli

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"diffusion/internal/config"
	"diffusion/internal/role"
	"diffusion/internal/secrets"
	"diffusion/internal/utils"
)

func AnsibleGalaxyInit() (string, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter role name: ")
	roleName, _ := reader.ReadString('\n')
	roleName = strings.TrimSpace(roleName)

	if roleName == "" {
		return "", fmt.Errorf("role name cannot be empty")
	}

	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	args := []string{
		"run",
	}

	// Add user mapping for Unix systems to avoid permission issues
	args = append(args, utils.GetUserMappingArgs()...)

	// Set HOME environment variable for ansible-galaxy config
	// containerHome := utils.GetContainerHomePath()

	args = append(args,
		// "-e", fmt.Sprintf("HOME=%s", containerHome),
		"-v", fmt.Sprintf("%s:/ansible", currentDir),
		"-w", "/ansible",
		fmt.Sprintf("ghcr.io/polar-team/diffusion-molecule-container:%s", utils.GetDefaultMoleculeTag()),
		"ansible-galaxy", "role", "init", roleName,
	)

	fmt.Printf("Initializing Ansible role: %s\n", roleName)

	err = utils.RunCommandHide("docker", args...)
	if err != nil {
		fmt.Printf("\033[31mInitializing of new role were failed: %v\033[0m", err)
	}
	// Create scenarios/default directory structure
	scenariosPath := filepath.Join(currentDir, roleName, "scenarios", "default")
	if err := os.MkdirAll(scenariosPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create scenarios directory: %w", err)
	}

	converge_content := `# Converge playbook
---
- name: Converge
  hosts: all
  tasks:
    - name: "Include Ansible role"
      ansible.builtin.include_role:
          name: "{{ lookup('env', 'MOLECULE_PROJECT_DIRECTORY') | basename }}"
      tags:
#        - YOUR_TAGS
`
	verify_content := `# Verify playbook
---
- name: Verify
  hosts: all
  gather_facts: false
# vars:
#    system_name: YOUR_PLATFORM_NAME
#    docker_user_uid: 1000
#  roles:
#    - role: tests/diffusion_tests
#      vars:
#        port_test: true
#        port: YOUR_PORT_NUMBER
`

	molecule_content := `# Molecule default scenario configuration
---
dependency:
  name: galaxy
  options:
    requirements-file: requirements.yml
driver:
  name: docker
# platforms:
#  - name: YOUR_PLATFORM_NAME
#    image: YOUR_TESTING_IMAGE_URL
#    privileged: true
#    pre_build_image: true
#    env:
#      YC_TOKEN: ${TOKEN}
#      VAULT_ADDR: ${VAULT_ADDR}
#      VAULT_TOKEN: ${VAULT_TOKEN}
#    command: /lib/systemd/systemd
#    cgroupns_mode: host
#    tmpfs:
#      - /tmp
#      - /run
#      - /run/lock
#    volumes:
#      - /etc/ssl/certs:/etc/ssl/certs
#      - /sys/fs/cgroup:/sys/fs/cgroup:rw
#      - dockerroot:/var/lib/docker:rw
provisioner:
   name: ansible
verifier:
   name: ansible
`
	// Create converge.yml
	convergePath := filepath.Join(currentDir, roleName, "scenarios", "default", "converge.yml")
	if err := os.WriteFile(convergePath, []byte(converge_content), 0644); err != nil {
		return "", fmt.Errorf("failed to create converge.yml: %w", err)
	}

	// Create molecule.yml
	moleculePath := filepath.Join(currentDir, roleName, "scenarios", "default", "molecule.yml")
	if err := os.WriteFile(moleculePath, []byte(molecule_content), 0644); err != nil {
		return "", fmt.Errorf("failed to create converge.yml: %w", err)
	}
	// Create verify.yml
	verifyPath := filepath.Join(currentDir, roleName, "scenarios", "default", "verify.yml")
	if err := os.WriteFile(verifyPath, []byte(verify_content), 0644); err != nil {
		return "", fmt.Errorf("failed to create verify.yml: %w", err)
	}

	// Create .gitignore file
	gitignoreContent := `**/molecule/*
**/roles/*
vars/secrets.yml
`
	gitignorePath := filepath.Join(currentDir, roleName, ".gitignore")
	if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
		return "", fmt.Errorf("failed to create .gitignore: %w", err)
	}

	fmt.Printf("Created .gitignore in %s\n", roleName)
	return roleName, nil
}

func MetaConfigSetup(roleName string) *role.Meta {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("What namespace of the role should be?: ")
	roleNamespace, _ := reader.ReadString('\n')
	roleNamespace = strings.TrimSpace(roleNamespace)

	fmt.Print("What company of the role should be?: ")
	roleCompany, _ := reader.ReadString('\n')
	roleCompany = strings.TrimSpace(roleCompany)

	fmt.Print("What author of the role should be?: ")
	roleAuthor, _ := reader.ReadString('\n')
	roleAuthor = strings.TrimSpace(roleAuthor)

	fmt.Print("Description of the role (optional): ")
	roleDescription, _ := reader.ReadString('\n')
	roleDescription = strings.TrimSpace(roleDescription)

	platformsList := []role.Platform{}
	fmt.Print("Enter platforms? (y/n): ")
	addPlatforms, _ := reader.ReadString('\n')
	addPlatforms = strings.TrimSpace(strings.ToLower(addPlatforms))

	for addPlatforms == "y" {
		fmt.Print("Platform OS name (e.g., ubuntu, centos): ")
		osName, _ := reader.ReadString('\n')
		osName = strings.TrimSpace(osName)

		if osName != "" {
			fmt.Print("Platform versions (comma-separated, e.g., 20.04,22.04): ")
			versionsInput, _ := reader.ReadString('\n')
			versionsInput = strings.TrimSpace(versionsInput)
			if versionsInput != "" {
				// Check if this OS already exists in the list
				existingIndex := -1
				for i, platform := range platformsList {
					if platform.OsName == osName {
						existingIndex = i
						break
					}
				}

				// Collect versions for this OS
				versions := []string{}

				// Add new versions
				for version := range strings.SplitSeq(versionsInput, ",") {
					version = strings.TrimSpace(version)
					if version != "" {
						versions = append(versions, version)
					}
				}

				if existingIndex != -1 {
					platformsList[existingIndex].Versions = versions
				} else {
					platformsList = append(platformsList, role.Platform{
						OsName:   osName,
						Versions: versions,
					})
				}
			}
		}

		fmt.Print("Add another platform? (y/n): ")
		addPlatforms, _ = reader.ReadString('\n')
		addPlatforms = strings.TrimSpace(strings.ToLower(addPlatforms))
	}

	fmt.Print("Galaxy Tags (comma-separated) (optional): ")
	galaxyTagsInput, _ := reader.ReadString('\n')
	galaxyTagsInput = strings.TrimSpace(galaxyTagsInput)
	galaxyTagsList := []string{}
	if galaxyTagsInput != "" {
		for t := range strings.SplitSeq(galaxyTagsInput, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				galaxyTagsList = append(galaxyTagsList, t)
			}
		}
	}

	fmt.Print("Collections required (comma-separated) (optional): ")
	collectionsInput, _ := reader.ReadString('\n')
	collectionsInput = strings.TrimSpace(collectionsInput)
	collectionsList := []string{}
	if collectionsInput != "" {
		for c := range strings.SplitSeq(collectionsInput, ",") {
			c = strings.TrimSpace(c)
			if c != "" {
				collectionsList = append(collectionsList, c)
			}
		}
	}

	roleSettings := &role.Meta{
		GalaxyInfo: &role.GalaxyInfo{
			RoleName:          roleName,
			Namespace:         roleNamespace,
			Company:           roleCompany,
			Author:            roleAuthor,
			Description:       roleDescription,
			License:           "MIT",
			MinAnsibleVersion: "2.10",
			Platforms:         platformsList,
			GalaxyTags:        galaxyTagsList,
		},
		Collections: collectionsList,
	}

	return roleSettings

}
func RequirementConfigSetup(collections []string) *role.Requirement {
	if collections == nil {
		collections = []string{}
	}
	reader := bufio.NewReader(os.Stdin)

	// Convert string collections to structured format
	collectionsList := []role.RequirementCollection{}
	for _, col := range collections {
		name, version := utils.ParseCollectionString(col)
		collectionsList = append(collectionsList, role.RequirementCollection{Name: name, Version: version})
	}

	rolesList := []role.RequirementRole{}
	fmt.Print("Enter roles to add? (y/n): ")
	addRoles, _ := reader.ReadString('\n')
	addRoles = strings.TrimSpace(strings.ToLower(addRoles))
	for addRoles == "y" {
		fmt.Print("Role name (e.g., geerlingguy.nginx): ")
		roleName, _ := reader.ReadString('\n')
		roleName = strings.TrimSpace(roleName)
		if roleName != "" {
			fmt.Print("Role source (e.g., git URL) (required): ")
			roleSrc, _ := reader.ReadString('\n')
			roleSrc = strings.TrimSpace(roleSrc)
			fmt.Print("Role SCM (default: git) (optional): ")
			roleScm, _ := reader.ReadString('\n')
			roleScm = strings.TrimSpace(roleScm)
			if roleScm == "" {
				roleScm = "git"
			}
			fmt.Print("Role version (default: main) (optional): ")
			roleVersion, _ := reader.ReadString('\n')
			roleVersion = strings.TrimSpace(roleVersion)
			if roleVersion == "" {
				roleVersion = "main"
			}
			newRole := role.RequirementRole{
				Name:    roleName,
				Src:     roleSrc,
				Scm:     roleScm,
				Version: roleVersion,
			}
			rolesList = append(rolesList, newRole)
		}
		fmt.Print("Add another role? (y/n): ")
		addRoles, _ = reader.ReadString('\n')
		addRoles = strings.TrimSpace(strings.ToLower(addRoles))
	}

	requirementSettings := &role.Requirement{
		Collections: collectionsList,
		Roles:       rolesList,
	}

	return requirementSettings

}

func VaultConfigHelper(integration bool) *config.HashicorpVault {
	return &config.HashicorpVault{
		HashicorpVaultIntegration: integration,
	}
}

func ArtifactSourcesHelper() []config.ArtifactSource {
	reader := bufio.NewReader(os.Stdin)
	var sources []config.ArtifactSource

	fmt.Print("Configure artifact sources for private repositories? (y/N): ")
	configureStr, _ := reader.ReadString('\n')
	configureStr = strings.TrimSpace(strings.ToLower(configureStr))

	if configureStr != "y" {
		fmt.Println("Skipping artifact source configuration. You can add sources later with 'diffusion artifact add'")
		return sources
	}

	for {
		fmt.Print("\nEnter artifact source name (or press Enter to finish): ")
		name, _ := reader.ReadString('\n')
		name = strings.TrimSpace(name)

		if name == "" {
			break
		}

		fmt.Printf("Enter URL for %s: ", name)
		url, _ := reader.ReadString('\n')
		url = strings.TrimSpace(url)

		fmt.Print("Store credentials in Vault? (y/N): ")
		useVaultStr, _ := reader.ReadString('\n')
		useVaultStr = strings.TrimSpace(strings.ToLower(useVaultStr))
		useVault := useVaultStr == "y"

		source := config.ArtifactSource{
			Name:     name,
			URL:      url,
			UseVault: useVault,
		}

		if useVault {
			fmt.Printf("Enter Vault path for %s (e.g., secret/data/artifacts): ", name)
			vaultPath, _ := reader.ReadString('\n')
			source.VaultPath = strings.TrimSpace(vaultPath)

			fmt.Printf("Enter Vault secret name for %s: ", name)
			vaultSecret, _ := reader.ReadString('\n')
			source.VaultSecretName = strings.TrimSpace(vaultSecret)

			fmt.Print("Enter Username Field in Vault (default: username): ")
			usernameField, _ := reader.ReadString('\n')
			usernameField = strings.TrimSpace(usernameField)
			if usernameField == "" {
				usernameField = "username"
			}
			source.VaultUsernameField = usernameField

			fmt.Print("Enter Token Field in Vault (default: token): ")
			tokenField, _ := reader.ReadString('\n')
			tokenField = strings.TrimSpace(tokenField)
			if tokenField == "" {
				tokenField = "token"
			}
			source.VaultTokenField = tokenField

			fmt.Printf("Credentials for '%s' will be retrieved from Vault at %s/%s (fields: %s, %s)\n",
				name, source.VaultPath, source.VaultSecretName, source.VaultUsernameField, source.VaultTokenField)
		} else {
			// Prompt for credentials and save locally
			fmt.Printf("Enter Username for %s: ", name)
			username, _ := reader.ReadString('\n')
			username = strings.TrimSpace(username)

			fmt.Printf("Enter Token/Password for %s: ", name)
			token, _ := reader.ReadString('\n')
			token = strings.TrimSpace(token)

			// Save credentials locally
			creds := &config.ArtifactCredentials{
				Name:     name,
				URL:      url,
				Username: username,
				Token:    token,
			}

			if err := secrets.SaveArtifactCredentials(creds); err != nil {
				fmt.Printf("\033[33mWarning: failed to save credentials for '%s': %v\033[0m\n", name, err)
			} else {
				fmt.Printf("\033[32mCredentials for '%s' saved locally (encrypted)\033[0m\n", name)
			}
		}

		sources = append(sources, source)
		fmt.Printf("\033[32mAdded artifact source: %s\033[0m\n", name)

		fmt.Print("Add another artifact source? (y/N): ")
		addAnother, _ := reader.ReadString('\n')
		addAnother = strings.TrimSpace(strings.ToLower(addAnother))
		if addAnother != "y" {
			break
		}
	}

	return sources
}

func TestsConfigSetup() *config.TestsSettings {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("What type of configuration you want use? remote / local / diffusion(Default): ")
	configType, _ := reader.ReadString('\n')
	configType = strings.TrimSpace(strings.ToLower(configType))

	if configType == "" {
		configType = "diffusion"
	}
	if configType != "remote" && configType != "local" && configType != "diffusion" {
		fmt.Fprintln(os.Stderr, "\033[31mInvalid configuration type. Allowed values are: remote, local, diffusion.\033[0m")
		os.Exit(1)
	}

	remoteReposList := []string{}
	if configType == "remote" {
		fmt.Print("Enter remote repository URLs seperated by comma, it should be public or from artifact URL path (if remote selected): ")
		remoteReposInput, _ := reader.ReadString('\n')
		remoteReposInput = strings.TrimSpace(remoteReposInput)
		if remoteReposInput != "" {
			for r := range strings.SplitSeq(remoteReposInput, ",") {
				r = strings.TrimSpace(r)
				if r != "" {
					remoteReposList = append(remoteReposList, r)
				}
			}
		}
	}
	testsSettings := &config.TestsSettings{
		Type:               configType,
		RemoteRepositories: remoteReposList,
	}

	return testsSettings
}
