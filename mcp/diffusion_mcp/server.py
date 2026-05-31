"""
Diffusion MCP Server

Provides tools for managing and troubleshooting Diffusion CLI local testing
environments, including:
- Diffusion config inspection (diffusion.toml, diffusion.lock)
- Molecule container management and docker exec helpers
- Molecule scenario file validation (molecule.yml, verify.yml)
- Dependency and cache status
- CLI command reference
"""

from __future__ import annotations

import json
import os
import shlex
import subprocess
import sys
from pathlib import Path
from typing import Any

from mcp.server.fastmcp import FastMCP

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

if sys.version_info >= (3, 11):
    import tomllib
else:
    import tomli as tomllib  # type: ignore[import-untyped]

import yaml

mcp = FastMCP("diffusion-mcp")


def _run(cmd: list[str], timeout: int = 30, cwd: str | None = None) -> dict[str, Any]:
    """Run a shell command and return structured output."""
    try:
        result = subprocess.run(
            cmd,
            capture_output=True,
            text=True,
            timeout=timeout,
            cwd=cwd,
        )
        return {
            "stdout": result.stdout.strip(),
            "stderr": result.stderr.strip(),
            "returncode": result.returncode,
        }
    except FileNotFoundError:
        return {
            "stdout": "",
            "stderr": f"Command not found: {cmd[0]}",
            "returncode": -1,
        }
    except subprocess.TimeoutExpired:
        return {
            "stdout": "",
            "stderr": f"Command timed out after {timeout}s",
            "returncode": -1,
        }


def _find_project_root(start: str | None = None) -> Path | None:
    """Walk up from *start* (or cwd) looking for diffusion.toml or diffusion.lock."""
    current = Path(start) if start else Path.cwd()
    for parent in [current, *current.parents]:
        if (parent / "diffusion.toml").exists():
            return parent
    # Fallback: accept diffusion.lock without a toml (edge case)
    current = Path(start) if start else Path.cwd()
    for parent in [current, *current.parents]:
        if (parent / "diffusion.lock").exists():
            return parent
    return None


def _load_toml(path: Path) -> dict[str, Any]:
    """Load a TOML file and return its contents as a dict."""
    with open(path, "rb") as f:
        return tomllib.load(f)


def _load_yaml(path: Path) -> Any:
    """Load a YAML file and return its contents."""
    with open(path, "r", encoding="utf-8") as f:
        return yaml.safe_load(f)


def _container_name(role: str) -> str:
    """Return the molecule container name for a role."""
    return f"molecule-{role}"


# ---------------------------------------------------------------------------
# Tool: get_diffusion_config
# ---------------------------------------------------------------------------


@mcp.tool()
def get_diffusion_config(project_path: str = "") -> str:
    """Read and return the diffusion.toml configuration for a project.

    Args:
        project_path: Path to the project root (auto-detected if empty).
    """
    root = Path(project_path) if project_path else _find_project_root()
    if root is None:
        return "Error: Could not find diffusion.toml. Provide project_path or run from a Diffusion project."

    toml_path = root / "diffusion.toml"
    if not toml_path.exists():
        return f"Error: {toml_path} does not exist."

    try:
        data = _load_toml(toml_path)
        return json.dumps(data, indent=2, default=str)
    except Exception as e:
        return f"Error reading diffusion.toml: {e}"


# ---------------------------------------------------------------------------
# Tool: get_lock_file
# ---------------------------------------------------------------------------


@mcp.tool()
def get_lock_file(project_path: str = "") -> str:
    """Read and return the diffusion.lock dependency lock file.

    Args:
        project_path: Path to the project root (auto-detected if empty).
    """
    root = Path(project_path) if project_path else _find_project_root()
    if root is None:
        return "Error: Could not find project root."

    lock_path = root / "diffusion.lock"
    if not lock_path.exists():
        return "No diffusion.lock found. Run 'diffusion deps lock' to generate one."

    try:
        data = _load_yaml(lock_path)
        return json.dumps(data, indent=2, default=str)
    except Exception as e:
        return f"Error reading diffusion.lock: {e}"


# ---------------------------------------------------------------------------
# Tool: list_molecule_containers
# ---------------------------------------------------------------------------


@mcp.tool()
def list_molecule_containers() -> str:
    """List all running Diffusion molecule containers (molecule-* naming convention).

    Returns container name, image, status, and health for each.
    """
    result = _run(
        [
            "docker",
            "ps",
            "-a",
            "--filter",
            "name=molecule-",
            "--format",
            '{"name":"{{.Names}}","image":"{{.Image}}","status":"{{.Status}}","state":"{{.State}}","ports":"{{.Ports}}"}',
        ]
    )
    if result["returncode"] != 0:
        return f"Error listing containers: {result['stderr']}"

    lines = [l for l in result["stdout"].splitlines() if l.strip()]
    if not lines:
        return "No molecule containers found."

    containers = []
    for line in lines:
        try:
            containers.append(json.loads(line))
        except json.JSONDecodeError:
            containers.append({"raw": line})

    return json.dumps(containers, indent=2)


# ---------------------------------------------------------------------------
# Tool: inspect_molecule_container
# ---------------------------------------------------------------------------


@mcp.tool()
def inspect_molecule_container(role: str) -> str:
    """Inspect a molecule container for a given role and return key details.

    Args:
        role: The role name (container will be molecule-<role>).
    """
    container = _container_name(role)
    result = _run(["docker", "inspect", container])
    if result["returncode"] != 0:
        return f"Container '{container}' not found or not running. Error: {result['stderr']}"

    try:
        data = json.loads(result["stdout"])
        if not data:
            return f"No data returned for container '{container}'."

        info = data[0]
        summary = {
            "name": info.get("Name", ""),
            "id": info.get("Id", "")[:12],
            "state": info.get("State", {}).get("Status", "unknown"),
            "health": info.get("State", {}).get("Health", {}).get("Status", "N/A"),
            "image": info.get("Config", {}).get("Image", ""),
            "created": info.get("Created", ""),
            "mounts": [
                {"source": m.get("Source", ""), "destination": m.get("Destination", "")}
                for m in info.get("Mounts", [])
            ],
            "env_keys": [
                e.split("=")[0]
                for e in info.get("Config", {}).get("Env", [])
                if not e.startswith("PATH=")
            ],
            "network": {
                name: {"ip": net.get("IPAddress", "")}
                for name, net in info.get("NetworkSettings", {})
                .get("Networks", {})
                .items()
            },
        }
        return json.dumps(summary, indent=2)
    except (json.JSONDecodeError, IndexError, KeyError) as e:
        return f"Error parsing inspect output: {e}"


# ---------------------------------------------------------------------------
# Tool: docker_exec_in_molecule
# ---------------------------------------------------------------------------


@mcp.tool()
def docker_exec_in_molecule(
    role: str, command: str, workdir: str = "", timeout: int = 300
) -> str:
    """Execute a command inside a molecule container for troubleshooting.

    Args:
        role: The role name (container will be molecule-<role>).
        command: Shell command to run inside the container.
        workdir: Optional working directory inside the container.
        timeout: Command timeout in seconds (default 300). Increase for long-running operations like converge/verify.
    """
    container = _container_name(role)
    docker_cmd = ["docker", "exec"]
    if workdir:
        docker_cmd.extend(["-w", workdir])
    docker_cmd.extend([container, "/bin/sh", "-c", command])

    result = _run(docker_cmd, timeout=timeout)
    output_parts = []
    if result["stdout"]:
        output_parts.append(result["stdout"])
    if result["stderr"]:
        output_parts.append(f"[stderr] {result['stderr']}")
    if result["returncode"] != 0:
        output_parts.append(f"[exit code: {result['returncode']}]")

    return "\n".join(output_parts) if output_parts else "(no output)"


