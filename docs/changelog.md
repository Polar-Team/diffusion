# Changelog

All notable changes to the Diffusion project will be documented in this file.

## [Unreleased]

### Added
- **`diffusion docs` Command**: Auto-generate role variable documentation in README.md
  - Scans `defaults/main.yml`, `vars/main.yml`, `templates/`, and `tasks/` for variables
  - Annotation markers: `#—|` (type), `#—?` (description), `#—!` (required), `#—&` (optional)
  - Supports types: string, int, bool, list, map, float, dict, path
  - Output placed between `<!-- begin role_variables -->` / `<!-- end role_variables -->` markers
  - Flags: `--path` / `-p` (role directory), `--dry-run` (preview without writing)

- **MCP Server**: Model Context Protocol server for AI assistant integration
  - 16 tools for container management, validation, troubleshooting, and CLI reference
  - Built with FastMCP (Python 3.11+), runs via `uv` or Docker container
  - Container image: `ghcr.io/polar-team/diffusion-mcp-server` (multi-arch: amd64, arm64)
  - Tools include: `get_diffusion_config`, `list_molecule_containers`, `inspect_molecule_container`, `docker_exec_in_molecule`, `check_molecule_yml`, `check_verify_yml`, `troubleshoot_molecule_container`, `run_diffusion_command`, and more
  - Supports Kiro, Claude Desktop, and any MCP-compatible client
  - CI/CD workflow: build per-arch → push to GHCR → create multi-arch manifest → smoke test

- **`diffusion deploy` Enhancements**:
  - `--ssh-key` flag: Pass SSH private keys as base64 (per-host or wildcard `*=<base64>`)
  - `--host-wait-max-attempts` flag: Maximum probe attempts (default: 20)
  - `--cache` flag: Enable deploy cache for fetched lock files (default: true)
  - `--cache-path` flag: Custom cache directory (default: `~/.diffusion/deploy-cache/`)
  - `--ci` flag: CI/CD mode with machine-readable output
  - Updated host-wait defaults: interval `15s` (was `5s`), timeout `10m` (was `5m`)

- **Terraform Provider — `diffusion_deploy` Resource**:
  - New `ssh_private_keys` attribute: Map of named SSH private keys in PEM format (auto base64-encoded)
  - Provider `artifact_source` block: Configure private Galaxy/git credentials with optional Vault integration
  - Updated provider defaults: `host_wait_interval` = `15s`, `host_wait_timeout` = `10m`

- **Terraform Provider — `diffusion_inventory` Data Source** (redesigned):
  - Now renders inventory from provided `hosts`, `groups`, and `variables` inputs
  - Returns `rendered` attribute with valid Ansible YAML inventory
  - No longer requires an existing `diffusion_deploy` resource — standalone usage

- **Makefile Targets**:
  - `make build-provider` — Build Terraform provider for current platform
  - `make dist-provider` — Build Terraform provider for all platforms
  - `make dist-all` — Build both diffusion CLI and provider for all platforms

### Changed
- **Go Version**: Upgraded to Go 1.25.4
- **Terraform Provider Registry**: Source updated to `Polar-Team/diffusion`
- **Molecule Container**: Docker DinD base updated to `29.5.3-dind-alpine3.23` (from `29.4.0`)
- **Molecule Container**: uv package manager updated to `0.11.19` (from `0.9.30`)
- **Molecule Container**: Alpine packages updated (git 2.52.0, curl 8.19.0, openssl 3.5.7, gcc 15.2.0)

## [0.8.3] - 2026-08-02

### Changed
- **Deploy SSH Key Handling**: Extract SSH key sanitization and file naming into separate functions for improved maintainability

## [0.8.2] - 2026-08-02

### Added
- **Deploy Fallback SSH Key**: Add fallback SSH key support for unmapped hosts — if a host has no explicit key mapping, the wildcard (`*`) key is used automatically

## [0.8.1] - 2026-08-02

### Fixed
- **Docs Module**: Added `inventory_dir`, `playbook_dir`, and `role_name` to built-in Ansible variable exclusions in docs generation

## [0.8.0] - 2026-08-02

