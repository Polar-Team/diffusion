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
| `cache-key-suffix` | Extra disambiguation appended to the cache key and restore prefixes (e.g. a matrix value). Sanitized to `[a-z0-9._-]` | No | `""` |

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

#### Cache keys

Every job gets its own cache entry, scoped by runner platform, role and Molecule
scenario. The key has the form:

```
diffusion-cache-<os>-<arch>-<role-slug>-<scenario>[-<suffix>]-<deps-hash>

# working-directory ".", scenario default
diffusion-cache-linux-x64-root-cdb4ee2a-default-0c4e4200a63646ca
# working-directory ".", scenario ubuntu
diffusion-cache-linux-x64-root-cdb4ee2a-ubuntu-9b8cb9cb19f7e167
```

| Segment | Source | Example |
|---------|--------|---------|
| `<os>` | `runner.os`, lowercased | `linux` |
| `<arch>` | `runner.arch`, lowercased | `x64` |
| `<role-slug>` | `working-directory`, sanitized, plus 8 hex chars of the raw path | `.` → `root-cdb4ee2a`, `roles/foo` → `roles-foo-e4a7b971` |
| `<scenario>` | `scenario` input, sanitized | `default`, `ubuntu` |
| `<suffix>` | `cache-key-suffix` input, sanitized (omitted when empty) | `image-2404` |
| `<deps-hash>` | 16 hex chars of sha256 over the dependency inputs | `0c4e4200a63646ca` |

`<deps-hash>` covers, in this order and skipping missing files:
`scenarios/<scenario>/requirements.yml`, `requirements.yml`, `diffusion.lock`,
`meta/main.yml`, `diffusion.toml` (with the `cache_id` line removed), and — only
when `cache-uv: true` — `pyproject.toml` and `uv.lock`.

Restore falls back progressively but always stays scoped:

1. `diffusion-cache-<os>-<arch>-<role>-<scenario>-` — same role + scenario, older dependency hash
2. `diffusion-cache-<os>-<arch>-<role>-` — same role, any scenario (roles/collections are shareable)

It never falls back to a bare `diffusion-cache-` prefix, so a job can no longer
restore an unrelated role's cache.

The save step is skipped when the restore was an exact hit (nothing new to
write), and `always()` keeps it running when tests fail. Because the key is
unique per OS/arch + role + scenario + dependency set, concurrent matrix jobs no
longer fail with `Unable to reserve cache with key ...`.

#### Cached path

The action caches whatever `diffusion cache path` reports — the absolute role
cache directory (for example `/home/runner/.diffusion/cache/role_<cache_id>`).
It uses the same resolver Molecule uses, so `[cache] cache_path` in
`diffusion.toml` is honoured (only if that directory exists; otherwise Diffusion
falls back to `~/.diffusion/cache` and the action emits a `::warning::`). The
role-specific directory is cached rather than the whole `~/.diffusion/cache`, so
entries stay small and scoped to the role. The same path is used for both
restore and save (the path is part of the `actions/cache` version). Older
Diffusion versions without `cache path` fall back to `~/.diffusion/cache` with a
notice.

#### Deterministic `cache_id`

Diffusion keeps cache data in `<cache dir>/role_<cache_id>/`. If
`diffusion.toml` has no `cache_id` (or an empty one), `diffusion cache enable`
generates a fresh random 16-hex ID on every run, which would make restored data
invisible.

The action checks `diffusion.toml` (read-only):

- a committed `cache_id` is always used as-is (`diffusion cache enable`);
- otherwise it derives `sha256("<owner>/<repo>/<working-directory>")`, keeps the
  first 16 hex chars, and runs `diffusion cache enable --cache-id <derived>`
  (the ID is validated as `[A-Za-z0-9_-]{1,64}` and persisted to `[cache]
  cache_id`). `diffusion.toml` is not edited by the action.

**Older Diffusion versions** (`--cache-id` and `cache path` require Diffusion
v0.9.1 or newer; older pinned `diffusion-version`s use the fallback) are detected via
`diffusion cache enable --help`. For those, the action falls back to injecting
`cache_id` into `diffusion.toml` with bash before `cache enable` (a warning is
emitted for dotted `cache.*` keys) and prints a `::notice::`. Upgrading
`diffusion-version` removes the fallback.

Committing `cache_id` yourself is optional but recommended if you want the ID to
be stable outside CI too.

### Matrix over scenarios

Each matrix leg computes a different key (`...-default-...` vs `...-ubuntu-...`),
so scenarios cache independently and in parallel:

```yaml
jobs:
  test:
    name: Molecule (${{ matrix.scenario }})
    runs-on: ubuntu-latest
    strategy:
      fail-fast: false
      matrix:
        scenario: [default, ubuntu]
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Run Diffusion Tests
        uses: Polar-Team/diffusion/diffusion-test@main
        with:
          scenario: ${{ matrix.scenario }}
          cache-enabled: 'true'
```

If you also run a matrix over runner images (not just `runs-on` values), add
`cache-key-suffix: ${{ matrix.image }}` so entries stay separated.

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
3. **Configure cache** (if enabled) - Runs `diffusion cache enable` (passing `--cache-id <derived>` only when no `cache_id` is committed in diffusion.toml; a committed one wins; TOML fallback on Diffusion older than v0.9.1), resolves the cache path with `diffusion cache path`, and restores cached artifacts from previous runs
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
