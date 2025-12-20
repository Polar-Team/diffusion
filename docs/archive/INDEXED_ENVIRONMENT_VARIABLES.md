# Indexed Environment Variables for Multiple Artifact Sources

## Overview

Diffusion now supports multiple private artifact repositories with indexed environment variables. Each configured artifact source gets its own set of environment variables (GIT_USER_N, GIT_PASSWORD_N, GIT_URL_N) where N is the index (1-10).

## Changes from Previous Version

### Before (Single Source)
```bash
GIT_USER=myuser
GIT_PASSWORD=mytoken
GIT_URL=https://artifacts.example.com
```

### After (Multiple Sources)
```bash
GIT_USER_1=user1
GIT_PASSWORD_1=token1
GIT_URL_1=https://repo1.example.com

GIT_USER_2=user2
GIT_PASSWORD_2=token2
GIT_URL_2=https://repo2.example.com

GIT_USER_3=user3
GIT_PASSWORD_3=token3
GIT_URL_3=https://repo3.example.com
```

## Configuration

### Multiple Local Sources

```toml
[[artifact_sources]]
name = "github-packages"
url = "https://maven.pkg.github.com/myorg"
use_vault = false

[[artifact_sources]]
name = "company-nexus"
url = "https://nexus.company.com"
use_vault = false

[[artifact_sources]]
name = "private-gitlab"
url = "https://gitlab.company.com"
use_vault = false
```

### Mixed Local and Vault Sources

```toml
[[artifact_sources]]
name = "dev-artifacts"
url = "https://dev-artifacts.company.com"
use_vault = false  # Stored locally, encrypted

[[artifact_sources]]
name = "prod-artifacts"
url = "https://prod-artifacts.company.com"
use_vault = true  # Stored in HashiCorp Vault
vault_path = "secret/data/prod"
vault_secret_name = "artifacts"

[vault]
enabled = true
secret_kv2_path = "secret/data"
secret_kv2_name = "default"
username_field = "git_username"
token_field = "git_token"
```

## Usage in Molecule

### Accessing in Ansible Playbooks

```yaml
---
- name: Clone from first private repository
  ansible.builtin.git:
    repo: "{{ lookup('env', 'GIT_URL_1') }}"
    dest: /opt/app1
    version: main
  environment:
    GIT_USERNAME: "{{ lookup('env', 'GIT_USER_1') }}"
    GIT_PASSWORD: "{{ lookup('env', 'GIT_PASSWORD_1') }}"

- name: Clone from second private repository
  ansible.builtin.git:
    repo: "{{ lookup('env', 'GIT_URL_2') }}"
    dest: /opt/app2
    version: main
  environment:
    GIT_USERNAME: "{{ lookup('env', 'GIT_USER_2') }}"
    GIT_PASSWORD: "{{ lookup('env', 'GIT_PASSWORD_2') }}"
```

### Using in requirements.yml

```yaml
---
roles:
  - name: role-from-repo1
    src: "{{ lookup('env', 'GIT_URL_1') }}/my-role.git"
    scm: git
    version: main

  - name: role-from-repo2
    src: "{{ lookup('env', 'GIT_URL_2') }}/another-role.git"
    scm: git
    version: main

  - name: role-from-repo3
    src: "{{ lookup('env', 'GIT_URL_3') }}/third-role.git"
    scm: git
    version: develop
```

### Dynamic Repository Selection

```yaml
---
- name: Clone from repository based on environment
  ansible.builtin.git:
    repo: "{{ lookup('env', 'GIT_URL_' + repo_index|string) }}"
    dest: "/opt/app{{ repo_index }}"
    version: main
  environment:
    GIT_USERNAME: "{{ lookup('env', 'GIT_USER_' + repo_index|string) }}"
    GIT_PASSWORD: "{{ lookup('env', 'GIT_PASSWORD_' + repo_index|string) }}"
  vars:
    repo_index: 1
```

## CLI Workflow

### 1. Add Multiple Artifact Sources

```bash
# Add first source
$ diffusion artifact add github-packages
Enter URL for github-packages: https://maven.pkg.github.com/myorg
Enter Username: myusername
Enter Token/Password: ghp_xxxxxxxxxxxx

# Add second source
$ diffusion artifact add company-nexus
Enter URL for company-nexus: https://nexus.company.com
Enter Username: john.doe
Enter Token/Password: my-secure-token

# Add third source
$ diffusion artifact add private-gitlab
Enter URL for private-gitlab: https://gitlab.company.com
Enter Username: gitlab-user
Enter Token/Password: gitlab-token
```

### 2. Configure in diffusion.toml

```toml
[[artifact_sources]]
name = "github-packages"
url = "https://maven.pkg.github.com/myorg"
use_vault = false

[[artifact_sources]]
name = "company-nexus"
url = "https://nexus.company.com"
use_vault = false

[[artifact_sources]]
name = "private-gitlab"
url = "https://gitlab.company.com"
use_vault = false
```

### 3. Run Molecule

```bash
$ diffusion molecule --role my-role --org mycompany
# Diffusion automatically loads credentials and sets:
# GIT_USER_1, GIT_PASSWORD_1, GIT_URL_1 (github-packages)
# GIT_USER_2, GIT_PASSWORD_2, GIT_URL_2 (company-nexus)
# GIT_USER_3, GIT_PASSWORD_3, GIT_URL_3 (private-gitlab)
```

