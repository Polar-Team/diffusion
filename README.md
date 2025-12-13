<div align="center">

![Diffusion Logo](img/logo.jpg)

# Diffusion

**A powerful Go-based CLI framework for simplifying Ansible role testing with Molecule**

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
- 💻 **Windows WSL Support**: Special features for Windows users including WSL compaction and VHD optimization
- 🎯 **Interactive Configuration**: User-friendly prompts for project setup

## 🛠️ Prerequisites

Before using Diffusion, ensure you have the following tools installed:

- **Docker**: For containerized testing environments
- **Go 1.25.4+**: For building from source (if needed)
- **Vault CLI**: (Optional) For HashiCorp Vault integration
- **YC CLI**: (Optional) For Yandex Cloud registry authentication
- **WSL2**: (Windows only) For WSL-based features

## 📥 Installation

### From Source

```bash
git clone https://github.com/Polar-Team/diffusion.git
cd diffusion
go build -o diffusion
```

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

### 3.  Run Molecule Tests

```bash
# Run convergence test
diffusion molecule --role my-role --org my-org

# Run with verification
diffusion molecule --role my-role --org my-org --verify

# Run linting
diffusion molecule --role my-role --org my-org --lint

# Run idempotence test
diffusion molecule --role my-role --org my-org --idempotence

# Run with specific tags
diffusion molecule --role my-role --org my-org --tag "my-tag"
```

## 📖 Commands

### `diffusion role`
Manage Ansible role configurations interactively.

```bash
# View current role configuration (requires existing role)
diffusion role

# Initialize a new role
diffusion role --init

# Add a role dependency
diffusion role add-role my-dependency --src https://github.com/user/role.git --version main

# Remove a role dependency
diffusion role remove-role my-dependency

# Add a collection
diffusion role add-collection community.general

# Remove a collection
diffusion role remove-collection community.general
```

**Note:** The `role` command without `--init` flag will display the current role configuration. If no role exists, it will show an error message. Use `diffusion role --init` to initialize a new role. If a role already exists in the current directory, the `--init` flag will warn you.

### `diffusion molecule`
Run Molecule testing workflows.

**Flags:**
- `--role, -r`: Role name (auto-detected from meta/main.yml)
- `--org, -o`: Organization/namespace prefix (auto-detected)
- `--tag, -t`: Ansible run tags (optional)
- `--verify`: Run molecule verify tests
- `--lint`: Run yamllint and ansible-lint
- `--idempotence`: Run molecule idempotence tests
- `--wipe`: Remove container and molecule role folder

### `diffusion show`
Display all Diffusion configuration in a readable format.

```bash
diffusion show
```

### `diffusion compact-wsl` (Windows Only)
Optimize Docker Desktop WSL2 VHDX files. 

```bash
diffusion compact-wsl --confirm
```

## ⚙️ Configuration

Diffusion uses a `diffusion. toml` file for configuration:

```toml
[container_registry]
registry_server = "ghcr.io"  # Default: ghcr.io
registry_provider = "Public"  # Options: YC, AWS, GCP, Public
molecule_container_name = "polar-team/diffusion-molecule-container"
molecule_container_tag = "latest-amd64"  # Auto-detected: latest-amd64 or latest-arm64

[vault]
enabled = true
secret_kv2_path = "secret/data/diffusion"
secret_kv2_name = "git-credentials"
username_field = "git_username"
token_field = "git_token"

url = "https://your-artifact-repo.com"

[yaml_lint]
extends = "default"
ignore = [". git/*", "molecule/**", "vars/*"]

[ansible_lint]
exclude_paths = ["molecule/default/tests/*. yml"]
warn_list = ["meta-no-info", "yaml[line-length]"]
skip_list = ["meta-incorrect", "role-name[path]"]
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
- **Yandex Cloud (YC)**: Automatic authentication with YC CLI
- **AWS ECR**: Support for AWS container registries
- **GCP Artifact Registry**: Google Cloud registry support
- **Public Registries**: Docker Hub and other public registries

### Windows-Specific Features
- WSL2 shutdown and compaction
- Docker Desktop VHDX optimization
- Automatic Docker Desktop restart

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

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
