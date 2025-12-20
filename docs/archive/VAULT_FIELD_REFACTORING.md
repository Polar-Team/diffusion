# Vault Field Names Refactoring

## Summary

Moved Vault field names (`username_field` and `token_field`) from the global `HashicorpVault` struct to individual `ArtifactSource` configurations. This allows each artifact source to specify its own field names when retrieving credentials from Vault.

## Changes Made

### 1. Structure Changes

#### `vault.go`
- **Removed** from `HashicorpVault`:
  - `UserNameField string`
  - `TokenField string`
- Kept only:
  - `HashicorpVaultIntegration bool`
  - `SecretKV2Path string` (deprecated, for backward compatibility)
  - `SecretKV2Name string` (deprecated, for backward compatibility)

#### `secrets.go`
- **Added** to `ArtifactSource`:
  - `VaultUsernameField string` - Field name for username in Vault secret
  - `VaultTokenField string` - Field name for token in Vault secret

### 2. Function Changes

#### `VaultConfigHelper()` in `main.go`
**Before:**
```go
func VaultConfigHelper(integration bool) *HashicorpVault {
    // Asked for username_field and token_field
    // Returned HashicorpVault with these fields populated
}
```

**After:**
```go
func VaultConfigHelper(integration bool) *HashicorpVault {
    // Only returns enabled/disabled status
    // No field name prompts
}
```

#### `ArtifactSourcesHelper()` in `main.go`
**Before:**
```go
// When Vault selected, only asked for:
// - vault_path
// - vault_secret_name
```

**After:**
```go
// When Vault selected, now asks for:
// - vault_path
// - vault_secret_name
// - vault_username_field (default: "username")
// - vault_token_field (default: "token")
```

#### `GetArtifactCredentialsFromVault()` in `secrets.go`
**Before:**
```go
// Used vaultConfig.UserNameField and vaultConfig.TokenField
username := result.Data.Data[vaultConfig.UserNameField].(string)
token := result.Data.Data[vaultConfig.TokenField].(string)
```

**After:**
```go
// Uses source.VaultUsernameField and source.VaultTokenField
usernameField := source.VaultUsernameField
if usernameField == "" {
    usernameField = "username" // default
}
tokenField := source.VaultTokenField
if tokenField == "" {
    tokenField = "token" // default
}
username := result.Data.Data[usernameField].(string)
token := result.Data.Data[tokenField].(string)
```

### 3. Configuration Changes

#### Old Configuration (No Longer Supported)
```toml
[vault]
enabled = true
secret_kv2_path = "secret/data/artifacts"
secret_kv2_name = "credentials"
username_field = "git_user"    # Global field name
token_field = "git_token"      # Global field name
```

#### New Configuration (Required)
```toml
[vault]
enabled = true

[[artifact_sources]]
name = "source1"
url = "https://git1.example.com"
use_vault = true
vault_path = "secret/data/artifacts"
vault_secret_name = "source1"
vault_username_field = "git_user"    # Per-source field name
vault_token_field = "git_token"      # Per-source field name

[[artifact_sources]]
name = "source2"
url = "https://git2.example.com"
use_vault = true
vault_path = "secret/data/prod"
vault_secret_name = "source2"
vault_username_field = "username"    # Different field names
vault_token_field = "password"
```

### 4. Show Command Updates

The `diffusion show` command now displays:
- Vault field names per artifact source (when using Vault)
- Deprecation warning for legacy Vault configuration
- Clear indication of storage type (Vault vs Local)

Example output:
```
[Artifact Sources]
  Source 1:
    Name:                  primary-git
    URL:                   https://git.example.com
    Storage:               Vault (secret/data/artifacts/primary-git)
    Username Field:        git_username
    Token Field:           git_token
  Source 2:
    Name:                  secondary-git
    URL:                   https://git2.example.com
    Storage:               Local (encrypted)
```

### 5. Legacy Configuration Handling

Legacy Vault configurations (with global `username_field` and `token_field`) are **no longer supported**. If detected, the application will:
1. Display an error message
2. Suggest migration to artifact_sources
3. Exit with error code 1

Users must migrate to the new per-source configuration.

### 6. Test Updates

Updated all tests in `config_helpers_test.go` to:
- Remove references to `UserNameField` and `TokenField` in `HashicorpVault`
- Add `VaultUsernameField` and `VaultTokenField` to Vault-based `ArtifactSource` test cases
- Validate that Vault sources have field names configured

## Benefits

1. **Flexibility**: Each artifact source can use different Vault field naming conventions
2. **Clarity**: Field names are configured where they're used (per source)
3. **Consistency**: All source-specific configuration is in `ArtifactSource`
4. **Simplicity**: `HashicorpVault` struct is simpler, only managing global Vault settings

## Migration Path

Users with legacy configurations should:
1. Run `diffusion show` to see current configuration
2. Note the current `username_field` and `token_field` values
3. Use `diffusion artifact add` to create new artifact sources with Vault
4. Specify the field names when prompted
5. Remove legacy `secret_kv2_path`, `secret_kv2_name`, `username_field`, and `token_field` from `[vault]` section

See [MIGRATION_GUIDE.md](MIGRATION_GUIDE.md) for detailed migration instructions.

## Testing

All 50 tests pass successfully:
- Vault configuration tests updated
- Artifact source structure tests include field name validation
- Integration tests verify per-source field name usage
- Build successful with no errors

## Documentation Updates

- [MIGRATION_GUIDE.md](MIGRATION_GUIDE.md) - Updated with per-source field information
- [ARTIFACT_MANAGEMENT.md](ARTIFACT_MANAGEMENT.md) - Updated Vault configuration examples
- [CHANGELOG.md](CHANGELOG.md) - Added breaking change notice
- [VAULT_FIELD_REFACTORING.md](VAULT_FIELD_REFACTORING.md) - This document