# ---------------------------------------------------------------------------
# Tool: get_container_logs
# ---------------------------------------------------------------------------


@mcp.tool()
def get_container_logs(role: str, tail: int = 100) -> str:
    """Get recent logs from a molecule container.

    Args:
        role: The role name (container will be molecule-<role>).
        tail: Number of recent log lines to return (default 100).
    """
    container = _container_name(role)
    result = _run(["docker", "logs", "--tail", str(tail), container])
    if result["returncode"] != 0:
        return f"Error getting logs for '{container}': {result['stderr']}"

    output = ""
    if result["stdout"]:
        output += result["stdout"]
    if result["stderr"]:
        output += ("\n" if output else "") + result["stderr"]
    return output or "(no logs)"


# ---------------------------------------------------------------------------
# Tool: check_molecule_yml
# ---------------------------------------------------------------------------


@mcp.tool()
def check_molecule_yml(
    project_path: str = "",
    scenario: str = "default",
) -> str:
    """Validate a molecule.yml scenario file and report issues.

    Checks for: driver config, platform definitions, provisioner settings,
    verifier type, and common misconfigurations.

    Args:
        project_path: Path to the project root (auto-detected if empty).
        scenario: Molecule scenario name (default: "default").
    """
    root = Path(project_path) if project_path else _find_project_root()
    if root is None:
        return "Error: Could not find project root."

    pathScenarios = root / "scenarios" / scenario / "molecule.yml"

    if pathScenarios.exists():
        mol_path = pathScenarios
    else:
        mol_path = None

    if mol_path is None:
        return f"No molecule.yml found for scenario '{scenario}'."

    try:
        data = _load_yaml(mol_path)
    except Exception as e:
        return f"YAML parse error in {mol_path}: {e}"

    if data is None:
        return f"molecule.yml at {mol_path} is empty."

    issues: list[str] = []
    warnings: list[str] = []
    info: list[str] = []

    # Driver check
    driver = data.get("driver", {})
    driver_name = driver.get("name", "missing")
    info.append(f"Driver: {driver_name}")
    if driver_name == "missing":
        issues.append("No driver specified. Expected 'docker' for Diffusion workflows.")

    # Platforms check
    platforms = data.get("platforms", [])
    if not platforms:
        issues.append("No platforms defined.")
    else:
        info.append(f"Platforms: {len(platforms)}")
        for i, p in enumerate(platforms):
            name = p.get("name", f"platform-{i}")
            image = p.get("image", "not set")
            info.append(
                f"  [{name}] image={image}, privileged={p.get('privileged', False)}"
            )
            if not p.get("image"):
                issues.append(f"Platform '{name}' has no image defined.")

    # Provisioner check
    provisioner = data.get("provisioner", {})
    prov_name = provisioner.get("name", "ansible")
    info.append(f"Provisioner: {prov_name}")

    # Verifier check
    verifier = data.get("verifier", {})
    verifier_name = verifier.get("name", "ansible")
    info.append(f"Verifier: {verifier_name}")

    # Scenario check
    scenario_cfg = data.get("scenario", {})
    if scenario_cfg:
        info.append(f"Scenario config: {json.dumps(scenario_cfg, default=str)}")

    # Diffusion layout check: scenarios/ without molecule/ symlink
    scenarios_dir = root / "scenarios" / scenario / "molecule.yml"
    molecule_dir = root / "molecule" / scenario / "molecule.yml"
    if mol_path == scenarios_dir and not molecule_dir.exists():
        warnings.append(
            f"molecule.yml found in scenarios/{scenario}/ but molecule/{
                scenario
            }/ does not exist. "
            "Diffusion creates the molecule/ symlink at runtime, but local 'molecule test' "
            "commands won't find this scenario without it."
        )

    # Build report
    report = [f"=== molecule.yml validation: {mol_path} ===", ""]
    if issues:
        report.append(f"ERRORS ({len(issues)}):")
        for issue in issues:
            report.append(f"  ✗ {issue}")
        report.append("")
    if warnings:
        report.append(f"WARNINGS ({len(warnings)}):")
        for w in warnings:
            report.append(f"  ⚠ {w}")
        report.append("")
    report.append("INFO:")
    for i in info:
        report.append(f"  {i}")

    if not issues:
        report.append("\n✓ No errors found.")

    return "\n".join(report)


# ---------------------------------------------------------------------------
# Tool: check_verify_yml
# ---------------------------------------------------------------------------


@mcp.tool()
def check_verify_yml(project_path: str = "", scenario: str = "default") -> str:
    """Validate a verify.yml file and check test structure.

    Checks for: proper playbook structure, role inclusion, variable definitions,
    tag usage, and references to test files.

    Args:
        project_path: Path to the project root (auto-detected if empty).
        scenario: Molecule scenario name (default: "default").
    """
    root = Path(project_path) if project_path else _find_project_root()
    if root is None:
        return "Error: Could not find project root."

    candidates = [
        root / "molecule" / scenario / "verify.yml",
        root / "scenarios" / scenario / "verify.yml",
    ]
    verify_path = None
    for c in candidates:
        if c.exists():
            verify_path = c
            break

    if verify_path is None:
        return (
            f"No verify.yml found for scenario '{scenario}'. Searched:\n"
            + "\n".join(f"  - {c}" for c in candidates)
        )

    try:
        data = _load_yaml(verify_path)
    except Exception as e:
        return f"YAML parse error in {verify_path}: {e}"

    if data is None:
        return f"verify.yml at {verify_path} is empty."

    issues: list[str] = []
    warnings: list[str] = []
    info: list[str] = []

    if not isinstance(data, list):
        issues.append("verify.yml should be a list of plays (YAML list at top level).")
        return f"ERRORS:\n  ✗ {issues[0]}"

    info.append(f"Plays: {len(data)}")

    for idx, play in enumerate(data):
        play_name = play.get("name", f"play-{idx}")
        hosts = play.get("hosts", "not set")
        info.append(f"\nPlay {idx + 1}: '{play_name}' (hosts: {hosts})")

        if hosts == "not set":
            issues.append(f"Play '{play_name}' has no 'hosts' defined.")

        # Check tasks
        tasks = play.get("tasks", [])
        pre_tasks = play.get("pre_tasks", [])
        roles = play.get("roles", [])

        info.append(
            f"  tasks: {len(tasks)}, pre_tasks: {len(pre_tasks)}, roles: {len(roles)}"
        )

        # Check for include_role with diffusion_tests
        for task in tasks:
            task_name = task.get("name", "unnamed")
            if "ansible.builtin.include_role" in task or "include_role" in task:
                role_info = task.get(
                    "ansible.builtin.include_role", task.get("include_role", {})
                )
                role_name = role_info.get("name", "unknown")
                info.append(f"  → include_role: {role_name}")

            # Check for vars
            task_vars = task.get("vars", {})
            if task_vars:
                var_keys = list(task_vars.keys())
                info.append(f"  → vars: {', '.join(var_keys)}")

                # Validate known diffusion_tests variables
                known_vars = [
                    "verify_ports",
                    "verify_docker_containers",
                    "verify_output_in_cmd",
                    "verify_uris",
                    "postgres_db",
                    "postgres_expected_tables",
                    "postgres_expected_records",
                    "postgres_expected_roles",
                ]
                for v in var_keys:
                    if v in known_vars:
                        info.append(
                            f"    ✓ {v}: {
                                len(task_vars[v])
                                if isinstance(task_vars[v], list)
                                else 'set'
                            }"
                        )

            # Check tags
            tags = task.get("tags", [])
            if tags:
                info.append(f"  → tags: {tags}")

        # Check for include_tasks
        for task in tasks:
            if "ansible.builtin.include_tasks" in task or "include_tasks" in task:
                include_file = task.get(
                    "ansible.builtin.include_tasks", task.get("include_tasks", "")
                )
                if isinstance(include_file, dict):
                    include_file = include_file.get("file", "unknown")
                info.append(f"  → include_tasks: {include_file}")

                # Check if the referenced file exists (Ansible resolves relative to playbook dir)
                ref_path = verify_path.parent / include_file
                if not ref_path.exists():
                    warnings.append(
                        f"Referenced file '{include_file}' not found at {ref_path}"
                    )

    # Build report
    report = [f"=== verify.yml validation: {verify_path} ===", ""]
    if issues:
        report.append(f"ERRORS ({len(issues)}):")
        for issue in issues:
            report.append(f"  ✗ {issue}")
        report.append("")
    if warnings:
        report.append(f"WARNINGS ({len(warnings)}):")
        for w in warnings:
            report.append(f"  ⚠ {w}")
        report.append("")
    report.append("INFO:")
    for i in info:
        report.append(f"  {i}")

    if not issues:
        report.append("\n✓ No errors found.")

    return "\n".join(report)


