<div align="center">

![Diffusion Logo](img/logo.jpg)

# Diffusion

**A Go-based CLI for simplifying Ansible role testing with Molecule**

[![Release](https://github.com/Polar-Team/diffusion/actions/workflows/release.yml/badge.svg)](https://github.com/Polar-Team/diffusion/actions/workflows/release.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/Polar-Team/diffusion)](https://github.com/Polar-Team/diffusion)

**[📖 Full Documentation →](https://polar-team.github.io/diffusion)**

</div>

---

## [Overview](https://polar-team.github.io/diffusion#overview)

Diffusion is a cross-platform CLI written in Go that streamlines Ansible role testing with Molecule. It provides containerized testing, registry authentication, HashiCorp Vault integration, linting, and dependency locking in a single tool.

## [Key Features](https://polar-team.github.io/diffusion#overview)

- 🔒 Dependency lock files for reproducible builds
- ⚡ Caching for roles, collections, Docker images, and Python packages
- 🚀 Ansible role management with interactive init
- 🐳 Docker-based Molecule testing workflow
- 🔐 HashiCorp Vault integration for credentials
- 📦 Yandex Cloud, AWS ECR, GCP, and public registry support
- 🔍 Built-in YAML and Ansible linting
- 🎯 CI/CD ready with `--ci` flag

## [Prerequisites](https://polar-team.github.io/diffusion#prereqs)

- **Docker** — required
- **Go 1.25.4+** — only if building from source
- **Vault / YC / AWS / gcloud CLI** — optional, per registry/vault usage

## [Installation](https://polar-team.github.io/diffusion#install)

```powershell
# Windows (Chocolatey) — recommended
choco install diffusion
```

```bash
# Go install
go install github.com/Polar-Team/diffusion@latest

# From source
git clone https://github.com/Polar-Team/diffusion.git && cd diffusion && make build
```

## [Quick Start](https://polar-team.github.io/diffusion#quickstart)

```bash
diffusion role --init          # scaffold a new Ansible role
diffusion deps init            # add dependency config
diffusion deps lock            # pin all versions to diffusion.lock
diffusion deps lock -s prod    # re-lock only the 'prod' scenario (others preserved)
diffusion deps lock --no-transitive   # skip nested dependency resolution
diffusion molecule             # converge
diffusion molecule --verify    # verify
diffusion molecule --lint      # lint
diffusion molecule --idempotence
diffusion molecule --destroy
```

```bash
# Add dependencies
diffusion role add-collection general --namespace community                              # collection from Galaxy
diffusion role add-collection foo --src https://github.com/org/ansible-collection-foo.git --version main   # collection from git
```

`deps lock`, `deps check` and `deps sync` accept `--scenario` / `-s`. Omitted means all scenarios. A non-default scenario skips `meta/main.yml` (it only ever holds default-scenario collections). If no `diffusion.lock` exists yet, a scoped lock generates the full file for every scenario.

### Transitive dependencies

When a git-sourced role or collection's repository itself contains a `diffusion.lock` (or just a `diffusion.toml`), `deps lock` pulls that repo's **default-scenario** collections and roles into your lock file, recursing through nested git dependencies up to a depth of 10. Version constraints from all sources are intersected, and every pulled-in entry records a `required_by` field naming the dependency that required it.

Duplicates and cycles are skipped with a warning — including a dependency that points back at your own repository, whose identity is detected from the git `origin` URL and the `role_name` in `meta/main.yml`. Only the remote's `default` scenario is imported; its other scenarios are its own testing concern.

Disable it per-run with `--no-transitive`, or permanently in `diffusion.toml`:

```toml
[dependencies]
transitive = false
```

Note that git-sourced collections cannot be expressed in `meta/main.yml` (it only accepts `namespace.name`), so they are skipped there and written to `requirements.yml` only.

## [Commands](https://polar-team.github.io/diffusion#cmd-molecule)

| Command | Description |
|---|---|
| [`diffusion molecule`](https://polar-team.github.io/diffusion#cmd-molecule) | Run Molecule workflows (converge, verify, lint, idempotence, destroy, wipe) |
| [`diffusion role`](https://polar-team.github.io/diffusion#cmd-role) | Manage role config and dependencies |
| [`diffusion deps`](https://polar-team.github.io/diffusion#cmd-deps) | Dependency management |
| [`diffusion cache`](https://polar-team.github.io/diffusion#cmd-cache) | Caching control |
| [`diffusion artifact`](https://polar-team.github.io/diffusion#cmd-artifact) | Private repo credentials |
| [`diffusion show`](https://polar-team.github.io/diffusion#cmd-show) | Display full configuration |

## [Configuration](https://polar-team.github.io/diffusion#config)

Diffusion uses `diffusion.toml` in your project directory. See also [Registry Support](https://polar-team.github.io/diffusion#registries), [Project Structure](https://polar-team.github.io/diffusion#structure), and [CI/CD Integration](https://polar-team.github.io/diffusion#cicd).

## Documentation

| Guide | Link |
|---|---|
| Dependency Management | [docs →](https://polar-team.github.io/diffusion#deps-guide) |
| Cache Feature | [docs →](https://polar-team.github.io/diffusion#cache-guide) |
| Artifact Management | [docs →](https://polar-team.github.io/diffusion#artifact-guide) |
| Building from Source | [docs →](https://polar-team.github.io/diffusion#building) |
| Verification Guide | [docs →](https://polar-team.github.io/diffusion#verification) |
| Unix Permissions | [docs →](https://polar-team.github.io/diffusion#unix-perms) |
| Role Version Constraints | [docs →](https://polar-team.github.io/diffusion#role-versions) |
| Migration Guide | [docs →](https://polar-team.github.io/diffusion#migration) |
| Changelog | [docs →](https://polar-team.github.io/diffusion#changelog) |

## Related Projects

- [diffusion-molecule-container](https://github.com/Polar-Team/diffusion-molecule-container) — official Docker container with Molecule + Ansible pre-installed
- [diffusion-ansible-tests-role](https://github.com/Polar-Team/diffusion-ansible-tests-role) — reusable verify.yml testing role

## License

MIT — see [LICENSE](LICENSE). Maintained by [Polar-Team](https://github.com/Polar-Team).
