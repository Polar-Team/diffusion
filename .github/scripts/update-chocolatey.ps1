param(
    [Parameter(Mandatory=$true)]
    [string]$Version,
    [Parameter(Mandatory=$true)]
    [string]$Amd64Checksum,
    [Parameter(Mandatory=$true)]
    [string]$Arm64Checksum,
    [Parameter(Mandatory=$true)]
    [string]$ArmChecksum
)

$ErrorActionPreference = 'Stop'

# Validate input parameters
if ([string]::IsNullOrWhiteSpace($Version)) {
    throw "Version parameter cannot be empty"
}
if ([string]::IsNullOrWhiteSpace($Amd64Checksum) -or `
    [string]::IsNullOrWhiteSpace($Arm64Checksum) -or `
    [string]::IsNullOrWhiteSpace($ArmChecksum)) {
    throw "All checksum parameters must be provided"
}

# Update nuspec version
$nuspecPath = "chocolatey/diffusion.nuspec"
if (-not (Test-Path $nuspecPath)) {
    throw "Nuspec file not found at: $nuspecPath"
}

$nuspec = Get-Content $nuspecPath -Raw
$nuspec = $nuspec -replace '<version>0.0.0</version>', "<version>$Version</version>"
Set-Content -Path $nuspecPath -Value $nuspec

Write-Host "Updated nuspec version to: $Version"

# Update install script with checksums
$installScript = "chocolatey/tools/chocolateyinstall.ps1"
if (-not (Test-Path $installScript)) {
    throw "Install script not found at: $installScript"
}

$scriptContent = Get-Content $installScript -Raw

# Replace the checksum logic to use architecture-specific checksums
$newChecksumLogic = @"
# Architecture-specific checksums
`$checksums = @{
    'amd64' = '$Amd64Checksum'
    'arm64' = '$Arm64Checksum'
    'arm'   = '$ArmChecksum'
}

`$checksum = `$checksums[`$arch]
"@

# Find the line with $url and insert checksum logic after it
$lines = $scriptContent -split "`r?`n"
$newLines = @()
$urlLineFound = $false

foreach ($line in $lines) {
    $newLines += $line
    if (-not $urlLineFound -and $line -match '^\$url = ') {
        $urlLineFound = $true
        # Add empty line and checksum logic after the $url line
        $newLines += ""
        $newLines += $newChecksumLogic.Split("`n")
    }
}

if (-not $urlLineFound) {
    throw "Could not find `$url line in install script"
}

# Replace the empty checksum placeholder
$updatedScript = ($newLines -join "`n") -replace "checksum\s*=\s*''", "checksum = `$checksum"

Set-Content -Path $installScript -Value $updatedScript

Write-Host "Updated install script with checksums"

# Verify the changes were applied
$verifyContent = Get-Content $installScript -Raw
if ($verifyContent -notmatch [regex]::Escape($Amd64Checksum)) {
    throw "Verification failed: AMD64 checksum not found in updated script"
}

Write-Host "Verification passed - checksums successfully added to install script"
