# Artifact Management

## Overview

Diffusion now supports managing multiple private artifact repository credentials with secure encrypted storage. Credentials can be stored either locally (encrypted) or in HashiCorp Vault.

## Features

- **Multiple Artifact Sources**: Configure multiple private repositories
- **Encrypted Local Storage**: Credentials encrypted using AES-256-GCM
- **Machine-Specific Encryption**: Encryption key derived from hostname + username
- **Vault Integration**: Optional HashiCorp Vault support for centralized secret management
- **Secure by Default**: Credentials stored in `~/.diffusion/` with 0600 permissions

## Security

### Encryption Details

- **Algorithm**: AES-256-GCM (Galois/Counter Mode)
- **Key Derivation**: SHA-256 hash of `hostname:username:diffusion-artifact-secrets`
- **Key Size**: 256 bits (32 bytes)
- **Nonce**: Randomly generated for each encryption operation
- **Storage Location**: `$HOME/.diffusion/secrets/<role-name>/<source-name>`
- **File Permissions**: 0600 (read/write for owner only)

### Security Properties

1. **Machine-Specific**: Credentials encrypted on one machine cannot be decrypted on another
2. **User-Specific**: Each user has their own encryption key
3. **Authenticated Encryption**: GCM mode provides both confidentiality and authenticity
4. **No Key Storage**: Encryption key is derived on-demand, never stored

## Commands

### Add Artifact Source

Store credentials for a private artifact repository:

#### Local Storage (Encrypted)

```bash
$ diffusion artifact add my-private-repo
Enter URL for my-private-repo: https://artifacts.example.com
Store credentials in Vault? (y/N): n
Enter Username: myuser
Enter Token/Password: ********
Credentials for 'my-private-repo' saved successfully (encrypted in ~/.diffusion/secrets/<role-name>/my-private-repo)
Added artifact source 'my-private-repo' to diffusion.toml
```

#### Vault Storage

```bash
$ diffusion artifact add vault-repo
Enter URL for vault-repo: https://vault-artifacts.example.com
Store credentials in Vault? (y/N): y
Enter Vault path for vault-repo (e.g., secret/data/artifacts): secret/data/prod
Enter Vault secret name for vault-repo: vault-repo
Enter Username Field in Vault (default: username): git_username
Enter Token Field in Vault (default: token): git_token
Artifact source 'vault-repo' configured to use Vault at secret/data/prod/vault-repo
Added artifact source 'vault-repo' to diffusion.toml
```

**Note**: The `artifact add` command automatically:
1. Saves credentials (locally encrypted or configures Vault)
2. Adds the artifact source to `diffusion.toml`
3. Updates existing sources if the name already exists

### List Artifact Sources

View all stored artifact sources:

```bash
$ diffusion artifact list
Stored Artifact Sources:
  ✓ my-private-repo - https://artifacts.example.com
  ✓ company-nexus - https://nexus.company.com
  ✓ github-packages - https://maven.pkg.github.com
```

### Show Artifact Details

Display details for a specific source (token is masked):

```bash
$ diffusion artifact show my-private-repo
Artifact Source: my-private-repo
URL: https://artifacts.example.com
Username: myuser
Token: abcd****************************xyz9
```

### Remove Artifact Source

Delete stored credentials and remove from configuration:

```bash
$ diffusion artifact remove my-private-repo
Local credentials for 'my-private-repo' removed successfully
Removed artifact source 'my-private-repo' from diffusion.toml
```

**Note**: The `artifact remove` command automatically:
1. Deletes local encrypted credentials (if they exist)
2. Removes the artifact source from `diffusion.toml`
3. Works for both local and Vault-based sources

## Configuration

### Local Storage (Default)

Credentials are automatically encrypted and stored locally:

```toml
[[artifact_sources]]
name = "my-private-repo"
url = "https://artifacts.example.com"
use_vault = false

[[artifact_sources]]
name = "company-nexus"
url = "https://nexus.company.com"
use_vault = false
```

