$ErrorActionPreference = 'Stop'

# NOTE: The checksum validation logic is injected during the build process
# by the .github/scripts/update-chocolatey.ps1 script. It inserts architecture-specific
# checksums after the $url definition and sets the $checksum variable accordingly.

$packageName = 'diffusion'
$toolsDir = "$(Split-Path -parent $MyInvocation.MyCommand.Definition)"
$version = $env:ChocolateyPackageVersion

# Determine system architecture
$arch = if ([Environment]::Is64BitOperatingSystem) {
    if ([Environment]::GetEnvironmentVariable("PROCESSOR_ARCHITECTURE") -eq 'ARM64') {
        'arm64'
    } else {
        'amd64'
    }
} else {
    'arm'
}

# GitHub release URL base
$releaseUrlBase = "https://github.com/Polar-Team/diffusion/releases/download/v$version"

# Determine the correct download URL based on architecture
$url = "$releaseUrlBase/diffusion-windows-$arch.zip"

# Set package parameters
$packageArgs = @{
    packageName    = $packageName
    unzipLocation  = $toolsDir
    url            = $url
    checksum       = ''
    checksumType   = 'sha256'
}

# Download and extract the package
Install-ChocolateyZipPackage @packageArgs

# Verify the executable exists
$exeName = "diffusion-windows-$arch.exe"
$exePath = Join-Path $toolsDir $exeName

if (Test-Path $exePath) {
    # Rename to diffusion.exe for easier access
    $targetPath = Join-Path $toolsDir "diffusion.exe"
    if (Test-Path $targetPath) {
        Remove-Item $targetPath -Force
    }
    Rename-Item -Path $exePath -NewName "diffusion.exe" -Force
    Write-Host "Diffusion has been installed successfully!" -ForegroundColor Green
    Write-Host "Run 'diffusion --help' to get started." -ForegroundColor Cyan
} else {
    throw "Installation failed: executable not found at $exePath"
}
