# Verifying Diffusion Binaries

All Diffusion releases are signed with Cosign and include SLSA Level 3 provenance to ensure supply chain security.

## Quick Verification

### 1. Verify Checksums

```bash
# Download archive and checksums
wget https://github.com/Polar-Team/diffusion/releases/download/v1.0.0/diffusion-linux-amd64.tar.gz
wget https://github.com/Polar-Team/diffusion/releases/download/v1.0.0/SHA256SUMS

# Verify checksum
sha256sum --check SHA256SUMS --ignore-missing
```

**Expected output:**
```
diffusion-linux-amd64.tar.gz: OK
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
wget https://github.com/Polar-Team/diffusion/releases/download/v1.0.0/diffusion-linux-amd64.tar.gz.sig
wget https://github.com/Polar-Team/diffusion/releases/download/v1.0.0/diffusion-linux-amd64.tar.gz.pem

# Verify signature
cosign verify-blob \
  --certificate diffusion-linux-amd64.tar.gz.pem \
  --signature diffusion-linux-amd64.tar.gz.sig \
  --certificate-identity-regexp="https://github.com/Polar-Team/diffusion" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  diffusion-linux-amd64.tar.gz
```

**Expected output:**
```
Verified OK
```

### 3. Verify SLSA Provenance (Recommended)

SLSA (Supply-chain Levels for Software Artifacts) provides the highest level of supply chain security.

```bash
# Install slsa-verifier
# For Linux
wget https://github.com/slsa-framework/slsa-verifier/releases/latest/download/slsa-verifier-linux-amd64
chmod +x slsa-verifier-linux-amd64
sudo mv slsa-verifier-linux-amd64 /usr/local/bin/slsa-verifier

# For macOS
brew install slsa-verifier

# Download SLSA provenance
wget https://github.com/Polar-Team/diffusion/releases/download/v1.0.0/multiple.intoto.jsonl

# Verify SLSA provenance
slsa-verifier verify-artifact diffusion-linux-amd64.tar.gz \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/Polar-Team/diffusion \
  --source-tag v1.0.0
```

**Expected output:**
```
Verified signature against tlog entry index <REKOR_INDEX> at URL: https://rekor.sigstore.dev/api/v1/log/entries/<ENTRY_ID>
Verified build using builder https://github.com/slsa-framework/slsa-github-generator/.github/workflows/generator_generic_slsa3.yml@refs/tags/v2.1.0 at commit <COMMIT_SHA>
Verifying artifact diffusion-linux-amd64.tar.gz: PASSED

PASSED: Verified SLSA provenance
```

*Note: Values like `<REKOR_INDEX>`, `<ENTRY_ID>`, and `<COMMIT_SHA>` will be specific to your verification and differ for each release.*

## Detailed Verification Steps

### Understanding the Security Model

Diffusion uses a multi-layered security approach:

1. **Checksums (SHA256)**: Verify file integrity
2. **Cosign Signatures**: Verify authenticity (keyless signing via Sigstore)
3. **SLSA Level 3 Provenance**: Verify build process and supply chain security

### Step-by-Step Verification

#### Step 1: Download Files

```bash
VERSION=1.0.0
ARCHIVE=diffusion-linux-amd64.tar.gz

# Download archive
wget https://github.com/Polar-Team/diffusion/releases/download/v${VERSION}/${ARCHIVE}

# Download verification files
wget https://github.com/Polar-Team/diffusion/releases/download/v${VERSION}/SHA256SUMS
wget https://github.com/Polar-Team/diffusion/releases/download/v${VERSION}/${ARCHIVE}.sig
wget https://github.com/Polar-Team/diffusion/releases/download/v${VERSION}/${ARCHIVE}.pem
wget https://github.com/Polar-Team/diffusion/releases/download/v${VERSION}/multiple.intoto.jsonl
```

#### Step 2: Verify Checksum

```bash
# Check if archive matches published checksum
sha256sum ${ARCHIVE}
grep ${ARCHIVE} SHA256SUMS

# Or verify automatically
sha256sum --check SHA256SUMS --ignore-missing
```