### Added
- **Deploy Caching**: Add caching support for fetched roles and collections with CI mode output
- **Deploy Host Wait Limit**: Add `--host-wait-max-attempts` flag to cap probe attempts
- **Deploy Test Suite**: Comprehensive test suite for the deploy package

### Fixed
- **Deploy Container Paths**: Use fixed container paths and improve SSH key handling
- **Deploy Inventory**: Pass inventory to container via base64-encoded environment variable instead of file mounting

## [0.7.10] - 2026-08-01

### Changed
- **Deploy Inventory**: Pass inventory to container via base64-encoded env var for reliability

## [0.7.9] - 2026-08-01

### Added
- **Deploy SSH Keys**: Support per-host SSH key injection via base64 encoding

## [0.7.8] - 2026-08-01

### Added
- **Deploy SSH Key Base64**: Add base64-encoded SSH key support for dynamic key injection into containers

## [0.7.7] - 2026-07-19

### Changed
- **Agent Configuration**: Externalize Kiro agent prompts to dedicated markdown files for better maintainability

## [0.7.6] - 2026-07-19

### Changed
- **Deploy Refactoring**: Extract Docker container lifecycle management and modernize type hints

## [0.7.5] - 2026-07-19

### Added
- **Deploy Sessions**: Add session-based container naming and improve code patterns

## [0.7.4] - 2026-07-19

### Changed
- **Deploy Architecture**: Extract Docker container lifecycle management into separate module

## [0.7.3] - 2026-07-05

### Added
- **MCP Docs Command**: Expand docs command support to handle multiple YAML files in `defaults/` directory
- **MCP Docker-in-Docker**: Add `docker_in_docker_in_molecule` tool for executing commands in scenario containers

### Changed
- **Docs Module**: Distinguish between duplicate vars and cross-referenced defaults/vars files

## [0.7.2] - 2026-07-05

### Added
- **Terminal Detection**: Add terminal detection and interactive mode helpers to prevent TTY failures in non-interactive environments

## [0.7.1] - 2026-07-05

### Changed
- **MCP Container**: Update Dockerfile versions for MCP server container

## [0.7.0] - 2026-07-04

### Added
- **`diffusion docs` Command**: Generate role variable documentation in README.md
  - Scans `defaults/main.yml`, `vars/main.yml`, `templates/`, `tasks/` for variables
  - Annotation syntax: `#—|` (type), `#—?` (description), `#—!` (required), `#—&` (optional)
  - Output placed between `<!-- begin role_variables -->` / `<!-- end role_variables -->` markers
  - Supports: string, int, bool, list, map, float, dict, path types
  - Flags: `--path` / `-p`, `--dry-run`
  - Multiline default values displayed correctly
  - Comprehensive E2E test suite

- **Deploy Command & Terraform Provider**: Full deployment system
  - `diffusion deploy` command for deploying Ansible roles to remote hosts
  - Terraform provider `diffusion_deploy` resource and `diffusion_inventory` data source
  - Auto-generated playbooks from role sources
  - Host reachability probing before deployment
  - Skip period for idempotent re-deployments
  - SSH key support outside `~/.ssh` directory

### Fixed
- **Molecule Scenario Flag**: Set `ANSIBLE_REMOTE_TMP` and fix `--scenario` flag in molecule steps

## [0.6.5] - 2026-06-13

### Added
- **Terraform Provider Repository**: Added terraform-provider-diffusion as a submodule

## [0.6.4] - 2026-05-11

### Added
- **Molecule Scenario Flag**: Pass `-s` scenario flag to molecule `create`/`converge`/`verify`/`idempotence`/`destroy` commands
- **CLI Scenario Flag**: Add `--scenario` / `-s` flag to the `molecule` command

## [0.6.3] - 2026-05-11

### Added
- **MCP Server**: Initial MCP server with Docker containerization and CI/CD pipeline
  - Tools for configuration reading, container management, validation, and troubleshooting
  - Multi-arch Docker image (`ghcr.io/polar-team/diffusion-mcp-server`)
  - GitHub Actions workflow for build, push, and manifest creation
  - Automated Dockerfile version updates

### Fixed
- **Docker Login**: Fix typo in docker login command inside container

## [0.6.2] - 2026-05-11

### Fixed
- **CI/CD**: Fix GITHUB_OUTPUT format in update-mcp-dockerfile workflow

