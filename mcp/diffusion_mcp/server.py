"""
Diffusion MCP Server

Provides tools for managing and troubleshooting Diffusion CLI local testing
environments, including:
- Diffusion config inspection (diffusion.toml, diffusion.lock)
- Molecule container management and docker exec helpers
- Molecule scenario file validation (molecule.yml, verify.yml)
- Dependency and cache status (incl. per-scenario lock analysis)
- CLI command reference, Terraform provider reference
- Ephemeral role patching (`diffusion patch`): static patch.yml validation and
  in-container overlay inspection
- Transitive (nested) dependency and git-sourced collection analysis
- Troubleshooting knowledge base (deps --scenario, transitive deps, git
  collections, patch bundles/overlays, deploy --ssh-key validation, CLI
  argument-injection guards, Terraform PEM normalisation, GitHub Actions
  diagnostics)
"""

from __future__ import annotations

import json
import os
import re
import shlex
import subprocess
import sys
from pathlib import Path
from typing import Any

from mcp.server.mcpserver import MCPServer

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

if sys.version_info >= (3, 11):
    import tomllib
else:
    import tomli as tomllib  # type: ignore[import-untyped]

import yaml

mcp = MCPServer("diffusion-mcp")


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


# --- SSH key name helpers (mirror internal/deploy/sanitize.go) ---------------

# Allowlist used by `diffusion deploy --ssh-key <name>=<base64>` and by the
# Terraform provider's ssh_private_keys map keys.
_SSH_KEY_NAME_ALLOWLIST = re.compile(r"^[A-Za-z0-9_.:*-]+$")
_SSH_KEY_WILDCARD_ENV = "WILDCARD"
_SSH_KEY_WILDCARD_FILE = "_wildcard_"


def _ssh_key_env_suffix(name: str) -> str:
    """Return the env var suffix diffusion uses for an SSH key name.

    '*' → 'WILDCARD'; otherwise '-', '.', ':', '/' → '_' and upper-cased.
    Full env var is 'SSH_KEY_' + suffix.
    """
    if name == "*":
        return _SSH_KEY_WILDCARD_ENV
    return (
        name.replace("-", "_").replace(".", "_").replace(":", "_").replace("/", "_")
    ).upper()


def _ssh_key_file_name(name: str) -> str:
    """Return the file name under /tmp/ssh-keys/ diffusion uses for a key name."""
    if name == "*":
        return _SSH_KEY_WILDCARD_FILE
    return name


def _validate_ssh_key_name(name: str) -> str | None:
    """Validate an SSH key/host name exactly like deploy.ValidateSSHKeyName.

    Returns None when valid, otherwise the diffusion-style error message.
    """
    if not name or not _SSH_KEY_NAME_ALLOWLIST.match(name):
        return (
            f"invalid SSH key/host name {name!r}: only letters, digits, "
            "'.', '-', '_', ':', '*' are allowed"
        )
    if name in (".", "..") or name.endswith(":.") or name.endswith(":.."):
        return f"invalid SSH key/host name {name!r}: dot-only path segments are not allowed"
    if name == "*":
        return None
    if (
        _ssh_key_env_suffix(name).lower() == _SSH_KEY_WILDCARD_ENV.lower()
        or _ssh_key_file_name(name) == _SSH_KEY_WILDCARD_FILE
    ):
        return (
            f"invalid SSH key/host name {name!r}: collides with the reserved "
            "wildcard ('*') key sentinel"
        )
    return None


# --- Scenario helpers (mirror internal/dependency/dependency_scenario.go) ------


def _discover_scenarios(root: Path) -> list[str]:
    """List scenario directories under scenarios/; fallback to ['default']."""
    scenarios_dir = root / "scenarios"
    found: list[str] = []
    if scenarios_dir.is_dir():
        found = sorted(p.name for p in scenarios_dir.iterdir() if p.is_dir())
    return found or ["default"]


def _toml_dependency_scenarios(root: Path) -> set[str]:
    """Return scenario prefixes referenced by [dependencies] in diffusion.toml."""
    toml_path = root / "diffusion.toml"
    if not toml_path.exists():
        return set()
    try:
        data = _load_toml(toml_path)
    except Exception:
        return set()
    deps = _ci_get(data, "dependencies", {}) or {}
    names: list[str] = []
    for section in ("collections", "roles"):
        entries = _ci_get(deps, section, []) or []
        for e in entries:
            n = _dep_name(e)
            if n:
                names.append(n)
    return {n.split(".", 1)[0] for n in names if "." in n}


def _lock_scenarios(lock: dict[str, Any]) -> dict[str, dict[str, int]]:
    """Group diffusion.lock collections/roles by their '<scenario>.' prefix."""
    out: dict[str, dict[str, int]] = {}
    for section in ("collections", "roles"):
        for e in lock.get(section, []) or []:
            n = e.get("name", "") if isinstance(e, dict) else str(e)
            scen = n.split(".", 1)[0] if "." in n else "(unprefixed)"
            out.setdefault(scen, {"collections": 0, "roles": 0})
            out[scen][section] += 1
    return out


# --- TOML key helpers ---------------------------------------------------------
#
# diffusion writes [dependencies] entries with capitalised keys (Name,
# Namespace, Version, Source, SourceURL, Src, Scm) while BurntSushi/toml also
# accepts other casings on read. Always look keys up case-insensitively.


def _ci_get(d: Any, key: str, default: Any = None) -> Any:
    """Case-insensitive dict lookup (mirrors BurntSushi/toml key matching)."""
    if not isinstance(d, dict):
        return default
    if key in d:
        return d[key]
    lk = key.lower()
    for k, v in d.items():
        if isinstance(k, str) and k.lower() == lk:
            return v
    return default


def _dep_name(e: Any) -> str:
    """Return the Name of a [dependencies] collection/role entry."""
    if isinstance(e, dict):
        return str(_ci_get(e, "name", "") or "")
    return str(e)


def _load_dependency_config(root: Path) -> dict[str, Any]:
    """Return the [dependencies] table of diffusion.toml ({} when absent)."""
    toml_path = root / "diffusion.toml"
    if not toml_path.exists():
        return {}
    try:
        return _ci_get(_load_toml(toml_path), "dependencies", {}) or {}
    except Exception:
        return {}


# --- CLI argument guard (mirror internal/utils.ValidateCLIArgument) ----------


def _validate_cli_argument(kind: str, value: str) -> str | None:
    """Reject values git/ansible-galaxy would parse as an option.

    diffusion refuses any URL, ref, version, role, scenario or container name
    starting with '-' (e.g. '--upload-pack=/bin/sh') before it reaches a
    git / ansible-galaxy / docker argument vector. Returns the diffusion-style
    error message, or None when the value is acceptable.
    """
    if value.startswith("-"):
        return f'refusing to use "{value}": {kind} looks like a command line option'
    return None


# --- Git / transitive dependency helpers (mirror internal/dependency) --------

_COLLECTION_URL_PREFIXES = (
    "ansible-collection-",
    "ansible_collection_",
    "ansible-collection_",
    "ansible_collection-",
)
_TRANSITIVE_MAX_DEPTH = 10  # dependency.DefaultMaxTransitiveDepth


def _is_git_url(s: str) -> bool:
    """utils.IsGitURL: '://' anywhere or an scp-style 'git@' prefix."""
    s = (s or "").strip()
    return bool(s) and ("://" in s or s.startswith("git@"))


def _derive_collection_short_name(git_url: str) -> str:
    """utils.DeriveCollectionShortName: repo basename used as diffusion.toml key.

    https://github.com/org/ansible-collection-foo.git -> foo
    git@github.com:org/my.repo.git                    -> my_repo
    """
    name = (git_url or "").strip().rstrip("/")
    idx = max(name.rfind("/"), name.rfind(":"))
    if idx != -1:
        name = name[idx + 1 :]
    if name.endswith(".git"):
        name = name[: -len(".git")]
    for prefix in _COLLECTION_URL_PREFIXES:
        if len(name) > len(prefix) and name.lower().startswith(prefix):
            name = name[len(prefix) :]
            break
    return name.replace(".", "_")


def _normalize_identity(identity: str) -> str:
    """dependency.normalizeIdentity: stable comparison key for URLs / names.

    https://GitHub.com/Org/Repo.git/ -> github.com/org/repo
    git@github.com:org/repo          -> github.com/org/repo
    community.general                -> community.general
    """
    s = (identity or "").strip().lower()
    if not s:
        return ""
    scp = False
    for pfx in ("https://", "http://", "ssh://", "git+ssh://", "git://"):
        if s.startswith(pfx):
            s = s[len(pfx) :]
            break
    if s.startswith("git@"):
        s = s[len("git@") :]
        scp = True
    at = s.find("@")
    if at != -1 and "/" not in s[:at]:
        s = s[at + 1 :]
        scp = True
    if scp:
        s = s.replace(":", "/", 1)
    if s.endswith("/"):
        s = s[:-1]
    if s.endswith(".git"):
        s = s[: -len(".git")]
    if s.endswith("/"):
        s = s[:-1]
    return s


def _lock_entry_identity(e: dict[str, Any]) -> str:
    """dependency.entryIdentity: git URL for git entries, else namespace.name."""
    src = str(e.get("src", "") or "")
    if src:
        return _normalize_identity(src)
    name = str(e.get("name", "") or "")
    if "." in name:
        name = name.split(".", 1)[1]
    ns = str(e.get("namespace", "") or "")
    return (f"{ns}.{name}" if ns else name).lower()


def _is_git_collection(e: dict[str, Any]) -> bool:
    """dependency.IsGitCollection for a diffusion.lock collection entry.

    The lock's 'scm' key holds the source type (LockFileEntry.Source).
    """
    scm = str(e.get("scm", "") or "")
    if scm == "git":
        return True
    return bool(e.get("src")) and scm != "galaxy"


def _resolve_git_ref(version: str) -> str:
    """dependency.ResolveGitRef: constraint -> '' (default branch), else ref."""
    if not version or version == "latest":
        return ""
    if version.startswith((">=", "<=", ">", "<", "==")):
        return ""
    return version


def _detect_self_identity(root: Path) -> list[str]:
    """dependency.DetectSelfIdentity: git origin URL, meta role_name, dir name."""
    identities: list[str] = []
    res = _run(["git", "-C", str(root), "remote", "get-url", "origin"], timeout=10)
    if res["returncode"] == 0 and res["stdout"]:
        identities.append(res["stdout"].strip())
    meta_path = root / "meta" / "main.yml"
    if meta_path.exists():
        try:
            meta = _load_yaml(meta_path) or {}
            gi = meta.get("galaxy_info", {}) or {}
            role_name = str(gi.get("role_name", "") or "")
            if role_name:
                identities.append(role_name)
                ns = str(gi.get("namespace", "") or "")
                if ns:
                    identities.append(f"{ns}.{role_name}")
        except Exception:
            pass
    if root.name:
        identities.append(root.name)
    return identities


# --- Patch helpers (mirror internal/patch) -------------------------------------

_CONTAINER_ROLES_PATH = "/root/.ansible/roles"  # config.ContainerRolesCachePath
_CONTAINER_PATCH_DIR = "/var/lib/diffusion-patch"  # config.ContainerPatchDir
_SAFE_CONTAINER_NAME = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")
_PATCH_MAX_TASK_ID_DEPTH = 32
_PATCH_CONDITION_KEYS = ("when", "changed_when", "failed_when")
_PATCH_TOP_KEY = "Bundles"  # yaml.v3 tag is case-sensitive
_PATCH_BUNDLE_KEYS = {"patch_bundle_name", "tasks_to_patch", "scenario", "role_name"}
_PATCH_TASK_KEYS = {
    "task_id",
    "new_module",
    "new_module_setup",
    "new_conditions",
    "new_become_user",
    "new_environment",
    "new_block",
    "new_file_src",
    "new_template_src",
}


def _parse_patch_task_id(task_id: Any) -> tuple[bool, list[int]] | str:
    """patch.splitTaskID + ParseTaskID.

    Accepts '1', '1.3.1' (tasks) and 'h1', 'H1.2' (handlers). Returns
    (is_handler, segments) or an error string.
    """
    raw = str(task_id if task_id is not None else "").strip()
    handler = False
    if len(raw) > 1 and raw[0] in "hH":
        handler = True
        raw = raw[1:].strip()
    if not raw:
        return "task id is empty"
    parts = raw.split(".")
    if len(parts) > _PATCH_MAX_TASK_ID_DEPTH:
        return f"task id {task_id!r} exceeds max depth of {_PATCH_MAX_TASK_ID_DEPTH}"
    out: list[int] = []
    for p in parts:
        if p == "":
            return f"invalid task id {task_id!r}: empty segment"
        if len(p) > 1 and p[0] == "0":
            return f"invalid task id {task_id!r}: segment {p!r} has leading zero"
        if not p.isdigit() or int(p) < 1:
            return f"invalid task id {task_id!r}: segment {p!r} must be a positive integer"
        out.append(int(p))
    return handler, out


def _canonical_patch_task_id(task_id: Any) -> str | None:
    parsed = _parse_patch_task_id(task_id)
    if isinstance(parsed, str):
        return None
    handler, parts = parsed
    return ("h" if handler else "") + ".".join(str(n) for n in parts)


def _patch_role_name_matches(bundle_role: str, requirement_role: str) -> bool:
    """patch.roleNameMatches: short bundle names match namespaced entries."""
    b, r = (bundle_role or "").strip(), (requirement_role or "").strip()
    if not b or not r:
        return False
    if b.lower() == r.lower():
        return True
    if "." in b:
        return False
    return b.lower() == r.rsplit(".", 1)[-1].lower()


def _toml_roles_for_patch(root: Path, bundle_scenario: str) -> list[str]:
    """Roles a bundle may target (patch.validateRoleName).

    A scoped bundle only sees '<scenario>.<role>' entries; an unscoped bundle
    (empty 'scenario') accepts entries from any scenario.
    """
    deps = _load_dependency_config(root)
    available: list[str] = []
    for entry in _ci_get(deps, "roles", []) or []:
        short = _dep_name(entry).strip()
        if bundle_scenario:
            prefix = bundle_scenario + "."
            if not short.startswith(prefix):
                continue
            short = short[len(prefix) :]
        elif "." in short:
            short = short.split(".", 1)[1]
        ns = str(_ci_get(entry, "namespace", "") or "").strip() if isinstance(entry, dict) else ""
        available.append(f"{ns}.{short}" if ns else short)
    return available


_YAML_NULLS = {"", "~", "null", "Null", "NULL"}


def _load_patch_yaml(path: Path) -> Any:
    """Load patch.yml keeping every scalar as its raw string (like yaml.v3
    decoding into Go string fields)."""
    with open(path, "r", encoding="utf-8") as f:
        return yaml.load(f, Loader=yaml.BaseLoader)  # noqa: S506 - BaseLoader is safe


def _yaml_truthy(v: Any) -> bool:
    """YAML 1.1/1.2 boolean truthiness for BaseLoader raw strings."""
    if isinstance(v, bool):
        return v
    return str(v).strip().lower() in ("true", "yes", "on", "y")


def _patch_overlay_path(root: Path, scenario: str, subdir: str, value: str) -> Path:
    return root / "scenarios" / (scenario or "default") / "patch" / subdir / Path(value)


