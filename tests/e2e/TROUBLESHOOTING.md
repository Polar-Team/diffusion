# E2E Tests Troubleshooting Guide

## Common Issues and Solutions

### 1. Vagrant Not Found

**Symptoms:**
```
vagrant: command not found
```

**Solution:**

**Linux/macOS:**
```bash
# Install Vagrant
# macOS
brew install vagrant

# Ubuntu/Debian
wget https://releases.hashicorp.com/vagrant/2.4.0/vagrant_2.4.0-1_amd64.deb
sudo dpkg -i vagrant_2.4.0-1_amd64.deb
```

**Windows:**
1. Download from https://www.vagrantup.com/downloads
2. Run installer
3. Restart PowerShell
4. Verify: `vagrant --version`

---

### 2. Provider Not Found

**Symptoms:**
```
No usable default provider could be found for your system.
```

**Solution:**

**Install VirtualBox (easiest):**
```bash
# macOS
brew install virtualbox

# Ubuntu/Debian
sudo apt-get install virtualbox

# Windows
choco install virtualbox
```

**Or install VMware:**
```bash
vagrant plugin install vagrant-vmware-desktop
```

---

### 3. VM Won't Start

**Symptoms:**
```
Timed out while waiting for the machine to boot
```

**Solution:**

```bash
# Destroy and recreate
vagrant destroy -f
vagrant up

# Or try with more verbose output
vagrant up --debug
```

**Check virtualization:**
```bash
# Linux
egrep -c '(vmx|svm)' /proc/cpuinfo
# Should return > 0

# Windows (PowerShell as Admin)
Get-ComputerInfo | Select-Object HyperVisorPresent
```

---

### 4. Tests Fail

**Symptoms:**
```
✗ Tests failed!
```

**Solution:**

**SSH into VM and debug:**
```bash
vagrant ssh

# Check Docker
docker --version
docker ps

# Check Go
go version

# Try building manually
cd /home/vagrant/diffusion
go build -o diffusion

# Run tests manually
go test -v

# Check permissions
ls -la /home/vagrant/test-role/molecule/
```

---

### 5. Permission Errors

**Symptoms:**
```
Permission denied
```

**Solution:**

**Check if running as vagrant user:**
```bash
vagrant ssh
whoami  # Should be 'vagrant'
id      # Check UID/GID
```

**Check Docker group:**
```bash
groups  # Should include 'docker'

# If not, add user to docker group
sudo usermod -aG docker vagrant
# Logout and login again
```

**Check file ownership:**
```bash
ls -la /home/vagrant/test-role/
# All files should be owned by vagrant:vagrant
```

---

### 6. Docker Not Working

**Symptoms:**
```
Cannot connect to the Docker daemon
```

**Solution:**

```bash
vagrant ssh

# Check Docker status
sudo systemctl status docker

# Start Docker if stopped
sudo systemctl start docker

# Check if user is in docker group
groups | grep docker

# Test Docker
docker run hello-world
```

---

### 7. Out of Disk Space

**Symptoms:**
```
No space left on device
```

**Solution:**

```bash
# Check disk usage
vagrant ssh
df -h

# Clean up Docker
docker system prune -a -f

# Or destroy and recreate VM
exit
vagrant destroy -f
vagrant up
```

---

### 8. Network Issues

**Symptoms:**
```
Could not resolve host
Failed to download
```

**Solution:**

```bash
vagrant ssh

# Check internet connectivity
ping -c 3 google.com

# Check DNS
cat /etc/resolv.conf

# Try updating packages
sudo apt-get update
```

**Or restart VM:**
```bash
exit
vagrant reload
```

---

### 9. Slow Performance

**Symptoms:**
- VM is very slow
- Tests take too long

**Solution:**

**Increase VM resources:**

Edit `Vagrantfile`:
```ruby
vmware.vmx["memsize"] = "8192"  # 8GB RAM
vmware.vmx["numvcpus"] = "4"    # 4 CPUs
```

