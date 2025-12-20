# Verifying Diffusion Binaries

All Diffusion releases are signed and include SLSA Level 3 provenance attestations to ensure supply chain security.

## Quick Verification

### 1. Verify Checksums

```bash
# Download binary and checksums
wget https://github.com/Polar-Team/diffusion/releases/download/v1.0.0/diffusion-linux-amd64
wget https://github.com/Polar-Team/diffusion/releases/download/v1.0.0/SHA256SUMS

# Verify checksum
sha256sum --check SHA256SUMS --ignore-missing
```

**Expected output:**
```
diffusion-linux-amd64: OK
```

### 2. Verify Cosign Signature (Keyless)

```bash
# Install Cosign
brew install cosign  # macOS
# or
wget https://github.com/sigstore/cosign/releases/latest/download/cosign-linux-amd64
chmod +x cosign-linux-amd64
sudo mv cosign-linux-amd64 /usr/local/bin/cosign

# Download signature and certificate
wget https://github.com/Polar-Team/diffusion/releases/download/v1.0.0/diffusion-linux-amd64.sig
wget https://github.com/Polar-Team/diffusion/releases/download/v1.0.0/diffusion-linux-amd64.pem

# Verify signature
cosign verify-blob \
  --certificate diffusion-linux-amd64.pem \
  --signature diffusion-linux-amd64.sig \
  --certificate-identity-regexp="https://github.com/Polar-Team/diffusion" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  diffusion-linux-amd64
```

**Expected output:**
```
Verified OK
```

### 3. Verify SLSA Provenance

```bash
# Install GitHub CLI
brew install gh  # macOS
# or
sudo apt install gh  # Ubuntu/Debian

# Authenticate
gh auth login

# Verify provenance
gh attestation verify diffusion-linux-amd64 --repo Polar-Team/diffusion
```

**Expected output:**
```
✓ Verification succeeded!
```

## Detailed Verification Steps

### Understanding the Security Model

Diffusion uses a multi-layered security approach:

1. **Checksums (SHA256)**: Verify file integrity
2. **Cosign Signatures**: Verify authenticity (keyless signing via Sigstore)
3. **SLSA Provenance**: Verify build provenance and supply chain

### Step-by-Step Verification

#### Step 1: Download Files

```bash
VERSION=1.0.0
BINARY=diffusion-linux-amd64

# Download binary
wget https://github.com/Polar-Team/diffusion/releases/download/v${VERSION}/${BINARY}

# Download verification files
wget https://github.com/Polar-Team/diffusion/releases/download/v${VERSION}/SHA256SUMS
wget https://github.com/Polar-Team/diffusion/releases/download/v${VERSION}/${BINARY}.sig
wget https://github.com/Polar-Team/diffusion/releases/download/v${VERSION}/${BINARY}.pem
```

#### Step 2: Verify Checksum

```bash
# Check if binary matches published checksum
sha256sum ${BINARY}
grep ${BINARY} SHA256SUMS

# Or verify automatically
sha256sum --check SHA256SUMS --ignore-missing
```

This ensures the binary hasn't been tampered with during download.

#### Step 3: Verify Cosign Signature

```bash
# Verify the binary was signed by Polar-Team's GitHub Actions
cosign verify-blob \
  --certificate ${BINARY}.pem \
  --signature ${BINARY}.sig \
  --certificate-identity-regexp="https://github.com/Polar-Team/diffusion" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  ${BINARY}
```

This verifies:
- The binary was signed by Polar-Team's GitHub Actions workflow
- The signature is valid and hasn't been tampered with
- The certificate chain is trusted

#### Step 4: Verify SLSA Provenance

```bash
# Verify the build provenance
gh attestation verify ${BINARY} --repo Polar-Team/diffusion
```

This verifies:
- The binary was built by GitHub Actions
- The build process is reproducible
- The source code matches the repository
- SLSA Level 3 compliance

### Inspecting Provenance

```bash
# Download provenance file
wget https://github.com/Polar-Team/diffusion/releases/download/v${VERSION}/diffusion-${VERSION}.intoto.jsonl

# View provenance details
cat diffusion-${VERSION}.intoto.jsonl | jq .
```

The provenance includes:
- Build environment details
- Source repository and commit
- Build command and parameters
- Builder identity
- Timestamp

## Platform-Specific Instructions

### Linux

