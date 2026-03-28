# E2E Test Runner for Diffusion (PowerShell)
# Usage: .\test.ps1 [-Destroy] [-Provision] [-SSH] [-OS <name>] [-Provider <name>] [-ListOS] [-Help]

param(
  [switch]$Destroy,
  [switch]$Provision,
  [switch]$SSH,
  [string]$OS = "ubuntu2404",
  [string]$Provider = "",
  [switch]$ListOS,
  [switch]$Help
)

# Available OS options
$OSOptions = @{
  "ubuntu2404" = "Ubuntu 24.04 LTS"
  "windows11" = "Windows 11"
  "macos15" = "macOS 15 Sequoia"
}

# Colors
function Write-ColorOutput
{
  param(
    [string]$Message,
    [string]$Color = "White"
  )
  Write-Host $Message -ForegroundColor $Color
}

function Write-Success
{
  param([string]$Message)
  Write-ColorOutput $Message "Green"
}

function Write-Error
{
  param([string]$Message)
  Write-ColorOutput $Message "Red"
}

function Write-Warning
{
  param([string]$Message)
  Write-ColorOutput $Message "Yellow"
}

function Write-Info
{
  param([string]$Message)
  Write-ColorOutput $Message "Cyan"
}

# Show help
if ($Help)
{
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
  Write-Host "  -Provider <n> Vagrant provider (auto-detected: hyperv when Core Isolation is on)"
  Write-Host "  -ListOS       List available OS options"
  Write-Host "  -Help         Show this help message"
  Write-Host ""
  Write-Host "Examples:"
  Write-Host "  .\test.ps1                                  # Run tests (auto-detect provider)"
  Write-Host "  .\test.ps1 -OS ubuntu2404                   # Run tests on Ubuntu 24.04"
  Write-Host "  .\test.ps1 -OS debian12 -Destroy            # Test Debian 12 and cleanup"
  Write-Host "  .\test.ps1 -Provider virtualbox             # Force VirtualBox provider"
  Write-Host "  .\test.ps1 -Provider hyperv                 # Force Hyper-V provider"
  Write-Host "  .\test.ps1 -ListOS                          # Show available OS options"
  Write-Host ""
  Write-Host "Provider auto-detection:" -ForegroundColor Cyan
  Write-Host "  On Windows, the script detects whether Hyper-V / Core Isolation is active."
  Write-Host "  If Hyper-V is enabled, it defaults to the 'hyperv' provider because"
  Write-Host "  VirtualBox cannot function with Core Isolation. Use -Provider to override."
  Write-Host ""
  exit 0
}

# List OS options if requested
if ($ListOS)
{
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
if (-not $OSOptions.ContainsKey($OS))
{
  Write-Error "Error: Unknown OS '$OS'"
  Write-Host "Use -ListOS to see available options"
  exit 1
}

Write-Info "Selected OS: $($OSOptions[$OS])"
Write-Host ""

# Check if Vagrant is installed
try
{
  $vagrantVersion = vagrant --version 2>$null
  if ($LASTEXITCODE -ne 0)
  {
    throw "Vagrant not found"
  }
  Write-Info "Found: $vagrantVersion"
} catch
{
  Write-Error "Error: Vagrant is not installed"
  Write-Host "Please install Vagrant from https://www.vagrantup.com/downloads"
  exit 1
}

if ($OS -notlike "windows11")
{
  $Provider = "virtualbox"
  Write-Info "Using 'virtualbox' provider for non-Windows OS"
}

# Auto-detect provider on Windows if not explicitly set
if (-not $Provider)
{
  try
  {
    # Check if Hyper-V is enabled (indicates Core Isolation / VBS is active)
    $hypervFeature = Get-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V 2>$null
    if ($hypervFeature -and $hypervFeature.State -eq "Enabled")
    {
      $Provider = "hyperv"
      Write-Info "Hyper-V detected (Core Isolation active). Using 'hyperv' provider."
      Write-Warning "Hyper-V provider requires an elevated (Administrator) shell."

      # Check if running as Administrator
      $isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
      if (-not $isAdmin)
      {
        Write-Error "Error: Please run this script as Administrator for Hyper-V provider."
        Write-Host "Right-click PowerShell -> 'Run as Administrator', then retry."
        exit 1
      }
    }
  } catch
  {
    # If detection fails (e.g., non-Windows or permission issue), fall through
    Write-Info "Could not detect Hyper-V status. Vagrant will use its default provider."
  }
}

if ($Provider)
{
  Write-Info "Provider: $Provider"
} else
{
  Write-Info "Provider: (Vagrant default)"
}

# Change to e2e directory
$scriptPath = Split-Path -Parent $MyInvocation.MyCommand.Path
Set-Location $scriptPath

# Set environment variable for Vagrant
$env:BOX_NAME = $OS

Write-Host ""

# Build vagrant command arguments
$vagrantProviderArgs = @()
if ($Provider)
{
  $vagrantProviderArgs = @("--provider", $Provider)
}

if ($Provision)
{
  Write-Warning "Re-running provisioning..."
  Write-Host ""
  vagrant provision
  $testResult = $LASTEXITCODE
} else
{
  Write-Warning "Starting VM and running tests..."
  Write-Host ""
  vagrant up @vagrantProviderArgs
  $testResult = $LASTEXITCODE
}

Write-Host ""

# Check if tests passed
if ($testResult -eq 0)
{
  Write-Success "=========================================="
  Write-Success "All tests passed!"
  Write-Success "=========================================="
} else
{
  Write-Error "=========================================="
  Write-Error "Tests failed!"
  Write-Error "=========================================="
  exit 1
}

Write-Host ""

# SSH if requested
if ($SSH)
{
  Write-Warning "Opening SSH session..."
  Write-Host ""
  vagrant ssh
}

# Destroy if requested
if ($Destroy)
{
  Write-Warning "Destroying VM..."
  vagrant destroy -f
  if ($LASTEXITCODE -eq 0)
  {
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