## [0.6.1] - 2026-05-10

### Added
- **MCP Docker CI**: Automated MCP Dockerfile version updates workflow

## [0.6.0] - 2026-05-02

### Added
- **Code Quality**: Comprehensive test coverage for CLI, cache, config, and molecule packages
- **Kiro Integration**: Agent configurations and steering documentation

### Fixed
- **Interactive Mode**: Add lowercase fix to subcommands in molecule.go for interactive role initialization
- **CI Mode**: Sync host dependency files into container during CI setup
- **CI Mode**: Lowercase namespace in `meta/main.yml` during CI setup
- **CI Mode**: Fix branch checkout for pull requests (use `GITHUB_HEAD_REF` instead of commit SHA)
- **CI Mode**: Copy scenarios contents instead of directory to prevent nested paths
- **CI Mode**: Skip spinner in RunCommandHide when CI mode is enabled
- **Yamllint**: Correct typo and expand yamllint rules support in config

### Changed
- **Molecule Force Flag**: Add `--force` flag to control ansible-galaxy reinstall behavior
- **Galaxy Requirements**: Install ansible-galaxy requirements before converge step
- **Diffusion-Update Action**: Add scenario input and remove cache inputs
- **AppArmor**: Disable unprivileged userns restriction in diffusion-test action

## [0.5.22] - 2026-05-02

### Fixed
- **Molecule**: Ensure molecule directory exists before container mount (OCI runtime error)
- **Molecule Paths**: Use relative paths instead of absolute `/opt/molecule` paths
- **Docker Exec**: Standardize docker exec working directory
- **Permissions**: Enable chown permissions and add working directory flag

## [0.5.21] - 2026-04-26

### Fixed
- **CI Verify**: Fix verify run path for DiffusionTests type

## [0.5.20] - 2026-04-26

### Added
- **OIDC Authentication**: Add OIDC token authentication support for container registries (`--oidc` flag)

## [0.5.19] - 2026-04-26

### Added
- **Registry Validation**: Add Yandex Container Registry URL format validation

## [0.5.18] - 2026-04-26

### Added
- **E2E Testing**: Expanded Linux e2e matrix (Ubuntu, Debian, Fedora, Alma Linux, ArchLinux)
- **E2E Testing**: Add role subcommand and deps smoke tests to e2e workflow

## [0.5.17] - 2026-04-26

### Changed
- **E2E Architecture**: Replace Vagrant approach with native Linux runners

## [0.5.16] - 2026-04-26

### Fixed
- **Deps Check**: Improve lock file validation and multi-scenario sync

## [0.5.15] - 2026-04-26

### Added
- **Supply Chain**: Add supply chain verification (Cosign + SLSA) to diffusion-test action
- **Docker/UV Cache**: Add DinD and UV cache support with CI docker-cp fallback

## [0.5.14] - 2026-04-26

### Fixed
- **CI/CD**: Fix version injection, DinD cache, verify tags, and Windows UV cache performance

## [0.5.13] - 2026-04-19

### Added
- **Chocolatey**: Add Cosign and SLSA verification to Chocolatey install script

## [0.5.12] - 2026-04-19

### Added
- **Chocolatey**: Add Chocolatey package configuration and publishing workflow
- **Checksum Validation**: Add checksum validation for all downloaded files

## [0.5.11] - 2026-04-19

### Fixed
- **Chocolatey**: Fix various Chocolatey packaging issues (nuspec, install scripts, checksums)

## [0.5.10] - 2026-04-19

### Added
- **GitHub Actions**: Add diffusion-test and diffusion-update composite actions for CI/CD

## [0.5.9] - 2026-04-05

### Fixed
- **Permissions**: Various Unix permission fixes for molecule directory on Linux

## [0.5.8] - 2026-04-04

### Changed
- **Documentation**: Migrate documentation to external HTML site, simplify README

## [0.5.7] - 2026-04-04

### Fixed
- **Linux Docker Exec OCI Error**: Fixed "OCI runtime exec failed: current working directory is outside of container mount namespace root" on Linux
  - Root cause: `molecule/` host directory not existing before `docker run`, causing the bind mount to fail silently and leaving `/opt/molecule` empty inside the container

## [0.5.6] - 2026-04-04

