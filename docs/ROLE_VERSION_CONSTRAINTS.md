# Role Version Constraints

This document describes how role version constraints work in diffusion, similar to collection version constraints.

## Overview

Roles now support version constraints across three files:
- **diffusion.toml** - stores version constraints (e.g., `>=1.0.0`)
- **diffusion.lock** - stores both constraints and resolved versions
- **requirements.yml** - stores resolved versions (e.g., `1.2.3`)

This approach ensures:
1. Version constraints are defined once in `diffusion.toml`
2. Resolved versions are locked in `diffusion.lock` for reproducibility
3. Requirements files use resolved versions for installation

## File Structure

### diffusion.toml
```toml
[dependencies]
[[dependencies.roles]]
name = "default.geerlingguy.docker"
src = "https://github.com/geerlingguy/ansible-role-docker.git"
scm = "git"
version = ">=6.0.0"  # Version constraint
```

### diffusion.lock
```yaml
roles:
  - name: geerlingguy.docker
    version: ">=6.0.0"           # Constraint from diffusion.toml
    resolved_version: "6.1.0"    # Actual resolved version
    type: role
    source: galaxy
    src: https://github.com/geerlingguy/ansible-role-docker.git
    scm: git
```

### requirements.yml
```yaml
roles:
  - name: geerlingguy.docker
    src: https://github.com/geerlingguy/ansible-role-docker.git
    scm: git
    version: "6.1.0"  # Resolved version from lock file
```

## Adding a Role

When adding a role with `diffusion role add-role`:

```bash
diffusion role add-role geerlingguy.docker \
  --src https://github.com/geerlingguy/ansible-role-docker.git \
  --version ">=6.0.0" \
  --scenario default
```

The command will:
1. Resolve the latest version from Galaxy (e.g., `6.1.0`)
2. Add to `requirements.yml` with resolved version `6.1.0`
3. Add to `diffusion.toml` with constraint `>=6.0.0`

If no version is specified:
```bash
diffusion role add-role geerlingguy.docker \
  --src https://github.com/geerlingguy/ansible-role-docker.git \
  --scenario default
```

The command will:
1. Resolve the latest version from Galaxy (e.g., `6.1.0`)
2. Add to `requirements.yml` with resolved version `6.1.0`
3. Add to `diffusion.toml` with constraint `>=6.1.0` (automatically generated)

## Version Constraint Logic

The version constraint logic follows these rules:

| Input Constraint | Resolved Version | Constraint in diffusion.toml |
|-----------------|------------------|------------------------------|
| (empty)         | 1.2.3            | >=1.2.3                      |
| latest          | 1.2.3            | >=1.2.3                      |
| main            | 1.2.3            | >=1.2.3                      |
| >=1.0.0         | 1.2.3            | >=1.0.0                      |
| ==1.0.0         | 1.0.0            | ==1.0.0                      |
| <=2.0.0         | 2.0.0            | <=2.0.0                      |

## Data Types

### RoleRequirement
Used in `diffusion.toml` to store version constraints:
```go
type RoleRequirement struct {
    Name    string `yaml:"name"`
    Src     string `yaml:"src,omitempty"`
    Scm     string `yaml:"scm,omitempty"`
    Version string `yaml:"version,omitempty"` // e.g., ">=1.0.0"
}
```

### RequirementRole
Used in `requirements.yml` to store resolved versions:
```go
type RequirementRole struct {
    Name    string `yaml:"name"`
    Src     string `yaml:"src,omitempty"`
    Version string `yaml:"version,omitempty"` // e.g., "1.2.3"
    Scm     string `yaml:"scm,omitempty"`
}
```

## Dependency Resolution

The `ResolveRoleDependencies()` method:
1. Loads roles from `requirements.yml` (resolved versions)
2. Overrides with constraints from `diffusion.toml` (takes precedence)
3. Returns `[]RoleRequirement` with constraints for lock file generation

## Lock File Generation

When generating the lock file:
1. Uses version constraints from `diffusion.toml`
2. Resolves actual versions from Galaxy API or Git repository
3. Stores both constraint and resolved version in lock file

### Git Version Constraint Resolution
When resolving versions from git repositories with constraints:
- Fetches all tags using `git ls-remote`
- Compares each tag against the version constraint
- Returns the latest tag that satisfies the constraint
- Supports all comparison operators: `>=`, `<=`, `==`, `>`, `<`

**Examples:**
- `>=6.0.0` → Resolves to latest tag >= 6.0.0 (e.g., `v7.9.0`)
- `<=7.0.0` → Resolves to latest tag <= 7.0.0 (e.g., `v7.0.0`)
- `==6.0.0` → Resolves to exact tag 6.0.0 (e.g., `v6.0.0`)
- `latest` or empty → Resolves to the absolute latest tag

## Syncing Dependencies

The `deps sync` command:
1. Reads resolved versions from `diffusion.lock`
2. Updates `requirements.yml` with resolved versions
3. Preserves constraints in `diffusion.toml`

## Example Workflow

1. **Add a role with constraint:**
   ```bash
   diffusion role add-role geerlingguy.docker \
     --src https://github.com/geerlingguy/ansible-role-docker.git \
     --version ">=6.0.0"
   ```

2. **Generate lock file:**
   ```bash
   diffusion deps lock
   ```
   This resolves `>=6.0.0` to actual version `6.1.0`

3. **Sync dependencies:**
   ```bash
   diffusion deps sync
   ```
   This updates `requirements.yml` with resolved version `6.1.0`

4. **Install dependencies:**
   ```bash
   ansible-galaxy install -r requirements.yml
   ```
   This installs the exact version `6.1.0`

## Benefits

1. **Reproducibility**: Lock file ensures same versions across environments
2. **Flexibility**: Constraints allow updates within specified ranges
3. **Clarity**: Separation of constraints (toml) and resolved versions (yml)
4. **Consistency**: Same pattern as collections for easier understanding
