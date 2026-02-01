param(
  [Parameter(Mandatory=$true)]
  [string]$Version,
  [Parameter(Mandatory=$true)]
  [string]$Amd64Checksum,
  [Parameter(Mandatory=$true)]
  [string]$Arm64Checksum,
  [Parameter(Mandatory=$true)]
  [string]$ArmChecksum,
  [Parameter(Mandatory=$true)]
  [string]$Amd64SigChecksum,
  [Parameter(Mandatory=$true)]
  [string]$Arm64SigChecksum,
  [Parameter(Mandatory=$true)]
  [string]$ArmSigChecksum,
  [Parameter(Mandatory=$true)]
  [string]$Amd64PemChecksum,
  [Parameter(Mandatory=$true)]
  [string]$Arm64PemChecksum,
  [Parameter(Mandatory=$true)]
  [string]$ArmPemChecksum,
  [Parameter(Mandatory=$true)]
  [string]$ProvenanceChecksum
)

$ErrorActionPreference = 'Stop'

# Validate input parameters
if ([string]::IsNullOrWhiteSpace($Version))
{
  throw "Version parameter cannot be empty"
}
if ([string]::IsNullOrWhiteSpace($Amd64Checksum) -or `
    [string]::IsNullOrWhiteSpace($Arm64Checksum) -or `
    [string]::IsNullOrWhiteSpace($ArmChecksum))
{
  throw "All archive checksum parameters must be provided"
}
if ([string]::IsNullOrWhiteSpace($Amd64SigChecksum) -or `
    [string]::IsNullOrWhiteSpace($Arm64SigChecksum) -or `
    [string]::IsNullOrWhiteSpace($ArmSigChecksum))
{
  throw "All signature checksum parameters must be provided"
}
if ([string]::IsNullOrWhiteSpace($Amd64PemChecksum) -or `
    [string]::IsNullOrWhiteSpace($Arm64PemChecksum) -or `
    [string]::IsNullOrWhiteSpace($ArmPemChecksum))
{
  throw "All certificate checksum parameters must be provided"
}
if ([string]::IsNullOrWhiteSpace($ProvenanceChecksum))
{
  throw "Provenance checksum parameter must be provided"
}

# Update nuspec version
$nuspecPath = "chocolatey/diffusion.nuspec"
if (-not (Test-Path $nuspecPath))
{
  throw "Nuspec file not found at: $nuspecPath"
}

$nuspec = Get-Content $nuspecPath -Raw
$nuspec = $nuspec -replace '<version>0.0.0</version>', "<version>$Version</version>"
Set-Content -Path $nuspecPath -Value $nuspec

Write-Host "Updated nuspec version to: $Version"

# Update install script with checksums
$installScript = "chocolatey/tools/chocolateyinstall.ps1"
if (-not (Test-Path $installScript))
{
  throw "Install script not found at: $installScript"
}

$scriptContent = Get-Content $installScript -Raw

# Replace the checksum logic to use architecture-specific checksums
$newChecksumLogic = @"
# Architecture-specific checksums for archives
`$checksums = @{
    'amd64' = '$Amd64Checksum'
    'arm64' = '$Arm64Checksum'
    'arm'   = '$ArmChecksum'
}

# Architecture-specific checksums for signature files
`$sigChecksums = @{
    'amd64' = '$Amd64SigChecksum'
    'arm64' = '$Arm64SigChecksum'
    'arm'   = '$ArmSigChecksum'
}

# Architecture-specific checksums for certificate files
`$pemChecksums = @{
    'amd64' = '$Amd64PemChecksum'
    'arm64' = '$Arm64PemChecksum'
    'arm'   = '$ArmPemChecksum'
}

# Provenance checksum (architecture-independent)
`$provenanceChecksum = '$ProvenanceChecksum'

`$checksum = `$checksums[`$arch]
`$sigChecksum = `$sigChecksums[`$arch]
`$pemChecksum = `$pemChecksums[`$arch]
"@

# Find the line with download paths comments and insert checksum logic after provenanceUrl
$lines = $scriptContent -split "`r?`n"
$newLines = @()
$checksumsBlockLineFound = $false

foreach ($line in $lines)
{
  $newLines += $line
  if (-not $checksumsBlockLineFound -and $line -match '^\#DO\s*NOT\s*EDIT\s*BELOW\s*CHECKSUMS\s*')
  {
    $checksumsBlockLineFound = $true
    # Add empty line and checksum logic after the provenanceUrl line
    $newLines += ""
    $newLines += $newChecksumLogic.Split("`n")
  }
}

if (-not $checksumsBlockLineFound)
{
  throw "Could not find DO NOT EDIT BELOW CHECKSUMS block."
}

# Replace the empty checksum placeholder
$updatedScript = ($newLines -join "`n") -replace "Checksum\s*=\s*''", "Checksum = `$checksum"

Set-Content -Path $installScript -Value $updatedScript

Write-Host "Updated install script with checksums"

# Verify the changes were applied
$verifyContent = Get-Content $installScript -Raw
if ($verifyContent -notmatch [regex]::Escape($Amd64Checksum))
{
  throw "Verification failed: AMD64 checksum not found in updated script"
}

Write-Host "Verification passed - checksums successfully added to install script"