# ---------------------------------------------------------------------------
# Tool: get_diffusion_cli_reference
# ---------------------------------------------------------------------------


@mcp.tool()
def get_diffusion_cli_reference(command: str = "") -> str:
    """Get comprehensive Diffusion CLI command reference with all flags, subcommands, and examples.

    Args:
        command: Specific command to get help for (e.g. "molecule", "deps", "cache",
                 "role", "artifact", "show"). Use "role add-role" or "deps init" for
                 subcommand details. Leave empty for the full command tree.
    """
    cli_ref: dict[str, dict[str, Any]] = {
        "molecule": {
            "description": "Run Molecule workflows inside a Docker-in-Docker container",
            "usage": "diffusion molecule [flags]",
            "notes": [
                "If no action flag is given, the default flow creates the container (if needed) and runs converge.",
                "Role name and org are auto-detected from meta/main.yml if present.",
                "In CI mode (--ci), the repo is cloned inside the container instead of volume-mounting.",
                "First run without --ci creates diffusion.toml interactively if it doesn't exist.",
            ],
            "flags": {
                "--role, -r": {
                    "description": "Role name",
                    "default": "(from meta/main.yml)",
                },
                "--org, -o": {
                    "description": "Organization / namespace prefix",
                    "default": "(from meta/main.yml)",
                },
                "--tag, -t": {
                    "description": "Ansible tags to run (comma-separated, e.g. 'install,configure')",
                    "default": "",
                },
                "--converge": {
                    "description": "Run molecule converge only (skip create if container exists)",
                    "default": "false",
                },
                "--verify": {
                    "description": "Run molecule verify (test execution)",
                    "default": "false",
                },
                "--testsoverwrite": {
                    "description": "Overwrite molecule tests folder for remote/diffusion test types",
                    "default": "false",
                },
                "--lint": {
                    "description": "Run yamllint + ansible-lint inside the container",
                    "default": "false",
                },
                "--idempotence": {
                    "description": "Run molecule idempotence check",
                    "default": "false",
                },
                "--destroy": {
                    "description": "Run molecule destroy (remove molecule instances inside DinD)",
                    "default": "false",
                },
                "--wipe": {
                    "description": "Remove the molecule container and the molecule/<role> folder entirely",
                    "default": "false",
                },
                "--ci": {
                    "description": "CI/CD mode: non-interactive, no TTY, clones repo inside container, uses docker cp for cache",
                    "default": "false",
                },
                "--oidc": {
                    "description": "Use OIDC token from env (TOKEN + provider vars: YC_CLOUD_ID/YC_FOLDER_ID for YC, AWS_REGION for AWS)",
                    "default": "false",
                },
                "--force": {
                    "description": "Force reinstall of roles/collections from requirements.yml before converge",
                    "default": "false",
                },
            },
            "workflow_order": [
                "1. Container creation (docker run with DinD image, volume mounts, env vars)",
                "2. Cache loading (roles, collections, UV packages, Docker images)",
                "3. CI repo clone or local file copy into /opt/molecule/<org>.<role>/",
                "4. Registry login inside container (provider-specific)",
                "5. uv-sync (install Python deps from pyproject.toml)",
                "6. ansible-galaxy install (if --force)",
                "7. molecule create + molecule converge (default flow)",
                "8. Permission fix (chown on /opt/molecule for Unix)",
            ],
            "examples": [
                "diffusion molecule                                      # Interactive setup + converge",
                "diffusion molecule --ci                                 # CI mode: create container + converge",
                "diffusion molecule --verify                             # Run all verification tests",
                "diffusion molecule --verify --tags ports                # Run only port tests",
                "diffusion molecule --ci --verify --tags 'ports,docker'  # Multiple tags",
                "diffusion molecule --lint                               # Run yamllint + ansible-lint",
                "diffusion molecule --idempotence                        # Idempotence check",
                "diffusion molecule --destroy                            # Destroy molecule instances (keep container)",
                "diffusion molecule --wipe                               # Full cleanup: destroy + remove container + folder",
                "diffusion molecule --converge --force                   # Force reinstall deps then converge",
            ],
            "container_naming": "molecule-<role_name> (e.g. molecule-nginx)",
            "container_image": "ghcr.io/polar-team/diffusion-molecule-container:<tag>",
            "key_paths_inside_container": {
                "/opt/molecule/": "Mounted or cloned role directory",
                "/opt/uv/.venv/": "Python virtual environment (Ansible, Molecule, linters)",
                "/root/.ansible/roles/": "Cached Ansible roles",
                "/root/.ansible/collections/": "Cached Ansible collections",
                "/root/.cache/uv/": "UV package cache",
                "/root/.cache/docker/": "Docker image cache (tarballs)",
            },
        },
        "role": {
            "description": "Manage Ansible role configuration, initialization, and dependencies",
            "usage": "diffusion role [flags]",
            "notes": [
                "Without flags or subcommands, displays current role config from meta/main.yml.",
                "Roles are stored per-scenario in diffusion.toml as '<scenario>.<role_name>'.",
                "Dots are forbidden in role/collection names (reserved as scenario prefixes).",
            ],
            "flags": {
                "--init, -i": {
                    "description": "Initialize a new Ansible role via ansible-galaxy init",
                    "default": "false",
                },
                "--scenario, -s": {
                    "description": "Molecule scenario folder to use",
                    "default": "default",
                },
            },
            "subcommands": {
                "add-role": {
                    "usage": "diffusion role add-role [role-name] [flags]",
                    "description": "Add a role dependency to diffusion.toml and update diffusion.lock",
                    "args": "[role-name] — name without namespace (use --namespace separately)",
                    "flags": {
                        "--scenario, -s": {
                            "description": "Molecule scenario folder",
                            "default": "default",
                        },
                        "--src": {
                            "description": "Source URL of the role (git URL ending in .git)",
                            "default": "",
                        },
                        "--scm": {
                            "description": "SCM type (auto-detected: 'git' if --src ends with .git, else 'galaxy')",
                            "default": "git",
                        },
                        "--version, -v": {
                            "description": "Version constraint (e.g. '>=1.0.0', '1.2.3', 'main')",
                            "default": "main",
                        },
                        "--namespace, -n": {
                            "description": "Galaxy namespace (required for Galaxy roles, e.g. 'geerlingguy')",
                            "default": "",
                        },
                    },
                    "examples": [
                        "diffusion role add-role docker --namespace geerlingguy",
                        "diffusion role add-role my-role --src https://github.com/org/role.git --version v2.0.0",
                        "diffusion role add-role my-role --src https://github.com/org/role.git  # auto-resolves version from git",
                    ],
                },
                "remove-role": {
                    "usage": "diffusion role remove-role [role-name] [flags]",
                    "description": "Remove a role from diffusion.toml (keeps it in requirements.yml until deps sync)",
                    "args": "[role-name] — name without namespace",
                    "flags": {
                        "--scenario, -s": {
                            "description": "Molecule scenario folder",
                            "default": "default",
                        },
                        "--namespace, -n": {
                            "description": "Galaxy namespace (optional)",
                            "default": "",
                        },
                    },
                    "examples": [
                        "diffusion role remove-role docker",
                        "diffusion role remove-role my-role --scenario production",
                    ],
                },
                "add-collection": {
                    "usage": "diffusion role add-collection [collection-name] [flags]",
                    "description": "Add a collection to diffusion.toml and update diffusion.lock",
                    "args": "[collection-name] — name without namespace (use --namespace separately)",
                    "flags": {
                        "--scenario, -s": {
                            "description": "Molecule scenario folder",
                            "default": "default",
                        },
                        "--namespace, -n": {
                            "description": "Galaxy namespace (required, e.g. 'community' for community.general)",
                            "default": "",
                        },
                    },
                    "examples": [
                        "diffusion role add-collection general --namespace community",
                        "diffusion role add-collection docker --namespace community --scenario production",
                    ],
                },
                "remove-collection": {
                    "usage": "diffusion role remove-collection [collection-name] [flags]",
                    "description": "Remove a collection from diffusion.toml and update diffusion.lock",
                    "args": "[collection-name] — name without namespace",
                    "flags": {
                        "--scenario, -s": {
                            "description": "Molecule scenario folder",
                            "default": "default",
                        },
                        "--namespace, -n": {
                            "description": "Galaxy namespace (optional)",
                            "default": "",
                        },
                    },
                    "examples": [
                        "diffusion role remove-collection general",
                    ],
                },
            },
            "examples": [
                "diffusion role                    # Show current role config",
                "diffusion role --init             # Initialize new role via ansible-galaxy",
            ],
        },
        "deps": {
            "description": "Dependency management — initialize, lock, check, resolve, and sync",
            "usage": "diffusion deps [subcommand]",
            "notes": [
                "Dependencies are tracked in diffusion.toml (constraints) and diffusion.lock (resolved versions).",
                "Collections and roles are stored per-scenario: '<scenario>.<name>' in the lock file.",
                "Python tool versions (ansible, molecule, etc.) are also tracked and resolved.",
            ],
            "subcommands": {
                "init": {
                    "usage": "diffusion deps init",
                    "description": "Initialize [dependencies] section in diffusion.toml with defaults and scan existing requirements.yml/meta.yml",
                    "behavior": [
                        "Creates default Python version config (min=3.11, max=3.13, pinned=3.13)",
                        "Sets default tool constraints: ansible>=10.0.0, molecule>=24.0.0, ansible-lint>=24.0.0, yamllint>=1.35.0",
                        "Scans scenarios/*/requirements.yml for existing collections and roles",
                        "Scans meta/main.yml for collections",
                        "Adds found dependencies with >= version constraints",
                    ],
                },
                "lock": {
                    "usage": "diffusion deps lock",
                    "description": "Generate or update diffusion.lock from current dependencies in meta/main.yml, requirements.yml, and diffusion.toml",
                    "behavior": [
                        "Resolves all collection and role versions from Galaxy API or git",
                        "Pins Python version and tool versions",
                        "Writes diffusion.lock (TOML format)",
                    ],
                },
                "check": {
                    "usage": "diffusion deps check",
                    "description": "Verify diffusion.lock is up-to-date with current YAML manifests",
                    "behavior": [
                        "Compares lock file against requirements.yml and meta.yml",
                        "Exits with code 1 if out of date (useful in CI)",
                    ],
                },
                "resolve": {
                    "usage": "diffusion deps resolve",
                    "description": "Display all dependencies with their resolved versions from diffusion.lock",
                    "behavior": [
                        "Shows Python version (pinned, min, max)",
                        "Shows tools with constraints and resolved versions",
                        "Shows collections per scenario with constraints and resolved versions",
                        "Shows roles per scenario with constraints and resolved versions",
                    ],
                },
                "sync": {
                    "usage": "diffusion deps sync",
                    "description": "Restore dependency versions from diffusion.lock back to requirements.yml and meta/main.yml",
                    "behavior": [
                        "Overwrites requirements.yml for each scenario with resolved versions from lock file",
                        "Updates meta/main.yml collections (default scenario only)",
                        "Useful for rollback or ensuring consistency after lock file changes",
                    ],
                },
            },
            "examples": [
                "diffusion deps init                # Initialize dependency tracking",
                "diffusion deps lock                # Generate/update lock file",
                "diffusion deps check               # Verify lock file is current (CI gate)",
                "diffusion deps resolve             # Show all resolved versions",
                "diffusion deps sync                # Restore versions from lock to YAML files",
            ],
        },
        "cache": {
            "description": "Manage Ansible role/collection and Docker/Python package caching",
            "usage": "diffusion cache [subcommand]",
            "notes": [
                "Cache is stored at ~/.diffusion/cache/role_<cache_id>/",
                "Cache ID is an 8-byte hex string auto-generated per role.",
                "Subdirectories: roles/, collections/, uv/ (Python packages), docker/ (image tarballs).",
                "On Windows, UV cache uses a precache staging path due to NTFS mount performance.",
                "In CI mode, cache is transferred via docker cp instead of volume mounts.",
            ],
            "subcommands": {
                "enable": {
                    "usage": "diffusion cache enable [flags]",
                    "description": "Enable caching for this role (generates cache ID if needed)",
                    "flags": {
                        "--docker": {
                            "description": "Enable Docker image caching (saves/loads DinD image tarballs)",
                            "default": "false",
                        },
                        "--uv": {
                            "description": "Enable UV/Python package caching",
                            "default": "false",
                        },
                    },
                    "examples": [
                        "diffusion cache enable                  # Enable roles+collections cache",
                        "diffusion cache enable --docker --uv    # Enable all cache types",
                    ],
                },
                "disable": {
                    "usage": "diffusion cache disable",
                    "description": "Disable all caching (preserves cache directory; use 'clean' to remove)",
                },
                "clean": {
                    "usage": "diffusion cache clean",
                    "description": "Remove the cache directory and all cached data for this role",
                    "behavior": [
                        "Shows per-type size breakdown (roles, collections, UV, Docker) before cleaning"
                    ],
                },
                "status": {
                    "usage": "diffusion cache status",
                    "description": "Show cache status: enabled/disabled, cache ID, path, and per-type size breakdown",
                },
                "list": {
                    "usage": "diffusion cache list",
                    "description": "List all cache directories in ~/.diffusion/cache/ with sizes",
                },
            },
            "examples": [
                "diffusion cache enable --docker --uv  # Enable full caching",
                "diffusion cache status                 # Check cache state and sizes",
                "diffusion cache list                   # List all role caches",
                "diffusion cache clean                  # Remove cache for current role",
                "diffusion cache disable                # Disable without removing",
            ],
        },
        "artifact": {
            "description": "Manage private artifact repository credentials (Vault or local encrypted)",
            "usage": "diffusion artifact [subcommand]",
            "notes": [
                "Credentials are stored either in HashiCorp Vault or locally encrypted at ~/.diffusion/secrets/<role>/<name>.",
                "Artifact sources are configured in diffusion.toml under [[artifact_sources]].",
                "Git credentials are passed to the molecule container as indexed env vars (GIT_USER_N, GIT_PASSWORD_N, GIT_URL_N).",
            ],
            "subcommands": {
                "add": {
                    "usage": "diffusion artifact add [source-name]",
                    "description": "Add credentials for a private artifact source (interactive)",
                    "behavior": [
                        "Prompts for URL",
                        "Asks whether to store in Vault or locally",
                        "Vault: prompts for vault_path, secret_name, username_field, token_field",
                        "Local: prompts for username and token/password, encrypts and saves",
                        "Updates diffusion.toml with the source configuration",
                    ],
                },
                "list": {
                    "usage": "diffusion artifact list",
                    "description": "List all stored artifact sources with their URLs",
                },
                "remove": {
                    "usage": "diffusion artifact remove [source-name]",
                    "description": "Remove stored credentials and config entry for an artifact source",
                },
                "show": {
                    "usage": "diffusion artifact show [source-name]",
                    "description": "Show details for an artifact source (token is masked)",
                },
            },
            "examples": [
                "diffusion artifact add my-galaxy       # Add private Galaxy server credentials",
                "diffusion artifact add my-git-repo     # Add private git repo credentials",
                "diffusion artifact list                # List all configured sources",
                "diffusion artifact show my-galaxy      # Show source details",
                "diffusion artifact remove my-galaxy    # Remove source and credentials",
            ],
        },
        "show": {
            "description": "Display the full diffusion.toml configuration in readable format",
            "usage": "diffusion show",
            "sections_displayed": [
                "Container Registry (server, provider, image name/tag)",
                "HashiCorp Vault integration status",
                "Artifact Sources (name, URL, storage type)",
                "YAML Lint Configuration (extends, ignore patterns, rules)",
                "Ansible Lint Configuration (excluded paths, warn list, skip list)",
            ],
        },
        "deploy": {
            "description": "Deploy Ansible roles to remote hosts using the diffusion molecule container",
            "usage": "diffusion deploy [flags]",
            "notes": [
                "Roles and collections are installed INSIDE the container — nothing is downloaded to the local machine.",
                "Fetches diffusion.lock from each --role-source, merges constraints, and runs ansible-playbook inside the container.",
                "When --playbook is omitted, a playbook is auto-generated from role_sources grouped by apply_to pattern.",
                "Remote state is written to ~/.diffusion/state on each host after every run.",
                "Skip logic: re-deploy is skipped if last run succeeded within --skip-period AND run_id matches.",
                "run_id is a SHA-256 of merged lock hash + inventory + playbook + extra-vars.",
            ],
            "flags": {
                "--role-source": {
                    "description": "Role source spec (repeatable). Comma-separated key=value pairs: scm=git|galaxy, version=<constraint>, url=<git-url>, galaxy=<namespace.name>, name=<override>, apply_to=<hosts-pattern>",
                    "required": True,
                    "examples": [
                        "scm=galaxy,version=>=6.0.0,galaxy=geerlingguy.docker",
                        "scm=git,version=main,url=https://github.com/org/role.git,name=myrole,apply_to=webservers",
                    ],
                },
                "--playbook": {
                    "description": "Path to an Ansible playbook. When omitted, a playbook is auto-generated from --role-source entries, grouped by apply_to pattern.",
                    "default": "(auto-generated)",
                },
                "--host": {
                    "description": "hostname=key=value,key=value — inventory host with Ansible connection variables (repeatable)",
                    "example": "web01=ansible_host=1.2.3.4,ansible_user=ubuntu",
                },
                "--group": {
                    "description": "groupname=host1,host2 — inventory host group (repeatable)",
                    "example": "webservers=web01,web02",
                },
                "--var": {
                    "description": "key=value — global inventory variable (repeatable)",
                },
                "--extra-var": {
                    "description": "key=value — extra variable passed to ansible-playbook --extra-vars (repeatable)",
                },
                "--skip-period": {
                    "description": "Skip re-deploy if last run succeeded within this period and inputs are identical. Go duration string (e.g. '24h'). Empty = always deploy.",
                    "default": "",
                },
                "--host-wait-initial-delay": {
                    "description": "Delay before the first host reachability probe (Go duration, e.g. '10s')",
                    "default": "10s",
                },
                "--host-wait-interval": {
                    "description": "Interval between host reachability probes (Go duration)",
                    "default": "5s",
                },
                "--host-wait-timeout": {
                    "description": "Hard deadline for all hosts to become reachable (Go duration)",
                    "default": "5m",
                },
            },
            "workflow_order": [
                "1. Fetch diffusion.lock from each --role-source (git shallow-clone or Galaxy download)",
                "2. Merge lock files — intersect constraints, re-resolve via Galaxy API",
                "3. Generate requirements.yml from merged lock",
                "4. Auto-generate playbook from role_sources (or use --playbook)",
                "5. Wait for hosts to be reachable via ansible.builtin.ping inside the container",
                "6. Run ansible-galaxy role/collection install inside the container",
                "7. Run ansible-playbook inside the container",
                "8. Write remote state to ~/.diffusion/state on each host",
            ],
            "auto_generated_playbook_example": (
                "---\n"
                "# Auto-generated by diffusion deploy\n\n"
                "- name: \"diffusion deploy | all\"\n"
                "  hosts: all\n"
                "  gather_facts: true\n"
                "  roles:\n"
                "    - role: geerlingguy.docker\n\n"
                "- name: \"diffusion deploy | webservers\"\n"
                "  hosts: webservers\n"
                "  gather_facts: true\n"
                "  roles:\n"
                "    - role: myorg.app\n"
            ),
            "examples": [
                "diffusion deploy --role-source scm=galaxy,version=>=6.0.0,galaxy=geerlingguy.docker --host web01=ansible_host=1.2.3.4",
                "diffusion deploy --role-source \"scm=git,version=main,url=https://github.com/org/role.git,name=myrole,apply_to=webservers\" --host web01=ansible_host=1.2.3.4",
                "diffusion deploy --playbook site.yml --role-source scm=galaxy,version=>=6.0.0,galaxy=ns.role --skip-period 24h",
            ],
        },
    }

    # Support subcommand lookups like "role add-role", "deps init", "cache enable"
    cmd_lower = command.lower().strip() if command else ""

    if cmd_lower:
        # Check for subcommand syntax: "role add-role", "deps init", etc.
        parts = cmd_lower.split(None, 1)
        parent = parts[0]
        sub = parts[1] if len(parts) > 1 else None

        if parent in cli_ref:
            if sub and "subcommands" in cli_ref[parent]:
                subs = cli_ref[parent]["subcommands"]
                if sub in subs:
                    return json.dumps({parent + " " + sub: subs[sub]}, indent=2)
                # Try matching with hyphens
                for key, val in subs.items():
                    if key.replace("-", " ") == sub or key == sub:
                        return json.dumps({parent + " " + key: val}, indent=2)
                available = ", ".join(subs.keys())
                return (
                    f"Unknown subcommand '{sub}' for '{parent}'. Available: {available}"
                )
            return json.dumps(cli_ref[parent], indent=2)
        return f"Unknown command '{command}'. Available: {', '.join(cli_ref.keys())}"

    # Return overview
    overview: dict[str, Any] = {
        "_global": {
            "binary": "diffusion",
            "description": "Molecule workflow helper — cross-platform CLI for Ansible role testing",
            "flags": {"--version": "Print version, Go version, OS/Arch"},
            "config_file": "diffusion.toml",
            "lock_file": "diffusion.lock",
        },
    }
    for cmd, info in cli_ref.items():
        entry: dict[str, Any] = {
            "description": info["description"],
            "usage": info.get("usage", ""),
        }
        if "subcommands" in info:
            entry["subcommands"] = list(info["subcommands"].keys())
        overview[cmd] = entry
    return json.dumps(overview, indent=2)


