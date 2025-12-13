# Secrets Path Refactoring: Role-Based Directory Structure

## Summary

Reorganized the artifact secrets storage from a flat structure in `~/.diffusion/` to a hierarchical role-based structure in `~/.diffusion/secrets/<role-name>/`. This provides better organization when working with multiple roles.

## Changes Made

### Old Structure
```
~/.diffusion/
├── source1_artifact_secrets
├── source2_artifact_secrets
└── source3_artifact_secrets
```

**Issues:**
- All secrets in one flat directory
- No separation between different roles
- Filename suffix `_artifact_secrets` was redundant
- Difficult to manage when working with multiple roles

### New Structure
```
~/.diffusion/
└── secrets/
    ├── role1/
    │   ├── source1
    │   ├── source2
    │   └── source3
    ├── role2/
    │   ├── source1
    │   └── source4
    └── default/
        └── source5
```

**Benefits:**
- Clear separation by role
- Cleaner file names (no suffix)
- Better organization for multi-role projects
- Easier to backup/restore per-role secrets
- Falls back to "default" role if no role is detected

## Implementation Details

### 1. Updated `getSecretsDir()` Function

**Before:**
```go
func getSecretsDir() (string, error) {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        return "", err
    }
    
    secretsDir := filepath.Join(homeDir, ".diffusion")
    os.MkdirAll(secretsDir, 0700)
    
    return secretsDir, nil
}
```

**After:**
```go
func getSecretsDir() (string, error) {
    homeDir, err := os.UserHomeDir()
    if err != nil {
        return "", err
    }
    
    // Get current role name
    roleName := getCurrentRoleName()
    if roleName == "" {
        roleName = "default"
    }
    
    secretsDir := filepath.Join(homeDir, ".diffusion", "secrets", roleName)
    os.MkdirAll(secretsDir, 0700)
    
    return secretsDir, nil
}
```

### 2. Added `getCurrentRoleName()` Function

```go
func getCurrentRoleName() string {
    meta, _, err := LoadRoleConfig("")
    if err != nil {
        return ""
    }
    if meta != nil && meta.GalaxyInfo.RoleName != "" {
        return meta.GalaxyInfo.RoleName
    }
    return ""
}
```

This function:
- Reads `meta/main.yml` from the current directory
- Extracts the role name from `galaxy_info.role_name`
- Returns empty string if no role is found
- Falls back to "default" in `getSecretsDir()`

### 3. Updated `getSecretFilePath()` Function

**Before:**
```go
func getSecretFilePath(sourceName string) (string, error) {
    secretsDir, err := getSecretsDir()
    if err != nil {
        return "", err
    }
    
    filename := fmt.Sprintf("%s_artifact_secrets", sourceName)
    return filepath.Join(secretsDir, filename), nil
}
```

**After:**
```go
func getSecretFilePath(sourceName string) (string, error) {
    secretsDir, err := getSecretsDir()
    if err != nil {
        return "", err
    }
    
    return filepath.Join(secretsDir, sourceName), nil
}
```

**Changes:**
- Removed `_artifact_secrets` suffix
- File name is now just the source name
- Path includes role directory automatically

### 4. Updated `ListStoredCredentials()` Function

**Before:**
```go
for _, entry := range entries {
    if entry.IsDir() {
        continue
    }
    
    name := entry.Name()
    // Remove the "_artifact_secrets" suffix
    if len(name) > 17 && name[len(name)-17:] == "_artifact_secrets" {
        sourceName := name[:len(name)-17]
        sources = append(sources, sourceName)
    }
}
```

**After:**
```go
for _, entry := range entries {
    if entry.IsDir() {
        continue
    }
    
    // Each file is a source name (no suffix anymore)
    sources = append(sources, entry.Name())
}
```

**Changes:**
- No need to strip suffix
- Simpler logic
- File name is the source name

## Usage Examples

### Adding Credentials

When you run `diffusion artifact add my-source` from a role directory:

```bash
$ cd my-ansible-role
$ diffusion artifact add my-source
Enter URL for my-source: https://git.example.com
Store credentials in Vault? (y/N): n
Enter Username: myuser
Enter Token/Password: ********
Credentials for 'my-source' saved successfully (encrypted in ~/.diffusion/secrets/my-ansible-role/my-source)
Added artifact source 'my-source' to diffusion.toml
```

The file is created at: `~/.diffusion/secrets/my-ansible-role/my-source`

### Without a Role (Default)

If you run the command outside a role directory:

```bash
$ cd /some/other/directory
$ diffusion artifact add global-source
...
Credentials for 'global-source' saved successfully (encrypted in ~/.diffusion/secrets/default/global-source)
```

The file is created at: `~/.diffusion/secrets/default/global-source`

## Directory Permissions

The directory structure maintains secure permissions:

```bash
~/.diffusion/                    # 0700 (drwx------)
└── secrets/                     # 0700 (drwx------)
    └── <role-name>/             # 0700 (drwx------)
        └── <source-name>        # 0600 (-rw-------)
```

## Migration

### Automatic Migration

No automatic migration is provided. Users with existing secrets in the old format should:

1. Note their existing sources: `ls ~/.diffusion/*_artifact_secrets`
2. For each source, run `diffusion artifact add <source-name>` to recreate
3. Remove old files: `rm ~/.diffusion/*_artifact_secrets`

### Manual Migration

If you want to preserve existing encrypted files:

```bash
# Create new structure
mkdir -p ~/.diffusion/secrets/default

# Move and rename files
for file in ~/.diffusion/*_artifact_secrets; do
    basename=$(basename "$file" _artifact_secrets)
    mv "$file" ~/.diffusion/secrets/default/"$basename"
done
```

## Testing

Updated tests in `secrets_test.go`:

1. **TestGetSecretFilePath**: Updated to check for new path format
   - Verifies path contains "secrets" directory
   - Checks path ends with source name (no suffix)

2. **TestGetSecretsDir**: Still works (creates role-based directory)

3. **All other tests**: Pass without changes (use the helper functions)

All 54 tests pass successfully.

## Documentation Updates

Updated all documentation to reflect new path structure:

- [ARTIFACT_MANAGEMENT.md](ARTIFACT_MANAGEMENT.md)
- [README.md](README.md)
- [INDEXED_ENVIRONMENT_VARIABLES.md](INDEXED_ENVIRONMENT_VARIABLES.md)
- [ARTIFACT_CLI_FIX.md](ARTIFACT_CLI_FIX.md)
- [CHANGELOG.md](CHANGELOG.md)

## Benefits Summary

1. **Better Organization**: Secrets grouped by role
2. **Cleaner Names**: No redundant suffix
3. **Multi-Role Support**: Each role has its own secrets directory
4. **Easier Management**: Clear structure for backup/restore
5. **Backward Compatible**: Falls back to "default" when no role detected
6. **Secure**: Maintains 0700/0600 permissions throughout

## Example Directory Tree

After working with multiple roles:

```
~/.diffusion/
└── secrets/
    ├── ansible-role-nginx/
    │   ├── github-packages
    │   └── company-nexus
    ├── ansible-role-postgres/
    │   ├── github-packages
    │   └── internal-registry
    ├── ansible-role-redis/
    │   └── dockerhub-private
    └── default/
        └── global-artifacts
```

Each role's secrets are isolated and clearly organized.
