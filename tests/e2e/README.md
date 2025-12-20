# Diffusion E2E Tests

End-to-end tests for Diffusion on Ubuntu 22.04 using Vagrant.

## Prerequisites

- [Vagrant](https://www.vagrantup.com/downloads) installed
- One of the following providers:
  - [VMware Desktop](https://www.vagrantup.com/docs/providers/vmware) (recommended)
  - [VirtualBox](https://www.virtualbox.org/)

**Windows Users:** See [WINDOWS.md](WINDOWS.md) for detailed Windows setup instructions.

## Running Tests

### Quick Start

**Linux/macOS:**
```bash
cd tests/e2e
./test.sh
```

**Windows (PowerShell):**
```powershell
cd tests/e2e
.\test.ps1
```

**Or using Vagrant directly:**
```bash
cd tests/e2e
vagrant up
```

This will:
1. Create an Ubuntu 22.04 VM
2. Install Docker, Go, and expect
3. Build diffusion
4. Run unit tests
5. Run E2E tests using expect for CLI automation
6. Verify Unix permissions

### View Test Output

**Using helper scripts:**
```bash
# Linux/macOS
./test.sh --provision

# Windows
.\test.ps1 -Provision
```

**Or directly:**
```bash
vagrant provision
```

### SSH into VM

```bash
vagrant ssh
```

### Clean Up

```bash
vagrant destroy -f
```

## What Gets Tested

### 1. Build & Unit Tests
- ✓ Diffusion builds successfully
- ✓ All Go unit tests pass

### 2. Role Initialization (using expect)
- ✓ `diffusion role --init` creates new role
- ✓ Automated input via expect script
- ✓ Role structure is correct
- ✓ Files have correct ownership

### 3. Configuration (using expect)
- ✓ `diffusion.toml` is created
- ✓ Automated multi-prompt input via expect
- ✓ Config has correct format
- ✓ Registry settings work

### 4. Unix Permissions
- ✓ `molecule/` directory owned by user (not root)
- ✓ Files in `molecule/` owned by user
- ✓ No permission errors during operations

### 5. Cache Feature
- ✓ Cache can be enabled
- ✓ Cache directory created with correct permissions
- ✓ Cache status shows correct info

### 6. Artifact Management
- ✓ Artifact commands work
- ✓ No permission errors

## Expect Automation

The E2E tests use `expect` for reliable CLI automation instead of EOF heredocs:

### Example: Role Initialization
```bash
expect <<'EXPECT_SCRIPT'
set timeout 30
spawn /home/vagrant/diffusion/diffusion role --init
expect "Enter role name:"
send "test-role\\r"
expect "What namespace of the role should be?:"
send "testorg\\r"
expect "What company of the role should be?:"
send "Test Company\\r"
expect "What author of the role should be?:"
send "Test Author\\r"
expect "Description of the role (optional):"
send "Test role for E2E testing\\r"
expect "Enter platforms? (y/n):"
send "n\\r"
expect "Galaxy Tags (comma-separated) (optional):"
send "test,e2e\\r"
expect "Collections required (comma-separated) (optional):"
send "\\r"
expect "Enter roles to add? (y/n):"
send "n\\r"
expect eof
EXPECT_SCRIPT
```

### Example: Molecule Configuration
```bash
expect <<'EXPECT_SCRIPT'
set timeout 60
spawn /home/vagrant/diffusion/diffusion molecule --role test-role --org myorg
expect "Enter registry URL*"
send "ghcr.io\\r"
expect "Is this a private registry*"
send "Public\\r"
expect "Enter image name*"
send "polar-team/diffusion-molecule-container\\r"
expect "Enter image tag*"
send "latest-amd64\\r"
expect "Do you want to add another artifact source*"
send "n\\r"
expect "Select secrets storage*"
send "local\\r"
expect eof
EXPECT_SCRIPT
```

### Benefits of Expect
- **Reliable**: Waits for prompts before sending input
- **Timeout handling**: Configurable timeouts prevent hanging
- **Pattern matching**: Flexible prompt detection with wildcards
- **Better error handling**: Detects when commands fail to prompt

## Test Environment

- **OS**: Ubuntu 22.04 LTS
- **RAM**: 4GB
- **CPUs**: 2
- **Docker**: Latest stable
- **Go**: 1.21.5
- **Expect**: Latest (for CLI automation)
- **User**: vagrant (non-root)

## Troubleshooting

### Helper Script Commands

| Action | Linux/macOS | Windows (PowerShell) |
|--------|-------------|---------------------|
| Run tests | `./test.sh` | `.\test.ps1` |
| Run and destroy | `./test.sh --destroy` | `.\test.ps1 -Destroy` |
| Run and SSH | `./test.sh --ssh` | `.\test.ps1 -SSH` |
| Re-provision | `./test.sh --provision` | `.\test.ps1 -Provision` |
| Show help | `./test.sh --help` | `.\test.ps1 -Help` |

### VM won't start

```bash
vagrant destroy -f
vagrant up
```

### Tests fail

SSH into VM and run manually:
```bash
vagrant ssh
cd /home/vagrant/diffusion
go test -v
```

### Permission errors

Check file ownership:
```bash
vagrant ssh
ls -la /home/vagrant/test-role/molecule/
```

### Docker issues

Check Docker status:
```bash
vagrant ssh
sudo systemctl status docker
docker ps
```

## Manual Testing

After `vagrant up`, you can SSH in and test manually:

```bash
vagrant ssh

# Build diffusion
cd /home/vagrant/diffusion
go build -o diffusion

# Create test role
mkdir -p ~/manual-test
cd ~/manual-test
/home/vagrant/diffusion/diffusion role --init

# Run molecule
cd <role-name>
/home/vagrant/diffusion/diffusion molecule --role myrole --org myorg

# Check permissions
ls -la molecule/
```

## CI/CD Integration

This Vagrantfile can be used in CI/CD pipelines:

```yaml
# GitHub Actions example
- name: Run E2E tests
  run: |
    cd tests/e2e
    vagrant up --provider=virtualbox
    vagrant ssh -c "cd /home/vagrant/diffusion && go test -v"
```

## Notes

- Tests run as non-root user (vagrant) to verify Unix permissions
- Docker runs in the VM, not on host
- All files are synced from `../../` (project root)
- Tests are idempotent - can be run multiple times
