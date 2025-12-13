# Migration Guide: Single URL to Multiple Artifact Sources

## Overview

Diffusion has been refactored to support multiple artifact sources with individual credential management. This guide helps you migrate from the old single-URL configuration to the new multi-source structure.

## What Changed

### Old Configuration (Deprecated)

```toml
url = "https://artifacts.example.com"

[vault]
enabled = true
secret_kv2_path = "secret/data/git"
secret_kv2_name = "credentials"
username_field = "git_username"  # REMOVED - now per-source
token_field = "git_token"        # REMOVED - now per-source
```

### New Configuration (Recommended)

```toml
[[artifact_sources]]
name = "primary"
url = "https://artifacts.example.com"
use_vault = true
vault_path = "secret/data/git"
vault_secret_name = "credentials"
vault_username_field = "git_username"  # Now per-source
vault_token_field = "git_token"        # Now per-source

[vault]
enabled = true  # Simplified - no field names here
```

## Migration Steps

### Step 1: Identify Your Current Configuration

Check your `diffusion.toml` file:

```bash
cat diffusion.toml | grep -A 5 "url ="
```

### Step 2: Choose Migration Path

#### Option A: Automatic Migration (Recommended)

Delete your `diffusion.toml` and run:

```bash
diffusion molecule --role test --org test
```

Follow the interactive prompts to configure artifact sources.

#### Option B: Manual Migration

Edit `diffusion.toml` manually:

**Before:**
```toml
url = "https://artifacts.example.com"

[vault]
enabled = true
secret_kv2_path = "secret/data/git"
secret_kv2_name = "credentials"
username_field = "git_username"
token_field = "git_token"
```

**After:**
```toml
[[artifact_sources]]
name = "primary"
url = "https://artifacts.example.com"
use_vault = true
vault_path = "secret/data/git"
vault_secret_name = "credentials"

[vault]
enabled = true
username_field = "username"
token_field = "token"
```

### Step 3: Add Credentials (if using local storage)

If you're not using Vault, add credentials:

```bash
diffusion artifact add primary
Enter URL for primary: https://artifacts.example.com
Enter Username: myuser
Enter Token/Password: mytoken
```

### Step 4: Verify Configuration

```bash
diffusion show
```

Look for the "Artifact Sources" section.

### Step 5: Test

```bash
diffusion molecule --role test --org test
```

Check the logs for:
```
Loaded credentials for artifact source 'primary' (GIT_*_1)
```

## Backward Compatibility

### Legacy Configuration Still Works

Your old configuration will continue to work:

```toml
url = "https://artifacts.example.com"

[vault]
enabled = true
secret_kv2_path = "secret/data/git"
secret_kv2_name = "credentials"
username_field = "git_username"
token_field = "git_token"
```

This will be loaded as `GIT_USER_1`, `GIT_PASSWORD_1`, `GIT_URL_1`.

**Warning:** You'll see a deprecation notice:
```
Using legacy Vault configuration. Consider migrating to artifact_sources.
```

### When to Migrate

Migrate if you:
- Want to use multiple artifact sources
- Want per-source credential management
- Want to mix local and Vault storage
- Want cleaner configuration

Don't migrate if:
- You only have one artifact source
- Your current setup works fine
- You're not ready to test changes

## Common Migration Scenarios

### Scenario 1: Single Vault Source

**Before:**
```toml
url = "https://nexus.company.com"

[vault]
enabled = true
secret_kv2_path = "secret/data/nexus"
secret_kv2_name = "credentials"
username_field = "nexus_user"
token_field = "nexus_token"
```

**After:**
```toml
[[artifact_sources]]
name = "company-nexus"
url = "https://nexus.company.com"
use_vault = true
vault_path = "secret/data/nexus"
vault_secret_name = "credentials"

[vault]
enabled = true
username_field = "nexus_user"
token_field = "nexus_token"
```

### Scenario 2: Single Local Source

**Before:**
```toml
url = "https://github.com/myorg"

[vault]
enabled = false
```

**After:**
```toml
[[artifact_sources]]
name = "github"
url = "https://github.com/myorg"
use_vault = false

[vault]
enabled = false
```

Then add credentials:
```bash
diffusion artifact add github
```

### Scenario 3: Multiple Sources (New Capability)

**New Configuration:**
```toml
[[artifact_sources]]
name = "github"
url = "https://github.com/myorg"
use_vault = false

[[artifact_sources]]
name = "nexus"
url = "https://nexus.company.com"
use_vault = true
vault_path = "secret/data/nexus"
vault_secret_name = "creds"

[[artifact_sources]]
name = "gitlab"
url = "https://gitlab.company.com"
use_vault = false

[vault]
enabled = true
username_field = "username"
token_field = "token"
```

Add local credentials:
```bash
diffusion artifact add github
diffusion artifact add gitlab
```

## Configuration Changes

### VaultConfigHelper Changes

