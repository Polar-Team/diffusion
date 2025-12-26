# Dependency Management

Complete guide to Diffusion's dependency management system for Python, Ansible tools, and collections.

## Table of Contents
- [Overview](#overview)
- [Quick Start](#quick-start)
- [Python Version Management](#python-version-management)
- [Tool Version Management](#tool-version-management)
- [Commands](#commands)
- [Lock File System](#lock-file-system)
- [Version Compatibility](#version-compatibility)
- [Configuration](#configuration)
- [Troubleshooting](#troubleshooting)

## Overview

Diffusion provides a comprehensive dependency management system that:
- **Validates Python versions** - Only allows tested versions (3.13.11, 3.12.10, 3.11.9)
- **Resolves tool versions** - Fetches actual versions from PyPI based on constraints
- **Manages collections** - Resolves Ansible Galaxy collection versions
- **Generates lock files** - Creates reproducible dependency snapshots
- **Passes to container** - Dynamically generates and passes pyproject.toml to container

## Quick Start

### 1. Initialize Dependencies
```bash
# Initialize dependency configuration in diffusion.toml
diffusion deps init
```

### 2. Configure Dependencies
Edit `diffusion.toml`:
```toml
[dependencies]
ansible = ">=13.0.0"
molecule = ">=24.0.0"
ansible_lint = ">=24.0.0"

[dependencies.python]
pinned = "3.13"  # Will expand to 3.13.11
min = "3.11"
max = "3.13"
```

### 3. Generate Lock File
```bash
# Resolve and lock all dependencies
diffusion deps lock
```

This queries PyPI and Galaxy to resolve:
- `ansible>=13.0.0` → `13.1.0`
- `molecule>=24.0.0` → `24.12.0`
- `community.general>=7.4.0` → `7.5.0`

### 4. View Resolved Versions
```bash
diffusion deps resolve
```

Output:
```
=== Resolved Dependencies ===

Python:
  Pinned: 3.13.11
  Min: 3.11 (major.minor)
  Max: 3.13 (major.minor)

Tools:
  ansible: 13.1.0 (constraint: >=13.0.0)
  molecule: 24.12.0 (constraint: >=24.0.0)
  ansible-lint: 24.12.0 (constraint: >=24.0.0)

Collections:
  community.general: 7.5.0 (constraint: >=7.4.0)
```

### 5. Run Molecule
```bash
# Container automatically uses versions from lock file
diffusion molecule --role myrole --org myorg
```

## Python Version Management

### Allowed Versions
Only three Python versions are supported:
- **3.13.11** (latest)
- **3.12.10**
- **3.11.9**

### Version Format Rules

#### Pinned Version (Full)
The `pinned` field must be a full version:
```toml
[dependencies.python]
pinned = "3.13.11"  # Full: major.minor.patch
```

#### Min/Max Versions (Major.Minor)
The `min` and `max` fields show only major.minor:
```toml
[dependencies.python]
min = "3.11"  # Major.minor only
max = "3.13"  # Major.minor only
```

#### Additional Versions (Not Used)
The `additional` field is automatically cleared and not passed to container.

### Automatic Normalization

The system automatically normalizes versions:

| Input | Normalized To |
|-------|---------------|
| `pinned = "3.13"` | `pinned = "3.13.11"` |
| `pinned = "3.12"` | `pinned = "3.12.10"` |
| `pinned = "3.11"` | `pinned = "3.11.9"` |
| `min = "3.11.9"` | `min = "3.11"` |
| `max = "3.13.11"` | `max = "3.13"` |

### Validation

Python versions are validated when:
1. Loading dependency configuration from `diffusion.toml`
2. Generating lock file with `diffusion deps lock`
3. Running molecule commands

**Invalid version example:**
```toml
[dependencies.python]
pinned = "3.10.5"  # ERROR
```

Error message:
```
invalid pinned Python version: Python version 3.10.5 is not allowed. 
Allowed versions: 3.13.11, 3.12.10, 3.11.9 (or 3.13, 3.12, 3.11)
```

### Container Environment

Only the **pinned version** is passed to container:
```bash
docker run -e PYTHON_PINNED_VERSION=3.13.11 ...
```

The container uses this to:
1. Set Python version in `.python-version` file
2. Create UV virtual environment with correct Python
3. Install all dependencies with specified Python version

## Tool Version Management

### Supported Tools
- **ansible** - Ansible automation platform
- **molecule** - Ansible testing framework
- **ansible-lint** - Ansible linting tool
- **yamllint** - YAML linting tool

### Version Constraints

Specify version constraints in `diffusion.toml`:
```toml
[dependencies]
ansible = ">=13.0.0"      # Minimum version
molecule = ">=24.0.0"     # Minimum version
ansible_lint = ">=24.0.0" # Minimum version
yamllint = ">=1.35.0"     # Minimum version
```

Supported operators:
- `>=` - Greater than or equal
- `<=` - Less than or equal
- `==` - Exact version
- `>` - Greater than
- `<` - Less than

### Version Resolution

When you run `diffusion deps lock`, the system:
1. Reads constraints from `diffusion.toml`
2. Queries PyPI for each tool
3. Resolves to latest version satisfying constraint
4. Stores both constraint and resolved version in lock file

**Example:**
```toml
# diffusion.toml
ansible = ">=13.0.0"
```

Resolves to:
```yaml
# diffusion.lock
tools:
  - name: ansible
    version: ">=13.0.0"      # Original constraint
    resolved_version: "13.1.0" # Actual version from PyPI
```

### Molecule from PyPI

Molecule is installed from PyPI, not from GitHub:
```toml
[project]
dependencies = [
    "ansible>=13.0.0",
    "molecule>=24.0.0",  # From PyPI
]
```

**Benefits:**
- Faster installation (no git clone)
- More reliable (PyPI availability)
- Standard Python packaging
- Specific versions, not git branches

## Commands

### `diffusion deps init`
Initialize dependency configuration in `diffusion.toml`:
```bash
diffusion deps init
```

Creates default configuration:
```toml
[dependencies]
ansible = ">=10.0.0"
molecule = ">=24.0.0"
ansible_lint = ">=24.0.0"
yamllint = ">=1.35.0"

[dependencies.python]
min = "3.11"
max = "3.13"
pinned = "3.13.11"
```

### `diffusion deps lock`
Generate or update `diffusion.lock` file:
```bash
diffusion deps lock
```

This:
1. Reads dependencies from `diffusion.toml`, `meta/main.yml`, `requirements.yml`
2. Queries PyPI for tool versions
3. Queries Galaxy for collection versions
4. Resolves Python dependencies for collections
5. Generates `diffusion.lock` with resolved versions

### `diffusion deps resolve`
Display resolved dependencies from lock file:
```bash
diffusion deps resolve
```

Shows:
- Python versions (pinned, min, max)
- Tool versions (resolved + constraint)
- Collection versions (resolved + constraint)
- Role versions (if any)

### `diffusion deps check`
Check if lock file is up-to-date:
```bash
diffusion deps check
```

Exits with:
- `0` - Lock file is up-to-date
- `1` - Lock file is out of date (run `diffusion deps lock`)

## Lock File System

### Structure

```yaml
version: "1.0"
generated: "2024-12-26T10:00:00Z"
hash: "abc123..."

python:
  min: "3.11"
  max: "3.13"
  pinned: "3.13.11"

tools:
  - name: ansible
    version: ">=13.0.0"
    resolved_version: "13.1.0"
    type: tool
    source: pypi
  - name: molecule
    version: ">=24.0.0"
    resolved_version: "24.12.0"
    type: tool
    source: pypi

collections:
  - name: community.general
    version: ">=7.4.0"
    resolved_version: "7.5.0"
    type: collection
    source: galaxy
    python_deps:
      docker: "7.1.0"
      requests: "2.32.3"

roles:
  - name: geerlingguy.docker
    version: ">=7.0.0"
    resolved_version: "7.4.1"
    type: role
    source: galaxy
```

### Fields

- **version**: Lock file format version
- **generated**: Timestamp of generation
- **hash**: Hash of all dependencies for validation
- **python**: Python version configuration
- **tools**: Python tools with resolved versions
- **collections**: Ansible collections with resolved versions
- **roles**: Ansible roles with resolved versions

### Validation

The lock file hash is computed from:
- All collection names and versions
- All role names and versions
- All tool names and versions
- Python version configuration

When you run `diffusion deps check`, it recomputes the hash and compares with the stored hash.

## Version Compatibility

### Python-Tool Compatibility Matrix

Diffusion automatically validates tool compatibility with Python versions:

| Python | Ansible | Molecule | ansible-lint |
|--------|---------|----------|--------------|
| 3.13   | 8-13    | 5-25     | 6-24         |
| 3.12   | 8-13    | 5-25     | 6-24         |
| 3.11   | 8-13    | 5-25     | 6-24         |

### Automatic Adjustment

If you specify incompatible versions, Diffusion warns and adjusts:

**Example:**
```toml
[dependencies.python]
pinned = "3.9"  # Old version

[dependencies]
ansible = ">=13.0.0"  # Requires Python 3.12+
```

Warning:
```
⚠ Python 3.9 is not compatible with Ansible 13.x (requires Python 3.12+)
⚠ Adjusting Ansible to version 9.x (compatible with Python 3.9)
```

Adjusted:
```yaml
tools:
  - name: ansible
    version: ">=9.0.0"  # Adjusted
    resolved_version: "9.12.0"
```

### Compatibility Functions

- `ValidateToolCompatibility()` - Checks if tool version is compatible with Python
- `GetCompatibleVersion()` - Returns compatible version range for tool
- `AdjustToolVersionsForPython()` - Adjusts tool versions for Python compatibility

## Configuration

### diffusion.toml

Complete example:
```toml
[dependencies]
ansible = ">=13.0.0"
molecule = ">=24.0.0"
ansible_lint = ">=24.0.0"
yamllint = ">=1.35.0"

[dependencies.python]
min = "3.11"
max = "3.13"
pinned = "3.13.11"

# Collections can also be specified here
[[dependencies.collections]]
name = "community.general"
version = ">=7.4.0"

[[dependencies.collections]]
name = "community.docker"
version = ">=3.0.0"
```

### meta/main.yml

Collections in meta file are also resolved:
```yaml
galaxy_info:
  # ...

dependencies: []

collections:
  - community.general>=7.4.0
  - community.docker>=3.0.0
```

### requirements.yml

Roles and collections in requirements:
```yaml
---
roles:
  - name: geerlingguy.docker
    version: ">=7.0.0"

collections:
  - name: community.general
    version: ">=7.4.0"
```

### Priority

When the same dependency is specified in multiple places:
1. `diffusion.toml` (highest priority)
2. `requirements.yml`
3. `meta/main.yml` (lowest priority)

## Troubleshooting

### Lock file out of date
```bash
# Regenerate lock file
diffusion deps lock
```

### Invalid Python version
```
Error: invalid pinned Python version: Python version 3.10.5 is not allowed
```

**Solution:** Use one of the allowed versions (3.13.11, 3.12.10, 3.11.9):
```toml
[dependencies.python]
pinned = "3.13"  # Will expand to 3.13.11
```

### Tool version incompatible with Python
```
Warning: Python 3.11 is not compatible with Ansible 13.x
```

**Solution:** Either upgrade Python or downgrade tool:
```toml
# Option 1: Upgrade Python
[dependencies.python]
pinned = "3.13"

# Option 2: Downgrade Ansible
[dependencies]
ansible = ">=10.0.0"
```

### PyPI query failed
```
Warning: Failed to resolve version for ansible: PyPI returned status 404
```

**Solution:** Check internet connection and PyPI availability. The system will use the constraint as-is if resolution fails.

### Collection not found on Galaxy
```
Warning: Failed to resolve version for community.unknown: not found
```

**Solution:** Verify collection name is correct on [Ansible Galaxy](https://galaxy.ansible.com/).

## Workflow

### Development Workflow

```bash
# 1. Initialize new role
diffusion role --init

# 2. Initialize dependencies
diffusion deps init

# 3. Configure dependencies in diffusion.toml
nvim diffusion.toml

# 4. Generate lock file
diffusion deps lock

# 5. View resolved versions
diffusion deps resolve

# 6. Run tests
diffusion molecule --converge

# 7. Update dependencies (when needed)
# Edit diffusion.toml, then:
diffusion deps lock
```

### CI/CD Workflow

```yaml
# .github/workflows/test.yml
name: Test
on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Check lock file
        run: diffusion deps check
      
      - name: Run tests
        run: diffusion molecule --ci --converge
```

### Migration from Old Versions

If you're upgrading from an older version:

1. **Remove old pyproject.toml** (if exists in container project)
2. **Initialize dependencies:**
   ```bash
   diffusion deps init
   ```
3. **Generate lock file:**
   ```bash
   diffusion deps lock
   ```
4. **Update Python version** (if using 3.10 or earlier):
   ```toml
   [dependencies.python]
   pinned = "3.11"  # or "3.12" or "3.13"
   ```

## Benefits

1. **Reproducibility** - Lock file ensures same versions everywhere
2. **Validation** - Only tested Python versions allowed
3. **Transparency** - See both constraints and resolved versions
4. **Simplicity** - No manual pyproject.toml management
5. **Compatibility** - Automatic validation and adjustment
6. **Speed** - Cached resolution results in lock file

## Related Documentation

- [Version Compatibility System](VERSION_COMPATIBILITY_COMPLETE.md) - Detailed compatibility matrix
- [Main README](../README.md) - General Diffusion documentation
- [Building Guide](building.md) - Building from source

---

**Last Updated:** December 26, 2024  
**Version:** 1.0