This ensures the archive hasn't been tampered with during download.

#### Step 3: Verify Cosign Signature

```bash
# Verify the archive was signed by Polar-Team's GitHub Actions
cosign verify-blob \
  --certificate ${ARCHIVE}.pem \
  --signature ${ARCHIVE}.sig \
  --certificate-identity-regexp="https://github.com/Polar-Team/diffusion" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  ${ARCHIVE}
```

This verifies:
- The archive was signed by Polar-Team's GitHub Actions workflow
- The signature is valid and hasn't been tampered with
- The certificate chain is trusted

#### Step 4: Verify SLSA Provenance (Highest Security)

```bash
# Verify the build process and supply chain
slsa-verifier verify-artifact ${ARCHIVE} \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/Polar-Team/diffusion \
  --source-tag v${VERSION}
```

This verifies:
- The archive was built by the official GitHub Actions workflow
- The build was triggered from the correct source repository
- The build process meets SLSA Level 3 requirements
- The build provenance is cryptographically signed and tamper-proof

## Platform-Specific Instructions

### Linux

```bash
# Ubuntu/Debian
sudo apt install cosign gh

# Install slsa-verifier
wget https://github.com/slsa-framework/slsa-verifier/releases/latest/download/slsa-verifier-linux-amd64
chmod +x slsa-verifier-linux-amd64
sudo mv slsa-verifier-linux-amd64 /usr/local/bin/slsa-verifier

# RHEL/AlmaLinux/Fedora
sudo dnf install cosign gh

# Arch Linux
sudo pacman -S cosign github-cli

# Verify archive
cosign verify-blob \
  --certificate diffusion-linux-amd64.tar.gz.pem \
  --signature diffusion-linux-amd64.tar.gz.sig \
  --certificate-identity-regexp="https://github.com/Polar-Team/diffusion" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  diffusion-linux-amd64.tar.gz

# Verify SLSA provenance
slsa-verifier verify-artifact diffusion-linux-amd64.tar.gz \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/Polar-Team/diffusion \
  --source-tag v1.0.0
```

### macOS

```bash
# Install tools
brew install cosign gh slsa-verifier

# Verify archive
cosign verify-blob \
  --certificate diffusion-darwin-arm64.tar.gz.pem \
  --signature diffusion-darwin-arm64.tar.gz.sig \
  --certificate-identity-regexp="https://github.com/Polar-Team/diffusion" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  diffusion-darwin-arm64.tar.gz

# Verify SLSA provenance
slsa-verifier verify-artifact diffusion-darwin-arm64.tar.gz \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/Polar-Team/diffusion \
  --source-tag v1.0.0
```

### Windows

```powershell
# Install Cosign
choco install cosign

# Install GitHub CLI
choco install gh

# Install slsa-verifier (download from GitHub releases)
# Download from: https://github.com/slsa-framework/slsa-verifier/releases/latest

# Verify (PowerShell)
cosign verify-blob `
  --certificate diffusion-windows-amd64.zip.pem `
  --signature diffusion-windows-amd64.zip.sig `
  --certificate-identity-regexp="https://github.com/Polar-Team/diffusion" `
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" `
  diffusion-windows-amd64.zip

# Verify SLSA provenance
slsa-verifier verify-artifact diffusion-windows-amd64.zip `
  --provenance-path multiple.intoto.jsonl `
  --source-uri github.com/Polar-Team/diffusion `
  --source-tag v1.0.0
```

## Automated Verification Script

Create a verification script for easy checking:

```bash
#!/bin/bash
# verify-diffusion.sh

set -e

VERSION=${1:-"1.0.0"}
ARCHIVE=${2:-"diffusion-linux-amd64.tar.gz"}
REPO="Polar-Team/diffusion"

echo "Verifying Diffusion ${VERSION} - ${ARCHIVE}"
echo "=========================================="

# Download files
echo "1. Downloading files..."
wget -q https://github.com/${REPO}/releases/download/v${VERSION}/${ARCHIVE}
wget -q https://github.com/${REPO}/releases/download/v${VERSION}/SHA256SUMS
wget -q https://github.com/${REPO}/releases/download/v${VERSION}/${ARCHIVE}.sig
wget -q https://github.com/${REPO}/releases/download/v${VERSION}/${ARCHIVE}.pem
wget -q https://github.com/${REPO}/releases/download/v${VERSION}/multiple.intoto.jsonl

# Verify checksum
echo "2. Verifying checksum..."
sha256sum --check SHA256SUMS --ignore-missing

# Verify signature
echo "3. Verifying Cosign signature..."
cosign verify-blob \
  --certificate ${ARCHIVE}.pem \
  --signature ${ARCHIVE}.sig \
  --certificate-identity-regexp="https://github.com/${REPO}" \
  --certificate-oidc-issuer="https://token.actions.githubusercontent.com" \
  ${ARCHIVE}

# Verify SLSA provenance
echo "4. Verifying SLSA provenance..."
slsa-verifier verify-artifact ${ARCHIVE} \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/${REPO} \
  --source-tag v${VERSION}

echo ""
echo "✓ All verifications passed!"
echo "Archive is authentic and safe to use."
echo ""
echo "To extract the archive:"
if [[ ${ARCHIVE} == *.tar.gz ]]; then
  echo "  tar -xzf ${ARCHIVE}"
else
  echo "  unzip ${ARCHIVE}"
fi
```

**Usage:**
```bash
chmod +x verify-diffusion.sh
./verify-diffusion.sh 1.0.0 diffusion-linux-amd64.tar.gz
```

## Troubleshooting

### Cosign Verification Fails

**Error:** `certificate identity does not match`

**Solution:** Ensure you're using the correct certificate identity regexp:
```bash
--certificate-identity-regexp="https://github.com/Polar-Team/diffusion"
```

### Checksum Mismatch

**Error:** `FAILED`

**Solution:**
1. Re-download the archive (may be corrupted)
2. Ensure you're downloading from the official GitHub releases page
3. Check your internet connection

### SLSA Verification Fails

**Error:** `FAILED: SLSA verification failed`

**Possible causes:**
1. **Incorrect source tag**: Make sure the tag matches exactly (e.g., `v1.0.0`)
2. **Wrong provenance file**: Ensure you downloaded `multiple.intoto.jsonl`
3. **Artifact name mismatch**: The archive name must match exactly what's in the provenance

**Solution:**
```bash
# Verify you have the correct version
slsa-verifier verify-artifact diffusion-linux-amd64.tar.gz \
  --provenance-path multiple.intoto.jsonl \
  --source-uri github.com/Polar-Team/diffusion \
  --source-tag v1.0.0  # Must match the release tag exactly
```

## Security Best Practices

1. **Always verify before running**: Never run unverified binaries or archives
2. **Use HTTPS**: Always download from `https://github.com`
3. **Check the repository**: Ensure it's `Polar-Team/diffusion`
4. **Verify all three**: Checksums, Cosign signatures, and SLSA provenance
5. **Keep tools updated**: Update Cosign and slsa-verifier regularly
6. **Verify on trusted systems**: Perform verification on a trusted, clean system

## Understanding SLSA Levels

Diffusion releases meet **SLSA Build Level 3** requirements:

- ✅ **Build Integrity**: Build process is isolated and tamper-proof
- ✅ **Provenance**: Complete build provenance is generated automatically
- ✅ **Non-falsifiable**: Provenance is cryptographically signed and verifiable
- ✅ **Dependency Tracking**: Build dependencies are tracked and recorded

For more information about SLSA, visit: https://slsa.dev

## Reporting Security Issues

If you discover a security vulnerability, please email:
- security@polar-team.com

Do not open public issues for security vulnerabilities.

## Additional Resources

- [Sigstore Documentation](https://docs.sigstore.dev/)
- [Cosign Documentation](https://github.com/sigstore/cosign)

---

<div align="center">
Made with ❤️ by Polar-Team
</div>
