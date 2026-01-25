# Chocolatey Package for Diffusion

This directory contains the Chocolatey package configuration for Diffusion.

## Files

- `diffusion.nuspec` - Package metadata and configuration
- `tools/chocolateyinstall.ps1` - Installation script that downloads and installs the appropriate binary with automatic security verification
- `tools/chocolateyuninstall.ps1` - Uninstallation script
- `tools/VERIFICATION.txt` - Verification instructions for package reviewers

## How it Works

The Chocolatey package automatically:
1. Detects the Windows architecture (amd64, arm64, or arm)
2. Downloads the appropriate release binary from GitHub
3. **Automatically verifies SHA256 checksum** (always)
4. **Automatically verifies Cosign signature** (if cosign is installed)
5. **Automatically verifies SLSA Level 3 provenance** (if slsa-verifier is installed)
6. Extracts and renames the binary to `diffusion.exe`
7. Adds it to the system PATH

## Security Verification

### Always Verified
- **SHA256 Checksum**: Automatically verified by Chocolatey during download

### Optional Automatic Verification
For enhanced security, the install script can automatically verify:
- **Cosign Signature**: Verifies the binary was signed by the official GitHub Actions workflow
- **SLSA Level 3 Provenance**: Verifies the build provenance and supply chain security

To enable full automatic verification, install the verification tools:
```powershell
# Install Cosign for signature verification
choco install cosign

# Install SLSA verifier for provenance verification
# Download from: https://github.com/slsa-framework/slsa-verifier/releases
```

The installation will proceed even if these tools are not installed, with SHA256 checksum verification as the baseline. However, having these tools installed provides the highest level of security assurance.

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
