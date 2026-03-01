# Diffusion GitHub Actions

Reusable GitHub Actions workflow for testing Ansible roles with Diffusion and Molecule.

## Quick Start

Add this to your role repository as `.github/workflows/test.yml`:

```yaml
---
name: Test

on:
  push:
    branches:
      - main
  pull_request:
    branches:
      - main

jobs:
  test:
    name: Molecule Tests
    uses: Polar-Team/diffusion/actions/diffusion-test.yml@main
    with:
      diffusion-version: 'latest'
      run-lint: true
      run-verify: true
      run-idempotence: true
```

That's it! This will run:
- ✅ Molecule converge
- ✅ Ansible and YAML linting
- ✅ Verification tests
- ✅ Idempotence tests

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `diffusion-version` | Diffusion version to use (e.g., `latest`, `v0.3.13`) | No | `latest` |
| `run-lint` | Run lint tests | No | `true` |
| `run-verify` | Run verify tests | No | `true` |
| `run-idempotence` | Run idempotence tests | No | `true` |
| `run-converge` | Run converge (always runs before other tests) | No | `true` |
| `working-directory` | Working directory for the role | No | `.` |
| `cache-enabled` | Enable caching for Ansible roles, collections, and optionally UV/Docker | No | `false` |
| `cache-uv` | Cache UV/Python packages (requires `cache-enabled: true`) | No | `false` |
| `cache-docker` | Cache DinD Docker images as tarballs (requires `cache-enabled: true`) | No | `false` |

## Examples

### Basic Usage

Run all tests with latest Diffusion version:

```yaml
jobs:
  test:
    uses: Polar-Team/diffusion/actions/diffusion-test.yml@main
    with:
      diffusion-version: 'latest'
      run-lint: true
      run-verify: true
      run-idempotence: true
```

### Skip Idempotence Test

Run only lint and verify tests:

```yaml
jobs:
  test:
    uses: Polar-Team/diffusion/actions/diffusion-test.yml@main
    with:
      run-lint: true
      run-verify: true
      run-idempotence: false
```

### Pin to Specific Version

Use a specific Diffusion version for reproducible builds:

```yaml
jobs:
  test:
    uses: Polar-Team/diffusion/actions/diffusion-test.yml@main
    with:
      diffusion-version: 'v0.3.13'
      run-lint: true
      run-verify: true
      run-idempotence: true
```

### Multi-Role Repository

Test multiple roles in a monorepo:

```yaml
jobs:
  test-role1:
    uses: Polar-Team/diffusion/actions/diffusion-test.yml@main
    with:
      working-directory: 'roles/role1'
      run-lint: true
      run-verify: true
      run-idempotence: true

  test-role2:
    uses: Polar-Team/diffusion/actions/diffusion-test.yml@main
    with:
      working-directory: 'roles/role2'
      run-lint: true
      run-verify: true
      run-idempotence: true
```

### Conditional Testing

Run different tests based on branch:

```yaml
jobs:
  quick-test:
    name: Quick Tests (PR)
    if: github.event_name == 'pull_request'
    uses: Polar-Team/diffusion/actions/diffusion-test.yml@main
    with:
      run-lint: true
      run-verify: true
      run-idempotence: false

  full-test:
    name: Full Test Suite (Main)
    if: github.ref == 'refs/heads/main'
    uses: Polar-Team/diffusion/actions/diffusion-test.yml@main
    with:
      run-lint: true
      run-verify: true
      run-idempotence: true
```

### Caching

Enable caching to speed up repeated CI runs. Cached items are persisted across
workflow runs using `actions/cache` and restored automatically. Ansible roles
and collections are always cached when caching is enabled; UV and Docker caches
are opt-in.

```yaml
jobs:
  test:
    name: Molecule Tests (cached)
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Run Diffusion Tests
        uses: Polar-Team/diffusion/diffusion-test@main
        with:
          cache-enabled: 'true'
          cache-uv: 'true'
          cache-docker: 'true'
          run-lint: 'true'
          run-verify: 'true'
          run-idempotence: 'true'
```

**What gets cached:**

| Type | Description | Flag |
|------|-------------|------|
| Ansible roles | Downloaded roles from `requirements.yml` | Always (when cache is enabled) |
| Ansible collections | Downloaded collections from `requirements.yml` | Always (when cache is enabled) |
| UV packages | Python packages installed via UV | `cache-uv: true` |
| Docker images | DinD images pulled inside the molecule container | `cache-docker: true` |

