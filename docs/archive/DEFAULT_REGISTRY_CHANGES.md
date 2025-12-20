# Default Container Registry Changes

## Summary

The default container registry configuration has been updated to use GitHub Container Registry (ghcr.io) with the official Polar Team Diffusion molecule container, with automatic architecture detection.

## Changes

### Previous Behavior

```bash
$ diffusion molecule
# First run prompts:
Enter RegistryServer: [user must type]
Enter RegistryProvider: [user must type]
Enter MoleculeContainerName: [user must type]
Enter MoleculeContainerTag (latest): [user must type or press enter]
```

### New Behavior

```bash
$ diffusion molecule
# First run prompts with defaults:
Enter RegistryServer (ghcr.io): [press enter for default]
Enter RegistryProvider (Public): [press enter for default]
Enter MoleculeContainerName (polar-team/diffusion-molecule-container): [press enter for default]
Enter MoleculeContainerTag (latest-amd64): [press enter for default, auto-detects architecture]
```

## Default Values

| Setting | Default Value | Notes |
|---------|--------------|-------|
| Registry Server | `ghcr.io` | GitHub Container Registry |
| Registry Provider | `Public` | No authentication required for public images |
| Container Name | `polar-team/diffusion-molecule-container` | Official Diffusion molecule container |
| Container Tag | `latest-amd64` or `latest-arm64` | Auto-detected based on system architecture |

## Architecture Detection

The container tag is automatically determined based on your system architecture:

- **AMD64 (x86_64)**: Uses `latest-amd64` tag
- **ARM64 (aarch64)**: Uses `latest-arm64` tag
- **Other architectures**: Defaults to `latest-amd64`

This is handled by the `GetDefaultMoleculeTag()` function which uses `runtime.GOARCH` to detect the architecture.

## Configuration File

The default configuration in `diffusion.toml` will look like:

```toml
[container_registry]
registry_server = "ghcr.io"
registry_provider = "Public"
molecule_container_name = "polar-team/diffusion-molecule-container"
molecule_container_tag = "latest-amd64"  # or latest-arm64 depending on your system

[vault]
enabled = false

url = "https://example.com/repo"

[yaml_lint]
extends = "default"
ignore = [".git/*", "molecule/**", "vars/*", "files/*", ".yamllint", ".ansible-lint"]

[ansible_lint]
exclude_paths = ["molecule/default/tests/*.yml", "molecule/default/tests/*/*/*.yml", "tests/test.yml"]
warn_list = ["meta-no-info", "yaml[line-length]"]
skip_list = ["meta-incorrect", "role-name[path]"]

[tests]
type = "diffusion"
```

## Benefits

1. **Faster Setup**: Users can press Enter to accept sensible defaults
2. **Official Container**: Uses the official Polar Team container from GitHub
3. **Architecture Support**: Automatically selects the correct image for your system
4. **Public Registry**: No authentication required for getting started
5. **Better UX**: Clear defaults shown in prompts

## Migration

### For Existing Users

If you already have a `diffusion.toml` file, your existing configuration will be preserved. No action is required.

### For New Users

Simply press Enter when prompted to accept the defaults, or type your custom values if needed.

### Custom Registry

You can still use custom registries (YC, AWS, GCP) by entering your values when prompted:

```bash
Enter RegistryServer (ghcr.io): cr.yandex/my-registry
Enter RegistryProvider (Public): YC
Enter MoleculeContainerName (polar-team/diffusion-molecule-container): my-molecule-image
Enter MoleculeContainerTag (latest-amd64): v1.0
```

## Examples

### Quick Start with Defaults

```bash
$ diffusion molecule --role webserver --org mycompany
# On first run, just press Enter 4 times to accept all defaults
Enter RegistryServer (ghcr.io): ↵
Enter RegistryProvider (Public): ↵
Enter MoleculeContainerName (polar-team/diffusion-molecule-container): ↵
Enter MoleculeContainerTag (latest-amd64): ↵
# Configuration saved, molecule workflow starts
```

### Custom Registry

```bash
$ diffusion molecule --role webserver --org mycompany
Enter RegistryServer (ghcr.io): cr.yandex/my-registry
Enter RegistryProvider (Public): YC
Enter MoleculeContainerName (polar-team/diffusion-molecule-container): custom-molecule
Enter MoleculeContainerTag (latest-amd64): v2.0
```

## Testing

New tests have been added to verify the defaults:

- `TestDefaultConstants` - Verifies all default constant values
- `TestGetDefaultMoleculeTagFormat` - Verifies tag format
- `TestGetDefaultMoleculeTagArchitectures` - Tests architecture detection
- `TestDefaultRegistryConfiguration` - Tests complete default config
- `TestValidateDefaultRegistryProvider` - Validates default provider

Run tests with:
```bash
go test -v -run TestDefault
```

## Technical Details

### New Constants (constants.go)

```go
const (
    DefaultRegistryServer        = "ghcr.io"
    DefaultRegistryProvider      = "Public"
    DefaultMoleculeContainerName = "polar-team/diffusion-molecule-container"
)
```

### New Function (helpers.go)

```go
// GetDefaultMoleculeTag returns the default molecule container tag based on architecture
func GetDefaultMoleculeTag() string {
    arch := runtime.GOARCH
    
    switch arch {
    case "amd64", "arm64":
        return fmt.Sprintf("latest-%s", arch)
    default:
        // Default to amd64 for unknown architectures
        return "latest-amd64"
    }
}
```

## Container Image

The official Diffusion molecule container is hosted at:
- **Repository**: https://github.com/orgs/Polar-Team/packages
- **Image**: `ghcr.io/polar-team/diffusion-molecule-container`
- **Tags**: `latest-amd64`, `latest-arm64`

## Compatibility

- ✅ **AMD64/x86_64**: Fully supported with `latest-amd64` tag
- ✅ **ARM64/aarch64**: Fully supported with `latest-arm64` tag (Apple Silicon, ARM servers)
- ⚠️ **Other architectures**: Falls back to `latest-amd64` (may require emulation)

## Rollback

If you need to use a different registry, simply:
1. Delete or edit your `diffusion.toml` file
2. Run `diffusion molecule` again
3. Enter your custom values when prompted

Or manually edit `diffusion.toml`:

```toml
[container_registry]
registry_server = "your-registry.com"
registry_provider = "YC"  # or AWS, GCP, Public
molecule_container_name = "your-container"
molecule_container_tag = "your-tag"
```
