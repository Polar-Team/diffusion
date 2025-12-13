# Artifact CLI Fix: Auto-save to diffusion.toml

## Issue

The `diffusion artifact add` and `diffusion artifact remove` commands were only managing the encrypted credential files in `~/.diffusion/secrets/<role-name>/`, but were not updating the `diffusion.toml` configuration file. This meant that artifact sources had to be manually added to the config file.

## Solution

Updated both commands to automatically manage the `diffusion.toml` configuration file.

## Changes Made

### 1. `artifact add` Command

**Before:**
- Only saved encrypted credentials to `~/.diffusion/secrets/<role-name>/<name>`
- Did not update `diffusion.toml`
- Users had to manually edit config file

**After:**
- Prompts for storage type (Vault or Local)
- For Vault: Collects Vault path, secret name, and field names
- For Local: Collects and encrypts credentials
- **Automatically adds/updates artifact source in `diffusion.toml`**
- Checks if source already exists and updates it
- Creates minimal config if `diffusion.toml` doesn't exist

**New Flow:**
```bash
$ diffusion artifact add my-repo
Enter URL for my-repo: https://git.example.com
Store credentials in Vault? (y/N): n
Enter Username: myuser
Enter Token/Password: ********
Credentials for 'my-repo' saved successfully (encrypted in ~/.diffusion/secrets/<role-name>/my-repo)
Added artifact source 'my-repo' to diffusion.toml  # NEW!
```

### 2. `artifact remove` Command

**Before:**
- Only deleted encrypted credentials
- Did not update `diffusion.toml`
- Left orphaned entries in config file

**After:**
- Deletes encrypted credentials (if they exist)
- **Automatically removes artifact source from `diffusion.toml`**
- Works for both local and Vault-based sources
- Provides clear feedback about what was removed

**New Flow:**
```bash
$ diffusion artifact remove my-repo
Local credentials for 'my-repo' removed successfully
Removed artifact source 'my-repo' from diffusion.toml  # NEW!
```

## Implementation Details

### artifact add Command

```go
// After collecting credentials/Vault info:

// Load existing config or create new one
config, err := LoadConfig()
if err != nil {
    config = &Config{
        ArtifactSources: []ArtifactSource{},
    }
}

// Check if source already exists
for i, existing := range config.ArtifactSources {
    if existing.Name == sourceName {
        // Update existing source
        config.ArtifactSources[i] = source
        SaveConfig(config)
        return
    }
}

// Add new source
config.ArtifactSources = append(config.ArtifactSources, source)
SaveConfig(config)
```

### artifact remove Command

```go
// Delete encrypted credentials (if they exist)
DeleteArtifactCredentials(sourceName)

// Remove from config file
config, err := LoadConfig()
if err != nil {
    return err
}

// Find and remove the source
for i, source := range config.ArtifactSources {
    if source.Name == sourceName {
        config.ArtifactSources = append(
            config.ArtifactSources[:i], 
            config.ArtifactSources[i+1:]...
        )
        break
    }
}

SaveConfig(config)
```

## Configuration Structure

After using `artifact add`, the `diffusion.toml` will contain:

### Local Storage Example
```toml
[[artifact_sources]]
name = "my-repo"
url = "https://git.example.com"
use_vault = false
```

### Vault Storage Example
```toml
[[artifact_sources]]
name = "vault-repo"
url = "https://vault.example.com"
use_vault = true
vault_path = "secret/data/artifacts"
vault_secret_name = "vault-repo"
vault_username_field = "git_username"
vault_token_field = "git_token"
```

## Benefits

1. **Automatic Configuration**: No manual editing of `diffusion.toml` required
2. **Consistency**: Credentials and configuration always in sync
3. **Update Support**: Re-running `artifact add` with same name updates the source
4. **Clean Removal**: `artifact remove` cleans up both credentials and config
5. **Error Prevention**: Reduces configuration errors from manual editing
6. **Better UX**: Single command does everything needed

## Testing

Created comprehensive tests in `artifact_cli_test.go`:

1. **TestArtifactAddToConfig**: Verifies sources are added to config
2. **TestArtifactRemoveFromConfig**: Verifies sources are removed from config
3. **TestArtifactUpdateInConfig**: Verifies existing sources can be updated
4. **TestArtifactVaultSourceInConfig**: Verifies Vault sources with field names

All tests pass successfully.

## Migration

No migration needed - this is a pure enhancement. Existing workflows continue to work, but now with automatic config management.

## Documentation Updates

- [ARTIFACT_MANAGEMENT.md](ARTIFACT_MANAGEMENT.md) - Updated command examples
- [CHANGELOG.md](CHANGELOG.md) - Added fix notes
- [ARTIFACT_CLI_FIX.md](ARTIFACT_CLI_FIX.md) - This document