The cache key is derived from `diffusion.toml`, `requirements.yml`, and
`diffusion.lock` so that dependency changes automatically invalidate the cache.

## Requirements

Your role repository must have:

1. **diffusion.toml** - Diffusion configuration file
2. **meta/main.yml** - Role metadata
3. **scenarios/default/** - Molecule scenario with:
   - `molecule.yml` - Molecule configuration
   - `converge.yml` - Convergence playbook
   - `verify.yml` - Verification tests (if using `run-verify: true`)
   - `requirements.yml` - Role/collection dependencies (optional)

## Example Role Structure

```
my-role/
├── .github/
│   └── workflows/
│       └── test.yml          # Your workflow file
├── defaults/
│   └── main.yml
├── tasks/
│   └── main.yml
├── meta/
│   └── main.yml              # Required
├── scenarios/
│   └── default/
│       ├── molecule.yml      # Required
│       ├── converge.yml      # Required
│       ├── verify.yml        # Required for verify tests
│       └── requirements.yml  # Optional
├── diffusion.toml            # Required
└── README.md
```

## Minimal diffusion.toml

```toml
[container_registry]
registry_server = "ghcr.io"
registry_provider = "Public"
molecule_container_name = "polar-team/diffusion-molecule-container"
molecule_container_tag = "latest-amd64"

[vault]
enabled = false

[yaml_lint]
extends = "default"
ignore = [".git/*", "molecule/**"]

[ansible_lint]
exclude_paths = ["molecule/", "tests/"]

[tests]
type = "local"

[cache]
enabled = false

[dependencies]
ansible = ">=13.0.0"
molecule = ">=24.0.0"
ansible_lint = ">=24.0.0"

[dependencies.python]
min = "3.11"
max = "3.13"
pinned = "3.13"
```

## What Happens During Testing

1. **Checkout** - Checks out your role repository
2. **Install Diffusion** - Downloads and installs Diffusion binary
3. **Configure cache** (if enabled) - Runs `diffusion cache enable` and restores cached artifacts from previous runs
4. **Converge** - Applies your role to test instance
5. **Lint** (optional) - Runs yamllint and ansible-lint
6. **Verify** (optional) - Runs verification tests
7. **Idempotence** (optional) - Tests role idempotence
8. **Cleanup** - Destroys test instances, saves cache artifacts, and cleans up

## Test Output

The workflow provides:
- Grouped output for each test phase
- Summary table with test results
- Clear pass/fail indicators

Example summary:

```
## Diffusion Test Results 🧪

| Test | Status |
|------|--------|
| Converge | ✅ Passed |
| Lint | ✅ Passed |
| Verify | ✅ Passed |
| Idempotence | ✅ Passed |
```

## Troubleshooting

### Tests Fail in CI but Pass Locally

Make sure you're using `--ci` flag when testing locally:
```bash
diffusion molecule --ci --converge
diffusion molecule --ci --verify
```

### Container Pull Errors

The workflow uses the public Diffusion container from `ghcr.io`. Make sure your `diffusion.toml` is configured correctly:

```toml
[container_registry]
registry_server = "ghcr.io"
registry_provider = "Public"
molecule_container_name = "polar-team/diffusion-molecule-container"
molecule_container_tag = "latest-amd64"
```

### Verify Tests Not Running

Make sure you have `scenarios/default/verify.yml` in your role and `run-verify: true` in your workflow.

### Idempotence Tests Failing

Idempotence tests ensure your role can be run multiple times without changes. Common issues:
- Tasks that always report "changed" status
- Non-idempotent commands or shell tasks
- Timestamp or random value generation

## More Examples

See the [examples/](examples/) directory for more workflow examples:
- [basic-test.yml](examples/basic-test.yml) - Basic usage
- [cached-test.yml](examples/cached-test.yml) - Caching enabled
- [custom-test.yml](examples/custom-test.yml) - Custom test selection
- [multi-role-test.yml](examples/multi-role-test.yml) - Multi-role testing
- [version-pinned-test.yml](examples/version-pinned-test.yml) - Version pinning

## Documentation

- [Diffusion Documentation](https://github.com/Polar-Team/diffusion)
- [Dependency Management](../../docs/DEPENDENCY_MANAGEMENT.md)
- [Molecule Documentation](https://molecule.readthedocs.io/)

## Support

For issues or questions:
- [GitHub Issues](https://github.com/Polar-Team/diffusion/issues)
- [Diffusion Repository](https://github.com/Polar-Team/diffusion)

---

**Made with ❤️ by Polar-Team**
