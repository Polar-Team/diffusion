# Diffusion Update GitHub Action

Automatically lock, check, sync, test, and create a PR for Diffusion dependency updates.

## How It Works

1. **Lock** — `diffusion deps lock` regenerates the lockfile
2. **Check** — `diffusion deps check` verifies if dependencies are in sync
3. If check passes (exit 0) → nothing to do, action exits cleanly
4. If check fails (exit ≠ 0) → **Sync** → **Test** → **Create PR**

Tests run the full Molecule suite (converge, lint, verify, idempotence) so the PR is only created when everything passes.

## Quick Start

```yaml
---
name: Dependency Update

on:
  schedule:
    - cron: "0 6 * * 1"
  workflow_dispatch:

permissions:
  contents: write
  pull-requests: write

jobs:
  update:
    name: Update Dependencies
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Run Diffusion Update
        uses: Polar-Team/diffusion/diffusion-update@main
        with:
          target-branch: "main"
```

## Inputs

| Input | Description | Required | Default |
|-------|-------------|----------|---------|
| `diffusion-version` | Diffusion version to use (e.g., `latest`, `v0.3.13`) | No | `latest` |
| `target-branch` | Target branch for the pull request | **Yes** | — |
| `working-directory` | Working directory for the role | No | `.` |
| `run-lint` | Run lint tests after sync | No | `true` |
| `run-verify` | Run verify tests after sync | No | `true` |
| `run-idempotence` | Run idempotence tests after sync | No | `true` |
| `cache-enabled` | Enable caching for Ansible roles, collections, and optionally UV/Docker | No | `false` |
| `cache-uv` | Cache UV/Python packages (requires `cache-enabled: true`) | No | `false` |
| `cache-docker` | Cache DinD Docker images as tarballs (requires `cache-enabled: true`) | No | `false` |
| `pr-title` | Title for the pull request | No | `chore(deps): update diffusion dependencies` |
| `pr-labels` | Comma-separated labels to apply to the PR | No | `dependencies,automated` |

## Outputs

| Output | Description |
|--------|-------------|
| `sync-required` | Whether a sync was required (`true`/`false`) |
| `pr-url` | URL of the created pull request (empty if no sync needed) |
| `pr-number` | Number of the created pull request (empty if no sync needed) |

## Permissions

The workflow calling this action needs:

```yaml
permissions:
  contents: write       # Push the update branch
  pull-requests: write  # Create the PR via gh cli
```

The `github.token` (automatic `GITHUB_TOKEN`) is used by default. If your repo requires a PAT for PR creation, pass it via environment.

## Examples

### Scheduled Weekly Update

```yaml
on:
  schedule:
    - cron: "0 6 * * 1"
  workflow_dispatch:

jobs:
  update:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Polar-Team/diffusion/diffusion-update@main
        with:
          target-branch: "main"
```

### Manual Trigger with Custom Branch

```yaml
on:
  workflow_dispatch:
    inputs:
      branch:
        description: "Target branch"
        required: true
        default: "develop"

jobs:
  update:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Polar-Team/diffusion/diffusion-update@main
        with:
          target-branch: ${{ github.event.inputs.branch }}
          pr-title: "chore: sync dependencies"
          pr-labels: "dependencies,bot"
```

### With Caching

```yaml
jobs:
  update:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Polar-Team/diffusion/diffusion-update@main
        with:
          target-branch: "main"
          cache-enabled: "true"
          cache-uv: "true"
          cache-docker: "true"
```

### Use Output in Subsequent Steps

```yaml
jobs:
  update:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Update deps
        id: update
        uses: Polar-Team/diffusion/diffusion-update@main
        with:
          target-branch: "main"

      - name: Notify on PR creation
        if: steps.update.outputs.sync-required == 'true'
        run: echo "PR created at ${{ steps.update.outputs.pr-url }}"
```

## Requirements

Same as [diffusion-test](../diffusion-test/README.md):

- `diffusion.toml` in the working directory
- `meta/main.yml` role metadata
- `scenarios/default/` Molecule scenario

## More Examples

See the [examples/](examples/) directory:
- [basic-update.yml](examples/basic-update.yml) — Weekly scheduled update
- [cached-update.yml](examples/cached-update.yml) — With caching enabled
- [custom-update.yml](examples/custom-update.yml) — Custom branch and labels

## Documentation

- [Diffusion Documentation](https://github.com/Polar-Team/diffusion)
- [Dependency Management](../../docs/DEPENDENCY_MANAGEMENT.md)
- [diffusion-test Action](../diffusion-test/README.md)

---

**Made with ❤️ by Polar-Team**
