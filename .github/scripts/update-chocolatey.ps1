param(
    [string]$Version,
    [string]$Amd64Checksum,
    [string]$Arm64Checksum,
    [string]$ArmChecksum
)

# Update nuspec version
$nuspecPath = "chocolatey/diffusion.nuspec"
$nuspec = Get-Content $nuspecPath -Raw
$nuspec = $nuspec -replace '<version>0.0.0</version>', "<version>$Version</version>"
Set-Content -Path $nuspecPath -Value $nuspec

Write-Host "Updated nuspec version to: $Version"

# Update install script with checksums
$installScript = "chocolatey/tools/chocolateyinstall.ps1"
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

# Insert checksum logic before packageArgs
$scriptContent = $scriptContent -replace "(\`$url = .+\n)", "`$1`n$newChecksumLogic`n"
$scriptContent = $scriptContent -replace "checksum\s*=\s*''", "checksum = `$checksum"

Set-Content -Path $installScript -Value $scriptContent

Write-Host "Updated install script with checksums"