### Added
- **Dependency Management System**: Comprehensive dependency management for Python, Ansible tools, and collections
  - **Python Version Constraints**: Only allows tested versions (3.13, 3.12, 3.11)
  - **Automatic Version Resolution**: Queries PyPI and Galaxy to resolve actual versions from constraints
  - **Lock File System**: Generates `diffusion.lock` with reproducible dependency snapshots
  - **Dynamic pyproject.toml**: Generates and passes pyproject.toml to container via environment variable
  - **Version Compatibility**: Validates tool compatibility with Python versions and auto-adjusts if needed
  - **Commands**: `deps init`, `deps lock`, `deps resolve`, `deps check`
  - **Removed**: `deps sync` command (no longer needed with dynamic generation)

### Changed
- **Python Version Format**: All versions now use major.minor format only (e.g., `3.11`, `3.13`)
- **Python Version Source**: Container now uses Python version from `diffusion.lock` instead of hardcoded constant
- **Molecule Installation**: Now installs from PyPI instead of GitHub for faster and more reliable installation

## [0.5.5] - 2026-03-28

### Added
- **E2E Testing**: Interactive Vagrant-based manual UX testing for Linux

## [0.5.4] - 2026-03-09

### Fixed
- **Molecule CI**: Fix various molecule CI test issues (verify paths, tests type handling)

## [0.5.3] - 2026-03-01

### Fixed
- **Deps and Roles**: Fix dependency check, YAML indentation, namespace/scenario refactoring
- **Role Adding**: Fix version bug when adding roles

## [0.5.2] - 2026-03-01

### Changed
- **README**: Update README to accurately reflect current project state

## [0.5.1] - 2026-02-22

### Added
- **GCP Authentication**: Implement GCP token retrieval mechanism with gcloud CLI
- **AWS Authentication**: Implement AWS CLI authentication for ECR private registry access

### Fixed
- **Security**: Prevent token leakage in error messages, sanitize AWS CLI output
- **Validation**: Improve AWS ECR registry format validation, reject URLs with extra parts

## [0.5.0] - 2026-02-22

### Added
- **SLSA Provenance**: Add SLSA Level 3 provenance generation to release workflow
- **GitHub Issue Templates**: Bug reports, features, enhancements, and discussions

### Changed
- **Project Structure**: Refactor into internal packages with factory pattern
- **Docker/UV Cache**: Add Docker image and UV package caching support
- **Release Process**: Parallelize binary builds, archive binaries for release

## [0.4.19] - 2026-02-01

### Fixed
- **Lock File**: Improve lock file validation and multi-scenario sync

## [0.4.18] - 2026-02-01

### Added
- **GCP Authentication**: Constants for GCP magic values, improve maintainability

## [0.4.17] - 2026-02-01

### Added
- **AWS Authentication**: Error handling for AWS CLI not installed, security hardening

## [0.4.16] - 2026-02-01

### Fixed
- **Role Operations**: Fix removing and adding dependencies not updating lock file

## [0.4.15] - 2026-02-01