# ---------------------------------------------------------------------------
# Tool: check_docker_environment
# ---------------------------------------------------------------------------


@mcp.tool()
def check_docker_environment() -> str:
    """Check the local Docker environment for Diffusion compatibility.

    Validates: Docker daemon, Docker version, Docker Compose, available images,
    running containers, and common issues (WSL2 credential helper, cgroup config).
    """
    checks: list[dict[str, Any]] = []

    # Docker version
    result = _run(["docker", "version", "--format", "{{.Server.Version}}"])
    if result["returncode"] == 0:
        checks.append(
            {"check": "Docker daemon", "status": "ok", "version": result["stdout"]}
        )
    else:
        checks.append(
            {"check": "Docker daemon", "status": "error", "detail": result["stderr"]}
        )
        return json.dumps(
            {
                "checks": checks,
                "summary": "Docker daemon is not running or not installed.",
            },
            indent=2,
        )

    # Docker info (storage driver, cgroup)
    result = _run(
        [
            "docker",
            "info",
            "--format",
            "{{.Driver}} | cgroup={{.CgroupDriver}} | os={{.OperatingSystem}}",
        ]
    )
    if result["returncode"] == 0:
        checks.append(
            {"check": "Docker info", "status": "ok", "detail": result["stdout"]}
        )

    # Check for molecule containers
    result = _run(
        [
            "docker",
            "ps",
            "-a",
            "--filter",
            "name=molecule-",
            "--format",
            "{{.Names}} ({{.Status}})",
        ]
    )
    containers = result["stdout"].splitlines() if result["stdout"] else []
    checks.append(
        {
            "check": "Molecule containers",
            "status": "ok",
            "count": len(containers),
            "containers": containers,
        }
    )

    # Check for diffusion molecule image
    result = _run(
        [
            "docker",
            "images",
            "--filter",
            "reference=*diffusion-molecule*",
            "--format",
            "{{.Repository}}:{{.Tag}} ({{.Size}})",
        ]
    )
    images = result["stdout"].splitlines() if result["stdout"] else []
    checks.append(
        {
            "check": "Molecule images",
            "status": "ok" if images else "warning",
            "images": images or ["No diffusion-molecule images found locally"],
        }
    )

    # Check Docker credential helper (common WSL2 issue)
    docker_config_path = Path.home() / ".docker" / "config.json"
    if docker_config_path.exists():
        try:
            with open(docker_config_path, "r") as f:
                docker_cfg = json.load(f)
            creds_store = docker_cfg.get("credsStore", "")
            if "desktop.exe" in creds_store:
                checks.append(
                    {
                        "check": "Docker credential helper",
                        "status": "warning",
                        "detail": f"credsStore='{creds_store}' — may cause issues in WSL2. "
                        "Fix: change 'desktop.exe' to 'desktop' in ~/.docker/config.json",
                    }
                )
            else:
                checks.append(
                    {
                        "check": "Docker credential helper",
                        "status": "ok",
                        "detail": f"credsStore='{creds_store}'",
                    }
                )
        except Exception:
            pass

    # Disk space
    result = _run(
        [
            "docker",
            "system",
            "df",
            "--format",
            "{{.Type}}: {{.Size}} (reclaimable: {{.Reclaimable}})",
        ]
    )
    if result["returncode"] == 0:
        checks.append(
            {
                "check": "Docker disk usage",
                "status": "ok",
                "detail": result["stdout"].splitlines(),
            }
        )

    summary = (
        "all checks passed"
        if all(c["status"] == "ok" for c in checks)
        else "some issues detected"
    )
    return json.dumps({"checks": checks, "summary": summary}, indent=2)


