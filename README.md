<div align="center">

![Diffusion Logo](img/logo.jpg)

# Diffusion

**A powerful Go-based CLI framework for simplifying Ansible role testing with Molecule**

[![Release](https://github.com/Polar-Team/diffusion/actions/workflows/release.yml/badge.svg)](https://github.com/Polar-Team/diffusion/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Polar-Team/diffusion)](https://github.com/Polar-Team/diffusion)

---

> *"To every action there is always opposed an equal reaction; or the mutual actions of two bodies upon each other are always equal, and directed to contrary parts."*  
> — Isaac Newton

We kept this in mind when creating Diffusion, to make Molecule testing more fun 🎉

</div>

---

## 📋 Overview

**Diffusion** is a cross-platform command-line tool written in Go that streamlines the workflow for testing Ansible roles using Molecule. It provides an integrated environment for role development, testing, and validation with built-in support for container registries, HashiCorp Vault integration, and linting tools.

## ✨ Key Features

- 🚀 **Ansible Role Management**: Initialize, configure, and manage Ansible roles with ease
- 🐳 **Docker-Based Testing**: Automated Molecule workflow with containerized testing environments
- 🔐 **HashiCorp Vault Integration**: Secure credential management for private repositories
- 📦 **Multiple Registry Support**: Works with Yandex Cloud (YC), AWS, GCP, and public registries
- 🔍 **Built-in Linting**: Integrated YAML and Ansible linting with customizable rules
- ✅ **Comprehensive Testing**: Support for convergence, verification, idempotence, and lint testing
- 🎯 **Interactive Configuration**: User-friendly prompts for project setup

## 🛠️ Prerequisites

Before using Diffusion, ensure you have the following tools installed:

- **Docker**: For containerized testing environments
- **Go 1.25.4+**: For building from source (if needed)
- **Vault CLI**: (Optional) For HashiCorp Vault integration
- **YC CLI**: (Optional) For Yandex Cloud registry authentication
- **AWS CLI**: (Optional) For AWS ECR registry authentication
- **gcloud CLI**: (Optional) For GCP Artifact Registry/GCR authentication

### 💡 Recommended Terminal Setup

For the best experience with Diffusion's colored output and Unicode symbols:

**Terminals:**
- [WezTerm](https://wezfurlong.org/wezterm/) - GPU-accelerated, cross-platform terminal with excellent Unicode support
- [Ghostty](https://ghostty.org/) - Fast, native terminal emulator with modern features

**Fonts:**
- [Nerd Fonts](https://www.nerdfonts.com/) - Patched fonts with icons and symbols
  - Recommended: FiraCode Nerd Font, JetBrains Mono Nerd Font, or Hack Nerd Font

These tools provide proper rendering of Diffusion's colored output, progress indicators, and status symbols.

## 📥 Installation

### From Source

```bash
git clone https://github.com/Polar-Team/diffusion.git
cd diffusion
make build
```

The binary will be in the `bin/` directory. See [Building Guide](docs/building.md) for more options including cross-compilation.

### Build for All Platforms

```bash
make dist
```

This creates binaries for Linux, macOS, and Windows (AMD64, ARM64, ARM). See [Building Guide](docs/building.md) for details.

### Using Go Install

```bash
go install github. com/Polar-Team/diffusion@latest
```

## 🚀 Quick Start

### 1. Initialize a New Role

```bash
diffusion role --init
```

This will guide you through creating a new Ansible role with the proper structure. 

### 2. Configure Diffusion

On first run, Diffusion will prompt you to configure:
- Container registry settings (default: `ghcr.io`)
- Molecule container details (default: `polar-team/diffusion-molecule-container:latest-{arch}`)
- HashiCorp Vault integration (optional)
- Linting rules

Configuration is stored in `diffusion.toml` in your project directory.

**Default Container Registry:**
- Registry Server: `ghcr.io`
- Registry Provider: `Public`
- Container Name: `polar-team/diffusion-molecule-container`
- Container Tag: `latest-amd64` or `latest-arm64` (auto-detected based on your system architecture)

### 3. Run Molecule Tests

Diffusion automatically detects role name and namespace from `meta/main.yml`:

```bash
# Run convergence test (auto-detects role and org from meta/main.yml)
diffusion molecule

# Run with verification
diffusion molecule --verify

# Run verification with specific tags
diffusion molecule --verify --tag "check-config"

# Run linting
diffusion molecule --lint

# Run idempotence test
diffusion molecule --idempotence

# Run converge with specific Ansible tags
diffusion molecule --converge --tag "install,configure"

# Run idempotence with specific tags
diffusion molecule --idempotence --tag "my-tag"

# Destroy test instances
diffusion molecule --destroy

# Clean up (remove container and molecule folder)
diffusion molecule --wipe
```

**Notes:**
- Role name and namespace are auto-detected from `meta/main.yml`
- The `--tag` flag works with `--converge`, `--verify`, and `--idempotence` commands
- If `meta/main.yml` is not found, use `--role` and `--org` flags to override:

```bash
diffusion molecule --role my-role --org my-org --verify
```

## 📖 Commands

### `diffusion cache`
Manage Ansible role and collection caching for faster builds.

```bash
# Enable cache for current role
diffusion cache enable

# Disable cache
diffusion cache disable

# Clean cache
diffusion cache clean

# Show cache status
diffusion cache status
```

**Benefits**: Caches downloaded roles and collections between runs, significantly speeding up repeated molecule tests. See [Cache Feature Documentation](docs/cache-feature.md) for details.

### `diffusion artifact`
Manage private artifact repository credentials with encrypted storage.

```bash
# Add credentials for a private repository
diffusion artifact add my-private-repo

# List all stored artifact sources
diffusion artifact list

# Show details for a source (token masked)
diffusion artifact show my-private-repo

# Remove stored credentials
diffusion artifact remove my-private-repo
```

**Security**: Credentials are encrypted using AES-256-GCM with a machine-specific key derived from hostname + username. Stored in `~/.diffusion/secrets/<role-name>/<source-name>` with 0700 directory permissions.

See [Artifact Management Documentation](docs/artifact-management.md) for detailed documentation.

### `diffusion role`
Manage Ansible role configurations interactively.

```bash
# View current role configuration (requires existing role)
diffusion role

# Initialize a new role
diffusion role --init

# Add a role dependency (default scenario)
diffusion role add-role my-dependency --src https://github.com/user/role.git --version main

# Add a role dependency to specific scenario
diffusion role add-role my-dependency --src https://github.com/user/role.git --scenario custom

# Remove a role dependency
diffusion role remove-role my-dependency

# Remove a role dependency from specific scenario
diffusion role remove-role my-dependency --scenario custom

# Add a collection
diffusion role add-collection community.general

# Add a collection to specific scenario
diffusion role add-collection community.general --scenario custom

# Remove a collection
diffusion role remove-collection community.general
```

**Scenario Flag:**
- Use `--scenario, -s` flag to manage dependencies in different Molecule scenarios
- Default scenario is `default` (molecule/default/)
- Allows managing separate requirements.yml files for different test scenarios

**Note:** The `role` command without `--init` flag will display the current role configuration. If no role exists, it will show an error message. Use `diffusion role --init` to initialize a new role. If a role already exists in the current directory, the `--init` flag will warn you.

### `diffusion molecule`
Run Molecule testing workflows.

**Flags:**
- `--role, -r`: Role name (auto-detected from meta/main.yml)
- `--org, -o`: Organization/namespace prefix (auto-detected from meta/main.yml)
- `--tag, -t`: Ansible run tags (comma-separated, works with --converge, --verify, and --idempotence)
- `--converge`: Run molecule converge (default behavior if no test flags specified)
- `--verify`: Run molecule verify tests
- `--lint`: Run yamllint and ansible-lint
- `--idempotence`: Run molecule idempotence tests
- `--destroy`: Run molecule destroy to clean up test instances
- `--testsoverwrite`: Overwrite molecule tests folder for remote or diffusion type
- `--wipe`: Remove container and molecule role folder
- `--ci`: CI/CD mode (non-interactive, skip TTY and permission fixes)

**CI/CD Mode:**

Use `--ci` flag in CI/CD pipelines to avoid TTY and permission errors:
- Removes `-ti` flags from docker exec (fixes "input device is not a TTY" errors)
- Skips permission fixes that fail in containerized environments
- Disables spinner animations for cleaner logs

```yaml
# GitHub Actions example
- name: Run Molecule tests
  run: diffusion molecule --ci --converge

# GitLab CI example
script:
  - diffusion molecule --ci --verify
```

**Note:** Test flags (`--converge`, `--verify`, `--lint`, `--idempotence`, `--destroy`) are mutually exclusive - only one can be used at a time.

**Examples:**
```bash
# Run converge (default)
diffusion molecule

# Run converge with specific tags
diffusion molecule --converge --tag "install,configure"

# Run in CI/CD mode
diffusion molecule --ci --converge

# Run verification tests
diffusion molecule --verify

# Run verification with specific tags
diffusion molecule --verify --tag "check-config"

# Run linting
diffusion molecule --lint

# Run idempotence test
diffusion molecule --idempotence

# Run idempotence with tags
diffusion molecule --idempotence --tag "install"

# Destroy test instances
diffusion molecule --destroy

# Override auto-detected role/org
diffusion molecule --role custom-role --org custom-org --verify

# Clean up after testing
diffusion molecule --wipe
```

**Typical workflow:**
```bash
# 1. Run converge to apply the role
diffusion molecule --converge

# 2. Run verification tests
diffusion molecule --verify

# 3. Run linting
diffusion molecule --lint

# 4. Test idempotence
diffusion molecule --idempotence

# 5. Destroy test instances
diffusion molecule --destroy

# 6. Clean up container and files
diffusion molecule --wipe
```

# 4. Test idempotence
diffusion molecule --idempotence

# 5. Clean up
diffusion molecule --wipe
```

**Testing Resources:**
- [diffusion-ansible-tests-role](https://github.com/Polar-Team/diffusion-ansible-tests-role) - Comprehensive testing role for verify.yml automation. Validates Docker containers, network ports, shell commands, HTTP endpoints, and PostgreSQL databases in your Molecule tests.
- [diffusion-molecule-container](https://github.com/Polar-Team/diffusion-molecule-container) - Official Docker container for Diffusion with Molecule, Ansible, and all required testing tools pre-installed. Use this as a base to create your own custom Diffusion container with additional tools or configurations.

### `diffusion show`
Display all Diffusion configuration in a readable format.

```bash
diffusion show
```

## ⚙️ Configuration

Diffusion uses a `diffusion.toml` file for configuration. The file is automatically created on first run with interactive prompts.

### Example 1: Public Registry with Local Artifact Storage

```toml
# Container Registry Settings
[container_registry]
registry_server = "ghcr.io"
registry_provider = "Public"
molecule_container_name = "polar-team/diffusion-molecule-container"
molecule_container_tag = "latest-amd64"

# HashiCorp Vault Integration
[vault]
enabled = false

# Artifact Sources (Private Repositories)
[[artifact_sources]]
name = "my-gitlab"
url = "https://gitlab.example.com"
use_vault = false

[[artifact_sources]]
name = "my-github"
url = "https://github.example.com"
use_vault = false

# YAML Linting Configuration
[yaml_lint]
extends = "default"
ignore = [".git/*", "molecule/**", "vars/*", "files/*", ".yamllint", ".ansible-lint"]

[yaml_lint.rules]
braces = { max-spaces-inside = 1, level = "warning" }
brackets = { max-spaces-inside = 1, level = "warning" }
comments = { min-spaces-from-content = 1 }
comments-indentation = false
octal-values = { forbid-implicit-octal = true }

[yaml_lint.rules.new-lines]
type = "platform"

# Ansible Linting Configuration
[ansible_lint]
exclude_paths = [
  "molecule/default/tests/*.yml",
  "molecule/default/tests/*/*/*.yml",
  "tests/test.yml"
]
warn_list = ["meta-no-info", "yaml[line-length]"]
skip_list = ["meta-incorrect", "role-name[path]"]

# Tests Configuration
[tests]
type = "local"

# Cache Configuration
[cache]
enabled = false
```

### Example 2: Yandex Cloud Registry with Vault Integration

```toml
# Container Registry Settings
[container_registry]
registry_server = "cr.yandex"
registry_provider = "YC"
molecule_container_name = "crp1234567890/diffusion-molecule-container"
molecule_container_tag = "latest-amd64"

# HashiCorp Vault Integration
[vault]
enabled = true

# Artifact Sources (Private Repositories with Vault)
[[artifact_sources]]
name = "gitlab-private"
url = "https://gitlab.company.com"
use_vault = true
vault_path = "secret/data/artifacts"
vault_secret_name = "gitlab-creds"
vault_username_field = "username"
vault_token_field = "token"

[[artifact_sources]]
name = "github-enterprise"
url = "https://github.company.com"
use_vault = true
vault_path = "secret/data/artifacts"
vault_secret_name = "github-creds"
vault_username_field = "username"
vault_token_field = "token"

# YAML Linting Configuration
[yaml_lint]
extends = "default"
ignore = [".git/*", "molecule/**", "vars/*", "files/*", ".yamllint", ".ansible-lint"]

[yaml_lint.rules]
braces = { max-spaces-inside = 1, level = "warning" }
brackets = { max-spaces-inside = 1, level = "warning" }
comments = { min-spaces-from-content = 1 }
comments-indentation = false
octal-values = { forbid-implicit-octal = true }

[yaml_lint.rules.new-lines]
type = "platform"

# Ansible Linting Configuration
[ansible_lint]
exclude_paths = [
  "molecule/default/tests/*.yml",
  "molecule/default/tests/*/*/*.yml",
  "tests/test.yml"
]
warn_list = ["meta-no-info", "yaml[line-length]"]
skip_list = ["meta-incorrect", "role-name[path]"]

# Tests Configuration
[tests]
type = "diffusion"

# Cache Configuration
[cache]
enabled = true
```

### Example 3: GCP Artifact Registry

```toml
# Container Registry Settings
[container_registry]
registry_server = "us-docker.pkg.dev"
registry_provider = "GCP"
molecule_container_name = "my-project/my-repo/diffusion-molecule-container"
molecule_container_tag = "latest-amd64"

# HashiCorp Vault Integration (optional)
[vault]
enabled = false

# Artifact Sources (optional)
[[artifact_sources]]
name = "github-private"
url = "https://github.company.com"
use_vault = false

# YAML Linting Configuration
[yaml_lint]
extends = "default"
ignore = [".git/*", "molecule/**", "vars/*", "files/*", ".yamllint", ".ansible-lint"]

[yaml_lint.rules]
braces = { max-spaces-inside = 1, level = "warning" }
brackets = { max-spaces-inside = 1, level = "warning" }
comments = { min-spaces-from-content = 1 }
comments-indentation = false
octal-values = { forbid-implicit-octal = true }

[yaml_lint.rules.new-lines]
type = "platform"

# Ansible Linting Configuration
[ansible_lint]
exclude_paths = [
  "molecule/default/tests/*.yml",
  "molecule/default/tests/*/*/*.yml",
  "tests/test.yml"
]
warn_list = ["meta-no-info", "yaml[line-length]"]
skip_list = ["meta-incorrect", "role-name[path]"]

# Tests Configuration
[tests]
type = "local"

# Cache Configuration
[cache]
enabled = false
```

### Configuration Options

**Container Registry:**
- `registry_server`: Container registry URL
  - Public: `ghcr.io`, `docker.io`
  - Yandex Cloud: `cr.yandex`
  - AWS: `<account-id>.dkr.ecr.<region>.amazonaws.com`
  - GCP: `gcr.io`, `<region>-docker.pkg.dev`
- `registry_provider`: Registry type - `YC`, `AWS`, `GCP`, or `Public`
- `molecule_container_name`: Container image name (with registry path for private registries)
- `molecule_container_tag`: Container tag (auto-detected: `latest-amd64` or `latest-arm64`)

**Passing Credentials to Test Containers:**

When using non-public registries or Vault integration, you can pass environment variables to test containers inside the Diffusion container via `molecule.yml`:

```yaml
# molecule/default/molecule.yml
platforms:
  - name: instance
    image: your-registry/test-image:latest
    env:
      TOKEN: "${TOKEN}"              # Pass registry token (YC, AWS, GCP, etc.)
      VAULT_ADDR: "${VAULT_ADDR}"    # Pass Vault address
      VAULT_TOKEN: "${VAULT_TOKEN}"  # Pass Vault token
```

**Note:** Environment variables are passed if they exist in your environment, regardless of whether you're using Public or Private registry configuration. This allows flexible credential management:
- `TOKEN`: Automatically passed for private registry authentication
- `VAULT_ADDR` and `VAULT_TOKEN`: Passed when available, enabling Vault access in test containers even with Public registries

This allows test containers to:
- Pull images from private registries (YC, AWS, GCP)
- Access HashiCorp Vault for secrets
- Authenticate with cloud services during tests

**Vault Integration:**
- `enabled`: Enable HashiCorp Vault for credential management
- Requires `VAULT_ADDR` and `VAULT_TOKEN` environment variables
- Can be passed to test containers via molecule.yml (see above)

**Artifact Sources:**
- Multiple private repositories can be configured
- `use_vault = false`: Credentials stored encrypted locally in `~/.diffusion/secrets/`
- `use_vault = true`: Credentials retrieved from HashiCorp Vault
- See [Artifact Management](docs/artifact-management.md) for details

**Tests:**

Diffusion supports three test types for Molecule verify stage:

1. **`local`** - Use tests from local `tests/` directory
   - Tests are copied from your project's `tests/` folder
   - Best for project-specific tests
   - No external dependencies

2. **`remote`** - Clone test roles from Git repositories
   - Clones test roles to `molecule/default/tests/`
   - Supports multiple test repositories
   - Can use artifact sources for private repositories
   - Add `remote_repositories` array with Git URLs
   - Example:
     ```toml
     [tests]
     type = "remote"
     remote_repositories = [
       "https://github.com/org/test-role1.git",
       "https://github.com/org/test-role2.git"
     ]
     ```

3. **`diffusion`** - Use the official Diffusion testing role
   - Automatically clones [diffusion-ansible-tests-role](https://github.com/Polar-Team/diffusion-ansible-tests-role)
   - Comprehensive testing framework for common scenarios
   - Validates Docker containers, ports, shell commands, HTTP endpoints, PostgreSQL
   - DRY approach with reusable test definitions
   - Best for standardized testing across multiple roles

**Cache:**
- `enabled = true`: Cache Ansible roles and collections between runs
- `cache_id` is auto-generated when cache is enabled
- See [Cache Feature](docs/cache-feature.md) for details

### Managing Configuration

```bash
# View current configuration
diffusion show

# Edit configuration file
# Linux/macOS
nvim diffusion.toml

# Windows
nvim diffusion.toml
```

## 📁 Project Structure

When you initialize a role, Diffusion creates:

```
role-name/
├── defaults/
├── files/
├── handlers/
├── meta/
│   └── main.yml          # Role metadata
├── tasks/
├── templates/
├── vars/
├── scenarios/
│   └── default/
│       ├── converge.yml   # Convergence playbook
│       ├── verify.yml     # Verification tests
│       ├── molecule.yml   # Molecule configuration
│       └── requirements.yml  # Role dependencies
└── . gitignore
```

## 🔐 HashiCorp Vault Integration

Diffusion can integrate with HashiCorp Vault to securely manage credentials:

1. Enable Vault integration during configuration
2. Configure the KV2 secret path and field names
3. Set `VAULT_ADDR` and `VAULT_TOKEN` environment variables
4. Diffusion will automatically fetch credentials when needed

## 🎨 Features in Detail

### Automated Role Testing
- **Create**: Spin up Docker containers for testing
- **Converge**: Apply your role to test instances
- **Verify**: Run custom verification tests
- **Idempotence**: Ensure your role is idempotent
- **Lint**: Validate YAML and Ansible best practices

### Registry Support

Diffusion provides automatic authentication for multiple container registries:

#### Yandex Cloud (YC)
- **Authentication**: Automatic with YC CLI
- **Command**: `yc iam create-token`
- **Registry Format**: `cr.yandex`
- **Setup**: Install [YC CLI](https://cloud.yandex.com/docs/cli/quickstart) and configure with `yc init`

#### AWS ECR (Elastic Container Registry)
- **Authentication**: Automatic with AWS CLI
- **Command**: `aws ecr get-login-password`
- **Registry Format**: `<account-id>.dkr.ecr.<region>.amazonaws.com`
- **Setup**: Install [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) and configure with `aws configure`

#### GCP (Google Cloud Platform)
- **Authentication**: Automatic with gcloud CLI
- **Command**: `gcloud auth print-access-token`
- **Registry Formats**: 
  - Container Registry: `gcr.io`, `us.gcr.io`, `eu.gcr.io`, `asia.gcr.io`
  - Artifact Registry: `<region>-docker.pkg.dev` (e.g., `us-docker.pkg.dev`, `europe-west1-docker.pkg.dev`)
- **Setup**: 
  1. Install [gcloud CLI](https://cloud.google.com/sdk/docs/install)
  2. Authenticate: `gcloud auth login`
  3. Set project: `gcloud config set project PROJECT_ID`
  4. (Optional) Configure Docker: `gcloud auth configure-docker` or `gcloud auth configure-docker <region>-docker.pkg.dev`

**Configuration Example for GCP Artifact Registry:**
```toml
[container_registry]
registry_server = "us-docker.pkg.dev"
registry_provider = "GCP"
molecule_container_name = "my-project/my-repo/diffusion-molecule-container"
molecule_container_tag = "latest-amd64"
```

**Configuration Example for GCP Container Registry:**
```toml
[container_registry]
registry_server = "gcr.io"
registry_provider = "GCP"
molecule_container_name = "my-project/diffusion-molecule-container"
molecule_container_tag = "latest-amd64"
```

#### Public Registries
- **Authentication**: None required
- **Registries**: Docker Hub (`docker.io`), GitHub Container Registry (`ghcr.io`), etc.
- **Setup**: No CLI installation needed

**Note**: When using GCP, AWS, or YC registries, ensure the respective CLI tool is installed, authenticated, and configured before running Diffusion. The authentication tokens are automatically retrieved and used for Docker login.

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📚 Documentation

- **[Dependency Management](dev-new-features/docs/DEPENDENCY_MANAGEMENT.md)** - Complete guide to Python, tool, and collection dependency management
- **[Building from Source](docs/building.md)** - Complete build guide with cross-compilation
- **[Verification Guide](docs/verification.md)** - Verify binary signatures and SLSA provenance
- **[Cache Feature](docs/cache-feature.md)** - Ansible role and collection caching for faster builds
- **[Artifact Management](docs/artifact-management.md)** - Managing private repository credentials
- **[Unix Permissions](docs/unix-permissions.md)** - How Diffusion handles permissions on Unix systems
- **[Migration Guide](docs/migration-guide.md)** - Upgrading from older versions
- **[Changelog](dev-new-features/docs/CHANGELOG.md)** - Version history and changes
- **[E2E Testing](tests/e2e/README.md)** - End-to-end testing with Vagrant

### Technical Archives
Historical documentation about implementation changes:
- [Archive](docs/archive/) - Technical implementation notes and refactoring docs

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details. 

## 🏢 Organization

Maintained by [Polar-Team](https://github.com/Polar-Team)

## 📞 Support

For issues, questions, or contributions, please visit the [GitHub Issues](https://github.com/Polar-Team/diffusion/issues) page.

---

<div align="center">
Made with ❤️ by Polar-Team
</div>