**Use VMware instead of VirtualBox:**
```bash
vagrant plugin install vagrant-vmware-desktop
vagrant up --provider=vmware_desktop
```

**Use SSD:**
- Move Vagrant VMs to SSD
- Set `VAGRANT_HOME` to SSD location

---

### 10. Windows-Specific Issues

#### Execution Policy Error

**Symptoms:**
```
.\test.ps1 cannot be loaded because running scripts is disabled
```

**Solution:**
```powershell
# Run as Administrator
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

#### Hyper-V Conflicts

**Symptoms:**
```
VMware and Hyper-V are not compatible
```

**Solution:**

**Option 1: Disable Hyper-V**
```powershell
# Run as Administrator
Disable-WindowsOptionalFeature -Online -FeatureName Microsoft-Hyper-V-All
# Restart computer
```

**Option 2: Use VirtualBox**
```powershell
vagrant up --provider=virtualbox
```

#### SSH Not Working

**Symptoms:**
```
ssh: command not found
```

**Solution:**

**Install OpenSSH Client:**
1. Settings → Apps → Optional Features
2. Add "OpenSSH Client"
3. Restart PowerShell

**Or use PuTTY:**
```powershell
vagrant ssh-config
# Use output to configure PuTTY
```

---

### 11. Build Fails

**Symptoms:**
```
go build failed
```

**Solution:**

```bash
vagrant ssh
cd /home/vagrant/diffusion

# Check Go version
go version

# Clean and rebuild
go clean
go build -v -o diffusion

# Check for errors
go test -v
```

---

### 12. Container Issues

**Symptoms:**
```
Error response from daemon
```

**Solution:**

```bash
vagrant ssh

# Check Docker daemon
sudo systemctl status docker

# Check Docker logs
sudo journalctl -u docker -n 50

# Restart Docker
sudo systemctl restart docker

# Test Docker
docker run hello-world
```

---

## Debugging Commands

### Check VM Status
```bash
vagrant status
vagrant global-status
```

### View Logs
```bash
vagrant up --debug > vagrant.log 2>&1
```

### SSH and Investigate
```bash
vagrant ssh
# Now you're in the VM
cd /home/vagrant/diffusion
ls -la
```

### Reload VM
```bash
vagrant reload
```

### Completely Reset
```bash
vagrant destroy -f
rm -rf .vagrant
vagrant up
```

### Check Vagrant Version
```bash
vagrant --version
```

### Check Plugins
```bash
vagrant plugin list
```

---

## Getting Help

### Collect Information

```bash
# System info
uname -a
vagrant --version
docker --version
go version

# VM status
vagrant status

# Logs
vagrant up --debug > debug.log 2>&1
```

### Report Issue

Include:
1. Operating system and version
2. Vagrant version
3. Provider (VMware/VirtualBox)
4. Error message
5. Debug logs
6. Steps to reproduce

---

## Prevention

### Before Running Tests

1. **Check disk space:**
   ```bash
   df -h
   ```

2. **Check Vagrant is working:**
   ```bash
   vagrant --version
   ```

3. **Check provider is installed:**
   ```bash
   # VirtualBox
   VBoxManage --version
   
   # VMware
   vagrant plugin list | grep vmware
   ```

4. **Update Vagrant:**
   ```bash
   # Check for updates
   vagrant version
   ```

### Regular Maintenance

```bash
# Clean up old VMs
vagrant global-status --prune
vagrant destroy -f

# Update Vagrant plugins
vagrant plugin update

# Clean Docker in VM
vagrant ssh -c "docker system prune -a -f"
```

---

## Still Having Issues?

1. Check [README.md](README.md) for detailed documentation
2. Check [WINDOWS.md](WINDOWS.md) for Windows-specific help
3. Try the [QUICKSTART.md](QUICKSTART.md) guide
4. Search Vagrant documentation: https://www.vagrantup.com/docs
5. Open an issue with debug logs
