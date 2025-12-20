# Operating System Support

Diffusion E2E tests support multiple operating systems through Vagrant. This document lists all supported OS configurations.

## Supported Operating Systems

### Ubuntu

| OS Version | Box Name | Vagrant Box | RAM | CPUs |
|------------|----------|-------------|-----|------|
| Ubuntu 22.04 LTS | `ubuntu2204` | `generic/ubuntu2204` | 4GB | 2 |
| Ubuntu 23.04 | `ubuntu2304` | `generic/ubuntu2304` | 4GB | 2 |
| Ubuntu 24.04 LTS | `ubuntu2404` | `generic/ubuntu2404` | 4GB | 2 |

**Default**: Ubuntu 22.04 LTS

### Debian

| OS Version | Box Name | Vagrant Box | RAM | CPUs |
|------------|----------|-------------|-----|------|
| Debian 12 (Bookworm) | `debian12` | `generic/debian12` | 4GB | 2 |
| Debian 13 (Trixie) | `debian13` | `generic/debian13` | 4GB | 2 |

### Windows

| OS Version | Box Name | Vagrant Box | RAM | CPUs | Notes |
|------------|----------|-------------|-----|------|-------|
| Windows 10 | `windows10` | `gusztavvargadr/windows-10` | 8GB | 4 | Requires license |
| Windows 11 | `windows11` | `gusztavvargadr/windows-11` | 8GB | 4 | Requires license |

**Note**: Windows boxes require more resources and a valid Windows license. See [WINDOWS.md](WINDOWS.md) for detailed setup.

### macOS

| OS Version | Box Name | Vagrant Box | RAM | CPUs | Notes |
|------------|----------|-------------|-----|------|-------|
| macOS 14 Sonoma | `macos14` | `yzgyyang/macos-14` | 8GB | 4 | Requires VMware Fusion on macOS host |
| macOS 15 Sequoia | `macos15` | `yzgyyang/macos-15` | 8GB | 4 | Requires VMware Fusion on macOS host |

**Note**: macOS boxes require VMware Fusion provider and can only run on macOS hosts due to Apple's licensing restrictions.

## Usage

### Select OS with Environment Variable

**Linux/macOS:**
```bash
BOX_NAME=ubuntu2404 vagrant up
BOX_NAME=debian12 vagrant up
```

**Windows (PowerShell):**
```powershell
$env:BOX_NAME="ubuntu2404"; vagrant up
$env:BOX_NAME="debian12"; vagrant up
```

### Using Helper Scripts

**Linux/macOS:**
```bash
./test.sh --os ubuntu2404
./test.sh --os debian12
./test.sh --os windows11
```

**Windows (PowerShell):**
```powershell
.\test.ps1 -OS ubuntu2404
.\test.ps1 -OS debian12
.\test.ps1 -OS windows11
```

### List Available OS Options

**Linux/macOS:**
```bash
./test.sh --list-os
```

**Windows (PowerShell):**
```powershell
.\test.ps1 -ListOS
```

## Provider Support

### VirtualBox
- ✅ Ubuntu (all versions)
- ✅ Debian (all versions)
- ✅ Windows (10, 11)
- ❌ macOS (not supported)

### VMware Desktop
- ✅ Ubuntu (all versions)
- ✅ Debian (all versions)
- ✅ Windows (10, 11)
- ✅ macOS (14, 15) - macOS host only

## Resource Requirements

### Minimum System Requirements

| OS Type | RAM | CPUs | Disk Space |
|---------|-----|------|------------|
| Linux (Ubuntu/Debian) | 4GB | 2 | 20GB |
| Windows | 8GB | 4 | 40GB |
| macOS | 8GB | 4 | 40GB |

### Recommended System Requirements

| OS Type | RAM | CPUs | Disk Space |
|---------|-----|------|------------|
| Linux (Ubuntu/Debian) | 8GB | 4 | 30GB |
| Windows | 16GB | 6 | 60GB |
| macOS | 16GB | 6 | 60GB |

## Testing Matrix

The following combinations are tested:

| OS | Docker | Go | Expect | Tests |
|----|--------|-----|--------|-------|
| Ubuntu 22.04 | ✅ | ✅ | ✅ | ✅ |
| Ubuntu 23.04 | ✅ | ✅ | ✅ | ✅ |
| Ubuntu 24.04 | ✅ | ✅ | ✅ | ✅ |
| Debian 12 | ✅ | ✅ | ✅ | ✅ |
| Debian 13 | ✅ | ✅ | ✅ | ✅ |
| Windows 10 | ❌ | ✅ | ❌ | ⚠️ |
| Windows 11 | ❌ | ✅ | ❌ | ⚠️ |
| macOS 14 | ❌ | ✅ | ❌ | ⚠️ |
| macOS 15 | ❌ | ✅ | ❌ | ⚠️ |

**Legend:**
- ✅ Fully supported and tested
- ⚠️ Partial support (build and unit tests only)
- ❌ Not available/supported

## Notes

- **Default OS**: Ubuntu 22.04 LTS is used when no OS is specified
- **Linux**: Full E2E testing with Docker, expect automation, and permission checks
- **Windows**: Build and unit tests only (no Docker-based E2E tests)
- **macOS**: Build and unit tests only (no Docker-based E2E tests)
- **Nested Virtualization**: Some providers may require nested virtualization support

## Troubleshooting

### Box Download Issues
If a box fails to download, try:
```bash
vagrant box add <box-name> --provider virtualbox
```

### Provider Not Available
Ensure your provider is installed:
- VirtualBox: https://www.virtualbox.org/
- VMware Desktop: https://www.vagrantup.com/docs/providers/vmware

### macOS Licensing
macOS boxes can only run on macOS hosts due to Apple's EULA. Running macOS VMs on non-Apple hardware violates the license agreement.

## See Also

- [Quick Start Guide](QUICKSTART.md)
- [Windows Setup](WINDOWS.md)
- [Troubleshooting](TROUBLESHOOTING.md)
- [Main README](../README.md)