When running molecule, these will be available as indexed environment variables:
- `GIT_USER_1`, `GIT_PASSWORD_1`, `GIT_URL_1` (for my-private-repo)
- `GIT_USER_2`, `GIT_PASSWORD_2`, `GIT_URL_2` (for company-nexus)

### Vault Integration

Configure Vault-backed credential storage with per-source field names:

```toml
[[artifact_sources]]
name = "my-private-repo"
url = "https://artifacts.example.com"
use_vault = true
vault_path = "secret/data/artifacts"
vault_secret_name = "my-private-repo"
vault_username_field = "username"  # Field name in Vault secret
vault_token_field = "token"        # Field name in Vault secret

[vault]
enabled = true
```

**Note**: Each artifact source can specify its own Vault field names, allowing different secrets to use different field naming conventions.

### Mixed Configuration

You can mix local and Vault storage:

```toml
[[artifact_sources]]
name = "dev-repo"
url = "https://dev.example.com"
use_vault = false  # Stored locally

[[artifact_sources]]
name = "prod-repo"
url = "https://prod.example.com"
use_vault = true  # Stored in Vault
vault_path = "secret/data/prod"
vault_secret_name = "artifacts"
vault_username_field = "git_username"  # Custom field names
vault_token_field = "git_token"

[vault]
enabled = true
```

## Environment Variables

When running molecule workflows, Diffusion automatically sets indexed environment variables for each configured artifact source:

### Variable Format

```bash
GIT_USER_1=user1
GIT_PASSWORD_1=token1
GIT_URL_1=https://repo1.example.com

GIT_USER_2=user2
GIT_PASSWORD_2=token2
GIT_URL_2=https://repo2.example.com

# ... up to GIT_*_10 (configurable via MaxArtifactSources)
```

### Using in Ansible/Molecule

Access these variables in your Ansible playbooks:

```yaml
---
- name: Clone from private repository
  ansible.builtin.git:
    repo: "{{ lookup('env', 'GIT_URL_1') }}"
    dest: /opt/myapp
    version: main
  environment:
    GIT_USERNAME: "{{ lookup('env', 'GIT_USER_1') }}"
    GIT_PASSWORD: "{{ lookup('env', 'GIT_PASSWORD_1') }}"
```

Or use in requirements.yml:

```yaml
---
roles:
  - name: my-private-role
    src: "{{ lookup('env', 'GIT_URL_1') }}/my-role.git"
    scm: git
    version: main
```

### Maximum Sources

By default, Diffusion supports up to 10 artifact sources (indexed 1-10). This can be adjusted by modifying the `MaxArtifactSources` constant.

## Usage Examples

### Example 1: GitHub Packages

```bash
# Add GitHub Packages credentials
$ diffusion artifact add github-packages
Enter URL for github-packages: https://maven.pkg.github.com/myorg
Enter Username: myusername
Enter Token/Password: ghp_xxxxxxxxxxxxxxxxxxxx

# Verify it's stored
$ diffusion artifact list
Stored Artifact Sources:
  ✓ github-packages - https://maven.pkg.github.com/myorg
```

### Example 2: Private Nexus Repository

```bash
# Add Nexus credentials
$ diffusion artifact add company-nexus
Enter URL for company-nexus: https://nexus.company.com/repository/maven-releases
Enter Username: john.doe
Enter Token/Password: my-secure-password

# Show details (token masked)
$ diffusion artifact show company-nexus
Artifact Source: company-nexus
URL: https://nexus.company.com/repository/maven-releases
Username: john.doe
Token: my-s****-password
```

### Example 3: Multiple Environments

```bash
# Development environment
$ diffusion artifact add dev-artifacts
Enter URL for dev-artifacts: https://dev-artifacts.company.com
Enter Username: dev-user
Enter Token/Password: dev-token

# Production environment
$ diffusion artifact add prod-artifacts
Enter URL for prod-artifacts: https://prod-artifacts.company.com
Enter Username: prod-user
Enter Token/Password: prod-token

# List all
$ diffusion artifact list
Stored Artifact Sources:
  ✓ dev-artifacts - https://dev-artifacts.company.com
  ✓ prod-artifacts - https://prod-artifacts.company.com
```

