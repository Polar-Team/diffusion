# Diffusion MCP Server

An MCP (Model Context Protocol) server for managing and troubleshooting Diffusion CLI local testing environments. Built with [FastMCP](https://github.com/modelcontextprotocol/python-sdk) and designed to run via `uv`.

## Tools

| Tool | Description |
|---|---|
| `get_diffusion_config` | Read and parse `diffusion.toml` configuration |
| `get_lock_file` | Read and parse `diffusion.lock` dependency lock file |
| `list_molecule_containers` | List all running `molecule-*` containers |
| `inspect_molecule_container` | Detailed inspection of a molecule container (state, mounts, env, network) |
| `docker_exec_in_molecule` | Execute a shell command inside a molecule container (configurable timeout, default 300 s) |
| `get_container_logs` | Get recent logs from a molecule container |
| `check_molecule_yml` | Validate a `molecule.yml` scenario file for common issues |
| `check_verify_yml` | Validate a `verify.yml` file and check test structure |
| `check_docker_environment` | Check local Docker environment for Diffusion compatibility |
| `get_cache_status` | Get Diffusion cache status and directory contents |
| `troubleshoot_molecule_container` | Run comprehensive diagnostics on a molecule container (incl. uv venv check) |
| `get_requirements_yml` | Read Ansible `requirements.yml` for a scenario |
| `list_molecule_scenarios` | List all Molecule scenarios with their key files |
| `run_diffusion_command` | Run safe, read-only diffusion CLI commands |
| `get_diffusion_cli_reference` | Get CLI command reference and usage information |
| `get_server_version` | Return MCP server version and Python environment info |

## Container Image

Pre-built multi-arch images are published to GHCR. Each image includes:
- **diffusion CLI binary** (compiled from Go source)
- **diffusion-mcp Python server** (FastMCP)
- **Docker CE CLI** (for container management tools)

```
ghcr.io/polar-team/diffusion-mcp-server:latest-amd64
ghcr.io/polar-team/diffusion-mcp-server:latest-arm64
ghcr.io/polar-team/diffusion-mcp-server:{version}-amd64
ghcr.io/polar-team/diffusion-mcp-server:{version}-arm64
```

Multi-arch manifest (auto-selects architecture):

```
ghcr.io/polar-team/diffusion-mcp-server:latest
ghcr.io/polar-team/diffusion-mcp-server:{version}
```

### Run from Docker

The container needs two mounts:
1. **Docker socket** — for container management tools (`list_molecule_containers`, `docker_exec_in_molecule`, etc.)
2. **Project directory** — for reading `diffusion.toml`, `molecule.yml`, `verify.yml`, running `diffusion` CLI commands

```bash
docker run --rm -i \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v $(pwd):/project \
  ghcr.io/polar-team/diffusion-mcp-server:latest
```

## Installation (local)

Requires Python 3.11+ and [uv](https://docs.astral.sh/uv/).

```bash
# From the mcp/ directory
uv sync
```

## Usage

### Standalone (stdio transport)

```bash
uv run diffusion-mcp
```

### MCP Configuration (uv — local)

Add to your `.kiro/settings/mcp.json` or `~/.kiro/settings/mcp.json`:

```json
{
  "mcpServers": {
    "diffusion": {
      "command": "uv",
      "args": ["run", "--directory", "<path-to>/dev-new-features/mcp", "diffusion-mcp"],
      "disabled": false,
      "autoApprove": [
        "get_diffusion_config",
        "get_lock_file",
        "list_molecule_containers",
        "get_diffusion_cli_reference",
        "check_molecule_yml",
        "check_verify_yml",
        "check_docker_environment",
        "get_cache_status",
        "list_molecule_scenarios",
        "get_requirements_yml"
      ]
    }
  }
}
```

### MCP Configuration (Docker)

```json
{
  "mcpServers": {
    "diffusion": {
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "-v", "/var/run/docker.sock:/var/run/docker.sock",
        "-v", "${workspaceFolder}:/project",
        "ghcr.io/polar-team/diffusion-mcp-server:latest"
      ],
      "disabled": false,
      "autoApprove": [
        "get_diffusion_config",
        "get_lock_file",
        "list_molecule_containers",
        "get_diffusion_cli_reference",
        "check_molecule_yml",
        "check_verify_yml",
        "check_docker_environment",
        "get_cache_status",
        "list_molecule_scenarios",
        "get_requirements_yml",
        "get_server_version"
      ]
    }
  }
}
```

### With Claude Desktop

```json
{
  "mcpServers": {
    "diffusion": {
      "command": "docker",
      "args": [
        "run", "--rm", "-i",
        "-v", "/var/run/docker.sock:/var/run/docker.sock",
        "-v", "/path/to/your/role:/project",
        "ghcr.io/polar-team/diffusion-mcp-server:latest"
      ]
    }
  }
}
```

## Tool Details

### Configuration & Dependencies

- **get_diffusion_config** — Parses `diffusion.toml` and returns container registry, tests, cache, dependency, and linting configuration.
- **get_lock_file** — Parses `diffusion.lock` and returns pinned Python, Ansible, collection, and role versions.
- **get_requirements_yml** — Reads the Ansible `requirements.yml` for a given scenario.
- **get_cache_status** — Reports cache enabled/disabled state, cache ID, and directory contents with sizes.

### Container Management

- **list_molecule_containers** — Lists all `molecule-*` containers with image, status, and state.
- **inspect_molecule_container** — Deep inspection: state, health, image, mounts, environment variable keys, network config.
- **docker_exec_in_molecule** — Run arbitrary shell commands inside a molecule container for debugging.
- **get_container_logs** — Tail container logs (default: last 100 lines).

### Validation

- **check_molecule_yml** — Validates driver, platforms, provisioner, verifier, and scenario configuration.
- **check_verify_yml** — Validates playbook structure, role inclusions, variable definitions, tags, and file references.

### Troubleshooting

- **check_docker_environment** — Validates Docker daemon, version, DinD readiness, credential helper config, and disk usage.
- **troubleshoot_molecule_container** — Runs 9-point diagnostic: container state, DinD, Python, Ansible, Molecule, uv, mounted volumes, disk usage, and scenario discovery.

### CLI Reference

- **get_diffusion_cli_reference** — Returns structured command reference for all Diffusion CLI commands with flags, subcommands, and examples.
- **run_diffusion_command** — Executes safe read-only commands (`show`, `deps check`, `deps resolve`, `cache status`, `cache list`, `artifact list`, `--version`).

## Development

```bash
# Install dev dependencies
uv sync

# Run the server in development mode with inspector
uv run mcp dev diffusion_mcp/server.py
```

## Build & Publish (Makefile)

| Target | Description |
|---|---|
| `make help` | Show all available targets |
| `make publish` | Full pipeline: cache → buildx → login → build+push |
| `make build-and-push-separate` | Build and push per-architecture tags to GHCR |
| `make build-local` | Build image locally as `diffusion-mcp-server:local` |
| `make test-local` | Build and run local smoke tests |
| `make build-and-save` | Build and save to tar file |
| `make load-local` | Load saved tar image |
| `make setup-buildx` | Create Docker Buildx multi-platform builder |
| `make login` | Login to GHCR |
| `make clean` | Remove buildx builder |
| `make clean-local` | Remove local images and tar files |
| `make show-platforms` | Show configured platforms and tags |

## CI/CD

The GitHub Actions workflow (`.github/workflows/docker-mcp.yml`) runs on pushes to `mcp/**`, tag pushes (`v*`), and PRs:

1. **build-and-push** — Prepares Go source context, builds per-architecture images (`amd64`, `arm64`), pushes to GHCR
2. **create-manifest** — Creates multi-arch manifest lists for `:latest` and `:{version}` tags (tag pushes only)
3. **test-image** — Smoke tests: MCP module load, Docker CLI, diffusion binary, project mount, dive efficiency check

## License

MIT
