# E2E Test Files

## Overview

Complete end-to-end testing setup for Diffusion on Ubuntu 22.04.

## Files

### Core Files

| File | Purpose |
|------|---------|
| `Vagrantfile` | VM configuration and test automation |
| `test.sh` | Linux/macOS test runner script |
| `test.ps1` | Windows PowerShell test runner script |

### Documentation

| File | Purpose |
|------|---------|
| `README.md` | Complete documentation and guide |
| `QUICKSTART.md` | Quick reference for common commands |
| `WINDOWS.md` | Windows-specific setup and troubleshooting |
| `FILES.md` | This file - overview of all files |

### Configuration

| File | Purpose |
|------|---------|
| `.gitignore` | Ignore Vagrant artifacts and test data |

## File Descriptions

### Vagrantfile

The main configuration file that:
- Sets up Ubuntu 22.04 VM
- Installs Docker and Go
- Builds diffusion from source
- Runs unit tests
- Runs E2E tests
- Verifies Unix permissions

**Key Features:**
- 4GB RAM, 2 CPUs
- Syncs project root to `/home/vagrant/diffusion`
- Runs tests as non-root user (vagrant)
- Automated provisioning

### test.sh (Linux/macOS)

Bash script for easy test execution.

**Usage:**
```bash
./test.sh [--destroy] [--provision] [--ssh] [--help]
```

**Features:**
- Color-coded output
- Error handling
- Helpful messages
- Option parsing

### test.ps1 (Windows)

PowerShell script for Windows users.

**Usage:**
```powershell
.\test.ps1 [-Destroy] [-Provision] [-SSH] [-Help]
```

**Features:**
- Color-coded output
- PowerShell parameter handling
- Windows-friendly error messages
- Same functionality as bash version

### README.md

Complete documentation including:
- Prerequisites
- Installation instructions
- Test descriptions
- Troubleshooting guide
- CI/CD integration examples
- Manual testing instructions

### QUICKSTART.md

Quick reference card with:
- Common commands (both bash and PowerShell)
- Expected output
- Quick troubleshooting tips

### WINDOWS.md

Windows-specific guide covering:
- Windows prerequisites
- Provider setup (VMware/VirtualBox)
- PowerShell execution policy
- Common Windows issues
- CI/CD on Windows
- Performance tips

## Directory Structure

```
tests/e2e/
├── Vagrantfile           # VM configuration
├── test.sh              # Linux/macOS runner
├── test.ps1             # Windows runner
├── README.md            # Main documentation
├── QUICKSTART.md        # Quick reference
├── WINDOWS.md           # Windows guide
├── FILES.md             # This file
├── .gitignore           # Git ignore rules
└── .vagrant/            # Vagrant state (ignored)
```

## Test Flow

1. **VM Creation**
   - Vagrant creates Ubuntu 22.04 VM
   - Allocates 4GB RAM, 2 CPUs

2. **Provisioning (as root)**
   - Updates apt packages
   - Installs Docker
   - Installs Go 1.21.5
   - Adds vagrant user to docker group

3. **Building (as vagrant user)**
   - Changes to `/home/vagrant/diffusion`
   - Runs `go build`
   - Runs `go test -v`

4. **E2E Tests (as vagrant user)**
   - Creates test role
   - Initializes diffusion config
   - Verifies permissions
   - Tests cache feature
   - Tests artifact management

5. **Verification**
   - Checks file ownership
   - Verifies no root-owned files
   - Confirms all operations work

## Usage Examples

### Basic Test Run

**Linux/macOS:**
```bash
cd tests/e2e
./test.sh
```

**Windows:**
```powershell
cd tests\e2e
.\test.ps1
```

### Test and Cleanup

**Linux/macOS:**
```bash
./test.sh --destroy
```

**Windows:**
```powershell
.\test.ps1 -Destroy
```

### Interactive Testing

**Linux/macOS:**
```bash
./test.sh --ssh
# Now in VM, run manual tests
```

**Windows:**
```powershell
.\test.ps1 -SSH
# Now in VM, run manual tests
```

### Re-run Tests

**Linux/macOS:**
```bash
./test.sh --provision
```

**Windows:**
```powershell
.\test.ps1 -Provision
```

## Maintenance

### Updating Go Version

Edit `Vagrantfile`, line ~70:
```ruby
GO_VERSION="1.21.5"  # Change this
```

### Updating VM Resources

Edit `Vagrantfile`, lines ~15-20:
```ruby
vmware.vmx["memsize"] = "4096"  # RAM in MB
vmware.vmx["numvcpus"] = "2"    # CPU count
```

### Adding New Tests

Edit `Vagrantfile`, add tests in the second provisioning block (starting around line 100).

## CI/CD Integration

Both scripts can be used in CI/CD:

**GitHub Actions:**
```yaml
- name: Run E2E Tests
  run: |
    cd tests/e2e
    ./test.sh --destroy
```

**Windows CI:**
```yaml
- name: Run E2E Tests
  run: |
    cd tests/e2e
    .\test.ps1 -Destroy
```

## Notes

- All tests run in Ubuntu VM, not on host OS
- This ensures consistent Unix environment
- Windows users test Unix permissions correctly
- VM is isolated and can be destroyed safely
- Tests are idempotent and can be re-run