## File Structure

```
$HOME/
└── .diffusion/
    ├── my-private-repo_artifact_secrets
    ├── company-nexus_artifact_secrets
    └── github-packages_artifact_secrets
```

Each file contains encrypted JSON:
```json
{
  "name": "my-private-repo",
  "url": "https://artifacts.example.com",
  "username": "myuser",
  "token": "my-secret-token"
}
```

## API Usage

### Programmatic Access

```go
// Save credentials
creds := &ArtifactCredentials{
    Name:     "my-repo",
    URL:      "https://example.com",
    Username: "user",
    Token:    "token",
}
err := SaveArtifactCredentials(creds)

// Load credentials
creds, err := LoadArtifactCredentials("my-repo")

// List all sources
sources, err := ListStoredCredentials()

// Delete credentials
err := DeleteArtifactCredentials("my-repo")
```

### Vault Integration

```go
source := &ArtifactSource{
    Name:            "my-repo",
    URL:             "https://example.com",
    UseVault:        true,
    VaultPath:       "secret/data/artifacts",
    VaultSecretName: "my-repo",
}

vaultConfig := &HashicorpVault{
    HashicorpVaultIntegration: true,
    UserNameField:             "username",
    TokenField:                "token",
}

creds, err := GetArtifactCredentials(source, vaultConfig)
```

## Security Best Practices

1. **Use Strong Tokens**: Generate long, random tokens for artifact repositories
2. **Rotate Regularly**: Update credentials periodically
3. **Limit Permissions**: Use read-only tokens when possible
4. **Backup Carefully**: If backing up `~/.diffusion/`, ensure backups are encrypted
5. **Use Vault for Teams**: For shared environments, use Vault instead of local storage
6. **Audit Access**: Monitor artifact repository access logs

## Troubleshooting

### Cannot Decrypt Credentials

**Problem**: Error decrypting credentials after moving to a new machine.

**Solution**: Credentials are machine-specific. Re-add them on the new machine:
```bash
diffusion artifact remove old-source
diffusion artifact add old-source
```

### Permission Denied

**Problem**: Cannot read/write to `~/.diffusion/`

**Solution**: Check directory permissions:
```bash
chmod 700 ~/.diffusion
chmod 700 ~/.diffusion/secrets
chmod 700 ~/.diffusion/secrets/<role-name>
chmod 600 ~/.diffusion/secrets/<role-name>/*
```

### Vault Connection Failed

**Problem**: Cannot retrieve credentials from Vault.

**Solution**: 
1. Verify `VAULT_ADDR` and `VAULT_TOKEN` environment variables
2. Check Vault path and secret name in configuration
3. Ensure Vault token has read permissions for the secret path

## Migration

### From Single URL to Multiple Sources

Old configuration:
```toml
url = "https://artifacts.example.com"
```

New configuration:
```toml
[[artifact_sources]]
name = "primary"
url = "https://artifacts.example.com"
use_vault = false
```

The old `url` field is kept for backward compatibility.

### From Environment Variables

If you previously used environment variables for credentials:

```bash
# Old way
export ARTIFACT_USER=myuser
export ARTIFACT_TOKEN=mytoken

# New way
diffusion artifact add my-repo
# Enter credentials when prompted
```

## Testing

Run tests for artifact management:

```bash
# All artifact tests
go test -v -run TestArtifact

# Encryption tests
go test -v -run TestEncrypt

# Secrets tests
go test -v -run TestSecret
```

## Future Enhancements

- [ ] Support for SSH keys
- [ ] Integration with system keychains (macOS Keychain, Windows Credential Manager)
- [ ] Credential expiration and rotation reminders
- [ ] Import/export functionality (encrypted)
- [ ] Multi-factor authentication support