# ---------------------------------------------------------------------------
# Tool: troubleshoot_molecule_container
# ---------------------------------------------------------------------------


@mcp.tool()
def troubleshoot_molecule_container(role: str) -> str:
    """Run a comprehensive diagnostic on a molecule container.

    Checks: container state, Docker-in-Docker status, Python/Ansible versions,
    molecule installation, network connectivity, mounted volumes, and common issues.

    Args:
        role: The role name (container will be molecule-<role>).
    """
    container = _container_name(role)
    diagnostics: list[dict[str, Any]] = []

    # 1. Container exists and is running
    result = _run(["docker", "inspect", "--format", "{{.State.Status}}", container])
    if result["returncode"] != 0:
        return json.dumps(
            {
                "container": container,
                "status": "not found",
                "suggestion": f"Container '{container}' does not exist. Run 'diffusion molecule -r {role} -o <org>' to create it.",
            },
            indent=2,
        )

    state = result["stdout"].strip()
    diagnostics.append({"check": "Container state", "result": state})

    if state != "running":
        diagnostics.append(
            {
                "check": "Container not running",
                "result": "error",
                "suggestion": f"Container is '{state}'. Try: docker start {container}",
            }
        )
        return json.dumps(
            {"container": container, "diagnostics": diagnostics}, indent=2
        )

    # 2. Docker-in-Docker status
    dind_result = _run(
        [
            "docker",
            "exec",
            container,
            "docker",
            "info",
            "--format",
            "{{.ServerVersion}}",
        ]
    )
    if dind_result["returncode"] == 0:
        diagnostics.append(
            {
                "check": "Docker-in-Docker",
                "result": "ok",
                "version": dind_result["stdout"],
            }
        )
    else:
        diagnostics.append(
            {
                "check": "Docker-in-Docker",
                "result": "error",
                "detail": dind_result["stderr"],
                "suggestion": "DinD daemon may not have started. Check container logs.",
            }
        )

    # 3. Python version
    py_result = _run(["docker", "exec", container, "python3", "--version"])
    diagnostics.append(
        {"check": "Python", "result": py_result["stdout"] or py_result["stderr"]}
    )

    # 4. Ansible version
    ansible_result = _run(
        ["docker", "exec", container, "/bin/sh", "-c", "ansible --version | head -1"]
    )
    diagnostics.append(
        {
            "check": "Ansible",
            "result": ansible_result["stdout"] or ansible_result["stderr"],
        }
    )

    # 5. Molecule version
    mol_result = _run(["docker", "exec", container, "molecule", "--version"])
    diagnostics.append(
        {"check": "Molecule", "result": mol_result["stdout"] or mol_result["stderr"]}
    )

    # 6. UV status
    uv_result = _run(["docker", "exec", container, "uv", "--version"])
    diagnostics.append(
        {"check": "uv", "result": uv_result["stdout"] or uv_result["stderr"]}
    )

    # 6a. UV virtual environment — check the venv exists and list installed packages
    venv_check = _run(
        [
            "docker",
            "exec",
            container,
            "/bin/sh",
            "-c",
            "test -d /opt/venv && echo 'venv present' || echo 'venv MISSING'",
        ]
    )
    diagnostics.append({"check": "uv venv (/opt/venv)", "result": venv_check["stdout"]})
    if "MISSING" not in venv_check.get("stdout", ""):
        pkg_result = _run(
            [
                "docker",
                "exec",
                container,
                "/bin/sh",
                "-c",
                "uv pip list --python /opt/venv/bin/python 2>/dev/null | head -30",
            ]
        )
        diagnostics.append(
            {
                "check": "uv pip list (top 30)",
                "result": pkg_result["stdout"] or pkg_result["stderr"],
            }
        )

    # 7. Check /opt/molecule contents
    ls_result = _run(["docker", "exec", container, "ls", "-la", "/opt/molecule/"])
    diagnostics.append(
        {
            "check": "/opt/molecule contents",
            "result": ls_result["stdout"] or "(empty or not mounted)",
        }
    )

    # 8. Disk usage inside container
    df_result = _run(["docker", "exec", container, "df", "-h", "/"])
    diagnostics.append({"check": "Disk usage", "result": df_result["stdout"]})

    # 9. Check molecule scenarios inside container
    scenarios_result = _run(
        [
            "docker",
            "exec",
            container,
            "/bin/sh",
            "-c",
            "find /opt/molecule -name molecule.yml -type f 2>/dev/null",
        ]
    )
    diagnostics.append(
        {
            "check": "Molecule scenarios found",
            "result": scenarios_result["stdout"] or "none",
        }
    )

    return json.dumps({"container": container, "diagnostics": diagnostics}, indent=2)


