# Changelog

All notable changes to the Diffusion project will be documented in this file.

## [Unreleased]

### Added
- **Ansible Cache Feature**: Persist Ansible roles and collections for faster role execution
  - CLI commands: `cache enable`, `cache disable`, `cache clean`, `cache status`, `cache list`
  - Automatic cache directory management at `~/.diffusion/cache/role_<cache_id>/`
  - Per-role isolated caching with unique cache IDs
  - Mounts only `roles/` and `collections/` subdirectories to avoid conflicts
  - Mounted to `/root/.ansible/roles` and `/root/.ansible/collections` in container
  - Significant performance improvement (3-10x faster on subsequent runs)
  - See [CACHE_FEATURE.md](CACHE_FEATURE.md) for details
- **Registry Provider Authentication**: Provider-specific CLI initialization and authentication
  - YC: Runs `yc` CLI init and logs into Yandex Cloud Registry
  - AWS: Placeholder for AWS CLI and ECR authentication
  - GCP: Placeholder for gcloud CLI and Artifact Registry authentication
  - Public: Skips all CLI initialization and authentication
  - Only executes provider-specific commands based on configuration
  - Prevents unnecessary CLI calls for public registries
- **Artifact Management System**: Secure credential storage for multiple private repositories
  - New `ArtifactSourcesHelper()` function for interactive onboarding
  - Automatic credential storage during initial configuration
  - Encrypted local storage using AES-256-GCM
  - Machine-specific encryption (hostname + username)
  - Support for HashiCorp Vault integration with per-source field names
  - Support for mixed local/Vault storage
  - CLI commands: `artifact add`, `artifact list`, `artifact show`, `artifact remove`
  - **`artifact add` now automatically saves to `diffusion.toml`**
  - **`artifact remove` now removes from `diffusion.toml`**
  - Credentials stored in `~/.diffusion/secrets/<role-name>/<source-name>`
  - Indexed environment variables: `GIT_USER_1`, `GIT_PASSWORD_1`, `GIT_URL_1`, etc.
  - Support for up to 10 artifact sources (configurable)
  - Backward compatible with single artifact URL configuration
  - See [ARTIFACT_MANAGEMENT.md](ARTIFACT_MANAGEMENT.md) for details

### Changed
- **Configuration Structure**: Refactored to use `artifact_sources` array instead of single `url`
  - `ArtifactUrl` field deprecated (kept for backward compatibility)
  - `VaultConfigHelper()` simplified - no longer asks for field names
  - Vault path/secret now configured per artifact source
  - **BREAKING**: Vault field names (`username_field`, `token_field`) moved from `HashicorpVault` to per-source `ArtifactSource`
  - Each artifact source can now specify its own Vault field names
  - Legacy Vault configuration (with global field names) no longer supported
- **Secrets Storage Path**: Reorganized to role-based directory structure
  - **Old**: `~/.diffusion/<source-name>_artifact_secrets`
  - **New**: `~/.diffusion/secrets/<role-name>/<source-name>`
  - Better organization for multi-role projects
  - Falls back to "default" role when no role detected
  - See [SECRETS_PATH_REFACTORING.md](SECRETS_PATH_REFACTORING.md) for details
- **Default Container Registry:** Changed to `ghcr.io` with `polar-team/diffusion-molecule-container`
  - Registry Server: `ghcr.io` (previously required manual input)
  - Registry Provider: `Public` (default)
  - Container Name: `polar-team/diffusion-molecule-container`
  - Container Tag: Auto-detected based on architecture (`latest-amd64` or `latest-arm64`)
- **BREAKING:** `diffusion role` command behavior updated
  - Without `--init` flag: Now displays current role configuration or returns an error if no role exists
  - Previously: Would prompt user to initialize a new role if config not found
  - With `--init` flag: Initializes a new role, warns if role already exists in current directory
  - **Migration:** Users who relied on the interactive prompt should now use `diffusion role --init` explicitly

### Added
- Architecture detection for container tags (`GetDefaultMoleculeTag()` function)
- Default container registry configuration with sensible defaults
- Comprehensive test suite with 35+ tests covering core functionality
- Performance optimizations:
  - Path caching for file existence checks (2,745x faster)
  - Buffered file I/O with 32KB buffers
  - Eliminated duplicate filesystem calls
- New helper functions in `helpers.go`:
  - `PathCache` for caching file existence checks
  - `EnsureDir` / `EnsureDirs` for idempotent directory creation
  - `ValidateRegistryProvider` / `ValidateTestsType` for input validation
  - `RemoveFromSlice` / `ContainsString` for slice operations
  - `SetEnvVars` for batch environment variable setting
- Constants extracted to `constants.go` for better maintainability
- Test files:
  - `main_test.go` - Core functionality tests
  - `config_test.go` - Configuration management tests
  - `role_test.go` - Ansible role management tests
  - `helpers_test.go` - Helper functions tests
  - `role_command_test.go` - Role command behavior tests
- Documentation:
  - `TESTING_AND_PERFORMANCE.md` - Comprehensive testing and performance documentation
  - `CHANGELOG.md` - This file

### Fixed
- Linter warnings for capitalized error strings
- Duplicate `os.Stat` calls in `copyIfExists` function
- Inconsistent error handling in role initialization

### Performance
- File copying: Optimized with buffered I/O
- Path existence checks: 2,745x faster with caching (11.87ns vs 32,592ns)
- Config loading: 132,935 ns/op with 197 allocations
- Reduced memory allocations through better resource management

## [Previous Versions]

See git history for changes prior to this changelog.