**Old Prompts:**
```
Enter SecretKV2Path (e.g., secret/data/diffusion):
Enter Git Username Field in Vault (default: git_username):
Enter Git Token Field in Vault (default: git_token):
```

**New Prompts:**
```
Enable Vault Integration for artifact sources? (y/N):
Enter Username Field in Vault (default: username):
Enter Token Field in Vault (default: token):
```

**Why:** Vault path and secret name are now per-source, not global.

### ArtifactSourcesHelper (New)

**New Interactive Flow:**
```
Configure artifact sources for private repositories? (y/N): y

Enter artifact source name (or press Enter to finish): github
Enter URL for github: https://github.com/myorg
Store credentials in Vault? (y/N): n
Enter Username for github: myuser
Enter Token/Password for github: ghp_xxxxx
Credentials for 'github' saved locally (encrypted)
Added artifact source: github

Add another artifact source? (y/N): y

Enter artifact source name (or press Enter to finish): nexus
Enter URL for nexus: https://nexus.company.com
Store credentials in Vault? (y/N): y
Enter Vault path for nexus (e.g., secret/data/artifacts): secret/data/nexus
Enter Vault secret name for nexus: credentials
Credentials for 'nexus' will be retrieved from Vault at secret/data/nexus/credentials
Added artifact source: nexus

Add another artifact source? (y/N): n
```

## Environment Variables

### No Changes Required

Environment variables remain the same:
- `GIT_USER_1`, `GIT_PASSWORD_1`, `GIT_URL_1`
- `GIT_USER_2`, `GIT_PASSWORD_2`, `GIT_URL_2`
- etc.

Your Ansible playbooks don't need updates.

## Troubleshooting

### Issue: "Using legacy Vault configuration" Warning

**Cause:** You're using the old configuration format.

**Solution:** Migrate to `artifact_sources` or ignore the warning (it still works).

### Issue: Credentials Not Loading

**Cause:** Missing credentials for local sources.

**Solution:**
```bash
diffusion artifact list  # Check what's stored
diffusion artifact add <source-name>  # Add missing credentials
```

### Issue: Vault Connection Failed

**Cause:** Vault path/secret moved from global to per-source.

**Solution:** Update each source with correct `vault_path` and `vault_secret_name`.

### Issue: Multiple Sources Not Working

**Cause:** Configuration syntax error.

**Solution:** Verify TOML syntax:
```bash
# Each source needs [[artifact_sources]] (double brackets)
[[artifact_sources]]
name = "source1"
...

[[artifact_sources]]
name = "source2"
...
```

## Testing Your Migration

### 1. Verify Configuration

```bash
diffusion show
```

Expected output:
```
[Artifact Sources]
  Source 1:
    Name:                  github
    URL:                   https://github.com/myorg
    Storage:               Local (encrypted)
  Source 2:
    Name:                  nexus
    URL:                   https://nexus.company.com
    Storage:               Vault (secret/data/nexus/credentials)
```

### 2. Test Credential Loading

```bash
diffusion molecule --role test --org test
```

Expected logs:
```
Loaded credentials for artifact source 'github' (GIT_*_1)
Loaded credentials for artifact source 'nexus' (GIT_*_2)
```

### 3. Verify Environment Variables

In your molecule container:
```bash
docker exec -it molecule-test env | grep GIT_
```

Expected:
```
GIT_USER_1=user1
GIT_PASSWORD_1=***
GIT_URL_1=https://github.com/myorg
GIT_USER_2=user2
GIT_PASSWORD_2=***
GIT_URL_2=https://nexus.company.com
```

## Rollback

If you need to rollback:

### 1. Restore Old Configuration

```toml
url = "https://artifacts.example.com"

[vault]
enabled = true
secret_kv2_path = "secret/data/git"
secret_kv2_name = "credentials"
username_field = "git_username"
token_field = "git_token"
```

### 2. Remove New Sections

Delete `[[artifact_sources]]` sections.

### 3. Test

```bash
diffusion molecule --role test --org test
```

You'll see:
```
Using legacy Vault configuration. Consider migrating to artifact_sources.
Loaded credentials from Vault (GIT_*_1)
```

## Benefits of Migration

1. **Multiple Sources**: Support for multiple private repositories
2. **Mixed Storage**: Use both local and Vault storage
3. **Per-Source Config**: Each source has its own Vault path/secret
4. **Better Organization**: Clear separation of concerns
5. **Easier Management**: Use `diffusion artifact` commands
6. **Future-Proof**: New features will use the new structure

## Need Help?

- Check [ARTIFACT_MANAGEMENT.md](ARTIFACT_MANAGEMENT.md) for detailed documentation
- Check [INDEXED_ENVIRONMENT_VARIABLES.md](INDEXED_ENVIRONMENT_VARIABLES.md) for environment variable details
- Run `diffusion artifact --help` for CLI help
- File an issue on GitHub if you encounter problems