# ---------------------------------------------------------------------------
# Tool: get_requirements_yml
# ---------------------------------------------------------------------------


@mcp.tool()
def get_requirements_yml(project_path: str = "", scenario: str = "default") -> str:
    """Read and return the Ansible requirements.yml for a scenario.

    Args:
        project_path: Path to the project root (auto-detected if empty).
        scenario: Molecule scenario name (default: "default").
    """
    root = Path(project_path) if project_path else _find_project_root()
    if root is None:
        return "Error: Could not find project root."

    candidates = [
        root / "molecule" / scenario / "requirements.yml",
        root / "scenarios" / scenario / "requirements.yml",
        root / "requirements.yml",
    ]

    for c in candidates:
        if c.exists():
            try:
                data = _load_yaml(c)
                return json.dumps(
                    {"path": str(c), "content": data}, indent=2, default=str
                )
            except Exception as e:
                return f"Error reading {c}: {e}"

    return f"No requirements.yml found. Searched:\n" + "\n".join(
        f"  - {c}" for c in candidates
    )


# ---------------------------------------------------------------------------
# Tool: list_molecule_scenarios
# ---------------------------------------------------------------------------


@mcp.tool()
def list_molecule_scenarios(project_path: str = "") -> str:
    """List all Molecule scenarios in a project with their key files.

    Args:
        project_path: Path to the project root (auto-detected if empty).
    """
    root = Path(project_path) if project_path else _find_project_root()
    if root is None:
        return "Error: Could not find project root."

    scenarios: list[dict[str, Any]] = []

    # Check molecule/ and scenarios/ directories
    for base_dir in [root / "molecule", root / "scenarios"]:
        if not base_dir.exists():
            continue
        for entry in sorted(base_dir.iterdir()):
            if entry.is_dir() and (entry / "molecule.yml").exists():
                scenario_info: dict[str, Any] = {
                    "name": entry.name,
                    "path": str(entry),
                    "files": {},
                }
                for fname in [
                    "molecule.yml",
                    "converge.yml",
                    "verify.yml",
                    "requirements.yml",
                    "prepare.yml",
                    "cleanup.yml",
                ]:
                    fpath = entry / fname
                    scenario_info["files"][fname] = fpath.exists()

                # Check for tests directory
                tests_dir = entry / "tests"
                if tests_dir.exists():
                    test_files = list(tests_dir.rglob("*.yml"))
                    scenario_info["test_files"] = [
                        str(f.relative_to(entry)) for f in test_files
                    ]

                scenarios.append(scenario_info)

    if not scenarios:
        return "No Molecule scenarios found in molecule/ or scenarios/ directories."

    return json.dumps(scenarios, indent=2)


