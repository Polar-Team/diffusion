# E2E Tests Quick Start

## List Available OS Options

### Linux/macOS:
```bash
cd tests/e2e
./test.sh --list-os
```

### Windows (PowerShell):
```powershell
cd tests/e2e
.\test.ps1 -ListOS
```

## Run Tests

### Linux/macOS:
```bash
cd tests/e2e
./test.sh                    # Default: Ubuntu 22.04
./test.sh --os ubuntu2404    # Ubuntu 24.04
./test.sh --os debian12      # Debian 12
```

### Windows (PowerShell):
```powershell
cd tests/e2e
.\test.ps1                   # Default: Ubuntu 22.04
.\test.ps1 -OS ubuntu2404    # Ubuntu 24.04
.\test.ps1 -OS debian12      # Debian 12
```

## Run Tests and Destroy VM

### Linux/macOS:
```bash
./test.sh --destroy
./test.sh --os ubuntu2404 --destroy
```

### Windows (PowerShell):
```powershell
.\test.ps1 -Destroy
.\test.ps1 -OS ubuntu2404 -Destroy
```

## Run Tests and SSH

### Linux/macOS:
```bash
./test.sh --ssh
```

### Windows (PowerShell):
```powershell
.\test.ps1 -SSH
```

## Re-run Tests (VM already exists)

### Linux/macOS:
```bash
./test.sh --provision
```

### Windows (PowerShell):
```powershell
.\test.ps1 -Provision
```

## Show Help

### Linux/macOS:
```bash
./test.sh --help
```

### Windows (PowerShell):
```powershell
.\test.ps1 -Help
```

## Manual Commands

```bash
# Start VM
vagrant up

# SSH into VM
vagrant ssh

# Re-run tests
vagrant provision

# Destroy VM
vagrant destroy -f

# Check VM status
vagrant status
```

## What Gets Tested

✓ Build on Ubuntu 22.04  
✓ Unit tests pass  
✓ Role initialization works (using expect)  
✓ Config creation works (using expect)  
✓ Unix permissions correct  
✓ Cache feature works  
✓ Artifact management works  

**Note**: Tests use `expect` for reliable CLI automation instead of EOF heredocs.  

## Expected Output

```
===========================================
All E2E tests passed! ✓
===========================================

Summary:
  - Docker version: Docker version 24.x.x
  - Go version: go version go1.21.5 linux/amd64
  - Diffusion built successfully
  - All unit tests passed
  - All E2E tests passed
  - Permissions verified on Unix system
```

## Troubleshooting

**VM won't start:**
```bash
vagrant destroy -f && vagrant up
```

**Tests fail:**
```bash
vagrant ssh
cd /home/vagrant/diffusion
go test -v
```

**Check permissions:**
```bash
vagrant ssh
ls -la /home/vagrant/test-role/molecule/
```