def _validate_patch_task(
    task: Any, root: Path, scenario: str, where: str
) -> tuple[list[str], list[str]]:
    """Mirror PatchingTask.validate plus apply-time limitations.

    Returns (errors, warnings). 'scenario' is the bundle's scenario field
    (empty -> default), which is what diffusion uses to locate overlays at
    validation time.
    """
    errors: list[str] = []
    warnings: list[str] = []
    if not isinstance(task, dict):
        return [f"{where}: task entry must be a mapping"], warnings

    unknown = sorted(set(task) - _PATCH_TASK_KEYS)
    if unknown:
        warnings.append(
            f"{where}: unknown key(s) {unknown} are silently ignored by diffusion "
            f"(allowed: {sorted(_PATCH_TASK_KEYS)})"
        )

    tid = task.get("task_id")
    if tid is None or str(tid).strip() == "":
        errors.append(f"{where}: task_id is required")
    else:
        parsed = _parse_patch_task_id(tid)
        if isinstance(parsed, str):
            errors.append(f"{where}: {parsed}")

    file_src = str(task.get("new_file_src", "") or "").strip()
    tpl_src = str(task.get("new_template_src", "") or "").strip()
    new_module = str(task.get("new_module", "") or "").strip()
    module_setup = task.get("new_module_setup") or []
    conditions = task.get("new_conditions") or []
    become = task.get("new_become_user")
    if isinstance(become, str) and become.strip() in _YAML_NULLS:
        become = None  # yaml.v3 leaves the *PatchBecomeSetup pointer nil
    env = task.get("new_environment") or []
    block = task.get("new_block") or []

    if not any([file_src, tpl_src, new_module, module_setup, conditions, become is not None, env, block]):
        errors.append(f"{where}: no patch action specified")
    if file_src and tpl_src:
        errors.append(f"{where}: new_file_src and new_template_src are mutually exclusive")
    if (file_src or tpl_src) and (new_module or module_setup):
        errors.append(f"{where}: src overlay cannot be combined with new_module/new_module_setup")

    for value, subdir, field in ((file_src, "files", "new_file_src"), (tpl_src, "templates", "new_template_src")):
        if not value:
            continue
        if Path(value).is_absolute() or value.startswith(("/", "\\")):
            errors.append(f"{where}: {field} must be relative, got {value!r}")
            continue
        norm = os.path.normpath(value)
        if norm == ".." or norm.startswith(".." + os.sep) or norm.startswith("../"):
            errors.append(f"{where}: {field} escapes the patch folder: {value!r}")
            continue
        full = _patch_overlay_path(root, scenario, subdir, value)
        if not full.exists():
            errors.append(f"{where}: {field} overlay not found: {full}")
        elif full.is_dir():
            errors.append(f"{where}: {field} overlay is a directory: {full}")

    for i, c in enumerate(conditions if isinstance(conditions, list) else []):
        if not isinstance(c, dict):
            errors.append(f"{where}: condition {i} must be a mapping with 'condition' and 'body'")
            continue
        key = str(c.get("condition", "") or "").strip().lower()
        if key not in _PATCH_CONDITION_KEYS:
            errors.append(
                f"{where}: condition {i}: condition key must be one of when, changed_when, "
                "failed_when (migrate: put the key in condition: and the expression in body:)"
            )
        body = c.get("body")
        body_s = str(body).strip() if body is not None else ""
        if not body_s:
            errors.append(f"{where}: condition {i} has empty body")
        elif body_s.lower() in ("true", "false"):
            warnings.append(
                f"{where}: static {key or 'when'} {body_s.lower()} — diffusion will warn "
                "'possible design problem, verify intent' at apply time"
            )

    for label, items in (("module setup", module_setup), ("environment", env)):
        for i, kv in enumerate(items if isinstance(items, list) else []):
            k = kv.get("key") if isinstance(kv, dict) else None
            if k is None or (isinstance(k, str) and k in _YAML_NULLS - {""}):
                errors.append(f"{where}: {label} {i} has nil key")
            elif isinstance(k, str) and not k.strip():
                errors.append(f"{where}: {label} {i} has empty key")

    if become is not None:
        if not isinstance(become, dict) or (
            not str(become.get("user", "") or "").strip() and not _yaml_truthy(become.get("become", ""))
        ):
            errors.append(f"{where}: become setup sets neither user nor become")

    if block:
        errors.append(
            f"{where}: new_block passes validation but apply fails with "
            "'new_block apply is not supported in v1'"
        )
        for i, sub in enumerate(block if isinstance(block, list) else []):
            e2, w2 = _validate_patch_task(sub, root, scenario, f"{where} block {i}")
            errors.extend(e2)
            warnings.extend(w2)

    if module_setup and not new_module:
        warnings.append(
            f"{where}: new_module_setup without new_module MERGES into existing module args; "
            "it fails with 'module args are not a mapping' for free-form tasks (e.g. command: foo)"
        )
    return errors, warnings


def _validate_patch_config(root: Path, scenario: str) -> dict[str, Any]:
    """Statically validate scenarios/<scenario>/patch.yml like diffusion does
    on load (PatchesConfig.validate), plus extra lint for silent pitfalls."""
    path = root / "scenarios" / scenario / "patch.yml"
    report: dict[str, Any] = {"patch_file": str(path), "scenario": scenario}
    errors: list[str] = []
    warnings: list[str] = []
    bundles_out: list[dict[str, Any]] = []

    if not path.exists():
        report["present"] = False
        report["note"] = (
            "No patch.yml: 'diffusion molecule' skips patching silently, but "
            "'diffusion patch list|check' fail with 'failed to read patch config file'."
        )
        report["errors"], report["warnings"], report["bundles"] = [], [], []
        return report
    report["present"] = True

    try:
        data = _load_patch_yaml(path)
    except Exception as e:
        report["errors"] = [f"failed to unmarshal patch config: {e}"]
        report["warnings"], report["bundles"] = [], []
        return report

    if data is None:
        data = {}
    if not isinstance(data, dict):
        report["errors"] = ["patch.yml must be a mapping with a top-level 'Bundles:' list"]
        report["warnings"], report["bundles"] = [], []
        return report

    bundles = data.get(_PATCH_TOP_KEY)
    if bundles is None:
        variant = next((k for k in data if isinstance(k, str) and k.lower() == "bundles"), None)
        if variant is not None:
            errors.append(
                f"top-level key is {variant!r} but diffusion only reads 'Bundles' (case-sensitive): "
                "the file parses to ZERO bundles and molecule runs unpatched"
            )
        else:
            warnings.append("no 'Bundles:' key — nothing to patch")
        bundles = []
    if not isinstance(bundles, list):
        errors.append("'Bundles' must be a list")
        bundles = []

    for k in data:
        if k != _PATCH_TOP_KEY and not (isinstance(k, str) and k.lower() == "bundles"):
            warnings.append(f"unknown top-level key {k!r} is ignored")

    seen_names: set[str] = set()
    for bi, b in enumerate(bundles):
        where_b = f"bundle[{bi}]"
        if not isinstance(b, dict):
            errors.append(f"{where_b}: must be a mapping")
            continue
        name = str(b.get("patch_bundle_name", "") or "").strip()
        where_b = f"bundle {name!r}" if name else where_b
        role = str(b.get("role_name", "") or "").strip()
        b_scen = str(b.get("scenario", "") or "").strip()
        tasks = b.get("tasks_to_patch") or []
        info: dict[str, Any] = {
            "name": name,
            "role_name": role,
            "scenario_field": b_scen or "(empty = any scenario)",
            "tasks": len(tasks) if isinstance(tasks, list) else 0,
        }
        bundles_out.append(info)

        unknown = sorted(set(b) - _PATCH_BUNDLE_KEYS)
        if unknown:
            warnings.append(f"{where_b}: unknown key(s) {unknown} are silently ignored")
        if not name:
            errors.append(f"{where_b}: bundle name is required")
        elif name in seen_names:
            errors.append(f"duplicate patch bundle name {name!r}")
        seen_names.add(name)

        if b_scen and b_scen != scenario:
            warnings.append(
                f"{where_b}: 'scenario: {b_scen}' differs from the patch.yml location "
                f"(scenarios/{scenario}/). Role/overlay validation uses '{b_scen}' while "
                f"apply reads overlays from scenarios/{scenario}/patch/ — keep them equal."
            )

        if not role:
            errors.append(f"{where_b}: role name is required")
        else:
            available = _toml_roles_for_patch(root, b_scen)
            info["roles_available_in_diffusion_toml"] = available
            if not _ci_get(_load_dependency_config(root), "roles"):
                errors.append(f"{where_b}: no roles declared in diffusion.toml [dependencies]")
            elif not available:
                errors.append(
                    f"{where_b}: no roles declared for scenario {b_scen!r} in diffusion.toml [dependencies]"
                )
            elif not any(_patch_role_name_matches(role, r) for r in available):
                errors.append(
                    f"{where_b}: role {role!r} not found in diffusion.toml [dependencies] "
                    f"for scenario {b_scen!r} (available roles: {', '.join(available)})"
                )
            else:
                short_hits = [r for r in available if _patch_role_name_matches(role, r)]
                if "." not in role and len(short_hits) > 1:
                    warnings.append(
                        f"{where_b}: short role name {role!r} matches {short_hits}; inside the "
                        "container this is 'ambiguous' — use the full namespace.role name"
                    )

        if not isinstance(tasks, list) or not tasks:
            errors.append(f"{where_b}: no tasks to patch")
            continue
        seen_ids: set[str] = set()
        for ti, t in enumerate(tasks):
            where_t = f"{where_b} task {ti}"
            e2, w2 = _validate_patch_task(t, root, b_scen, where_t)
            errors.extend(e2)
            warnings.extend(w2)
            canon = _canonical_patch_task_id(t.get("task_id") if isinstance(t, dict) else None)
            if canon:
                if canon in seen_ids:
                    errors.append(f"{where_b}: duplicate task_id {canon!r}")
                seen_ids.add(canon)

    report["bundles"] = bundles_out
    report["errors"] = errors
    report["warnings"] = warnings
    return report


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

    lines = [line for line in result["stdout"].splitlines() if line.strip()]
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
# Tool: docker_in_docker_in_molecule
# ---------------------------------------------------------------------------