# ---------------------------------------------------------------------------
# Tool: run_diffusion_command
# ---------------------------------------------------------------------------


@mcp.tool()
def run_diffusion_command(
    subcommand: str,
    args: str = "",
    project_path: str = "",
) -> str:
    """Run a diffusion CLI command and return the output.

    Only allows safe, read-only or non-destructive commands.

    Args:
        subcommand: The diffusion subcommand (e.g. "show", "deps check", "cache status").
        args: Additional arguments as a string.
        project_path: Working directory (auto-detected if empty).
    """
    # Allowlist of safe subcommands
    safe_commands = {
        "show",
        "deps check",
        "deps resolve",
        "role",
        "deploy",
        "--version",
    }

    sub_lower = subcommand.strip().lower()
    if sub_lower not in safe_commands:
        return (
            f"Command '{subcommand}' is not in the safe allowlist. "
            f"Allowed: {', '.join(sorted(safe_commands))}. "
            "For destructive operations, run them directly in your terminal."
        )

    root = Path(project_path) if project_path else _find_project_root()
    cwd = str(root) if root else None

    cmd_parts = ["diffusion"] + subcommand.strip().split()
    if args:
        cmd_parts.extend(shlex.split(args))

    result = _run(cmd_parts, timeout=30, cwd=cwd)
    output_parts = []
    if result["stdout"]:
        output_parts.append(result["stdout"])
    if result["stderr"]:
        output_parts.append(f"[stderr] {result['stderr']}")
    if result["returncode"] != 0:
        output_parts.append(f"[exit code: {result['returncode']}]")

    return "\n".join(output_parts) if output_parts else "(no output)"


# ---------------------------------------------------------------------------
# Tool: get_terraform_provider_reference
# ---------------------------------------------------------------------------


