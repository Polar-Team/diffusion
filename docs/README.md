# Diffusion Documentation

Welcome to the Diffusion documentation! This directory contains comprehensive guides and references for using Diffusion.

## 📖 User Guides

### [Building from Source](building.md)
Complete guide to building Diffusion for different platforms:
- Quick start with Make
- Cross-compilation for Linux, macOS, Windows
- Platform-specific builds (AMD64, ARM64, ARM)
- CI/CD integration examples

### [Verification Guide](verification.md)
Learn how to verify Diffusion binary authenticity and integrity:
- Checksum verification with SHA256
- Cosign signature verification (keyless)
- SLSA Level 3 provenance verification
- Platform-specific instructions
- Automated verification scripts

### [Cache Feature](cache-feature.md)
Learn how to use Ansible role and collection caching to speed up your molecule tests. Covers:
- Enabling and disabling cache
- Cache management commands
- How caching works
- Performance benefits

### [Artifact Management](artifact-management.md)
Comprehensive guide to managing private repository credentials:
- Adding and removing artifact sources
- Secure credential storage
- HashiCorp Vault integration
- Multiple artifact source support

### [Unix Permissions](unix-permissions.md)
Understanding how Diffusion handles file permissions on Unix systems:
- Docker-in-Docker (DinD) requirements
- Container-based permission fixes
- Why the main container runs as root
- Troubleshooting permission issues

### [Migration Guide](migration-guide.md)
Guide for upgrading from older versions of Diffusion:
- Breaking changes
- Configuration updates
- Migration steps

### [Changelog](changelog.md)
Complete version history and release notes.

## 🧪 Testing

### [E2E Testing](../tests/e2e/README.md)
End-to-end testing documentation for local development:
- Running tests with Vagrant
- Multiple OS support (Ubuntu, Debian, Windows, macOS)
- Expect-based CLI automation
- Local development testing

**Additional E2E Documentation:**
- [Quick Start Guide](../tests/e2e/docs/QUICKSTART.md)
- [OS Support](../tests/e2e/docs/OS_SUPPORT.md)
- [Windows Setup](../tests/e2e/docs/WINDOWS.md)
- [Troubleshooting](../tests/e2e/docs/TROUBLESHOOTING.md)

## 🗄️ Technical Archives

The [archive](archive/) directory contains historical documentation about implementation changes and refactoring:
- `ARTIFACT_CLI_FIX.md` - Artifact CLI command fixes
- `DEFAULT_REGISTRY_CHANGES.md` - Default registry configuration changes
- `INDEXED_ENVIRONMENT_VARIABLES.md` - Environment variable indexing
- `ROLE_COMMAND_CHANGES.md` - Role command refactoring
- `SECRETS_PATH_REFACTORING.md` - Secrets storage path changes
- `TESTING_AND_PERFORMANCE.md` - Testing and performance improvements
- `VAULT_FIELD_REFACTORING.md` - Vault field configuration changes

These documents are kept for historical reference and may contain outdated information.

## 🚀 Quick Links

- [Main README](../README.md) - Project overview and quick start
- [GitHub Repository](https://github.com/Polar-Team/diffusion)
- [Issues](https://github.com/Polar-Team/diffusion/issues)

## 💡 Need Help?

If you can't find what you're looking for:
1. Check the [main README](../README.md) for quick start guides
2. Browse the documentation files above
3. Visit our [GitHub Issues](https://github.com/Polar-Team/diffusion/issues) page
4. Check the [E2E testing documentation](../tests/e2e/README.md) for testing examples

---

<div align="center">
Made with ❤️ by Polar-Team
</div>