@mcp.tool()
def docker_in_docker_in_molecule(
    container_name: str, command: str, workdir: str = "", timeout: int = 300
) -> str:
    """Execute a command inside scenario container for troubleshooting.

    Args:
        container_name: Name of container defined in molecule.yml (e.g. "Ubunut-24-04")
        command: Shell command to run inside the scenario container
        workdir: Optional working directory inside the container
        timeout: Command timeout in seconds (default 300). Increase for network operations.
    """
    molecule_container = _container_name(container_name)
    scenario_docker_cmd = ["docker", "exec", molecule_container, "docker", "exec"]

    if workdir:
        scenario_docker_cmd.extend(["-w", workdir])
    scenario_docker_cmd.extend([container_name, "/bin/sh", "-c", command])

    result = _run(scenario_docker_cmd, timeout=timeout)
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
            f"""molecule.yml found in scenarios/{scenario}/ but molecule/{
                scenario
            }/ does not exist."""
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
                # (diffusion-ansible-tests-role defaults/main.yml)
                known_vars = [
                    "verify_ports",
                    "verify_docker_containers",
                    "verify_docker_user",
                    "verify_docker_rootless",
                    "verify_docker_env_override",
                    "verify_output_in_cmd",
                    "verify_uris",
                    "uri_test_ip",
                    "uri_validate_certs",
                    "uri_timeout",
                    "uri_expected_status",
                    "uri_follow_redirects",
                    "postgres_host",
                    "postgres_port",
                    "postgres_db",
                    "postgres_user",
                    "postgres_password",
                    "postgres_expected_tables",
                    "postgres_expected_records",
                    "postgres_expected_roles",
                ]
                for v in var_keys:
                    if v in known_vars:
                        info.append(
                            f"""    ✓ {v}: {
                                len(task_vars[v])
                                if isinstance(task_vars[v], list)
                                else "set"
                            }"""
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
                 "role", "artifact", "show", "docs", "deploy", "patch"). Use
                 "role add-collection", "deps lock" or "patch apply" for
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
                "Scenario patches: when scenarios/<scenario>/patch.yml declares bundles, converge, verify and the default flow AUTO-APPLY them inside the container right before molecule runs and remove the overlay again afterwards (see 'diffusion patch'). A patch error FAILS the run ('patch apply failed: ...') — testing pristine roles silently would give false confidence. Without patch.yml nothing changes.",
                "Before auto-patching (and without --force) diffusion runs 'ansible-galaxy install -r molecule/<scenario>/requirements.yml' so the targeted roles exist in the container; a failure there is only a warning ('warning: galaxy install before patching failed').",
                "--force now runs 'ansible-galaxy install --force -r molecule/<scenario>/requirements.yml' as a separate best-effort step BEFORE patching (previously chained into the converge command), so a forced reinstall can never wipe the patch overlay.",
                "--destroy and --wipe first remove any leftover patch overlay (bind mounts under /root/.ansible/roles and /var/lib/diffusion-patch/<scenario>) so molecule sees pristine roles.",
                "CI mode clones the repository inside the container with up to 10 attempts; after the final failure: 'failed to clone repository —container after 10 attempts: <err>'.",
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
                "--scenario, -s": {
                    "description": "Molecule scenario name to run (maps to scenarios/<name>/). Also selects which requirements.yml is installed.",
                    "default": "default",
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
                    "description": "Force reinstall of roles/collections from requirements.yml before converge (best-effort, runs before scenario patches are applied)",
                    "default": "false",
                },
            },
            "workflow_order": [
                "1. Container creation (docker run with DinD image, volume mounts, env vars)",
                "2. Cache loading (roles, collections, UV packages, Docker images)",
                "3. CI repo clone or local file copy into /opt/molecule/<org>.<role>/",
                "4. Registry login inside container (provider-specific)",
                "5. uv-sync (install Python deps from pyproject.toml)",
                "6. ansible-galaxy install --force (if --force, best-effort)",
                "7. Scenario patches (only if scenarios/<scenario>/patch.yml has bundles): ansible-galaxy install, then copy → patch → bind-mount overlay inside the container",
                "8. molecule create + molecule converge (default flow) / molecule verify",
                "9. Patch overlay removed (bind mounts unmounted)",
                "10. Permission fix (chown on /opt/molecule for Unix)",
            ],
            "examples": [
                "diffusion molecule                                      # Interactive setup + converge",
                "diffusion molecule --ci                                 # CI mode: create container + converge",
                "diffusion molecule --verify                             # Run all verification tests",
                "diffusion molecule --verify --tag ports                 # Run only port tests",
                "diffusion molecule --ci --verify -t 'ports,docker'      # Multiple tags",
                "diffusion molecule --lint                               # Run yamllint + ansible-lint",
                "diffusion molecule --idempotence                        # Idempotence check",
                "diffusion molecule --destroy                            # Destroy molecule instances (keep container)",
                "diffusion molecule --wipe                               # Full cleanup: destroy + remove container + folder",
                "diffusion molecule --converge --force                   # Force reinstall deps then converge",
                "diffusion molecule --ci --scenario production --verify  # Run verify for a non-default scenario",
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
                "/var/lib/diffusion-patch/<scenario>/work/<role>": "Patched role copy, bind-mounted over /root/.ansible/roles/<role> while patched",
                "/var/lib/diffusion-patch/<scenario>/backup/<role>": "Pristine copy of the role taken before patching (audit)",
            },
        },
        "role": {
            "description": "Manage Ansible role configuration, initialization, and dependencies",
            "usage": "diffusion role [flags]",
            "notes": [
                "Without flags or subcommands, displays current role config from meta/main.yml.",
                "Roles are stored per-scenario in diffusion.toml as '<scenario>.<role_name>'.",
                "Dots are forbidden in role/collection names (reserved as scenario prefixes).",
                "Values passed as positional arguments to ansible-galaxy/git (role names, URLs, versions) must not start with '-': 'refusing to use \"<v>\": <kind> looks like a command line option'.",
            ],
            "flags": {
                "--init, -i": {
                    "description": "Initialize a new Ansible role via ansible-galaxy init (role name must not start with '-')",
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
                    "description": "Add a Galaxy or git-sourced collection to diffusion.toml and re-lock the --scenario",
                    "args": "[collection-name] — short name without namespace and without dots (may carry a constraint, e.g. 'general>=9.0.0'; an explicit --version wins)",
                    "flags": {
                        "--scenario, -s": {
                            "description": "Molecule scenario folder",
                            "default": "default",
                        },
                        "--namespace, -n": {
                            "description": "Galaxy namespace — REQUIRED for Galaxy collections, optional (stored for reference only) for git collections",
                            "default": "",
                        },
                        "--src": {
                            "description": "Git URL of the collection. Setting it switches the collection to a git source (diffusion.toml: Source = <scm>, SourceURL = <url>)",
                            "default": "",
                        },
                        "--scm": {
                            "description": "SCM type, only used together with --src",
                            "default": "git",
                        },
                        "--version, -v": {
                            "description": "Version, tag, branch or constraint. Empty/'latest': Galaxy → resolve latest and store '>=<v>'; git → resolve highest remote tag and store '>=<v>' (falls back to branch 'main' when no usable tag). A branch name (main, develop) or explicit constraint is stored verbatim.",
                            "default": "",
                        },
                    },
                    "behavior": [
                        "Galaxy without --namespace fails: '--namespace/-n is required for Galaxy collections.'",
                        "Git collections are written to requirements.yml in ansible-galaxy git form: '- name: <git url>\\n  type: git\\n  version: <ref>'.",
                        "Git collections cannot be expressed in meta/main.yml (it only accepts namespace.name) and are skipped there.",
                        "Prints 'Note: git collections cannot be listed in meta/main.yml — they are written to requirements.yml only' after a git add.",
                    ],
                    "examples": [
                        "diffusion role add-collection general --namespace community",
                        "diffusion role add-collection docker --namespace community --scenario production",
                        "diffusion role add-collection foo --src https://github.com/org/ansible-collection-foo.git --version main",
                        "diffusion role add-collection foo --src https://github.com/org/ansible-collection-foo.git   # resolves highest tag → '>=<tag>'",
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
                "lock, check and sync accept --scenario/-s to operate on a single scenario. Default (empty) = all scenarios discovered under scenarios/ (falls back to 'default' if scenarios/ is absent).",
                "A scenario is valid if scenarios/<name>/ exists, OR (fallback) if any [dependencies] entry in diffusion.toml is prefixed '<name>.'. Otherwise: error 'scenario \"<name>\" not found (no scenarios/<name> directory)'.",
                "meta/main.yml (default-scenario collections) is processed only when the selector is empty or 'default' — a scoped run for another scenario never touches meta/main.yml.",
                "'diffusion role add-role/remove-role/add-collection/remove-collection' pass their --scenario to the lock update, so they perform a scenario-scoped lock merge.",
                "diffusion.lock is a YAML file (with a generated header comment); diffusion.toml is TOML with capitalised entry keys (Name, Namespace, Version, Source, SourceURL, Src, Scm).",
                "Transitive dependencies (default ON): a git-sourced role/collection whose repo contains a diffusion.lock (or only a diffusion.toml) contributes its DEFAULT-scenario collections and roles to your lock, re-prefixed with the scenario that pulled them in. Recurses through nested git deps up to depth 10; constraints from all sources are intersected and re-resolved. Pulled-in entries carry 'required_by: <normalised identity of the parent>'.",
                "Disable transitive resolution per run with 'deps lock --no-transitive' or permanently with 'transitive = false' under [dependencies] in diffusion.toml.",
                "Git-sourced collections: diffusion.toml entry with Source = \"git\" and SourceURL = <url>; lock entry with scm: git and src: <url>; requirements.yml '- name: <url>, type: git, version: <ref>'. Never written to meta/main.yml.",
                "The lock hash of a project with only Galaxy collections is unchanged by the git-collection feature (Source/SourceURL are only hashed when they carry information).",
            ],
            "diffusion_toml_dependencies_keys": {
                "transitive": "bool, optional — default true. false disables nested dependency resolution.",
                "collections[].Source": "'galaxy' (default when empty) or 'git'",
                "collections[].SourceURL": "git URL — REQUIRED when Source is not galaxy, otherwise deps lock fails",
                "roles[].Src / roles[].Scm": "git URL and SCM type for git roles",
            },
            "lock_entry_fields": {
                "name": "'<scenario>.<short name>'",
                "namespace": "Galaxy namespace",
                "version": "declared constraint",
                "resolved_version": "pinned version",
                "scm": "source type: galaxy | git (LockFileEntry.Source)",
                "src": "git URL for git roles/collections",
                "required_by": "NEW — identity of the dependency that pulled this entry in transitively (empty = direct dependency)",
                "python_deps": "pip packages required by the collection",
            },
            "scenario_flag": {
                "flag": "--scenario, -s",
                "default": "(empty = all scenarios)",
                "applies_to": ["lock", "check", "sync"],
                "validation": "scenarios/<name>/ directory OR '<name>.' prefix in diffusion.toml [dependencies]",
            },
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
                        "Recognises git collections in requirements.yml (type: git or a URL-shaped name) and stores them as '<scenario>.<repo basename>' with Source='git' / SourceURL=<url>; 'ansible-collection-' / 'ansible_collection_' prefixes are stripped and remaining dots become '_'",
                        "Never reads diffusion.lock, so transitive (required_by) entries never leak into diffusion.toml as direct dependencies",
                    ],
                },
                "lock": {
                    "usage": "diffusion deps lock [--scenario <name>] [--no-transitive]",
                    "description": "Generate or update diffusion.lock from current dependencies in meta/main.yml, requirements.yml, and diffusion.toml",
                    "flags": {
                        "--scenario, -s": {
                            "description": "Only regenerate entries for this scenario and merge them into the existing lock file",
                            "default": "(empty = full regeneration for all scenarios)",
                        },
                        "--no-transitive": {
                            "description": "Lock direct dependencies only — do not resolve nested dependencies of git-sourced roles and collections (overrides [dependencies].transitive)",
                            "default": "false",
                        },
                    },
                    "behavior": [
                        "Resolves all collection and role versions from Galaxy API or git",
                        "Pins Python version and tool versions",
                        "Writes diffusion.lock (YAML format, with a generated header comment)",
                        "Fails loudly when a non-Galaxy collection has no SourceURL: 'collection <name>: missing SourceURL for non-Galaxy source \"git\"' (previously skipped silently)",
                        "Transitive (unless disabled): shallow-clones each git dependency (git clone --depth 1 --no-tags --branch <ref> -- <url>; constraints like '>=1.0' clone the default branch), imports the remote's default-scenario entries from its diffusion.lock (preferred) or diffusion.toml, BFS up to depth 10.",
                        "Transitive warnings (yellow, non-fatal): 'could not fetch dependencies of <id>: ...', 'skipping <id> required by <parent>: already resolved (duplicate or cycle)', 'max transitive depth 10 reached at <id>: not descending further', 'could not resolve <id>: ...; leaving constraint <c>', 'transitive: could not determine repository identity (no git origin / meta role_name); cycle detection relies on dependency graph only'.",
                        "Self-identity (cycle protection): git 'origin' URL, meta/main.yml role_name and namespace.role_name, and the working directory name. URLs are compared normalised (https://GitHub.com/Org/Repo.git/ == git@github.com:org/repo).",
                        "Private transitive git deps reuse [[artifact_sources]] credentials (Vault or local) via GIT_USER_* / GIT_PASSWORD_* / GIT_URL_* env vars.",
                        "Tools and Python version always come from the direct lock — a remote role never narrows the container runtime.",
                        "With --scenario: entries of OTHER scenarios are preserved verbatim (original order), then the fresh entries of the target scenario are appended. Tools/Python metadata are global and always taken fresh.",
                        "With --scenario but NO existing diffusion.lock: falls back to a FULL generation for all scenarios (prints 'No existing diffusion.lock found — generating full lock file for all scenarios'). A scenario-only lock file is never written.",
                        "The lock hash is always computed over ALL scenarios, so a scoped lock has identical hash semantics to a full lock.",
                    ],
                    "examples": [
                        "diffusion deps lock                 # full regeneration",
                        "diffusion deps lock -s production   # scoped merge for 'production' only",
                        "diffusion deps lock --no-transitive # direct dependencies only",
                    ],
                },
                "check": {
                    "usage": "diffusion deps check [--scenario <name>]",
                    "description": "Verify diffusion.lock is up-to-date with current YAML manifests",
                    "flags": {
                        "--scenario, -s": {
                            "description": "Only check requirements.yml of this scenario (meta/main.yml is checked only for 'default' or when omitted)",
                            "default": "(empty = all scenarios)",
                        },
                    },
                    "behavior": [
                        "Compares lock file against requirements.yml and meta.yml",
                        "Exits with code 1 if out of date (useful in CI)",
                        "Git collections are compared by repository URL (requirements.yml 'name' is the URL); they are ignored for meta/main.yml",
                        "Scoped mismatch message: \"Lock file is not fitting yaml manifests for scenario <name>. Run 'diffusion deps sync -s <name>' to update.\"",
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
                        "Git collections are displayed as '<name> (git: <url>)'",
                        "Transitive entries are suffixed with '(via <required_by>)'",
                    ],
                },
                "sync": {
                    "usage": "diffusion deps sync [--scenario <name>]",
                    "description": "Restore dependency versions from diffusion.lock back to requirements.yml and meta/main.yml",
                    "flags": {
                        "--scenario, -s": {
                            "description": "Only sync requirements.yml of this scenario (meta/main.yml is synced only for 'default' or when omitted)",
                            "default": "(empty = all scenarios)",
                        },
                    },
                    "behavior": [
                        "Overwrites requirements.yml for each scenario with resolved versions from lock file",
                        "Updates meta/main.yml collections (default scenario only; skipped for a scoped non-default scenario)",
                        "Useful for rollback or ensuring consistency after lock file changes",
                        "Git collections are written as '- name: <url>, type: git, version: <ref>' and skipped for meta/main.yml with '- skipping git collection <url> (not expressible in meta/main.yml)'",
                        "Console lines for transitive entries end in '(via <id>)' — console only, requirements.yml stays plain YAML",
                        "Requires an existing diffusion.lock — otherwise: \"lock file not found. Run 'diffusion deps lock' first\"",
                    ],
                },
            },
            "examples": [
                "diffusion deps init                # Initialize dependency tracking",
                "diffusion deps lock                # Generate/update lock file",
                "diffusion deps lock -s production  # Scoped lock merge for one scenario",
                "diffusion deps lock --no-transitive # Skip nested dependency resolution",
                "diffusion deps check               # Verify lock file is current (CI gate)",
                "diffusion deps check -s production # Verify one scenario only",
                "diffusion deps resolve             # Show all resolved versions",
                "diffusion deps sync                # Restore versions from lock to YAML files",
                "diffusion deps sync -s production  # Restore one scenario only",
            ],
            "github_action_integration": {
                "action": "diffusion-update",
                "input": "scenario",
                "default": "(empty = all scenarios locked/checked/synced; molecule tests then run against 'default')",
                "validation": "Must match ^[A-Za-z0-9._-]+$ — otherwise the action fails with '::error::Invalid scenario name'",
                "note": "Passed to the CLI as a single token '--scenario=<name>' via DIFFUSION_SCENARIO_ARGS",
            },
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
        "docs": {
            "description": "Generate or update inline documentation comments in defaults/ directory based on special marker signs",
            "usage": "diffusion docs [flags]",
            "notes": [
                "Scans all YAML files under the defaults/ directory (not just main.yml) for special comment markers.",
                "Variables can be split across multiple files under defaults/ (e.g. defaults/main.yml, defaults/network.yml, defaults/database.yml).",
                "Uses '#-' prefix (no space) followed by a marker character (|, ?, !, &) as documentation directives.",
                "The --dry-run flag previews changes without writing to disk.",
                "Comments are placed above or below the variable they document.",
            ],
            "flags": {
                "--dry-run": {
                    "description": "Show what changes would be made without writing to disk",
                    "default": "false",
                },
                "--role, -r": {
                    "description": "Role name (auto-detected from meta/main.yml if omitted)",
                    "default": "(from meta/main.yml)",
                },
            },
            "comment_markers": {
                "#-|": {
                    "description": "Type annotation — declares the variable's data type",
                    "usage": "Place above a variable declaration or above #-! / #-& markers to declare the type",
                    "values": [
                        "string",
                        "int",
                        "float",
                        "bool",
                        "list",
                        "dict",
                        "path",
                        "raw",
                        "json",
                        "yaml",
                    ],
                    "example": '#-| string\napp_name: "myapp"',
                },
                "#-?": {
                    "description": "Description — human-readable explanation of the variable's purpose",
                    "usage": "Place after the variable declaration or after #-! / #-& to describe the variable",
                    "example": "#-| int\nhttp_port: 8080\n#-? Port number for the HTTP listener",
                },
                "#-!": {
                    "description": "Required variable marker — variable is OMITTED from defaults and must be provided by the user",
                    "usage": "After '#-! ' (with a space), write the variable NAME only (no colon, no value). The variable has no YAML declaration — it is documented purely via this comment. Must have '#-| <type>' above and '#-? <description>' below, same as regular variables.",
                    "syntax": "#-! <variable_name>",
                    "example": "#-| string\n#-! api_key\n#-? API key for authentication",
                },
                "#-&": {
                    "description": "Optional variable marker — variable is OMITTED from defaults because its default is defined in Jinja2 template logic (e.g. {{ var | default('value') }})",
                    "usage": "After '#-& ' (with a space), write the variable NAME only (no colon, no value). The variable has no YAML declaration — it is documented purely via this comment. Must have '#-| <type>' above and '#-? <description>' below, same as regular variables.",
                    "syntax": "#-& <variable_name>",
                    "example": "#-| string\n#-& fallback_url\n#-? Fallback URL (uses default in template)",
                },
            },
            "annotation_block_patterns": [
                "Declared variable:   #-| <type>  →  var_name: <default>  →  #-? <description>",
                "Required (no default): #-| <type>  →  #-! <var_name>  →  #-? <description>",
                "Optional (Jinja2 default): #-| <type>  →  #-& <var_name>  →  #-? <description>",
            ],
            "full_example": (
                "# --- Example: variables can live in any .yml file under defaults/ ---\n"
                "# e.g. defaults/main.yml, defaults/network.yml, defaults/database.yml\n"
                "\n"
                "#-| int\n"
                "http_port: 8080\n"
                "#-? Port number for the HTTP listener\n"
                "\n"
                "#-| string\n"
                'app_name: "myapp"\n'
                "#-? Application display name\n"
                "\n"
                "#-| list\n"
                "allowed_hosts:\n"
                "  - localhost\n"
                "#-? List of allowed hostnames\n"
                "\n"
                "#-| string\n"
                "#-! api_key\n"
                "#-? API key for external service (no default, user must provide)\n"
                "\n"
                "#-| string\n"
                "#-& db_host\n"
                "#-? Database host (default set in template via | default('localhost'))\n"
            ),
            "yaml_compliance_note": (
                "All markers use '#-' (hash + dash) as the prefix, immediately followed by a semantic character (|, ?, !, &). "
                "The | and ? markers apply to the adjacent variable declaration. "
                "The ! and & markers are standalone — they carry the variable name themselves (no YAML declaration needed)."
            ),
            "builtin_exclusions": (
                "The docs scanner automatically excludes Ansible/Jinja2 built-in variables "
                "from documentation output. This includes: loop, item, inventory_dir, playbook_dir, "
                "role_name, ansible_hostname, ansible_os_family, ansible_distribution, "
                "ansible_architecture, ansible_managed, ansible_facts, inventory_hostname, "
                "group_names, groups, hostvars, omit, and other standard Ansible magic variables."
            ),
            "examples": [
                "diffusion docs                    # Generate/update docs from all files in defaults/",
                "diffusion docs --dry-run          # Preview changes without writing",
                "diffusion docs --role myrole      # Generate docs for a specific role",
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
                "The lock-merge step emits collections, roles and tools in sorted key order, so merged locks (and run_id inputs) are stable across runs.",
                "Role-source URLs, refs, Galaxy names and versions starting with '-' are rejected before git/ansible-galaxy is invoked ('refusing to clone \"<v>\": argument looks like a git option' / 'refusing to use \"<v>\": <kind> looks like a command line option'). git invocations also pass '--' before positionals.",
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
                "--playbook, -p": {
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
                    "description": "key=value — global inventory variable (repeatable). Supports group-scoped format: 'groupname.key=value'",
                },
                "--extra-var": {
                    "description": "key=value — extra variable passed to ansible-playbook --extra-vars (repeatable)",
                },
                "--ssh-key": {
                    "description": (
                        "SSH private key as base64 (repeatable). Format: 'hostname=<base64>' or '*=<base64>' for all hosts. "
                        "Supports routing: per-host (inventory hostname as key), per-group ('group:<groupname>=<base64>'), "
                        "wildcard ('*=<base64>'), or fallback (any other name). Priority: per-host > group > fallback/wildcard."
                    ),
                    "examples": [
                        '"web01=<base64-encoded-pem>"',
                        '"*=<base64-encoded-pem>"',
                        '"group:webservers=<base64-encoded-pem>"',
                    ],
                    "name_validation": {
                        "allowed_characters": "^[A-Za-z0-9_.:*-]+$ (letters, digits, '.', '-', '_', ':', '*')",
                        "rejected": [
                            "Empty name or any shell metacharacter / whitespace / quote / newline",
                            "Dot-only path segments: '.', '..', or names ending in ':.' / ':..'",
                            "Names colliding with the reserved wildcard sentinels: env suffix 'WILDCARD' (case-insensitive, e.g. 'wildcard') or filename '_wildcard_'",
                            "A value without '=': error --ssh-key \"<value>\": expected format \"hostname=<base64>\"",
                        ],
                        "error_format": "--ssh-key \"<value>\": invalid SSH key/host name \"<name>\": only letters, digits, '.', '-', '_', ':', '*' are allowed",
                        "where_enforced": "CLI flag parsing (fast usage error) AND defensively inside the deploy package (container args, host-wait probe, inventory patching, failure-state writer) because SSHKeys is also set directly by the Terraform provider",
                    },
                },
                "--skip-period": {
                    "description": "Skip re-deploy if last run succeeded within this period and inputs are identical. Go duration string (e.g. '24h'). Empty = always deploy.",
                    "default": "",
                },
                "--host-wait-initial-delay": {
                    "description": "Pause before the first host reachability probe (Go duration, e.g. '30s')",
                    "default": "10s",
                },
                "--host-wait-interval": {
                    "description": "Interval between host reachability probes (Go duration)",
                    "default": "15s",
                },
                "--host-wait-timeout": {
                    "description": "Hard deadline for all hosts to become reachable (Go duration)",
                    "default": "10m",
                },
                "--host-wait-max-attempts": {
                    "description": "Maximum number of probe attempts before failing. Set to 0 to disable (rely on timeout only).",
                    "default": "20",
                },
                "--cache": {
                    "description": "Enable caching of deployed roles and collections. Uses RunID as cache key. Cache persists across runs at ~/.diffusion/deploy-cache/<runID[0:16]>/",
                    "default": "true",
                },
                "--cache-path": {
                    "description": "Custom base path for the deploy cache directory. Default: ~/.diffusion/deploy-cache/",
                    "default": "~/.diffusion/deploy-cache/",
                },
                "--ci": {
                    "description": "CI/CD mode. Prints cache path for CI system integration (e.g. actions/cache). Non-interactive.",
                    "default": "false",
                },
            },
            "ssh_key_routing": {
                "description": "SSH key routing determines which hosts receive which private key",
                "priority_order": [
                    "1. Per-host: key name matches the inventory hostname exactly",
                    "2. Per-group: key name is 'group:<groupname>' — applies to all hosts in that group",
                    "3. Fallback/Wildcard: key name is '*' or any other name — applies to all unmatched hosts",
                ],
                "behavior": [
                    "Keys are passed as base64-encoded env vars into the container: SSH_KEY_<SANITIZED_NAME> where '-', '.', ':', '/' become '_' and the result is upper-cased (e.g. 'group:webservers' → SSH_KEY_GROUP_WEBSERVERS)",
                    "The '*' wildcard key maps to env var SSH_KEY_WILDCARD and file /tmp/ssh-keys/_wildcard_ (never a literal '*', which would be glob-expanded in the shell)",
                    "Container decodes keys to /tmp/ssh-keys/<host> at runtime (chmod 600)",
                    "Inventory is patched with ansible_ssh_private_key_file pointing to the decoded key",
                    "All key names are validated (see --ssh-key name_validation) before any shell command, env var or file path is built",
                ],
            },
            "deploy_caching": {
                "description": "Deploy caching persists ansible-galaxy install results across runs with the same RunID",
                "cache_structure": "~/.diffusion/deploy-cache/<runID[0:16]>/{roles,collections}",
                "behavior": [
                    "Cache directory is mounted read-write into the container",
                    "ansible-galaxy install writes to the cache on first deploy",
                    "Subsequent deploys with same RunID skip re-download",
                    "In CI mode (--ci), cache path is printed for integration with CI cache systems (e.g. actions/cache)",
                ],
            },
            "workflow_order": [
                "1. Fetch diffusion.lock from each --role-source (git shallow-clone or Galaxy download)",
                "2. Merge lock files — intersect constraints, re-resolve via Galaxy API",
                "3. Generate requirements.yml from merged lock",
                "4. Auto-generate playbook from role_sources (or use --playbook)",
                "5. Ensure deploy cache directory (if --cache enabled)",
                "6. Wait for hosts to be reachable via ansible.builtin.ping inside the container",
                "7. Run ansible-galaxy role/collection install inside the container (from cache if available)",
                "8. Run ansible-playbook inside the container",
                "9. Write remote state to ~/.diffusion/state on each host",
            ],
            "auto_generated_playbook_example": (
                "---\n"
                "# Auto-generated by diffusion deploy\n\n"
                '- name: "diffusion deploy | all"\n'
                "  hosts: all\n"
                "  gather_facts: true\n"
                "  roles:\n"
                "    - role: geerlingguy.docker\n\n"
                '- name: "diffusion deploy | webservers"\n'
                "  hosts: webservers\n"
                "  gather_facts: true\n"
                "  roles:\n"
                "    - role: myorg.app\n"
            ),
            "examples": [
                "diffusion deploy --role-source scm=galaxy,version=>=6.0.0,galaxy=geerlingguy.docker --host web01=ansible_host=1.2.3.4",
                'diffusion deploy --role-source "scm=git,version=main,url=https://github.com/org/role.git,name=myrole,apply_to=webservers" --host web01=ansible_host=1.2.3.4',
                "diffusion deploy --playbook site.yml --role-source scm=galaxy,version=>=6.0.0,galaxy=ns.role --skip-period 24h",
                'diffusion deploy --role-source scm=galaxy,version=>=7.0.0,galaxy=geerlingguy.docker --host web01=ansible_host=1.2.3.4 --ssh-key "*=<base64>"',
                "diffusion deploy --role-source scm=git,version=main,url=https://github.com/org/role.git --ci --cache --host-wait-max-attempts 30",
                'diffusion deploy --role-source scm=git,version=main,url=https://github.com/org/role.git --ssh-key "group:webservers=<base64>" --host web01=ansible_host=1.2.3.4',
            ],
        },
        "patch": {
            "description": "Inspect and patch external Ansible roles (ephemeral, per scenario) inside the running molecule container",
            "usage": "diffusion patch [subcommand]",
            "notes": [
                "Patch bundles live in scenarios/<scenario>/patch.yml; overlay sources live in scenarios/<scenario>/patch/files/ and scenarios/<scenario>/patch/templates/.",
                "Patching never mutates the host: the role is copied inside the container to /var/lib/diffusion-patch/<scenario>/work/<role> (pristine copy in .../backup/<role>), streamed to a throwaway host temp dir, patched, streamed back and BIND-MOUNTED over /root/.ansible/roles/<role>.",
                "The patch dir lives under /var/lib (container writable layer), NOT /tmp: the molecule image mounts /tmp as tmpfs and 'docker cp' cannot read out of a tmpfs mount.",
                "'diffusion molecule' (converge, verify, default flow) auto-applies the scenario's bundles before molecule runs and removes the overlay afterwards. 'diffusion patch apply' leaves the overlay in place until the container is destroyed, 'diffusion molecule --destroy/--wipe' runs, or the next molecule run replaces it.",
                "patch.yml is fully validated on load — ANY invalid bundle makes 'diffusion molecule' fail with 'patch apply failed: invalid patch config: ...'.",
                "Role names in bundles must be declared in diffusion.toml [dependencies] roles for the bundle's scenario. Run diffusion patch from the project root (diffusion.toml is read from the current directory).",
                "Task IDs are POSITIONAL (tree order from 'patch analyze'). Upgrading the external role can shift IDs — re-run 'patch analyze' and 'patch check' after every role version bump.",
                "Without --role, diff/apply auto-detect the FIRST running molecule-* container; pass --role when several are running.",
                "Use MCP tools check_patch_config (static lint of patch.yml) and inspect_patch_overlays (live mounts in the container).",
            ],
            "patch_yml_schema": {
                "top_level": "Bundles: — a list. The key is case-sensitive ('bundles:' parses to zero bundles).",
                "bundle": {
                    "patch_bundle_name": "string, required, unique within the file",
                    "role_name": "string, required — 'namespace.role' or short 'role' (short matches a namespaced diffusion.toml entry; ambiguous short names fail inside the container)",
                    "scenario": "string, optional — scopes the diffusion.toml role lookup and overlay validation; empty = accept roles of any scenario. Keep equal to the patch.yml directory.",
                    "tasks_to_patch": "list, required (non-empty), task_id unique per bundle",
                },
                "task": {
                    "task_id": "required — dotted positive integers ('1', '1.3.1', no leading zeros, max depth 32); handlers use an 'h' prefix ('h1', 'h1.2'). Must address a LEAF, never a branch.",
                    "new_module": "rename the task's module (e.g. ansible.builtin.copy → ansible.builtin.template)",
                    "new_module_setup": "list of {key, value}. With new_module: REPLACES module args. Without: MERGES into existing args (must be a mapping; free-form args fail).",
                    "new_conditions": "list of {condition: when|changed_when|failed_when, body: <expr>}. Multiple bodies per key render as a list. Body 'true'/'false' renders a YAML bool and emits a 'possible design problem' warning.",
                    "new_become_user": "{user: <name>, become: true|false} — at least one must be set",
                    "new_environment": "list of {key, value} — REPLACES the task's environment mapping",
                    "new_file_src": "file under scenarios/<scenario>/patch/files/ copied over the leaf's recorded src (files/<src> in the role). Relative, must exist, no '..' escape. Only for file-module leaves (copy etc.), not template.",
                    "new_template_src": "file under scenarios/<scenario>/patch/templates/ copied over templates/<src>. Only for 'template' leaves.",
                    "new_block": "accepted by the schema but NOT supported in v1 — apply fails with 'new_block apply is not supported in v1'",
                },
                "constraints": [
                    "At least one action per task ('no patch action specified')",
                    "new_file_src and new_template_src are mutually exclusive",
                    "A src overlay cannot be combined with new_module/new_module_setup",
                    "Overlays need a static, relative leaf src (Jinja or absolute src → unsupported)",
                    "action:/local_action: tasks cannot be mutated in v1",
                ],
                "example": (
                    "Bundles:\n"
                    "  - patch_bundle_name: docker-hardening\n"
                    "    role_name: geerlingguy.docker\n"
                    "    scenario: default\n"
                    "    tasks_to_patch:\n"
                    "      - task_id: \"1.3\"\n"
                    "        new_conditions:\n"
                    "          - condition: when\n"
                    "            body: ansible_os_family == 'Debian'\n"
                    "        new_become_user:\n"
                    "          become: true\n"
                    "      - task_id: \"2.1\"\n"
                    "        new_template_src: daemon.json.j2   # scenarios/default/patch/templates/daemon.json.j2\n"
                    "      - task_id: h1\n"
                    "        new_module_setup:\n"
                    "          - key: state\n"
                    "            value: restarted\n"
                ),
            },
            "task_tree": {
                "leaf": "real module execution — patchable, gets a dotted ID",
                "branch": "include_tasks / import_tasks / include_role / import_role / block — visual-only grouping, never a patch target ('task_id \"X\" is a branch (visual-only, patch a leaf instead)')",
                "opaque_branch": "include_role/import_role (always), or include with a dynamic, missing, cyclic or out-of-root target — not expanded",
                "numbering": "tasks/main.yml top level → 1,2,3; tasks included from branch 1 → 1.1,1.2; block/rescue/always children of 2 → 2.1,2.2; handlers/main.yml → h1,h2",
            },
            "subcommands": {
                "analyze": {
                    "usage": "diffusion patch analyze <role> [flags]",
                    "description": "Print the task/branch tree of an installed external role with dotted leaf IDs",
                    "flags": {
                        "--scenario, -s": {"description": "Molecule scenario to operate on", "default": "default"},
                        "--format": {"description": "Output format: tree|yaml", "default": "tree"},
                        "--no-color": {"description": "Disable ANSI colors", "default": "false"},
                        "--tag": {"description": "Highlight tasks with these tags (repeatable or comma-separated)", "default": ""},
                    },
                    "behavior": [
                        "Role lookup on the host: diffusion cache <cache>/roles/, ./molecule/<role>, ~/.ansible/roles (full name, short name, then '<ns>.<short>' scan)",
                        "Fallback: read-only 'docker cp' of the role out of the first running molecule-* container into a temp dir",
                        "Requires tasks/main.yml ('no tasks/main.yml under <path>: not an analyzable role')",
                        "Tree output annotates leaves overlaid by the scenario's patch.yml with '<= patch/files|templates/<name>'",
                    ],
                    "examples": [
                        "diffusion patch analyze geerlingguy.docker",
                        "diffusion patch analyze geerlingguy.docker -s production --format yaml",
                        "diffusion patch analyze docker --tag install --no-color",
                    ],
                },
                "list": {
                    "usage": "diffusion patch list [-s <scenario>]",
                    "description": "List patch bundles of a scenario and summarise each task action (module->, args(n), file<=, template<=, when(n), changed(n), failed(n), become, env, block(n))",
                    "flags": {"--scenario, -s": {"description": "Molecule scenario", "default": "default"}},
                    "behavior": ["Fails when scenarios/<scenario>/patch.yml does not exist or does not validate"],
                },
                "check": {
                    "usage": "diffusion patch check [-s <scenario>]",
                    "description": "Validate every bundle (diffusion.toml role, task IDs, overlays) and verify each task ID resolves to a LEAF of the installed role analysis",
                    "flags": {"--scenario, -s": {"description": "Molecule scenario", "default": "default"}},
                    "behavior": [
                        "Prints 'OK <bundle> (role <r>, <n> tasks)' or 'FAIL <bundle>: <reason>' per bundle",
                        "Exit 1 with 'patch check failed for scenario <s>' if any bundle fails",
                        "Overlay kind is checked against the leaf module (template ↔ new_template_src, file modules ↔ new_file_src)",
                    ],
                },
                "diff": {
                    "usage": "diffusion patch diff [-s <scenario>] [-r <role>] [--bundle <name>]",
                    "description": "Dry-run apply inside the running molecule container: report what would change; nothing is written back and no overlay is mounted",
                    "flags": {
                        "--scenario, -s": {"description": "Molecule scenario", "default": "default"},
                        "--role, -r": {"description": "Role under test (container molecule-<role>); default: first running molecule container", "default": ""},
                        "--bundle": {"description": "Only process this bundle name", "default": "(all)"},
                    },
                },
                "apply": {
                    "usage": "diffusion patch apply [-s <scenario>] [-r <role>] [--bundle <name>] [--dry-run] [--force]",
                    "description": "Apply patch bundles inside the running molecule-<role> container (bind-mount overlay)",
                    "flags": {
                        "--scenario, -s": {"description": "Molecule scenario", "default": "default"},
                        "--role, -r": {"description": "Role under test (container molecule-<role>); default: first running molecule container", "default": ""},
                        "--bundle": {"description": "Only process this bundle name", "default": "(all)"},
                        "--dry-run": {"description": "Report changes without writing (same as 'patch diff')", "default": "false"},
                        "--force": {"description": "Apply despite analysis drift (task line/name changed since analyze)", "default": "false"},
                    },
                    "behavior": [
                        "Requires a converged container ('diffusion molecule --converge' first)",
                        "Self-heals first: unmounts stale overlays under /root/.ansible/roles and removes /var/lib/diffusion-patch/<scenario>; a mount that resists umount and umount -l aborts the apply",
                        "If one bundle fails, overlays of bundles applied earlier in the same run are removed again",
                        "Prints the per-bundle summary (patched tasks, overlays, files changed, warnings) and 'overlay: <container>:<work> -> <roles path>'",
                        "Rewritten task files use 4-space indentation, keep the original '---' marker and a blank line between top-level tasks",
                    ],
                    "examples": [
                        "diffusion patch apply -r myrole",
                        "diffusion patch apply -r myrole -s production --bundle docker-hardening",
                        "diffusion patch apply -r myrole --dry-run",
                    ],
                },
            },
            "examples": [
                "diffusion patch analyze geerlingguy.docker      # find leaf IDs",
                "diffusion patch list                            # show bundles of scenario 'default'",
                "diffusion patch check -s production             # validate against installed roles",
                "diffusion patch diff -r myrole                  # dry-run inside molecule-myrole",
                "diffusion patch apply -r myrole                 # mount patched roles",
                "diffusion molecule --converge                   # auto-applies scenario patches",
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
            "test -d /opt/uv/.venv && echo 'venv present' || "
            "(test -d /opt/venv && echo 'venv present (legacy path)' || echo 'venv MISSING')",
        ]
    )
    diagnostics.append(
        {"check": "uv venv (/opt/uv/.venv)", "result": venv_check["stdout"]}
    )
    if "MISSING" not in venv_check.get("stdout", ""):
        # Determine the correct venv path
        venv_python = "/opt/uv/.venv/bin/python"
        pkg_result = _run(
            [
                "docker",
                "exec",
                container,
                "/bin/sh",
                "-c",
                f"uv pip list --python {venv_python} 2>/dev/null | head -30 || "
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

    # 10. Patch overlays (diffusion patch / auto-patching during molecule runs)
    overlay_result = _run(
        [
            "docker",
            "exec",
            container,
            "/bin/sh",
            "-c",
            f"awk '{{print $2}}' /proc/self/mounts | grep '^{_CONTAINER_ROLES_PATH}/' ; "
            f"echo '---'; ls -1 {_CONTAINER_PATCH_DIR} 2>/dev/null",
        ]
    )
    mounts_part, _, dirs_part = overlay_result.get("stdout", "").partition("---")
    overlay_mounts = [m for m in mounts_part.splitlines() if m.strip()]
    patch_scenarios = [d for d in dirs_part.splitlines() if d.strip()]
    diagnostics.append(
        {
            "check": "Patch overlays",
            "result": "active" if overlay_mounts else "none",
            "mounted_over": overlay_mounts,
            "patch_dir_scenarios": patch_scenarios,
            "suggestion": (
                "Overlays persist after 'diffusion patch apply' by design, but should NOT remain "
                "after a 'diffusion molecule' run finished. Use inspect_patch_overlays for details. "
                "'diffusion molecule --destroy/--wipe' always unmount them; converge/verify/'patch apply' "
                "self-heal only while the scenario's patch.yml still declares bundles."
                if overlay_mounts
                else ""
            ),
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

    candidates = [root / "scenarios" / scenario / "requirements.yml"]

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
    for base_dir in [root / "scenarios"]:
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
                    "patch.yml",
                ]:
                    fpath = entry / fname
                    scenario_info["files"][fname] = fpath.exists()

                # Patch overlay sources (diffusion patch new_file_src / new_template_src)
                patch_dir = entry / "patch"
                if patch_dir.is_dir():
                    scenario_info["patch_overlays"] = sorted(
                        str(f.relative_to(entry)).replace("\\", "/")
                        for f in patch_dir.rglob("*")
                        if f.is_file()
                    )

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

    Only allows safe, read-only or non-destructive commands. 'patch analyze',
    'patch list', 'patch check' and 'patch diff' are read-only ('patch diff'
    is a dry-run that never writes back into the container); 'patch apply'
    is NOT allowed — run it in a terminal.

    Args:
        subcommand: The diffusion subcommand (e.g. "show", "deps check", "cache status",
                    "patch analyze" with args="geerlingguy.docker --no-color").
        args: Additional arguments as a string.
        project_path: Working directory (auto-detected if empty).
    """
    # Allowlist of safe subcommands
    safe_commands = {
        "show",
        "deps check",
        "deps resolve",
        "cache status",
        "cache list",
        "docs --dry-run",
        "role",
        "deploy",
        "patch analyze",
        "patch list",
        "patch check",
        "patch diff",
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
# Tool: update_diffusion_docs
# ---------------------------------------------------------------------------


@mcp.tool()
def update_diffusion_docs(
    project_path: str = "",
    role: str = "",
) -> str:
    """Run `diffusion docs --dry-run` and return the output.

    Shows what documentation changes would be made to the role's
    defaults/main.yml comments without actually writing them.

    Args:
        project_path: Path to the project root (auto-detected if empty).
        role: Role name to pass via --role flag (optional).
    """
    root = Path(project_path) if project_path else _find_project_root()
    cwd = str(root) if root else None

    cmd_parts = ["diffusion", "docs", "--dry-run"]
    if role:
        cmd_parts.extend(["--role", role])

    result = _run(cmd_parts, timeout=60, cwd=cwd)

    output_parts = []
    if result["stdout"]:
        output_parts.append(result["stdout"])
    if result["stderr"]:
        output_parts.append(f"[stderr] {result['stderr']}")
    if result["returncode"] != 0:
        output_parts.append(f"[exit code: {result['returncode']}]")

    return "\n".join(output_parts) if output_parts else "(no output)"


# ---------------------------------------------------------------------------
# Tool: troubleshoot_deploy
# ---------------------------------------------------------------------------


@mcp.tool()
def troubleshoot_deploy(project_path: str = "") -> str:
    """Diagnose common diffusion deploy issues.

    Checks: diffusion binary availability, Docker daemon, molecule container
    image presence, SSH key accessibility, cache directory permissions,
    and common configuration problems.

    Args:
        project_path: Path to the project root (auto-detected if empty).
    """
    diagnostics: list[dict[str, Any]] = []

    # 1. Diffusion binary
    result = _run(["diffusion", "--version"])
    if result["returncode"] == 0:
        diagnostics.append(
            {"check": "Diffusion binary", "status": "ok", "version": result["stdout"]}
        )
    else:
        diagnostics.append(
            {
                "check": "Diffusion binary",
                "status": "error",
                "detail": result["stderr"] or "diffusion not found on PATH",
                "suggestion": "Install diffusion or ensure it is on PATH.",
            }
        )
        return json.dumps(
            {"diagnostics": diagnostics, "summary": "diffusion binary not found"},
            indent=2,
        )

    # 2. Docker daemon
    result = _run(["docker", "info", "--format", "{{.ServerVersion}}"])
    if result["returncode"] == 0:
        diagnostics.append(
            {"check": "Docker daemon", "status": "ok", "version": result["stdout"]}
        )
    else:
        diagnostics.append(
            {
                "check": "Docker daemon",
                "status": "error",
                "detail": result["stderr"],
                "suggestion": "Docker must be running for deploy to work.",
            }
        )

    # 3. Molecule container image availability
    result = _run(
        [
            "docker",
            "images",
            "--filter",
            "reference=*diffusion-molecule*",
            "--format",
            "{{.Repository}}:{{.Tag}}",
        ]
    )
    images = result["stdout"].splitlines() if result["stdout"] else []
    if images:
        diagnostics.append(
            {"check": "Molecule container image", "status": "ok", "images": images}
        )
    else:
        diagnostics.append(
            {
                "check": "Molecule container image",
                "status": "warning",
                "detail": "No diffusion-molecule images found locally. "
                "Image will be pulled on first deploy.",
            }
        )

    # 4. Deploy cache directory
    cache_base = Path.home() / ".diffusion" / "deploy-cache"
    if cache_base.exists():
        cache_entries = [d.name for d in cache_base.iterdir() if d.is_dir()]
        diagnostics.append(
            {
                "check": "Deploy cache",
                "status": "ok",
                "path": str(cache_base),
                "entries": len(cache_entries),
            }
        )
    else:
        diagnostics.append(
            {
                "check": "Deploy cache",
                "status": "info",
                "detail": f"No deploy cache directory at {cache_base}. "
                "Will be created on first deploy with --cache.",
            }
        )

    # 5. SSH key common issues
    ssh_dir = Path.home() / ".ssh"
    if ssh_dir.exists():
        key_files = [
            f.name
            for f in ssh_dir.iterdir()
            if f.is_file() and not f.name.endswith(".pub")
        ]
        diagnostics.append(
            {
                "check": "SSH keys (~/.ssh)",
                "status": "ok",
                "keys_found": len(key_files),
            }
        )
    else:
        diagnostics.append(
            {
                "check": "SSH keys (~/.ssh)",
                "status": "warning",
                "detail": "~/.ssh directory not found. "
                "SSH key file references in inventory may fail.",
            }
        )

    # 6. diffusion.toml presence
    root = Path(project_path) if project_path else _find_project_root()
    if root and (root / "diffusion.toml").exists():
        diagnostics.append(
            {
                "check": "diffusion.toml",
                "status": "ok",
                "path": str(root / "diffusion.toml"),
            }
        )
    else:
        diagnostics.append(
            {
                "check": "diffusion.toml",
                "status": "warning",
                "detail": "No diffusion.toml found. "
                "Deploy may still work with CLI flags alone.",
            }
        )

    summary = (
        "all checks passed"
        if all(d["status"] == "ok" for d in diagnostics)
        else "some issues detected — review diagnostics"
    )
    return json.dumps({"diagnostics": diagnostics, "summary": summary}, indent=2)


# ---------------------------------------------------------------------------
# Tool: check_deploy_cache
# ---------------------------------------------------------------------------


@mcp.tool()
def check_deploy_cache(cache_path: str = "") -> str:
    """Inspect the diffusion deploy cache directory structure.

    Shows cache entries (by RunID prefix), their sizes, and contents.
    Useful for debugging cache hits/misses during deploy operations.

    Args:
        cache_path: Custom cache base path. Default: ~/.diffusion/deploy-cache/
    """
    base = (
        Path(cache_path)
        if cache_path
        else (Path.home() / ".diffusion" / "deploy-cache")
    )

    if not base.exists():
        return (
            f"No deploy cache directory at {base}. "
            "Cache is created on first deploy with --cache enabled."
        )

    entries: list[dict[str, Any]] = []
    for entry in sorted(base.iterdir()):
        if not entry.is_dir():
            continue
        info: dict[str, Any] = {
            "run_id_prefix": entry.name,
            "path": str(entry),
        }
        # Check subdirectories
        for sub in ["roles", "collections"]:
            sub_path = entry / sub
            if sub_path.exists():
                items = list(sub_path.iterdir())
                total_size = sum(
                    f.stat().st_size for f in sub_path.rglob("*") if f.is_file()
                )
                info[sub] = {
                    "items": len(items),
                    "size_mb": round(total_size / (1024 * 1024), 2),
                }
            else:
                info[sub] = {"items": 0, "size_mb": 0}

        entries.append(info)

    if not entries:
        return f"Deploy cache directory exists at {base} but is empty."

    return json.dumps(
        {"cache_path": str(base), "entries": entries, "total_entries": len(entries)},
        indent=2,
    )


# ---------------------------------------------------------------------------
# Tool: troubleshoot_ssh_keys
# ---------------------------------------------------------------------------


@mcp.tool()
def troubleshoot_ssh_keys(
    role: str = "", project_path: str = "", key_names: str = ""
) -> str:
    """Diagnose SSH key issues for diffusion deploy.

    Checks: key file existence, key format (PEM headers), file permissions,
    common base64 encoding problems, and --ssh-key / ssh_private_keys NAME
    validity (allowlist ^[A-Za-z0-9_.:*-]+$, dot-only segments, wildcard
    sentinel collisions). Works both for file-based keys and base64-injected
    keys (used by Terraform provider).

    Args:
        role: Optional role name to check inside a running molecule container.
        project_path: Path to the project root (auto-detected if empty).
        key_names: Optional comma-separated list of SSH key names you intend to
                   pass (e.g. "web01,group:webservers,*"). Each is validated
                   exactly like `diffusion deploy --ssh-key` and the resulting
                   env var / file name is shown.
    """
    diagnostics: list[dict[str, Any]] = []

    # Validate intended key names (mirrors deploy.ValidateSSHKeyName)
    if key_names:
        for raw_name in [n.strip() for n in key_names.split(",")]:
            err = _validate_ssh_key_name(raw_name)
            if err:
                diagnostics.append(
                    {
                        "check": f"SSH key name: {raw_name!r}",
                        "status": "error",
                        "detail": err,
                        "suggestion": "Rename the key. Allowed: letters, digits, '.', '-', '_', ':', '*'. "
                        "Use '*' for the wildcard (not 'wildcard'/'_wildcard_'), "
                        "'group:<name>' for a group key, or the exact inventory hostname.",
                    }
                )
            else:
                diagnostics.append(
                    {
                        "check": f"SSH key name: {raw_name!r}",
                        "status": "ok",
                        "env_var": f"SSH_KEY_{_ssh_key_env_suffix(raw_name)}",
                        "container_path": f"/tmp/ssh-keys/{_ssh_key_file_name(raw_name)}",
                        "routing": (
                            "wildcard (all hosts)"
                            if raw_name == "*"
                            else "per-group"
                            if raw_name.startswith("group:")
                            else "per-host (if it matches an inventory hostname) else fallback"
                        ),
                    }
                )

    # Check ~/.ssh directory
    ssh_dir = Path.home() / ".ssh"
    if ssh_dir.exists():
        for f in sorted(ssh_dir.iterdir()):
            if (
                f.is_file()
                and not f.name.endswith(".pub")
                and not f.name == "known_hosts"
            ):
                try:
                    content = f.read_text(encoding="utf-8", errors="replace")
                    first_line = content.split("\n")[0] if content else ""
                    is_pem = first_line.startswith("-----BEGIN")
                    size = f.stat().st_size
                    diagnostics.append(
                        {
                            "check": f"SSH key: {f.name}",
                            "status": "ok" if is_pem else "warning",
                            "pem_format": is_pem,
                            "size_bytes": size,
                            "first_line": first_line[:40] + "..."
                            if len(first_line) > 40
                            else first_line,
                            "suggestion": ""
                            if is_pem
                            else "Key does not start with PEM header. May not be a valid private key.",
                        }
                    )
                except Exception as e:
                    diagnostics.append(
                        {
                            "check": f"SSH key: {f.name}",
                            "status": "error",
                            "detail": str(e),
                        }
                    )
    else:
        diagnostics.append(
            {"check": "~/.ssh directory", "status": "warning", "detail": "Not found"}
        )

    # If a role is specified, check SSH keys inside the molecule container
    if role:
        container = _container_name(role)
        result = _run(
            [
                "docker",
                "exec",
                container,
                "/bin/sh",
                "-c",
                "ls -la /tmp/ssh-keys/ 2>/dev/null || echo 'NO_SSH_KEYS_DIR'",
            ]
        )
        if "NO_SSH_KEYS_DIR" in result.get("stdout", ""):
            diagnostics.append(
                {
                    "check": "Container SSH keys (/tmp/ssh-keys/)",
                    "status": "info",
                    "detail": "No SSH keys directory in container. "
                    "Keys are created at runtime when --ssh-key flags are used.",
                }
            )
        else:
            diagnostics.append(
                {
                    "check": "Container SSH keys (/tmp/ssh-keys/)",
                    "status": "ok",
                    "detail": result["stdout"],
                }
            )

        # Check SSH env vars in container
        env_result = _run(
            [
                "docker",
                "exec",
                container,
                "/bin/sh",
                "-c",
                "env | grep ^SSH_KEY_ | cut -d= -f1",
            ]
        )
        ssh_env_keys = env_result["stdout"].splitlines() if env_result["stdout"] else []
        diagnostics.append(
            {
                "check": "Container SSH_KEY_* env vars",
                "status": "ok" if ssh_env_keys else "info",
                "vars": ssh_env_keys or ["none"],
            }
        )

    summary = (
        "all checks passed"
        if all(d.get("status") == "ok" for d in diagnostics)
        else "review diagnostics for potential issues"
    )
    return json.dumps({"diagnostics": diagnostics, "summary": summary}, indent=2)


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
                    "default": "15s",
                    "description": "Default interval between host probes.",
                },
                "host_wait_timeout": {
                    "type": "string",
                    "optional": True,
                    "default": "10m",
                    "description": "Default hard deadline for host reachability.",
                },
            },
            "example": (
                "terraform {\n"
                "  required_providers {\n"
                "    diffusion = {\n"
                '      source  = "registry.terraform.io/diffusion/diffusion"\n'
                '      version = ">= 0.1.0"\n'
                "    }\n"
                "  }\n"
                "}\n\n"
                'provider "diffusion" {\n'
                '  diffusion_binary  = "/usr/local/bin/diffusion"\n'
                '  registry_server   = "ghcr.io"\n'
                '  registry_provider = "Public"\n'
                '  vault_addr        = "https://vault.example.com"\n'
                "  vault_token       = var.vault_token\n"
                '  host_wait_timeout = "5m"\n'
                "}"
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
                "variables": {
                    "type": "map(object)",
                    "optional": True,
                    "description": (
                        "Map of group name → { vars = { key = value } }. "
                        "Use key 'all' for global variables applied to the all group. "
                        "Other keys set variables on the corresponding child group."
                    ),
                    "nested_attributes": {
                        "vars": "map(string) — variables for this group",
                    },
                    "examples": [
                        'variables = { all = { vars = { env = "production" } } }',
                        'variables = { webservers = { vars = { http_port = "80" } } }',
                    ],
                },
                "extra_vars": "map(string) — extra vars for ansible-playbook --extra-vars",
                "ssh_private_keys": {
                    "type": "map(string)",
                    "optional": True,
                    "sensitive": True,
                    "description": (
                        "Map of named SSH private keys in PEM format (raw text). "
                        "Each value is base64-encoded automatically before passing to diffusion. "
                        "Key naming controls which hosts receive each key."
                    ),
                    "pem_normalization": (
                        "Literal two-character '\\n' escape sequences in a key value are converted to real newlines "
                        "before base64 encoding. This fixes PEM keys that arrive with escaped newlines from Terraform "
                        "interpolation (e.g. keys read from JSON/tfvars strings), which would otherwise decode to an "
                        "invalid single-line PEM file inside the container."
                    ),
                    "name_validation": (
                        "Map keys are forwarded verbatim as --ssh-key names and must satisfy the CLI allowlist "
                        "^[A-Za-z0-9_.:*-]+$ ; names such as 'wildcard' or '_wildcard_' are rejected because they "
                        "collide with the '*' sentinel."
                    ),
                    "routing_rules": {
                        "per-host": "Use inventory host name as key (e.g. 'waf-01') — applies only to that host",
                        "per-group": "Prefix with 'group:' (e.g. 'group:checkpoint_waf') — applies to all hosts in that group",
                        "wildcard": "Use '*' — applies to all hosts via --private-key flag",
                        "fallback": "Any other name (e.g. 'default') — applies to all hosts without a more specific key",
                    },
                    "priority_order": "per-host > group > fallback/wildcard",
                    "validators": ["Map keys must not contain '=' character"],
                    "examples": [
                        "{ default = tls_private_key.ssh.private_key_openssh }",
                        '{ "group:webservers" = tls_private_key.web.private_key_openssh }',
                        '{ "waf-01" = tls_private_key.waf.private_key_openssh }',
                    ],
                },
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
                "  role_sources = [\n"
                "    {\n"
                '      scm     = "galaxy"\n'
                '      version = ">=6.0.0"\n'
                '      galaxy  = "geerlingguy.docker"\n'
                "    },\n"
                "    {\n"
                '      scm      = "git"\n'
                '      version  = "main"\n'
                '      url      = "https://github.com/myorg/ansible-app.git"\n'
                '      name     = "app"\n'
                '      apply_to = "webservers"\n'
                "    }\n"
                "  ]\n\n"
                "  hosts = {\n"
                '    web01 = { vars = { ansible_host = "1.2.3.4", ansible_user = "ubuntu" } }\n'
                '    web02 = { vars = { ansible_host = "1.2.3.5", ansible_user = "ubuntu" } }\n'
                "  }\n"
                '  groups    = { webservers = ["web01", "web02"] }\n\n'
                "  variables = {\n"
                '    all        = { vars = { env = "production" } }\n'
                '    webservers = { vars = { http_port = "80" } }\n'
                "  }\n\n"
                "  ssh_private_keys = {\n"
                '    "group:webservers" = tls_private_key.web.private_key_openssh\n'
                "  }\n\n"
                '  skip_if_succeeded_within = "24h"\n'
                '  host_wait_timeout        = "10m"\n'
                "}"
            ),
        },
        "inventory": {
            "type": "data_source",
            "name": "diffusion_inventory",
            "description": (
                "Renders an Ansible YAML inventory from provided hosts, groups, "
                "and per-group variables without triggering any deployment. "
                "Useful for inspection, debugging, or passing the rendered inventory to other resources."
            ),
            "arguments": {
                "hosts": {
                    "type": "map(object)",
                    "optional": True,
                    "description": "Map of hostname → { vars = { key = value } }",
                },
                "groups": {
                    "type": "map(list(string))",
                    "optional": True,
                    "description": "Map of group name → list of host names",
                },
                "variables": {
                    "type": "map(object)",
                    "optional": True,
                    "description": (
                        "Map of group name → { vars = { key = value } }. "
                        "Use key 'all' for global variables applied to the all group. "
                        "Other keys set variables on the corresponding child group."
                    ),
                },
            },
            "exported_attributes": {
                "rendered": "string — the rendered Ansible YAML inventory",
            },
            "example": (
                'data "diffusion_inventory" "preview" {\n'
                "  hosts = {\n"
                '    web01 = { vars = { ansible_host = "1.2.3.4", ansible_user = "ubuntu" } }\n'
                "  }\n"
                '  groups = { webservers = ["web01"] }\n'
                "  variables = {\n"
                '    all        = { vars = { env = "staging" } }\n'
                '    webservers = { vars = { http_port = "80" } }\n'
                "  }\n"
                "}\n\n"
                'output "inventory_yaml" {\n'
                "  value = data.diffusion_inventory.preview.rendered\n"
                "}"
            ),
        },
    }

    r = resource.lower().strip()
    if r:
        if r in ref:
            return json.dumps(ref[r], indent=2)
        return f"Unknown resource '{resource}'. Available: {', '.join(ref.keys())}"
    return json.dumps(ref, indent=2)


# ---------------------------------------------------------------------------
# Tool: check_patch_config
# ---------------------------------------------------------------------------


@mcp.tool()
def check_patch_config(project_path: str = "", scenario: str = "default") -> str:
    """Statically validate scenarios/<scenario>/patch.yml for `diffusion patch`.

    Mirrors the validation diffusion runs when it loads patch.yml (which also
    happens automatically during `diffusion molecule` converge/verify):
    top-level 'Bundles' key (case-sensitive), unique bundle names, role_name
    declared in diffusion.toml [dependencies] for the bundle scenario, task_id
    syntax ('1.3.1' / 'h1'), duplicate IDs, at least one action, overlay files
    present under scenarios/<scenario>/patch/files|templates, mutual
    exclusions, condition keys, become/env keys. Also flags pitfalls that pass
    validation but break later (new_block unsupported in v1, static boolean
    conditions, ignored unknown keys, scenario field mismatch).

    It does NOT resolve task IDs against the installed role — use
    `run_diffusion_command("patch check", "-s <scenario>")` for that.

    Args:
        project_path: Path to the project root (auto-detected if empty).
        scenario: Scenario whose patch.yml to check (default: "default").
    """
    root = Path(project_path) if project_path else _find_project_root()
    if root is None:
        return "Error: Could not find project root."
    scen = (scenario or "default").strip()
    if not _SAFE_CONTAINER_NAME.match(scen):
        return f"Error: invalid scenario {scen!r}: must be a plain scenario name"

    report = _validate_patch_config(root, scen)

    # Overlay sources on disk vs. referenced in patch.yml (informational).
    patch_dir = root / "scenarios" / scen / "patch"
    if patch_dir.is_dir():
        on_disk = sorted(
            str(f.relative_to(patch_dir)).replace("\\", "/")
            for f in patch_dir.rglob("*")
            if f.is_file()
        )
        report["overlay_sources_on_disk"] = on_disk
        try:
            data = _load_patch_yaml(root / "scenarios" / scen / "patch.yml") or {}
            referenced: set[str] = set()

            def _collect(tasks: Any) -> None:
                for t in tasks or []:
                    if not isinstance(t, dict):
                        continue
                    if t.get("new_file_src"):
                        referenced.add("files/" + str(t["new_file_src"]).strip())
                    if t.get("new_template_src"):
                        referenced.add("templates/" + str(t["new_template_src"]).strip())
                    _collect(t.get("new_block"))

            for b in (data.get(_PATCH_TOP_KEY) or []) if isinstance(data, dict) else []:
                if isinstance(b, dict):
                    _collect(b.get("tasks_to_patch"))
            unused = [f for f in on_disk if f not in referenced]
            if unused:
                report["unreferenced_overlay_sources"] = unused
        except Exception:
            pass

    if report.get("present"):
        report["summary"] = (
            "ok — patch.yml would load; run 'diffusion patch check' to resolve task IDs"
            if not report["errors"]
            else f"{len(report['errors'])} error(s) — 'diffusion molecule' would fail with "
            "'patch apply failed: invalid patch config: ...'"
        )
        report["next_steps"] = [
            f"diffusion patch analyze <role> -s {scen}   # confirm leaf IDs",
            f"diffusion patch check -s {scen}            # IDs vs installed role",
            f"diffusion patch diff -s {scen} -r <role>   # dry-run in container",
        ]
    return json.dumps(report, indent=2, default=str)


# ---------------------------------------------------------------------------
# Tool: inspect_patch_overlays
# ---------------------------------------------------------------------------


@mcp.tool()
def inspect_patch_overlays(role: str, scenario: str = "") -> str:
    """Inspect `diffusion patch` bind-mount overlays inside a molecule container.

    Shows which /root/.ansible/roles/<role> directories are currently
    overlaid, the work/backup trees under /var/lib/diffusion-patch/<scenario>,
    and (optionally per scenario) which files differ between the pristine
    backup and the patched work copy.

    Interpretation: overlays persist after `diffusion patch apply` by design;
    after a `diffusion molecule` converge/verify finished they should be gone.
    `diffusion molecule --destroy/--wipe` always remove leftovers; converge,
    verify and `patch apply` self-heal only while patch.yml declares bundles.

    Args:
        role: The role under test (container will be molecule-<role>).
        scenario: Optional scenario to diff work vs backup (empty = list all).
    """
    container = _container_name(role)
    if not _SAFE_CONTAINER_NAME.match(container):
        return f"Error: invalid container name {container!r}"
    scen = scenario.strip()
    if scen and not _SAFE_CONTAINER_NAME.match(scen):
        return f"Error: invalid scenario {scen!r}: must be a plain scenario name"

    state = _run(["docker", "inspect", "-f", "{{.State.Running}}", container])
    if state["returncode"] != 0 or state["stdout"].strip() != "true":
        return json.dumps(
            {
                "container": container,
                "running": False,
                "detail": state["stderr"] or "container not running",
                "suggestion": f"Start it with 'diffusion molecule -r {role} --converge'. "
                "diffusion patch diff/apply fail with 'container molecule-<role> is not running'.",
            },
            indent=2,
        )

    script = (
        f"awk '{{print $2}}' /proc/self/mounts | grep '^{_CONTAINER_ROLES_PATH}/' ; "
        "echo '---'; "
        f"for s in $(ls -1 {_CONTAINER_PATCH_DIR} 2>/dev/null); do "
        f"for kind in work backup; do "
        f"for r in $(ls -1 {_CONTAINER_PATCH_DIR}/$s/$kind 2>/dev/null); do echo \"$s $kind $r\"; done; "
        "done; done"
    )
    res = _run(["docker", "exec", container, "/bin/sh", "-c", script])
    mounts_part, _, tree_part = res.get("stdout", "").partition("---")
    mounts = [m.strip() for m in mounts_part.splitlines() if m.strip()]

    tree: dict[str, dict[str, list[str]]] = {}
    for line in tree_part.splitlines():
        parts = line.split()
        if len(parts) == 3:
            s, kind, r = parts
            tree.setdefault(s, {"work": [], "backup": []})[kind].append(r)

    overlays: list[dict[str, Any]] = []
    for m in mounts:
        role_dir = m[len(_CONTAINER_ROLES_PATH) + 1 :]
        owners = [s for s, t in tree.items() if role_dir in t.get("work", [])]
        overlays.append(
            {
                "mounted_over": m,
                "role_dir": role_dir,
                "work_dir_scenarios": owners,
                "status": "ok" if owners else "orphan — no matching work dir (mount source deleted?)",
            }
        )

    report: dict[str, Any] = {
        "container": container,
        "roles_path": _CONTAINER_ROLES_PATH,
        "patch_dir": _CONTAINER_PATCH_DIR,
        "active_overlays": overlays,
        "patch_tree": tree,
    }

    if scen:
        diffs: dict[str, Any] = {}
        for r in tree.get(scen, {}).get("work", []):
            if not _SAFE_CONTAINER_NAME.match(r):
                continue
            base = f"{_CONTAINER_PATCH_DIR}/{scen}"
            d = _run(
                [
                    "docker",
                    "exec",
                    container,
                    "/bin/sh",
                    "-c",
                    f"diff -rq {base}/backup/{r} {base}/work/{r} 2>&1 | head -50",
                ]
            )
            diffs[r] = d["stdout"].splitlines() or ["(no differences or diff unavailable)"]
        report["work_vs_backup"] = diffs

    if overlays:
        report["note"] = (
            "Overlays are expected after 'diffusion patch apply'. If they are left over from an "
            "interrupted 'diffusion molecule' run: 'diffusion molecule --destroy' / '--wipe' always "
            "unmount them; converge/verify/'patch apply' self-heal only while the scenario's "
            "patch.yml still declares bundles. A mount that resists 'umount -l' produces "
            "'stale patch overlay still mounted in container ...' — restart the container."
        )
    elif tree:
        report["note"] = (
            "Patch work/backup trees exist but nothing is mounted: a dry-run or a finished molecule "
            "run left the staging copies behind. Harmless; cleaned on the next apply."
        )
    else:
        report["note"] = "No patch overlays and no patch work tree in this container."
    return json.dumps(report, indent=2)


# ---------------------------------------------------------------------------
# Tool: check_transitive_dependencies
# ---------------------------------------------------------------------------


@mcp.tool()
def check_transitive_dependencies(project_path: str = "", scenario: str = "") -> str:
    """Analyse transitive (nested) dependencies and git-sourced collections.

    Reports, without touching the network:
    - whether transitive resolution is enabled ([dependencies].transitive,
      default true; `deps lock --no-transitive` overrides per run)
    - the self-identity diffusion uses for cycle detection (git origin URL,
      meta/main.yml role_name / namespace.role_name, directory name) and
      whether it is "weak" (directory name only)
    - direct vs transitive lock entries (required_by) grouped by parent
    - git collections in diffusion.toml / diffusion.lock, including the fatal
      "missing SourceURL for non-Galaxy source" misconfiguration
    - URLs/versions starting with '-' (rejected by the CLI argument guard)
    - lock entries that point back at this repository, and duplicate
      identities within one scenario

    Args:
        project_path: Path to the project root (auto-detected if empty).
        scenario: Optional scenario prefix to restrict the lock analysis.
    """
    root = Path(project_path) if project_path else _find_project_root()
    if root is None:
        return "Error: Could not find project root."

    errors: list[str] = []
    warnings: list[str] = []
    report: dict[str, Any] = {"project_root": str(root)}

    deps = _load_dependency_config(root)
    transitive = _ci_get(deps, "transitive")
    report["transitive_enabled"] = True if transitive is None else bool(transitive)
    report["transitive_source"] = (
        "default (true)" if transitive is None else "diffusion.toml [dependencies].transitive"
    )
    report["max_depth"] = _TRANSITIVE_MAX_DEPTH

    self_ids = _detect_self_identity(root)
    report["self_identity"] = self_ids
    report["self_identity_normalised"] = sorted({_normalize_identity(i) for i in self_ids if i})
    if len(self_ids) <= 1:
        warnings.append(
            "Weak self-identity (no git origin / meta role_name): deps lock prints "
            "'transitive: could not determine repository identity ...' and a dependency that "
            "depends back on this repo is only stopped by the visited set."
        )

    # diffusion.toml collections / roles
    toml_git_cols: list[dict[str, Any]] = []
    for col in _ci_get(deps, "collections", []) or []:
        if not isinstance(col, dict):
            continue
        name = _dep_name(col)
        source = str(_ci_get(col, "source", "") or "")
        url = str(_ci_get(col, "sourceurl", "") or "")
        version = str(_ci_get(col, "version", "") or "")
        if source and source != "galaxy":
            toml_git_cols.append({"name": name, "source": source, "source_url": url, "version": version})
            if not url:
                errors.append(
                    f'collection {name}: missing SourceURL for non-Galaxy source "{source}" — '
                    "deps lock now FAILS (previously skipped silently). Add SourceURL = \"<git url>\"."
                )
        if source == "" and url:
            warnings.append(
                f"collection {name}: SourceURL set but Source empty — diffusion treats it as Galaxy "
                "and ignores the URL. Set Source = \"git\"."
            )
        for kind, val in (("git URL", url), ("version constraint", version)):
            if val:
                err = _validate_cli_argument(kind, val)
                if err:
                    errors.append(f"collection {name}: {err}")
    for r in _ci_get(deps, "roles", []) or []:
        if not isinstance(r, dict):
            continue
        name = _dep_name(r)
        for kind, val in (
            ("git URL", str(_ci_get(r, "src", "") or "")),
            ("version constraint", str(_ci_get(r, "version", "") or "")),
        ):
            if val:
                err = _validate_cli_argument(kind, val)
                if err:
                    errors.append(f"role {name}: {err}")
    report["diffusion_toml_git_collections"] = toml_git_cols

    # meta/main.yml cannot carry git collections
    meta_path = root / "meta" / "main.yml"
    if meta_path.exists():
        try:
            meta = _load_yaml(meta_path) or {}
            for c in meta.get("collections", []) or []:
                if isinstance(c, str) and _is_git_url(c):
                    errors.append(
                        f"meta/main.yml lists git URL {c!r} under collections — meta only accepts "
                        "'namespace.name'. Keep git collections in requirements.yml / diffusion.toml."
                    )
        except Exception:
            pass

    # diffusion.lock analysis
    lock_path = root / "diffusion.lock"
    if not lock_path.exists():
        report["lock_file_present"] = False
        warnings.append("No diffusion.lock — run 'diffusion deps lock'.")
    else:
        report["lock_file_present"] = True
        try:
            lock = _load_yaml(lock_path) or {}
        except Exception as e:
            return f"Error parsing diffusion.lock: {e}"

        self_norm = {_normalize_identity(i) for i in self_ids if i}
        prefix = f"{scenario.strip()}." if scenario.strip() else ""
        direct: list[str] = []
        by_parent: dict[str, list[str]] = {}
        lock_git_cols: list[dict[str, Any]] = []
        seen: dict[tuple[str, str], str] = {}

        for section in ("collections", "roles"):
            for e in lock.get(section, []) or []:
                if not isinstance(e, dict):
                    continue
                name = str(e.get("name", "") or "")
                if prefix and not name.startswith(prefix):
                    continue
                scen = name.split(".", 1)[0] if "." in name else "(unprefixed)"
                ident = _lock_entry_identity(e)
                label = f"{section[:-1]} {name}"
                rb = str(e.get("required_by", "") or "")
                if rb:
                    by_parent.setdefault(rb, []).append(label)
                else:
                    direct.append(label)
                if section == "collections" and _is_git_collection(e):
                    lock_git_cols.append(
                        {
                            "name": name,
                            "src": e.get("src", ""),
                            "resolved_version": e.get("resolved_version", ""),
                            "required_by": rb,
                        }
                    )
                if ident and ident in self_norm:
                    warnings.append(
                        f"{label} resolves to THIS repository ({ident}) — a dependency cycle that "
                        "self-identity should have skipped. Check git origin / meta role_name."
                    )
                key = (scen, f"{section}:{ident}")
                if ident and key in seen:
                    warnings.append(
                        f"{label} duplicates {seen[key]} (same identity {ident!r} in scenario {scen}) — "
                        "requirements.yml would list it twice."
                    )
                elif ident:
                    seen[key] = label
                for kind, val in (
                    ("git URL", str(e.get("src", "") or "")),
                    ("ref", _resolve_git_ref(str(e.get("resolved_version", "") or e.get("version", "") or ""))),
                ):
                    if val:
                        err = _validate_cli_argument(kind, val)
                        if err:
                            errors.append(f"{label}: {err}")

        report["lock_direct_entries"] = direct
        report["lock_transitive_entries_by_parent"] = by_parent
        report["lock_git_collections"] = lock_git_cols
        if by_parent and not report["transitive_enabled"]:
            warnings.append(
                "diffusion.lock contains required_by entries but transitive resolution is disabled — "
                "the next 'deps lock' will drop them."
            )

    report["errors"] = errors
    report["warnings"] = warnings
    report["summary"] = (
        "ok"
        if not errors and not warnings
        else f"{len(errors)} error(s), {len(warnings)} warning(s)"
    )
    report["hints"] = [
        "diffusion deps lock --no-transitive   # lock direct deps only",
        "diffusion deps resolve                # shows '(via <id>)' and '(git: <url>)'",
        "get_troubleshooting_guide('transitive') for warning meanings",
    ]
    return json.dumps(report, indent=2, default=str)


# ---------------------------------------------------------------------------
# Tool: check_lock_file_scenarios
# ---------------------------------------------------------------------------


@mcp.tool()
def check_lock_file_scenarios(project_path: str = "", scenario: str = "") -> str:
    """Analyse how diffusion.lock, scenarios/ and diffusion.toml agree per scenario.

    Useful before/after `diffusion deps lock|check|sync --scenario <name>`:
    - lists scenarios discovered under scenarios/ (fallback: default)
    - lists scenario prefixes present in diffusion.lock and in diffusion.toml
    - predicts whether a given --scenario value would be accepted by the CLI
      (scenarios/<name>/ exists OR '<name>.' prefix in diffusion.toml)
    - warns when a scoped lock would fall back to full generation (no lock file)
    - flags lock entries whose scenario has no directory (stale scenarios)

    Args:
        project_path: Path to the project root (auto-detected if empty).
        scenario: Optional --scenario value to validate (empty = all).
    """
    root = Path(project_path) if project_path else _find_project_root()
    if root is None:
        return "Error: Could not find project root."

    report: dict[str, Any] = {"project_root": str(root)}
    warnings: list[str] = []

    dir_scenarios = _discover_scenarios(root)
    has_scenarios_dir = (root / "scenarios").is_dir()
    report["scenarios_dir_present"] = has_scenarios_dir
    report["scenarios_from_directory"] = dir_scenarios

    toml_scenarios = sorted(_toml_dependency_scenarios(root))
    report["scenarios_from_diffusion_toml"] = toml_scenarios

    lock_path = root / "diffusion.lock"
    lock_data: dict[str, Any] | None = None
    if lock_path.exists():
        try:
            lock_data = _load_toml(lock_path)
        except Exception:
            try:
                lock_data = _load_yaml(lock_path)
            except Exception as e:
                return f"Error parsing diffusion.lock: {e}"
    report["lock_file_present"] = lock_data is not None

    if lock_data is not None:
        per_scen = _lock_scenarios(lock_data)
        report["lock_entries_by_scenario"] = per_scen
        for scen in per_scen:
            if scen != "(unprefixed)" and scen not in dir_scenarios and scen not in toml_scenarios:
                warnings.append(
                    f"diffusion.lock has entries for scenario '{scen}' but neither scenarios/{scen}/ "
                    f"nor a '{scen}.' prefix in diffusion.toml exists — a full 'diffusion deps lock' will drop them."
                )
        for scen in dir_scenarios:
            if scen not in per_scen and (root / "scenarios" / scen / "requirements.yml").exists():
                warnings.append(
                    f"scenarios/{scen}/requirements.yml exists but diffusion.lock has no '{scen}.' entries. "
                    f"Run 'diffusion deps lock -s {scen}' (or a full 'diffusion deps lock')."
                )
    else:
        warnings.append(
            "No diffusion.lock. 'diffusion deps lock --scenario X' will FALL BACK to a full generation "
            "for all scenarios; 'diffusion deps sync' / 'deps check' will fail until a lock exists."
        )

    # Predict CLI acceptance of the selector
    sel = scenario.strip()
    if sel:
        accepted = (
            (root / "scenarios" / sel).is_dir()
            or (sel == "default" and not has_scenarios_dir)
            or sel in toml_scenarios
        )
        verdict: dict[str, Any] = {
            "selector": sel,
            "accepted_by_cli": accepted,
            "touches_meta_main_yml": sel == "default",
        }
        if not accepted:
            verdict["expected_error"] = (
                f'scenario "{sel}" not found (no scenarios/{sel} directory)'
            )
            verdict["suggestion"] = (
                f"Create scenarios/{sel}/ (with requirements.yml) or add a '{sel}.<name>' "
                "dependency to diffusion.toml, or omit --scenario to operate on all scenarios."
            )
        report["selector_check"] = verdict
    else:
        report["selector_check"] = {
            "selector": "(empty = all scenarios)",
            "resolves_to": dir_scenarios,
            "touches_meta_main_yml": True,
        }

    report["warnings"] = warnings
    report["summary"] = "ok" if not warnings else f"{len(warnings)} warning(s)"
    return json.dumps(report, indent=2, default=str)


# ---------------------------------------------------------------------------
# Tool: get_troubleshooting_guide
# ---------------------------------------------------------------------------


_TROUBLESHOOTING_CASES: dict[str, dict[str, Any]] = {
    "deps-scenario-not-found": {
        "area": "deps",
        "symptom": 'scenario "<name>" not found (no scenarios/<name> directory)',
        "trigger": "diffusion deps lock|check|sync --scenario <name> (or role add-role/… --scenario <name>)",
        "cause": "The scenario is neither a directory under scenarios/ nor referenced as a '<name>.' prefix by any dependency in diffusion.toml.",
        "fix": [
            "Check spelling: scenario names are case-sensitive directory names.",
            "Create scenarios/<name>/requirements.yml, or add a '<name>.<dep>' entry to diffusion.toml.",
            "Omit --scenario to operate on all discovered scenarios.",
            "Use MCP tool check_lock_file_scenarios(scenario=<name>) to see what the CLI will accept.",
        ],
    },
    "deps-scoped-lock-full-fallback": {
        "area": "deps",
        "symptom": "No existing diffusion.lock found — generating full lock file for all scenarios",
        "trigger": "diffusion deps lock --scenario <name> when diffusion.lock does not exist",
        "cause": "A scenario-scoped lock is a MERGE into an existing lock. Without one, diffusion refuses to write a lock containing only one scenario and regenerates everything instead.",
        "fix": [
            "This is informational, not an error. Commit the full lock; subsequent -s runs will merge.",
            "If resolution of another scenario fails during the fallback, fix that scenario first or temporarily remove it.",
        ],
    },
    "deps-check-scoped-mismatch": {
        "area": "deps",
        "symptom": "Lock file is not fitting yaml manifests for scenario <name>. Run 'diffusion deps sync -s <name>' to update. (exit 1)",
        "trigger": "diffusion deps check --scenario <name>",
        "cause": "requirements.yml of that scenario (and meta/main.yml if <name> == default) disagrees with resolved versions in diffusion.lock.",
        "fix": [
            "To make YAML follow the lock: diffusion deps sync -s <name>",
            "To make the lock follow YAML: diffusion deps lock -s <name> then deps check -s <name>",
            "Note meta/main.yml is only compared for 'default' or when --scenario is omitted.",
        ],
    },
    "deps-meta-not-updated-for-scenario": {
        "area": "deps",
        "symptom": "meta/main.yml collections unchanged after deps sync -s <non-default>",
        "trigger": "diffusion deps sync --scenario production",
        "cause": "By design meta/main.yml only carries default-scenario collections; scoped runs for other scenarios never touch it.",
        "fix": ["Run 'diffusion deps sync' (all) or 'diffusion deps sync -s default'."],
    },
    "deps-lock-drops-scenario": {
        "area": "deps",
        "symptom": "Entries for a scenario vanished from diffusion.lock after a full 'diffusion deps lock'",
        "trigger": "diffusion deps lock (no --scenario)",
        "cause": "Full generation only includes scenarios discovered under scenarios/ (+ diffusion.toml). A scenario directory was removed/renamed while stale lock entries remained.",
        "fix": [
            "Restore the scenarios/<name>/ directory, or accept the removal.",
            "Use check_lock_file_scenarios to spot stale lock scenarios before locking.",
        ],
    },
    "deploy-ssh-key-format": {
        "area": "deploy",
        "symptom": '--ssh-key "<value>": expected format "hostname=<base64>"',
        "trigger": "diffusion deploy --ssh-key <value> without an '=' separator",
        "cause": "Previously such values were silently ignored; they are now a hard usage error.",
        "fix": ["Use 'name=<base64>' e.g. --ssh-key \"*=$(base64 -w0 id_rsa)\"."],
    },
    "deploy-ssh-key-invalid-name": {
        "area": "deploy",
        "symptom": "invalid SSH key/host name \"<name>\": only letters, digits, '.', '-', '_', ':', '*' are allowed",
        "trigger": "diffusion deploy --ssh-key, or Terraform diffusion_deploy.ssh_private_keys map key",
        "cause": "Key names are interpolated into shell commands, env var names and file paths inside the container, so they are restricted to a strict allowlist (^[A-Za-z0-9_.:*-]+$). Spaces, quotes, '/', '$', ';' etc. are rejected.",
        "fix": [
            "Use the inventory hostname, 'group:<groupname>', '*' or a plain alphanumeric fallback name.",
            "Validate with MCP tool troubleshoot_ssh_keys(key_names='web01,group:web,*').",
        ],
    },
    "deploy-ssh-key-dot-segment": {
        "area": "deploy",
        "symptom": 'invalid SSH key/host name "<name>": dot-only path segments are not allowed',
        "trigger": "--ssh-key name of '.', '..', or ending in ':.' / ':..'",
        "cause": "Would produce a path-traversal-like file name under /tmp/ssh-keys/.",
        "fix": ["Choose a real hostname or group name."],
    },
    "deploy-ssh-key-wildcard-collision": {
        "area": "deploy",
        "symptom": "invalid SSH key/host name \"<name>\": collides with the reserved wildcard (\"*\") key sentinel",
        "trigger": "--ssh-key name such as 'wildcard', 'WILDCARD', or '_wildcard_'",
        "cause": "The '*' key is stored as env var SSH_KEY_WILDCARD and file /tmp/ssh-keys/_wildcard_; another key sanitising to the same names would silently clobber it.",
        "fix": ["Use '*' for the actual wildcard key and any other name (e.g. 'default') for a fallback key."],
    },
    "deploy-failure-state-not-written": {
        "area": "deploy",
        "symptom": "warning: could not write failure state to remote hosts: invalid SSH key/host name …",
        "trigger": "Deploy failed AND SSHKeys contained an invalid name (only reachable via direct API use, e.g. an older Terraform provider)",
        "cause": "The failure-state writer validates key names defensively and refuses to build a shell command from an unsafe name.",
        "fix": ["Fix the key name; the deploy itself would already have failed for the same reason via the CLI."],
    },
    "terraform-pem-escaped-newlines": {
        "area": "terraform",
        "symptom": "Load key \"/tmp/ssh-keys/<name>\": invalid format / error in libcrypto during host wait or playbook",
        "trigger": "diffusion_deploy.ssh_private_keys value built from a string that contains literal '\\n' sequences (tfvars JSON, templatefile, remote state)",
        "cause": "The PEM arrived as one line with escaped newlines. Provider versions from 009ca4e onward normalise literal '\\n' to real newlines before base64 encoding; older providers do not.",
        "fix": [
            "Upgrade the terraform-provider-diffusion.",
            "Or pass tls_private_key.<x>.private_key_openssh / file() output directly, which already contains real newlines.",
            "Inside the container: docker exec molecule-<role> head -1 /tmp/ssh-keys/<name> should print '-----BEGIN …'.",
        ],
    },
    "action-update-invalid-scenario": {
        "area": "github-actions",
        "symptom": "::error::Invalid scenario name: '<value>'. Must match ^[A-Za-z0-9._-]+$",
        "trigger": "diffusion-update action with a 'scenario' input containing spaces or shell characters",
        "cause": "The action forwards the value as a single '--scenario=<name>' token and rejects anything that could word-split or inject.",
        "fix": ["Pass a plain scenario directory name, or leave 'scenario' empty to process all scenarios (tests then run on 'default')."],
    },
    "action-test-cgroup-user-slice": {
        "area": "github-actions",
        "symptom": "WARNING: /sys/fs/cgroup/user.slice/user-1000.service is missing; rootless volume mounting may not work",
        "trigger": "diffusion-test action on Ubuntu/Debian runners (step 'Check cgroup v2 user.slice for rootless volume mounting')",
        "cause": "Rootless Docker inside the molecule container relies on cgroup v2 delegation for the runner's user manager. The check is non-fatal and only diagnostic.",
        "fix": [
            "On self-hosted runners: ensure systemd user manager is running (loginctl enable-linger <user>) and cgroup v2 is enabled.",
            "The AppArmor relaxation step (kernel.apparmor_restrict_unprivileged_userns=0) is now non-fatal ('|| true') — failure there no longer aborts the job.",
        ],
    },
    # ------------------------------------------------------------------ patch
    "patch-config-missing": {
        "area": "patch",
        "symptom": "failed to read patch config file: open scenarios/<scenario>/patch.yml: no such file or directory",
        "trigger": "diffusion patch list | check (wrong -s, or run outside the project root)",
        "cause": "list/check require scenarios/<scenario>/patch.yml relative to the CURRENT directory. (diffusion molecule and patch diff/apply silently no-op when it is absent.)",
        "fix": [
            "Run from the project root and pass the right --scenario/-s.",
            "Create scenarios/<scenario>/patch.yml with a top-level 'Bundles:' list.",
        ],
    },
    "patch-bundles-key-case": {
        "area": "patch",
        "symptom": "No patch bundles defined for scenario <s> — although patch.yml has bundles; molecule runs unpatched",
        "trigger": "patch.yml uses 'bundles:' (or any casing other than 'Bundles:')",
        "cause": "The top-level YAML key is matched case-sensitively ('Bundles'). Other casings and unknown keys are ignored silently, so the file parses to zero bundles.",
        "fix": [
            "Rename the top-level key to 'Bundles:'.",
            "Run MCP tool check_patch_config(scenario=<s>) — it flags casing and ignored keys.",
        ],
    },
    "patch-invalid-config": {
        "area": "patch",
        "symptom": "patch apply failed: invalid patch config: patch bundle \"<b>\" task <i>: patching task \"<id>\": <reason>",
        "trigger": "diffusion molecule --converge/--verify (auto-patch) or any diffusion patch subcommand",
        "cause": "patch.yml is validated in full on load. Typical reasons: 'no patch action specified', 'new_file_src and new_template_src are mutually exclusive', 'src overlay cannot be combined with new_module/new_module_setup', '<field> overlay not found: scenarios/<s>/patch/...', '<field> escapes the patch folder', 'condition <n> has empty body', 'module setup <n> has empty key', 'become setup sets neither user nor become', 'duplicate task_id', 'duplicate patch bundle name', 'no tasks to patch', 'bundle name is required'.",
        "fix": [
            "Fix the reported field; overlay files must exist under scenarios/<scenario>/patch/files|templates/ with a relative path.",
            "Validate offline with MCP tool check_patch_config before running molecule.",
        ],
    },
    "patch-role-not-declared": {
        "area": "patch",
        "symptom": "patch bundle \"<b>\": role \"<r>\" not found in diffusion.toml [dependencies] for scenario \"<s>\" (available roles: …) / no roles declared for scenario \"<s>\" / no roles declared in diffusion.toml [dependencies]",
        "trigger": "Loading patch.yml",
        "cause": "Only roles declared under [dependencies] roles for the bundle's 'scenario' field can be patched. A scoped bundle only sees '<scenario>.<role>' entries; an empty scenario field accepts any scenario.",
        "fix": [
            "diffusion role add-role <name> -n <namespace> -s <scenario>, or fix role_name/scenario in the bundle.",
            "Short names ('docker') match namespaced entries ('geerlingguy.docker'); a namespaced bundle name must match in full.",
        ],
    },
    "patch-condition-key": {
        "area": "patch",
        "symptom": "condition key must be one of when, changed_when, failed_when (migrate: put the key in condition: and the expression in body:)",
        "trigger": "new_conditions entry with an old-style or misspelt key",
        "cause": "Each new_conditions item is {condition: when|changed_when|failed_when, body: <expr>}.",
        "fix": ["Rewrite as '- condition: when\\n  body: ansible_os_family == \"Debian\"'."],
    },
    "patch-task-id-invalid": {
        "area": "patch",
        "symptom": "invalid task id \"<id>\": segment \"<x>\" has leading zero | must be a positive integer | empty segment / task id exceeds max depth of 32",
        "trigger": "task_id in patch.yml",
        "cause": "IDs are dotted positive integers ('1', '1.3.1'); handlers use an 'h' prefix ('h1'). diffusion reads the raw scalar text, so unquoted 1.10 stays '1.10' — but quoting is still recommended for other YAML tooling.",
        "fix": ["Copy IDs from 'diffusion patch analyze <role>' (e.g. task_id: \"1.3.1\")."],
    },
    "patch-task-id-branch-or-unknown": {
        "area": "patch",
        "symptom": "patch bundle \"<b>\": task_id \"<id>\" is a branch (visual-only, patch a leaf instead) / unknown task_id \"<id>\" (re-run analyze?)",
        "trigger": "diffusion patch check | diff | apply, or molecule auto-patch",
        "cause": "include_tasks/import_tasks/include_role/import_role/block nodes are branches and never patchable; IDs are positional, so a role version bump can shift or remove them.",
        "fix": [
            "Re-run 'diffusion patch analyze <role> -s <s>' and target a leaf ID.",
            "After every external role upgrade run 'diffusion patch check' — a shifted ID may otherwise patch a different task.",
        ],
    },
    "patch-drift": {
        "area": "patch",
        "symptom": "task <id> drifted since analyze (run analyze again or use --force) / task <id> no longer exists in <file> (role changed since analyze?)",
        "trigger": "diffusion patch apply",
        "cause": "The located YAML node's line or name differs from the analysis (role content changed between analysis and apply).",
        "fix": ["Re-run analyze/check and update task IDs; use --force only if you verified the target task."],
    },
    "patch-overlay-kind-mismatch": {
        "area": "patch",
        "symptom": "task <id> uses module template: use new_template_src / uses module \"<m>\": use new_file_src for non-template tasks / references no files / records no src to overlay / has a dynamic src \"{{ … }}\" (unsupported) / has an absolute src (unsupported)",
        "trigger": "diffusion patch check | apply with new_file_src/new_template_src",
        "cause": "Overlays only replace the leaf's recorded static, relative src: templates/<src> for template tasks, files/<src> for file modules.",
        "fix": ["Pick the matching field, or patch the src argument via new_module_setup instead."],
    },
    "patch-unsupported-v1": {
        "area": "patch",
        "symptom": "new_block apply is not supported in v1 / action:/local_action: tasks are not supported in v1 / module args are not a mapping: use new_module to replace",
        "trigger": "diffusion patch apply/diff or molecule auto-patch",
        "cause": "v1 limitations: new_block validates but cannot be applied; action:-form tasks cannot be mutated; new_module_setup without new_module MERGES into args, which fails for free-form args (e.g. 'command: echo hi').",
        "fix": [
            "Drop new_block; patch the individual leaves instead.",
            "For free-form tasks set new_module (same module) together with new_module_setup to REPLACE the args.",
        ],
    },
    "patch-static-bool-warning": {
        "area": "patch",
        "symptom": "warning: task <id>: static when false (task never runs) — possible design problem, verify intent",
        "trigger": "new_conditions body 'true' or 'false'",
        "cause": "A literal boolean freezes behaviour regardless of facts (always/never runs, always/never changed/failed). Non-fatal advisory.",
        "fix": ["Intended (e.g. disabling a task in tests)? Ignore. Otherwise use a real expression."],
    },
    "patch-no-running-container": {
        "area": "patch",
        "symptom": "no running molecule-* container found (run 'diffusion molecule --converge' first) / container molecule-<r> is not running; run 'diffusion molecule -r <r> --converge' first",
        "trigger": "diffusion patch diff | apply (and analyze when the role is not installed on the host)",
        "cause": "Patching happens inside the running molecule container; without --role the first running molecule-* container is used.",
        "fix": [
            "diffusion molecule -r <role> --converge, then retry.",
            "With several molecule containers running always pass --role/-r.",
        ],
    },
    "patch-role-not-in-container": {
        "area": "patch",
        "symptom": "role not installed in container: role \"<r>\" is not installed in container molecule-<x> under /root/.ansible/roles; run 'diffusion molecule --converge' once or check scenarios/<scenario>/requirements.yml",
        "trigger": "patch diff/apply or molecule auto-patch",
        "cause": "The bundle's role is not in the container's roles path (missing from requirements.yml, or the pre-patch 'ansible-galaxy install' failed — look for 'warning: galaxy install before patching failed').",
        "fix": [
            "Add the role to the scenario (diffusion role add-role … -s <s>; diffusion deps sync -s <s>).",
            "Converge once, then retry; inspect with docker_exec_in_molecule(role, 'ls /root/.ansible/roles').",
        ],
    },
    "patch-role-ambiguous": {
        "area": "patch",
        "symptom": "role \"<short>\" is ambiguous in container molecule-<x>: matches [a.<short> b.<short>] — use the full namespace.role name in patch.yml",
        "trigger": "Short role_name matching several namespaced installs",
        "cause": "diffusion refuses to patch an arbitrary match.",
        "fix": ["Set role_name to the full 'namespace.role'."],
    },
    "patch-stale-overlay": {
        "area": "patch",
        "symptom": "stale patch overlay still mounted in container molecule-<x>: <path> (a process may be holding it; restart the container or run 'diffusion molecule --wipe')",
        "trigger": "patch apply / molecule auto-patch / --destroy after an interrupted run",
        "cause": "The self-heal step could not unmount (umount and umount -l failed). The work tree is deliberately kept so the live mount does not point at a deleted dir.",
        "fix": [
            "docker restart molecule-<role> (or diffusion molecule --wipe) and retry.",
            "Inspect with MCP tool inspect_patch_overlays(role).",
        ],
    },
    "patch-staging-failed": {
        "area": "patch",
        "symptom": "staging role <r> in container molecule-<x>: copying the role into the work dir failed | creating the patch directory tree failed | removing the previous work/backup copies failed …",
        "trigger": "patch apply / molecule auto-patch",
        "cause": "The in-container copy to /var/lib/diffusion-patch/<scenario>/{work,backup} failed — typically no space left in the container writable layer or a read-only filesystem.",
        "fix": [
            "Check df -h / inside the container (troubleshoot_molecule_container).",
            "Free space: diffusion molecule --wipe, docker system prune.",
        ],
    },
    "patch-bind-mount-failed": {
        "area": "patch",
        "symptom": "failed to bind-mount patched role over /root/.ansible/roles/<r>: … permission denied / must be superuser",
        "trigger": "patch apply / molecule auto-patch",
        "cause": "'mount --bind' inside the container needs CAP_SYS_ADMIN; the diffusion molecule container normally runs privileged (Docker-in-Docker). A custom/non-privileged image or runtime breaks it.",
        "fix": ["Use the standard diffusion-molecule-container image started by 'diffusion molecule' (privileged)."],
    },
    "patch-apply-failed-molecule": {
        "area": "patch",
        "symptom": "patch apply failed: <reason> (converge/verify exits non-zero before molecule runs)",
        "trigger": "diffusion molecule (converge, verify, default flow) with scenarios/<s>/patch.yml present",
        "cause": "By design a patch error FAILS the run — silently testing pristine roles would give false confidence.",
        "fix": [
            "Look up <reason> in this guide (query 'patch').",
            "Temporarily move patch.yml away to test unpatched roles.",
        ],
    },
    # ------------------------------------------------------- transitive deps
    "deps-transitive-fetch-failed": {
        "area": "deps",
        "symptom": "could not fetch dependencies of <id>: git clone failed: …",
        "trigger": "diffusion deps lock (transitive enabled) with a private or unreachable git dependency",
        "cause": "Each git dependency is shallow-cloned to read its diffusion.lock/diffusion.toml. A fetch failure is a WARNING — the rest still resolves, but that repo's nested deps are missing from the lock.",
        "fix": [
            "Add credentials: diffusion artifact add <name> (URL must match the repo host); they are injected as GIT_USER_*/GIT_PASSWORD_*/GIT_URL_*.",
            "Check the ref exists (non-constraint versions are used as --branch).",
            "Or run diffusion deps lock --no-transitive.",
        ],
    },
    "deps-transitive-duplicate-or-cycle": {
        "area": "deps",
        "symptom": "skipping <id> required by <parent>: already resolved (duplicate or cycle)",
        "trigger": "diffusion deps lock",
        "cause": "Informational: the identity is already a direct dep, was pulled in by another parent (diamond), or points back at this repo (cycle).",
        "fix": ["No action needed. To pin a different version, declare the dependency directly in diffusion.toml."],
    },
    "deps-transitive-max-depth": {
        "area": "deps",
        "symptom": "max transitive depth 10 reached at <id>: not descending further",
        "trigger": "diffusion deps lock with a very deep git dependency chain",
        "cause": "Recursion is bounded at depth 10.",
        "fix": ["Declare the deeper dependencies directly, or flatten the chain."],
    },
    "deps-transitive-weak-identity": {
        "area": "deps",
        "symptom": "transitive: could not determine repository identity (no git origin / meta role_name); cycle detection relies on dependency graph only",
        "trigger": "diffusion deps lock in a repo without 'origin' remote and without meta/main.yml role_name",
        "cause": "Self-identity falls back to the directory name only, so a remote that depends back on this repo is fetched once before the visited set stops it.",
        "fix": ["git remote add origin <url> and/or set galaxy_info.role_name (+ namespace) in meta/main.yml."],
    },
    "deps-transitive-unresolved": {
        "area": "deps",
        "symptom": "could not resolve <id>: <err>; leaving constraint <c>",
        "trigger": "Remote ships only a diffusion.toml (no lock) and the constraint cannot be resolved",
        "cause": "Entries without a resolved version are resolved after the walk; on failure the constraint (or 'main' for git, 'latest' otherwise) is stored.",
        "fix": ["Ask the remote to commit a diffusion.lock, or pin the dependency directly."],
    },
    "deps-transitive-entries-dropped": {
        "area": "deps",
        "symptom": "required_by entries vanished from diffusion.lock after deps lock",
        "trigger": "deps lock --no-transitive, or [dependencies] transitive = false, or the parent's repo no longer lists them in its default scenario",
        "cause": "Transitive entries are re-derived from their parent on every lock; only the remote's 'default' scenario is imported.",
        "fix": ["Re-enable transitive resolution, or declare the dependency directly."],
    },
    "deps-git-collection-missing-source-url": {
        "area": "deps",
        "symptom": "failed to generate lock file: collection <scenario>.<name>: missing SourceURL for non-Galaxy source \"git\"",
        "trigger": "diffusion deps lock (also role add-*/remove-* re-locks)",
        "cause": "A collection with Source != galaxy has no SourceURL. Previously silently skipped (incomplete lock), now a hard error.",
        "fix": [
            "Add SourceURL = \"<git url>\" to the entry in diffusion.toml, or re-add it: diffusion role add-collection <name> --src <url>.",
            "Check with MCP tool check_transitive_dependencies.",
        ],
    },
    "deps-git-collection-meta-skipped": {
        "area": "deps",
        "symptom": "- skipping git collection <url> (not expressible in meta/main.yml)",
        "trigger": "diffusion deps sync for the default scenario",
        "cause": "meta/main.yml collections only accept 'namespace.name'; git collections are written to requirements.yml only. Informational.",
        "fix": ["Nothing to do; ensure consumers install from requirements.yml."],
    },
    "role-add-collection-namespace-required": {
        "area": "role",
        "symptom": "--namespace/-n is required for Galaxy collections.",
        "trigger": "diffusion role add-collection <name> without --namespace and without --src",
        "cause": "Without --src the collection is resolved from Galaxy, which needs namespace.name.",
        "fix": [
            "Galaxy: diffusion role add-collection general --namespace community",
            "Git: diffusion role add-collection foo --src https://github.com/org/ansible-collection-foo.git",
        ],
    },
    "role-add-collection-no-tag": {
        "area": "role",
        "symptom": "No usable tag found for <url> — using branch 'main'",
        "trigger": "diffusion role add-collection <name> --src <url> without --version",
        "cause": "The highest remote tag is resolved and stored as '>=<tag>'; without tags (or if ls-remote fails) diffusion falls back to branch 'main'.",
        "fix": ["Pass --version <tag|branch|constraint> explicitly, or tag the collection repo."],
    },
    # ----------------------------------------------------------------- security
    "security-option-like-argument": {
        "area": "security",
        "symptom": "refusing to use \"-<x>\": <git URL|version constraint|role name|galaxy name|version|role|scenario|container> looks like a command line option / refusing to clone \"-<x>\": argument looks like a git option",
        "trigger": "Any URL, ref, version, role, scenario or container name starting with '-' — directly or from a third-party diffusion.lock/diffusion.toml during transitive resolution or deploy --role-source",
        "cause": "Argument-injection hardening: values such as '--upload-pack=…' would be executed by git. git calls also pass '--' before positionals; ansible-galaxy role install/init have no '--', hence the prefix guard.",
        "fix": [
            "Fix the offending value (it is never legitimate).",
            "If it comes from a remote dependency's lock file, treat that repository as untrusted and use --no-transitive.",
        ],
    },
    # ------------------------------------------------------------------ molecule
    "molecule-ci-clone-retries": {
        "area": "molecule",
        "symptom": "failed to clone repository —container after 10 attempts: <err>",
        "trigger": "diffusion molecule --ci (repo cloned inside the container)",
        "cause": "The in-container 'git clone --single-branch --branch $GIT_BRANCH $GIT_REMOTE' failed 10 times (network, auth, or wrong branch/remote).",
        "fix": [
            "Check GIT_BRANCH / GIT_REMOTE (derived from GITHUB_REF_NAME / GITHUB_HEAD_REF in Actions) and that the branch exists remotely.",
            "Private repos: make sure artifact source credentials (GIT_USER_N/GIT_PASSWORD_N/GIT_URL_N) are configured.",
            "Reproduce: docker_exec_in_molecule(role, 'cd /tmp && git clone --single-branch --branch \"$GIT_BRANCH\" \"$GIT_REMOTE\" repo').",
        ],
    },
    # ---------------------------------------------------------------- tests role
    "tests-role-postgresql-5-params": {
        "area": "tests-role",
        "symptom": "Unsupported parameters for (community.postgresql.postgresql_query / postgresql_*) module: db, port",
        "trigger": "diffusion molecule --verify with postgres tests, community.postgresql >= 5.0.0 and an older diffusion-ansible-tests-role",
        "cause": "community.postgresql 5.x requires login_db / login_port instead of db / port. diffusion-ansible-tests-role switched its tasks to the new names (user-facing vars postgres_db / postgres_port are unchanged).",
        "fix": [
            "Update the tests role (diffusion molecule --verify --testsoverwrite for diffusion/remote test types).",
            "Or pin community.postgresql < 5.0.0 in the scenario until the tests role is updated.",
        ],
    },
}


@mcp.tool()
def get_troubleshooting_guide(query: str = "") -> str:
    """Look up known Diffusion failure modes and their fixes.

    Covers deps --scenario scoping, transitive dependencies and git-sourced
    collections, `diffusion patch` bundles/overlays, deploy --ssh-key name
    validation, CLI argument-injection guards, molecule CI clone retries,
    diffusion-ansible-tests-role compatibility, Terraform provider PEM
    normalisation, and diffusion-test / diffusion-update GitHub Action
    diagnostics.

    Args:
        query: Case id (e.g. "patch-bundles-key-case"), an area ("deps",
               "patch", "role", "deploy", "security", "molecule",
               "tests-role", "terraform", "github-actions"), or free text
               matched against ids/symptoms/triggers/causes/fixes.
               Empty = list all cases.
    """
    q = query.strip().lower()
    if not q:
        return json.dumps(
            {
                cid: {"area": c["area"], "symptom": c["symptom"]}
                for cid, c in _TROUBLESHOOTING_CASES.items()
            },
            indent=2,
        )
    if q in _TROUBLESHOOTING_CASES:
        return json.dumps({q: _TROUBLESHOOTING_CASES[q]}, indent=2)

    matches = {
        cid: c
        for cid, c in _TROUBLESHOOTING_CASES.items()
        if q == c["area"]
        or q in cid
        or q in c["symptom"].lower()
        or q in c["trigger"].lower()
        or q in c["cause"].lower()
        or any(q in f.lower() for f in c.get("fix", []))
    }
    if not matches:
        return (
            f"No troubleshooting case matched {query!r}. "
            f"Available ids: {', '.join(_TROUBLESHOOTING_CASES)}"
        )
    return json.dumps(matches, indent=2)


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

    for pkg in ("diffusion-mcp", "mcp"):
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