@mcp.tool()
def get_terraform_provider_reference(resource: str = "") -> str:
    """Get the full reference for the diffusion Terraform provider.

    Covers: provider configuration, diffusion_deploy resource, and
    diffusion_inventory data source.

    Args:
        resource: Specific resource/data-source to look up:
                  "provider", "deploy", or "inventory".
                  Leave empty for the full reference.
    """
    ref: dict[str, Any] = {
        "provider": {
            "description": (
                "The diffusion Terraform provider wraps the diffusion CLI binary. "
                "All deploy logic lives in the CLI — the provider builds CLI arguments "
                "and executes them. Requires diffusion binary on PATH or provider config."
            ),
            "source": "registry.terraform.io/diffusion/diffusion",
            "binary": "diffusion-terraform-provider",
            "build": "make build-provider  # or: make dist-provider (all 8 platforms)",
            "schema": {
                "diffusion_binary": {
                    "type": "string",
                    "optional": True,
                    "description": "Path to diffusion binary. Default: 'diffusion' on PATH.",
                },
                "registry_server": {
                    "type": "string",
                    "optional": True,
                    "description": "Container registry server (e.g. ghcr.io).",
                },
                "registry_provider": {
                    "type": "string",
                    "optional": True,
                    "description": "Registry provider: Public | YC | AWS | GCP.",
                },
                "container_name": {
                    "type": "string",
                    "optional": True,
                    "description": "Molecule container image name.",
                },
                "container_tag": {
                    "type": "string",
                    "optional": True,
                    "description": "Molecule container image tag.",
                },
                "vault_addr": {
                    "type": "string",
                    "optional": True,
                    "description": "HashiCorp Vault address (VAULT_ADDR).",
                },
                "vault_token": {
                    "type": "string",
                    "optional": True,
                    "sensitive": True,
                    "description": "HashiCorp Vault token (VAULT_TOKEN).",
                },
                "host_wait_initial_delay": {
                    "type": "string",
                    "optional": True,
                    "default": "10s",
                    "description": "Default initial delay before first host probe.",
                },
                "host_wait_interval": {
                    "type": "string",
                    "optional": True,
                    "default": "5s",
                    "description": "Default interval between host probes.",
                },
                "host_wait_timeout": {
                    "type": "string",
                    "optional": True,
                    "default": "5m",
                    "description": "Default hard deadline for host reachability.",
                },
            },
            "example": (
                'terraform {\n'
                '  required_providers {\n'
                '    diffusion = {\n'
                '      source  = "registry.terraform.io/diffusion/diffusion"\n'
                '      version = ">= 0.1.0"\n'
                '    }\n'
                '  }\n'
                '}\n\n'
                'provider "diffusion" {\n'
                '  diffusion_binary  = "/usr/local/bin/diffusion"\n'
                '  registry_server   = "ghcr.io"\n'
                '  registry_provider = "Public"\n'
                '  vault_addr        = "https://vault.example.com"\n'
                '  vault_token       = var.vault_token\n'
                '  host_wait_timeout = "5m"\n'
                '}'
            ),
        },
        "deploy": {
            "type": "resource",
            "name": "diffusion_deploy",
            "description": (
                "Deploys Ansible roles to remote hosts using the diffusion molecule container. "
                "Roles and collections are installed INSIDE the container. "
                "Playbook is auto-generated from role_sources when not supplied. "
                "Delete is a no-op — deployments are one-way."
            ),
            "arguments": {
                "role_sources": {
                    "type": "list(object)",
                    "required": True,
                    "description": "Remote role repos to fetch diffusion.lock from.",
                    "nested_attributes": {
                        "scm": "string — 'git' or 'galaxy' (required)",
                        "version": "string — version constraint or git ref (required)",
                        "url": "string — git repo URL (required when scm=git)",
                        "galaxy": "string — Galaxy role name namespace.name (required when scm=galaxy)",
                        "name": "string — role name override in auto-generated playbook (optional)",
                        "apply_to": "string — Ansible hosts pattern for auto-generated play (optional, default: 'all')",
                    },
                },
                "playbook": "string, optional — path to existing playbook; omit to auto-generate",
                "hosts": "map(object) — hostname => { vars = { key = value } }",
                "groups": "map(list(string)) — group name => list of host names",
                "variables": "map(string) — global inventory variables",
                "extra_vars": "map(string) — extra vars for ansible-playbook --extra-vars",
                "skip_if_succeeded_within": "string — Go duration (e.g. '24h'). Skip if inputs identical and last run recent.",
                "host_wait_initial_delay": "string — override provider default initial delay",
                "host_wait_interval": "string — override provider default probe interval",
                "host_wait_timeout": "string — override provider default hard deadline",
            },
            "computed_attributes": {
                "run_id": "SHA-256 of all deploy inputs, first 16 hex chars",
                "last_deployed": "RFC3339 timestamp of last successful deploy",
                "merged_lock_hash": "Hash of merged diffusion.lock across all role sources",
                "inventory_rendered": "Rendered Ansible YAML inventory (for debugging)",
            },
            "example": (
                'resource "diffusion_deploy" "app" {\n'
                '  role_sources = [\n'
                '    {\n'
                '      scm     = "galaxy"\n'
                '      version = ">=6.0.0"\n'
                '      galaxy  = "geerlingguy.docker"\n'
                '    },\n'
                '    {\n'
                '      scm      = "git"\n'
                '      version  = "main"\n'
                '      url      = "https://github.com/myorg/ansible-app.git"\n'
                '      name     = "app"\n'
                '      apply_to = "webservers"\n'
                '    }\n'
                '  ]\n\n'
                '  hosts = {\n'
                '    web01 = { vars = { ansible_host = "1.2.3.4", ansible_user = "ubuntu" } }\n'
                '    web02 = { vars = { ansible_host = "1.2.3.5", ansible_user = "ubuntu" } }\n'
                '  }\n'
                '  groups    = { webservers = ["web01", "web02"] }\n'
                '  variables = { env = "production" }\n\n'
                '  skip_if_succeeded_within = "24h"\n'
                '  host_wait_timeout        = "10m"\n'
                '}'
            ),
        },
        "inventory": {
            "type": "data_source",
            "name": "diffusion_inventory",
            "description": (
                "Reads and parses a rendered Ansible YAML inventory, "
                "exposing structured host and group data."
            ),
            "arguments": {
                "inventory_yaml": "string — raw Ansible YAML inventory to parse (e.g. diffusion_deploy.app.inventory_rendered)",
            },
            "exported_attributes": {
                "hosts": "map(map(string)) — parsed hosts with connection variables",
                "groups": "map(list(string)) — parsed groups with member host names",
                "vars": "map(string) — parsed global all-group variables",
            },
            "example": (
                'data "diffusion_inventory" "hosts" {\n'
                '  inventory_yaml = diffusion_deploy.app.inventory_rendered\n'
                '}\n\n'
                'output "host_ips" {\n'
                '  value = data.diffusion_inventory.hosts.hosts\n'
                '}'
            ),
        },
    }

    r = resource.lower().strip()
    if r:
        if r in ref:
            return json.dumps(ref[r], indent=2)
        return (
            f"Unknown resource '{resource}'. "
            f"Available: {', '.join(ref.keys())}"
        )
    return json.dumps(ref, indent=2)


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------


@mcp.tool()
def get_server_version() -> str:
    """Return the version of the Diffusion MCP server and its key dependencies.

    Useful for confirming which server version is connected and checking
    that the Python environment is healthy.
    """
    import importlib.metadata

    info: dict[str, Any] = {}

    for pkg in ("diffusion-mcp", "mcp", "fastmcp"):
        try:
            info[pkg] = importlib.metadata.version(pkg)
        except importlib.metadata.PackageNotFoundError:
            info[pkg] = "not installed"

    info["python"] = sys.version
    return json.dumps(info, indent=2)


def main():
    """Run the Diffusion MCP server."""
    mcp.run()


if __name__ == "__main__":
    main()
