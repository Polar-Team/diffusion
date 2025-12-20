# E2E Test Runner for Diffusion (PowerShell)
# Usage: .\test.ps1 [-Destroy] [-Provision] [-SSH] [-OS <name>] [-ListOS] [-Help]

param(
    [switch]$Destroy,
    [switch]$Provision,
    [switch]$SSH,
    [string]$OS = "ubuntu2204",
    [switch]$ListOS,
    [switch]$Help
)

# Available OS options
$OSOptions = @{
    "ubuntu2204" = "Ubuntu 22.04 LTS"
    "ubuntu2304" = "Ubuntu 23.04"
    "ubuntu2404" = "Ubuntu 24.04 LTS"
    "debian12" = "Debian 12 (Bookworm)"
    "debian13" = "Debian 13 (Trixie)"
    "windows11" = "Windows 11"
    "windows10" = "Windows 10"
    "macos15" = "macOS 15 Sequoia"
    "macos14" = "macOS 14 Sonoma"
}

# Colors
function Write-ColorOutput {
    param(
        [string]$Message,
        [string]$Color = "White"
    )
    Write-Host $Message -ForegroundColor $Color
}

function Write-Success {
    param([string]$Message)
    Write-ColorOutput $Message "Green"
}

function Write-Error {
    param([string]$Message)
    Write-ColorOutput $Message "Red"
}

function Write-Warning {
    param([string]$Message)
    Write-ColorOutput $Message "Yellow"
}

function Write-Info {
    param([string]$Message)
    Write-ColorOutput $Message "Cyan"
}

# Show help
if ($Help) {
    Write-Host ""
    Write-Host "Diffusion E2E Test Runner" -ForegroundColor Green
    Write-Host ""
    Write-Host "Usage: .\test.ps1 [OPTIONS]"
    Write-Host ""
    Write-Host "Options:"
    Write-Host "  -Destroy      Destroy VM after tests"
    Write-Host "  -Provision    Re-run provisioning only"
    Write-Host "  -SSH          SSH into VM after tests"
    Write-Host "  -OS <name>    Select OS to test (default: ubuntu2204)"
    Write-Host "  -ListOS       List available OS options"
    Write-Host "  -Help         Show this help message"
    Write-Host ""
    Write-Host "Examples:"
    Write-Host "  .\test.ps1                           # Run tests on Ubuntu 22.04"
    Write-Host "  .\test.ps1 -OS ubuntu2404            # Run tests on Ubuntu 24.04"
    Write-Host "  .\test.ps1 -OS debian12 -Destroy     # Test Debian 12 and cleanup"
    Write-Host "  .\test.ps1 -ListOS                   # Show available OS options"
    Write-Host ""
    exit 0
}

# List OS options if requested
if ($ListOS) {
    Write-Host ""
    Write-Info "Available OS options:"
    $OSOptions.GetEnumerator() | Sort-Object Name | ForEach-Object {
        Write-Host "  " -NoNewline
        Write-Success $_.Key -NoNewline
        Write-Host " - $($_.Value)"
    }
    Write-Host ""
    Write-Host "Usage: .\test.ps1 -OS <name>"
    Write-Host ""
    exit 0
}

Write-Success "=========================================="
Write-Success "Diffusion E2E Test Runner"
Write-Success "=========================================="
Write-Host ""

# Validate OS selection
if (-not $OSOptions.ContainsKey($OS)) {
    Write-Error "Error: Unknown OS '$OS'"
    Write-Host "Use -ListOS to see available options"
    exit 1
}

Write-Info "Selected OS: $($OSOptions[$OS])"
Write-Host ""

# Check if Vagrant is installed
try {
    $vagrantVersion = vagrant --version 2>$null
    if ($LASTEXITCODE -ne 0) {
        throw "Vagrant not found"
    }
    Write-Info "Found: $vagrantVersion"
} catch {
    Write-Error "Error: Vagrant is not installed"
    Write-Host "Please install Vagrant from https://www.vagrantup.com/downloads"
    exit 1
}

# Change to e2e directory
$scriptPath = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $scriptPath

# Set environment variable for Vagrant
$env:BOX_NAME = $OS

Write-Host ""

if ($Provision) {
    Write-Warning "Re-running provisioning..."
    Write-Host ""
    vagrant provision
    $testResult = $LASTEXITCODE
} else {
    Write-Warning "Starting VM and running tests..."
    Write-Host ""
    vagrant up
    $testResult = $LASTEXITCODE
}

Write-Host ""

# Check if tests passed
if ($testResult -eq 0) {
    Write-Success "=========================================="
    Write-Success "All tests passed!"
    Write-Success "=========================================="
} else {
    Write-Error "=========================================="
    Write-Error "Tests failed!"
    Write-Error "=========================================="
    exit 1
}

Write-Host ""

# SSH if requested
if ($SSH) {
    Write-Warning "Opening SSH session..."
    Write-Host ""
    vagrant ssh
}

# Destroy if requested
if ($Destroy) {
    Write-Warning "Destroying VM..."
    vagrant destroy -f
    if ($LASTEXITCODE -eq 0) {
        Write-Success "VM destroyed"
    }
}

Write-Host ""
Write-Success "Done!"
Write-Host ""
Write-Host "Useful commands:"
Write-Host "  vagrant ssh              - SSH into the VM"
Write-Host "  vagrant provision        - Re-run tests"
Write-Host "  vagrant destroy -f       - Destroy the VM"
Write-Host "  vagrant status           - Check VM status"
Write-Host ""
