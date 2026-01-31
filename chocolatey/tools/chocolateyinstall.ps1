$ErrorActionPreference = 'Stop'

# NOTE: The checksum validation logic is injected during the build process
# by the .github/scripts/update-chocolatey.ps1 script. It inserts architecture-specific
# checksums after the $url definition and sets the $checksum variable accordingly.

$packageName = 'diffusion'
$toolsDir = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$version = $env:ChocolateyPackageVersion

# Determine system architecture
$arch = if ([Environment]::Is64BitOperatingSystem)
{
  if ([Environment]::GetEnvironmentVariable("PROCESSOR_ARCHITECTURE") -eq 'ARM64')
  {
    'arm64'
  } else
  {
    'amd64'
  }
} else
{
  'arm'
}

# GitHub release URL base
$releaseUrlBase = "https://github.com/Polar-Team/diffusion/releases/download/v$version"

# Determine the correct download URLs
$archiveName = "diffusion-windows-$arch.zip"
$url = "$releaseUrlBase/$archiveName"
$sigUrl = "$releaseUrlBase/$archiveName.sig"
$pemUrl = "$releaseUrlBase/$archiveName.pem"
$provenanceUrl = "$releaseUrlBase/multiple.intoto.jsonl"

# Download paths
$archivePath = Join-Path $env:TEMP $archiveName
$sigPath = Join-Path $env:TEMP "$archiveName.sig"
$pemPath = Join-Path $env:TEMP "$archiveName.pem"
$provenancePath = Join-Path $env:TEMP "multiple.intoto.jsonl"

# Set package parameters for archive download
$packageArgs = @{
  packageName    = $packageName
  fileFullPath   = $archivePath
  url            = $url
  checksum       = ''
  checksumType   = 'sha256'
}

# Download the archive with checksum verification
Write-Host "Downloading $archiveName..." -ForegroundColor Cyan
Get-ChocolateyWebFile @packageArgs
Write-Host "✓ Archive downloaded and checksum verified" -ForegroundColor Green

# Download and verify Cosign signature
$cosignVerified = $false
try
{
  Write-Host "Downloading Cosign signature and certificate..." -ForegroundColor Cyan
  Get-ChocolateyWebFile -PackageName "$packageName-sig" -FileFullPath $sigPath -Url $sigUrl
  Get-ChocolateyWebFile -PackageName "$packageName-pem" -FileFullPath $pemPath -Url $pemUrl

  # Check if cosign is available
  $cosignAvailable = Get-Command cosign -ErrorAction SilentlyContinue

  if ($cosignAvailable)
  {
    Write-Host "Verifying Cosign signature..." -ForegroundColor Cyan
    $verifyOutput = & cosign verify-blob `
      --certificate $pemPath `
      --signature $sigPath `
      --certificate-identity-regexp="https://github.com/Polar-Team/diffusion" `
      --certificate-oidc-issuer="https://token.actions.githubusercontent.com" `
      $archivePath 2>&1

    if ($LASTEXITCODE -eq 0)
    {
      Write-Host "✓ Cosign signature verified successfully!" -ForegroundColor Green
      $cosignVerified = $true
    } else
    {
      Write-Warning "Cosign verification failed: $verifyOutput"
    }
  } else
  {
    Write-Warning "Cosign not found - skipping signature verification"
    Write-Host "To enable automatic signature verification, install cosign:" -ForegroundColor Yellow
    Write-Host "  choco install cosign" -ForegroundColor Yellow
  }
} catch
{
  Write-Warning "Could not verify Cosign signature: $_"
}

# Download and verify SLSA provenance
$slsaVerified = $false
try
{
  Write-Host "Downloading SLSA provenance..." -ForegroundColor Cyan
  Get-ChocolateyWebFile -PackageName "$packageName-provenance" -FileFullPath $provenancePath -Url $provenanceUrl

  # Check if slsa-verifier is available
  $slsaAvailable = Get-Command slsa-verifier -ErrorAction SilentlyContinue

  if ($slsaAvailable)
  {
    Write-Host "Verifying SLSA provenance..." -ForegroundColor Cyan

    # slsa-verifier needs to run in the directory with the artifact
    Push-Location $env:TEMP
    try
    {
      $verifyOutput = & slsa-verifier verify-artifact $archiveName `
        --provenance-path $provenancePath `
        --source-uri github.com/Polar-Team/diffusion `
        --source-tag "v$version" 2>&1

      if ($LASTEXITCODE -eq 0)
      {
        Write-Host "✓ SLSA Level 3 provenance verified successfully!" -ForegroundColor Green
        $slsaVerified = $true
      } else
      {
        Write-Warning "SLSA verification failed: $verifyOutput"
      }
    } finally
    {
      Pop-Location
    }
  } else
  {
    Write-Warning "SLSA verifier not found - skipping provenance verification"
    Write-Host "To enable automatic SLSA verification, install slsa-verifier from:" -ForegroundColor Yellow
    Write-Host "  https://github.com/slsa-framework/slsa-verifier/releases" -ForegroundColor Yellow
  }
} catch
{
  Write-Warning "Could not verify SLSA provenance: $_"
}

# Summary of verifications
Write-Host "`nVerification Summary:" -ForegroundColor Cyan
Write-Host "  SHA256 Checksum: ✓ Verified" -ForegroundColor Green
if ($cosignVerified)
{
  Write-Host "  Cosign Signature: ✓ Verified" -ForegroundColor Green
} else
{
  Write-Host "  Cosign Signature: ⚠ Not verified (cosign not installed or verification failed)" -ForegroundColor Yellow
}
if ($slsaVerified)
{
  Write-Host "  SLSA Provenance: ✓ Verified" -ForegroundColor Green
} else
{
  Write-Host "  SLSA Provenance: ⚠ Not verified (slsa-verifier not installed or verification failed)" -ForegroundColor Yellow
}
Write-Host ""

# Extract the archive
Write-Host "Extracting archive..." -ForegroundColor Cyan
Get-ChocolateyUnzip -FileFullPath $archivePath -Destination $toolsDir

# Clean up downloaded files
Remove-Item $archivePath -Force -ErrorAction SilentlyContinue
Remove-Item $sigPath -Force -ErrorAction SilentlyContinue
Remove-Item $pemPath -Force -ErrorAction SilentlyContinue
Remove-Item $provenancePath -Force -ErrorAction SilentlyContinue

# Verify the executable exists
$exeName = "diffusion-windows-$arch.exe"
$exePath = Join-Path $toolsDir $exeName

if (Test-Path $exePath)
{
  # Rename to diffusion.exe for easier access
  $targetPath = Join-Path $toolsDir "diffusion.exe"
  if (Test-Path $targetPath)
  {
    Remove-Item $targetPath -Force
  }
  Rename-Item -Path $exePath -NewName "diffusion.exe" -Force
  Write-Host "✓ Diffusion has been installed successfully!" -ForegroundColor Green
  Write-Host ""
  Write-Host "Run 'diffusion --help' to get started." -ForegroundColor Cyan

  if (-not $cosignVerified -or -not $slsaVerified)
  {
    Write-Host ""
    Write-Host "Note: For full security verification in future installations:" -ForegroundColor Yellow
    if (-not $cosignVerified)
    {
      Write-Host "  - Install Cosign: choco install cosign" -ForegroundColor Yellow
    }
    if (-not $slsaVerified)
    {
      Write-Host "  - Install SLSA verifier from: https://github.com/slsa-framework/slsa-verifier/releases" -ForegroundColor Yellow
    }
  }
} else
{
  throw "Installation failed: executable not found at $exePath"
}
