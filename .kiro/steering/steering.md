---
inclusion: always
---

# Diffusion — Technical Steering Document

## Project Overview

Diffusion is a cross-platform CLI written in Go that streamlines Ansible role testing with Molecule. It provides containerized testing, container registry authentication (YC, AWS ECR, GCP, Public), HashiCorp Vault integration for credentials, built-in YAML/Ansible linting, dependency locking, and caching — all in a single binary.

- **Module**: `diffusion`
- **Language**: Go 1.25.4+
- **CLI framework**: [spf13/cobra](https://github.com/spf13/cobra)
- **Config format**: TOML (`diffusion.toml`), YAML (`meta/main.yml`, `requirements.yml`)
- **Lock file**: `diffusion.lock`
- **License**: MIT

## Development Rule — dev-new-features

All agent changes, new feature implementation, refactoring, and experimental work MUST be done inside the `dev-new-features/` directory. This directory mirrors the main project structure and acts as the active development branch within the repo.

```
dev-new-features/
├── cmd/diffusion/         # CLI entry point
├── internal/              # All internal packages (same layout as root)
│   ├── cache/
│   ├── cli/
│   ├── config/
│   ├── dependency/
│   ├── galaxy/
│   ├── molecule/
│   ├── registry/
│   ├── role/
│   ├── secrets/
│   └── utils/
├── tests/e2e/
├── docs/
├── Makefile
├── go.mod / go.sum
└── README.md
```

**Do NOT modify files in the root-level `cmd/`, `internal/`, or other source directories directly.** Always work in `dev-new-features/` unless explicitly told otherwise.

## Main CLI Commands

Entry point: `cmd/diffusion/main.go` → calls `internal/cli.Execute()`

| Command | Description |
|---|---|
| `diffusion molecule` | Run Molecule workflows (converge, verify, lint, idempotence, destroy, wipe) |
| `diffusion role` | Manage Ansible role config, init new roles, add/remove roles and collections |
| `diffusion deps` | Dependency management — init, lock, check, resolve, sync |
| `diffusion cache` | Caching control — enable, disable, clean, status, list |
| `diffusion artifact` | Private artifact repository credentials — add, list, remove, show |
| `diffusion show` | Display full diffusion configuration |

## CLI Flags Reference

### Global

| Flag | Description |
|---|---|
| `--version` | Print version, Go version, OS/Arch |

### `diffusion molecule`

| Flag | Short | Default | Description |
|---|---|---|---|
| `--role` | `-r` | — | Role name |
| `--org` | `-o` | — | Organization prefix |
| `--tag` | `-t` | — | Ansible tags (comma-separated) |
| `--converge` | — | `false` | Run molecule converge |
| `--verify` | — | `false` | Run molecule verify |
| `--testsoverwrite` | — | `false` | Overwrite molecule tests folder |
| `--lint` | — | `false` | Run yamllint + ansible-lint |
| `--idempotence` | — | `false` | Run molecule idempotence |
| `--destroy` | — | `false` | Run molecule destroy |
| `--wipe` | — | `false` | Remove container and molecule role folder |
| `--ci` | — | `false` | CI/CD mode (non-interactive) |
| `--oidc` | — | `false` | Use OIDC token from environment |
| `--force` | — | `false` | Force reinstall of roles/collections |

### `diffusion role`

| Flag | Short | Default | Description |
|---|---|---|---|
| `--init` | `-i` | `false` | Initialize a new Ansible role via ansible-galaxy |
| `--scenario` | `-s` | `default` | Molecule scenario folder to use |

#### `diffusion role add-role [name]`

| Flag | Short | Default | Description |
|---|---|---|---|
| `--scenario` | `-s` | `default` | Molecule scenario folder |
| `--src` | — | — | Source URL of the role |
| `--scm` | — | `git` | SCM type (e.g., git) |
| `--version` | `-v` | `main` | Version constraint |
| `--namespace` | `-n` | — | Galaxy namespace (required for Galaxy roles) |

#### `diffusion role remove-role [name]`

| Flag | Short | Default | Description |
|---|---|---|---|
| `--scenario` | `-s` | `default` | Molecule scenario folder |
| `--namespace` | `-n` | — | Galaxy namespace |

### `diffusion deps` subcommands

| Subcommand | Description |
|---|---|
| `init` | Initialize dependency config in `diffusion.toml`, scan existing `requirements.yml` |
| `lock` | Generate or update `diffusion.lock` |
| `check` | Verify lock file is up-to-date with YAML manifests |
| `resolve` | Display all dependencies with resolved versions |
| `sync` | Restore versions from lock file to `requirements.yml` and `meta.yml` |

### `diffusion cache` subcommands

| Subcommand | Description |
|---|---|
| `enable` | Enable Ansible cache (supports `--docker`, `--uv` flags) |
| `disable` | Disable cache |
| `clean` | Remove cached data |
| `status` | Show cache status |
| `list` | List cached items |

### `diffusion artifact` subcommands

| Subcommand | Description |
|---|---|
| `add [name]` | Add credentials for a private artifact source (supports Vault or local encrypted) |
| `list` | List configured artifact sources |
| `remove [name]` | Remove an artifact source |
| `show [name]` | Show details of an artifact source |

## Internal Package Structure

| Package | Responsibility |
|---|---|
| `internal/cli` | Cobra command definitions, flag binding, CLI entry point |
| `internal/config` | `diffusion.toml` load/save, defaults, validation |
| `internal/molecule` | Molecule workflow execution (converge, lint, verify, idempotence, destroy, wipe) |
| `internal/role` | Ansible role management — parse/save `meta/main.yml` and `requirements.yml` |
| `internal/dependency` | Dependency resolution, lock file generation (`diffusion.lock`) |
| `internal/registry` | Container registry auth (YC, AWS ECR, GCP, OIDC, Public) |
| `internal/secrets` | Credential encryption, HashiCorp Vault client integration |
| `internal/cache` | Role/collection/Docker/Python package caching |
| `internal/galaxy` | Ansible Galaxy API integration, version resolution |
| `internal/utils` | Shared utility functions |

## Key Dependencies

| Dependency | Purpose |
|---|---|
| `github.com/spf13/cobra` | CLI framework |
| `github.com/BurntSushi/toml` | TOML config parsing |
| `github.com/hashicorp/vault-client-go` | HashiCorp Vault integration |
| `gopkg.in/yaml.v3` | YAML parsing |

## Build & Test

```bash
make build          # Build for current platform
make test           # Run unit tests
make dist           # Cross-compile for all platforms
make clean          # Remove build artifacts
make version        # Show current version
```

Cross-compilation targets: `linux-amd64`, `linux-arm64`, `linux-arm`, `darwin-amd64`, `darwin-arm64`, `windows-amd64`, `windows-arm64`, `windows-arm`.

Version is injected at build time via `-ldflags` from git tags (semver).
