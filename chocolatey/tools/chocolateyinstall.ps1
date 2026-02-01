$ErrorActionPreference = 'Stop'

# NOTE: The checksum validation logic is injected during the build process
# by the .github/scripts/update-chocolatey.ps1 script. It inserts architecture-specific
# checksums after the $url definition and sets the $checksum variable accordingly.

$packageName = 'diffusion'
# $toolsDir = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
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

# Download paths
$unzipLocation = Split-Path -Parent $MyInvocation.MyCommand.Definition

#DO NOT EDIT BELOW CHECSUMS - they are auto-generated during the build process




# Set package parameters for archive download
$packageArgs = @{
  PackageName    = $packageName
  UnzipLocation   = $unzipLocation
  Url            = $url
  Checksum       = ''
  ChecksumType   = 'sha256'
}

# Download the archive with checksum verification
Write-Host "Downloading $archiveName..." -ForegroundColor Cyan
Install-ChocolateyZipPackage @packageArgs
Write-Host "✓ Archive downloaded and checksum verified" -ForegroundColor Green