```bash
# Ubuntu/Debian
sudo apt install cosign gh

# RHEL/AlmaLinux/Fedora
sudo dnf install cosign gh

# Arch Linux
sudo pacman -S cosign github-cli

# Verify
cosign verify-blob \
  --certificate diffusion-linux-amd64.pem \
  --signature diffusion-linux-amd64.sig \
  --certificate-identity-regexp="https://github.com/Polar-Team/diffusion" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  diffusion-linux-amd64
```

### macOS

```bash
# Install tools
brew install cosign gh

# Verify
cosign verify-blob \
  --certificate diffusion-darwin-arm64.pem \
  --signature diffusion-darwin-arm64.sig \
  --certificate-identity-regexp="https://github.com/Polar-Team/diffusion" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  diffusion-darwin-arm64
```

### Windows

```powershell
# Install Cosign
choco install cosign

# Install GitHub CLI
choco install gh

# Verify (PowerShell)
cosign verify-blob `
  --certificate diffusion-windows-amd64.exe.pem `
  --signature diffusion-windows-amd64.exe.sig `
  --certificate-identity-regexp="https://github.com/Polar-Team/diffusion" `
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" `
  diffusion-windows-amd64.exe
```

## Automated Verification Script

Create a verification script for easy checking:

```bash
#!/bin/bash
# verify-diffusion.sh

set -e

VERSION=${1:-"1.0.0"}
BINARY=${2:-"diffusion-linux-amd64"}
REPO="Polar-Team/diffusion"

echo "Verifying Diffusion ${VERSION} - ${BINARY}"
echo "=========================================="

# Download files
echo "1. Downloading files..."
wget -q https://github.com/${REPO}/releases/download/v${VERSION}/${BINARY}
wget -q https://github.com/${REPO}/releases/download/v${VERSION}/SHA256SUMS
wget -q https://github.com/${REPO}/releases/download/v${VERSION}/${BINARY}.sig
wget -q https://github.com/${REPO}/releases/download/v${VERSION}/${BINARY}.pem

# Verify checksum
echo "2. Verifying checksum..."
sha256sum --check SHA256SUMS --ignore-missing

# Verify signature
echo "3. Verifying Cosign signature..."
cosign verify-blob \
  --certificate ${BINARY}.pem \
  --signature ${BINARY}.sig \
  --certificate-identity-regexp="https://github.com/${REPO}" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  ${BINARY}

# Verify provenance
echo "4. Verifying SLSA provenance..."
gh attestation verify ${BINARY} --repo ${REPO}

echo ""
echo "✓ All verifications passed!"
echo "Binary is authentic and safe to use."
```

**Usage:**
```bash
chmod +x verify-diffusion.sh
./verify-diffusion.sh 1.0.0 diffusion-linux-amd64
```

## Troubleshooting

### Cosign Verification Fails

**Error:** `certificate identity does not match`

**Solution:** Ensure you're using the correct certificate identity regexp:
```bash
--certificate-identity-regexp="https://github.com/Polar-Team/diffusion"
```

### Provenance Verification Fails

**Error:** `no attestations found`

**Solution:** 
1. Ensure you're authenticated with GitHub CLI: `gh auth login`
2. Check the repository name is correct: `Polar-Team/diffusion`
3. Verify the binary name matches exactly

### Checksum Mismatch

**Error:** `FAILED`

**Solution:**
1. Re-download the binary (may be corrupted)
2. Ensure you're downloading from the official GitHub releases page
3. Check your internet connection

## Security Best Practices

1. **Always verify before running**: Never run unverified binaries
2. **Use HTTPS**: Always download from `https://github.com`
3. **Check the repository**: Ensure it's `Polar-Team/diffusion`
4. **Verify all three**: Checksums, signatures, and provenance
5. **Keep tools updated**: Update Cosign and GitHub CLI regularly

## Reporting Security Issues

If you discover a security vulnerability, please email:
- security@polar-team.com

Do not open public issues for security vulnerabilities.

## Additional Resources

- [Sigstore Documentation](https://docs.sigstore.dev/)
- [SLSA Framework](https://slsa.dev/)
- [GitHub Attestations](https://docs.github.com/en/actions/security-guides/using-artifact-attestations-to-establish-provenance-for-builds)
- [Cosign Documentation](https://github.com/sigstore/cosign)

---

<div align="center">
Made with ❤️ by Polar-Team
</div>
