# Chocolatey Package for Diffusion

This directory contains the Chocolatey package configuration for Diffusion.

## Files

- `diffusion.nuspec` - Package metadata and configuration
- `tools/chocolateyinstall.ps1` - Installation script that downloads and installs the appropriate binary
- `tools/chocolateyuninstall.ps1` - Uninstallation script

## How it Works

The Chocolatey package automatically:
1. Detects the Windows architecture (amd64, arm64, or arm)
2. Downloads the appropriate release binary from GitHub
3. Verifies the checksum
4. Extracts and renames the binary to `diffusion.exe`
5. Adds it to the system PATH

## Installation

Users can install Diffusion using:

```powershell
choco install diffusion
```

## Upgrading

```powershell
choco upgrade diffusion
```

## Uninstallation

```powershell
choco uninstall diffusion
```

## Publishing

The package is automatically published to Chocolatey by the GitHub Actions release workflow when a new tag is pushed.

The workflow:
1. Builds the Chocolatey package
2. Updates the nuspec with the release version
3. Generates checksums for Windows binaries
4. Publishes to Chocolatey using the CHOCOLATEY_API_KEY secret
