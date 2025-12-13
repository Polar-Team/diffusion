# Changelog

All notable changes to the Diffusion project will be documented in this file.

## [Unreleased]

### Changed
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
