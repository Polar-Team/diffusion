# E2E Tests on Windows

## Prerequisites

1. **Install Vagrant**
   - Download from https://www.vagrantup.com/downloads
   - Add to PATH during installation

2. **Install a Provider**
   - **VMware Workstation** (recommended)
     - Download from https://www.vmware.com/products/workstation-pro.html
     - Install Vagrant VMware plugin: `vagrant plugin install vagrant-vmware-desktop`
   - **VirtualBox** (free alternative)
     - Download from https://www.virtualbox.org/

3. **Enable Hyper-V** (if using VMware)
   - Open PowerShell as Administrator
   - Run: `Enable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V -All`
   - Restart computer

## Running Tests

### Using PowerShell Script (Recommended)

Open PowerShell in the `tests/e2e` directory:

```powershell
# Run tests
.\test.ps1

# Run tests and destroy VM after
.\test.ps1 -Destroy

# Run tests and open SSH session
.\test.ps1 -SSH

# Re-run tests (VM already exists)
.\test.ps1 -Provision

# Show help
.\test.ps1 -Help
```

### Using Vagrant Directly

```powershell
# Start VM and run tests
vagrant up

# Re-run tests
vagrant provision

# SSH into VM
vagrant ssh

# Check status
vagrant status

# Destroy VM
vagrant destroy -f
```

## Common Issues

### Execution Policy Error

If you get "cannot be loaded because running scripts is disabled":

```powershell
# Run as Administrator
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

### Vagrant Not Found

Add Vagrant to PATH:
1. Search for "Environment Variables" in Windows
2. Edit "Path" in User variables
3. Add: `C:\HashiCorp\Vagrant\bin`
4. Restart PowerShell

### Provider Not Found

**For VMware:**
```powershell
vagrant plugin install vagrant-vmware-desktop
```

**For VirtualBox:**
- Ensure VirtualBox is installed
- Restart computer after installation

### Hyper-V Conflicts

If using VMware and getting Hyper-V errors:

```powershell
# Disable Hyper-V (run as Administrator)
Disable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V-All

# Or use VirtualBox instead
vagrant up --provider=virtualbox
```

### SSH Issues

If `vagrant ssh` doesn't work:

1. **Install OpenSSH Client** (Windows 10/11):
   - Settings → Apps → Optional Features
   - Add "OpenSSH Client"

2. **Or use PuTTY**:
   ```powershell
   vagrant ssh-config
   # Use the output to configure PuTTY
   ```

## Expected Output

```
==========================================
Diffusion E2E Test Runner
==========================================

Found: Vagrant 2.x.x
Starting VM and running tests...

Bringing machine 'default' up with 'vmware_desktop' provider...
==> default: Cloning VMware VM: 'generic/ubuntu2204'
...
==> default: Running provisioner: shell...
    default: ==========================================
    default: Installing system dependencies...
    default: ==========================================
...
    default: ==========================================
    default: All E2E tests passed! ✓
    default: ==========================================

==========================================
✓ All tests passed!
==========================================

Done!

Useful commands:
  vagrant ssh              - SSH into the VM
  vagrant provision        - Re-run tests
  vagrant destroy -f       - Destroy the VM
  vagrant status           - Check VM status
```

## Performance Tips

1. **Use VMware** - Faster than VirtualBox on Windows
2. **Allocate more RAM** - Edit Vagrantfile to increase memory
3. **Use SSD** - Store VMs on SSD for better performance
4. **Disable antivirus** - Temporarily for Vagrant directory

## Troubleshooting Commands

```powershell
# Check Vagrant version
vagrant --version

# Check installed plugins
vagrant plugin list

# Check VM status
vagrant status

# View VM logs
vagrant up --debug

# Reload VM
vagrant reload

# Completely reset
vagrant destroy -f
vagrant up
```

## Integration with Windows Terminal

Add to Windows Terminal settings:

```json
{
    "name": "Diffusion E2E Tests",
    "commandline": "powershell.exe -NoExit -Command \"cd C:\\path\\to\\diffusion\\tests\\e2e; .\\test.ps1\"",
    "icon": "🧪"
}
```

## CI/CD on Windows

### GitHub Actions

```yaml
name: E2E Tests (Windows)

on: [push, pull_request]

jobs:
  test:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Install Vagrant
        run: choco install vagrant
      
      - name: Install VirtualBox
        run: choco install virtualbox
      
      - name: Run E2E Tests
        run: |
          cd tests/e2e
          .\test.ps1 -Destroy
```

### Azure Pipelines

```yaml
trigger:
  - main

pool:
  vmImage: 'windows-latest'

steps:
  - task: PowerShell@2
    inputs:
      targetType: 'inline'
      script: |
        choco install vagrant virtualbox -y
        cd tests/e2e
        .\test.ps1 -Destroy
```

## Notes

- Tests run in Ubuntu VM, not Windows directly
- This ensures Unix permission testing works correctly
- VM is isolated from Windows host
- All Docker operations happen inside the VM
- Files are synced between Windows and VM