## Environment Variable Details

### Variable Naming Convention

- **Pattern**: `GIT_{TYPE}_{INDEX}`
- **Types**: USER, PASSWORD, URL
- **Index Range**: 1 to MaxArtifactSources (default: 10)

### Constants

```go
const (
    EnvGitUserPrefix   = "GIT_USER_"     // Prefix for username variables
    EnvGitPassPrefix   = "GIT_PASSWORD_" // Prefix for password variables
    EnvGitURLPrefix    = "GIT_URL_"      // Prefix for URL variables
    MaxArtifactSources = 10              // Maximum number of sources
)
```

### Docker Container

All indexed variables are automatically passed to the molecule container:

```bash
docker run ... \
  -e GIT_USER_1=user1 \
  -e GIT_PASSWORD_1=pass1 \
  -e GIT_URL_1=https://repo1.example.com \
  -e GIT_USER_2=user2 \
  -e GIT_PASSWORD_2=pass2 \
  -e GIT_URL_2=https://repo2.example.com \
  ...
```

## Backward Compatibility

### Legacy Single Source

Old configuration still works:

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

### Migration Path

1. **Keep existing config** - It will work as `GIT_*_1`
2. **Add new sources** - They become `GIT_*_2`, `GIT_*_3`, etc.
3. **Update playbooks** - Change from `GIT_USER` to `GIT_USER_1` when ready

## Security Considerations

### Local Storage
- Each source's credentials encrypted separately
- Machine-specific encryption keys
- File permissions: 0600 (owner only)
- Location: `~/.diffusion/secrets/<role-name>/<source-name>`

### Vault Storage
- Centralized secret management
- Per-source Vault paths
- Configurable field names
- Audit logging (Vault feature)

### Environment Variables
- Only set when running molecule
- Passed securely to Docker container
- Not logged or displayed
- Cleared after container creation

## Troubleshooting

### Variables Not Set

**Problem**: Environment variables not appearing in container

**Solution**: 
1. Verify artifact sources are configured in `diffusion.toml`
2. Check credentials are stored: `diffusion artifact list`
3. Ensure source names match configuration
4. Check logs for credential loading errors

### Wrong Index

**Problem**: Variables at wrong index (e.g., expecting GIT_*_2 but getting GIT_*_3)

**Solution**: Index is based on order in `diffusion.toml`. Reorder `[[artifact_sources]]` sections.

### Vault Connection Failed

**Problem**: Cannot load credentials from Vault

**Solution**:
1. Check `VAULT_ADDR` and `VAULT_TOKEN` environment variables
2. Verify `vault_path` and `vault_secret_name` in configuration
3. Ensure Vault token has read permissions
4. Test Vault connection: `vault kv get <path>/<secret>`

### Mixed Sources Not Working

**Problem**: Some sources load, others don't

**Solution**:
1. Check each source individually: `diffusion artifact show <name>`
2. Verify Vault configuration for Vault-backed sources
3. Check file permissions for local sources
4. Review logs for specific error messages

## Testing

### Verify Environment Variables

Create a test playbook:

```yaml
---
- name: Test environment variables
  hosts: localhost
  tasks:
    - name: Display all GIT variables
      ansible.builtin.debug:
        msg: |
          GIT_USER_1: {{ lookup('env', 'GIT_USER_1') }}
          GIT_URL_1: {{ lookup('env', 'GIT_URL_1') }}
          GIT_USER_2: {{ lookup('env', 'GIT_USER_2') }}
          GIT_URL_2: {{ lookup('env', 'GIT_URL_2') }}
```

### Run Tests

```bash
# Test credential storage
go test -v -run TestMultipleArtifactSourcesSimulation

# Test environment variables
go test -v -run TestIndexedEnvironmentVariables

# Test maximum sources
go test -v -run TestMaxArtifactSources
```

## Best Practices

1. **Naming Convention**: Use descriptive source names (e.g., `github-packages`, `company-nexus`)
2. **Order Matters**: Index is based on order in config file
3. **Document Indices**: Comment which index each source uses
4. **Limit Sources**: Only configure sources you actually use
5. **Rotate Credentials**: Update tokens regularly using `diffusion artifact add`
6. **Use Vault for Production**: Local storage for development, Vault for production
7. **Test Locally First**: Verify credentials work before adding to CI/CD

## Example: Complete Setup

```toml
# Development repositories (local storage)
[[artifact_sources]]
name = "dev-github"
url = "https://github.com/myorg"
use_vault = false

[[artifact_sources]]
name = "dev-gitlab"
url = "https://gitlab.company.com"
use_vault = false

# Production repositories (Vault storage)
[[artifact_sources]]
name = "prod-nexus"
url = "https://nexus.prod.company.com"
use_vault = true
vault_path = "secret/data/prod"
vault_secret_name = "nexus"

[[artifact_sources]]
name = "prod-artifactory"
url = "https://artifactory.prod.company.com"
use_vault = true
vault_path = "secret/data/prod"
vault_secret_name = "artifactory"

[vault]
enabled = true
username_field = "username"
token_field = "token"
```

This configuration provides:
- `GIT_*_1`: dev-github (local)
- `GIT_*_2`: dev-gitlab (local)
- `GIT_*_3`: prod-nexus (Vault)
- `GIT_*_4`: prod-artifactory (Vault)