### Fixed
- **Diffusion-Test Action**: Fix verify action path (#3)

## [0.4.14] - 2026-02-01

### Changed
- **Dependency Workflow**: Improve dependency management workflow and configuration

## [0.4.13] - 2026-02-01

### Fixed
- **Cache**: Fix wrong cache customPath condition, change collections mounting point

## [0.4.12] - 2026-02-01

### Added
- **Dependency Management**: Comprehensive system with Python version constraints and lock file support

## [0.4.11] - 2026-02-01

### Fixed
- **Running Parameters**: Updated running parameters for new versions

## [0.4.10] - 2026-02-01

### Fixed
- **Cache**: Fix cache list command description

## [0.4.9] - 2026-01-31

### Fixed
- **Release**: Fix release CI workflow and chocolatey publishing

## [0.4.8] - 2026-01-31

### Added
- **Chocolatey**: Initial Chocolatey package for Windows installation

## [0.4.7] - 2026-01-25

### Added
- **File Validation**: Add checksum validation for all downloaded files in releases

## [0.4.6] - 2026-01-25

### Fixed
- **Release**: Fix various Chocolatey nuspec and install script issues

## [0.4.5] - 2026-01-17

### Added
- **CI/CD**: Add GitHub Actions for testing with diffusion

## [0.4.4] - 2026-01-16

### Added
- **Docker/UV Cache**: Add DinD/UV cache support with CI docker-cp fallback and GitHub Action integration

## [0.4.3] - 2026-01-15

### Fixed
- **CI/CD**: Fix version injection, DinD cache, verify tags, and Windows UV cache performance

## [0.4.2] - 2025-12-30

### Fixed
- **Verify Action**: Fix verify run CI wrong path for DiffusionTests type
- **CI Cache**: Improve cache handling with restore and save actions

## [0.4.1] - 2025-12-27

### Added
- **OIDC Authentication**: Add OIDC token authentication support for container registries
- **YC Registry Validation**: Add Yandex Container Registry URL format validation

## [0.4.0] - 2025-12-25

### Added
- **Worktree Development**: Add worktree-based development workflow

## [0.3.14] - 2025-12-23

### Changed
- **Code Readability**: Improved multiline command formatting in CI mode

## [0.3.13] - 2025-12-23

### Changed
- **Code Readability**: Improved multiline command formatting in CI mode
  - Refactored long git clone command to multiline format for better readability
  - Added proper line continuation with backslashes
  - Fixed log message typo (`/tmp/role/tests` → `/tmp/repo/tests`)

## [0.3.12] - 2025-12-23

### Fixed
- **CI Mode**: Rollback CI setup to 0.3.0 version for stability

## [0.3.11] - 2025-12-23

### Fixed
- **CI Mode Lint Configuration**: Fixed yamllint and ansible-lint file creation in CI mode
  - Now writes to correct container paths (`/opt/molecule/org.role/`)
  - Uses base64 encoding for safe content transfer to container
  - Fixes docker exec call to use roleFlag instead of hardcoded "CI"
  - Handles YAML content with quotes, newlines, and special characters safely
  - Resolves exit code 1 errors when running lint commands in CI mode

## [0.3.10] - 2025-12-23

### Added
- **CI Mode**: New `--ci` flag for CI/CD environments
  - Clones repository inside container instead of using volume mounts
  - Automatically detects git remote URL and commit SHA from current repository
  - Passes repository information as environment variables (`GIT_REMOTE`, `GIT_SHA`) to container
  - Container workflow: clone to `/tmp/repo` → checkout specific commit → copy files to `/opt/molecule/org.role/`
  - Skips ansible-galaxy role init (not needed in CI)
  - Skips volume mount of `/opt/molecule` (avoids permission and timing issues)
  - Skips permission fixes (no volume mount to fix)
  - Skips host-side file copying (all operations inside container)
  - Works with any git provider (GitHub, GitLab, Bitbucket, self-hosted)
  - Ensures reproducible builds by checking out specific commit SHA
  - Eliminates volume mount timing and permission issues in CI runners
- **Windows E2E Testing**: Cygwin expect automation for Windows
- **WSL2 Docker Support**: Improved Docker credential helper error detection
- **Enhanced Error Logging**: Better Docker error diagnostics

### Changed
- **Git Operations**: Improved git command execution with timeouts and proper error handling
- **Cgroup Mount**: Made conditional based on path existence for WSL2 compatibility

## [0.3.9] - 2025-12-22

### Fixed
- **CI Mode**: Rollback CI setup to 0.3.0 version

## [0.3.8] - 2025-12-22

### Fixed
- **CI Mode**: Correct role init check order in CI mode

## [0.3.7] - 2025-12-22

### Fixed
- **CI Mode**: Ensure clean state by removing existing container before start

## [0.3.6] - 2025-12-22

### Fixed
- **CI Mode**: Copy role files before container start

## [0.3.5] - 2025-12-22

### Fixed
- **CI Mode**: Fix copying files issue in CI mode

## [0.3.4] - 2025-12-22

### Added
- **CI Mode**: Initial CI mode implementation and Docker error handling improvements

## [0.3.3] - 2025-12-22

### Added
- **Ansible Cache Feature**: Persist Ansible roles and collections for faster role execution
  - CLI commands: `cache enable`, `cache disable`, `cache clean`, `cache status`, `cache list`
  - Automatic cache directory management at `~/.diffusion/cache/role_<cache_id>/`
  - Per-role isolated caching with unique cache IDs
  - Mounts only `roles/` and `collections/` subdirectories to avoid conflicts
  - Significant performance improvement (3–10× faster on subsequent runs)
- **Registry Provider Authentication**: Provider-specific CLI initialization
  - YC: Runs `yc` CLI init and logs into Yandex Cloud Registry
  - AWS / GCP: Placeholder implementations
  - Public: Skips all CLI initialization
- **Artifact Management System**: Secure credential storage for multiple private repositories
  - Encrypted local storage using AES-256-GCM (machine-specific key)
  - HashiCorp Vault integration with per-source field names
  - CLI: `artifact add`, `artifact list`, `artifact show`, `artifact remove`
  - Indexed environment variables: `GIT_USER_N`, `GIT_PASSWORD_N`, `GIT_URL_N`
  - Support for up to 10 artifact sources

### Changed
- **Configuration Structure**: Refactored to use `artifact_sources` array instead of single `url`
  - **BREAKING**: Vault field names moved from `[vault]` to per-source `[[artifact_sources]]`
- **Secrets Storage Path**: `~/.diffusion/secrets/<role>/<source>` (was `~/.diffusion/<source>_artifact_secrets`)
- **Default Container Registry**: Changed to `ghcr.io` with `polar-team/diffusion-molecule-container`
- **BREAKING**: `diffusion role` without `--init` now displays config instead of prompting to initialize

### Added
- Architecture detection for container tags
- Comprehensive test suite (35+ tests)
- Performance optimizations (path caching 2,745× faster, buffered I/O)
- Helper functions: `PathCache`, `EnsureDir`, `ValidateRegistryProvider`, etc.

## [0.3.2] - 2025-12-22

### Fixed
- Minor CI fixes for release workflow

## [0.3.1] - 2025-12-22

### Fixed
- Initial release workflow stabilization

## [0.3.0] - 2025-12-22

### Added
- **Molecule Scenario Validation**: Container-side validation for `molecule.yml`
- **Destroy Command**: `--destroy` flag for molecule test instances
- **Verify Tags**: `--tag` support for `--verify` command
- **CI/CD Mode**: Initial `--ci` flag (non-interactive mode for pipelines)

### Changed
- **Release Workflow**: Restrict to tags from main branch only

## [0.2.9] - 2025-12-21

### Fixed
- **SLSA**: Fix SLSA generator version for artifact v4 compatibility

## [0.2.8] - 2025-12-21

### Fixed
- **Release**: Fix SLSA generator to v0.2.0 for artifact v4 compatibility

## [0.2.7] - 2025-12-21

### Fixed
- **Release**: Update SLSA generator to v2.1.0 and fix artifact handling

## [0.2.6] - 2025-12-21

### Added
- **Release Signing**: Cosign keyless signing for release artifacts

## [0.2.5] - 2025-12-21

### Added
- **SLSA Provenance**: Add SLSA provenance generation to release workflow

## [0.2.4] - 2025-12-21

### Added
- **Release Workflow**: Automated release with binary signing and provenance

## [0.2.3] - 2025-12-20

### Fixed
- Release workflow fixes and stabilization

## [0.2.2] - 2025-12-20

### Fixed
- Release workflow fixes

## [0.2.1] - 2025-12-20

### Fixed
- Release workflow fixes

## [0.2.0] - 2025-12-20

### Added
- **Version Command**: `diffusion --version` with build info (OS, Arch, Go version)
- **Makefile**: Complete build system with cross-compilation targets
- **Release Workflow**: GitHub Actions release with signing and SLSA provenance

## [0.1.2-alpha] - 2025-12-20

### Fixed
- Initial bug fixes and stabilization

## [0.1.0-alpha] - 2025-12-20

### Added
- **Initial Release**: First alpha release of Diffusion CLI
  - `diffusion molecule` — Run Molecule testing workflows (converge, verify, lint, idempotence)
  - `diffusion role` — Ansible role management and initialization
  - Container registry authentication (YC, Public)
  - HashiCorp Vault integration for credentials
  - TOML configuration (`diffusion.toml`)
  - Docker-based testing with diffusion-molecule-container
  - Cross-platform support (Linux, macOS, Windows)
