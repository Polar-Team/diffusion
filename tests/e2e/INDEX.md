# E2E Tests Documentation Index

Complete guide to Diffusion end-to-end testing on Ubuntu 22.04.

## 📚 Documentation Files

### Getting Started
- **[QUICKSTART.md](QUICKSTART.md)** - Start here! Quick commands and examples
- **[README.md](README.md)** - Complete documentation and guide

### Platform-Specific
- **[WINDOWS.md](WINDOWS.md)** - Windows setup, troubleshooting, and tips

### Reference
- **[FILES.md](FILES.md)** - Overview of all files and their purposes
- **[TROUBLESHOOTING.md](TROUBLESHOOTING.md)** - Common issues and solutions

## 🚀 Quick Start

### Linux/macOS
```bash
cd tests/e2e
./test.sh
```

### Windows
```powershell
cd tests\e2e
.\test.ps1
```

## 📖 Documentation Guide

### New Users
1. Read [QUICKSTART.md](QUICKSTART.md) first
2. Run tests using helper scripts
3. If issues, check [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

### Windows Users
1. Read [WINDOWS.md](WINDOWS.md) for setup
2. Use PowerShell script: `.\test.ps1`
3. Check Windows-specific troubleshooting

### Advanced Users
1. Read [README.md](README.md) for full details
2. Check [FILES.md](FILES.md) to understand structure
3. Modify `Vagrantfile` for custom tests

### Troubleshooting
1. Check [TROUBLESHOOTING.md](TROUBLESHOOTING.md) first
2. Try `vagrant destroy -f && vagrant up`
3. Use `vagrant up --debug` for logs

## 📋 What Each File Contains

| File | Best For | Contains |
|------|----------|----------|
| [QUICKSTART.md](QUICKSTART.md) | Quick reference | Common commands, examples |
| [README.md](README.md) | Complete guide | Full documentation, all details |
| [WINDOWS.md](WINDOWS.md) | Windows users | Windows setup, PowerShell, CI/CD |
| [FILES.md](FILES.md) | Understanding structure | File descriptions, architecture |
| [TROUBLESHOOTING.md](TROUBLESHOOTING.md) | Fixing issues | Common problems, solutions |
| INDEX.md | Navigation | This file - documentation map |

## 🎯 Common Tasks

### Run Tests
- **Quick:** [QUICKSTART.md](QUICKSTART.md#run-tests)
- **Detailed:** [README.md](README.md#running-tests)
- **Windows:** [WINDOWS.md](WINDOWS.md#running-tests)

### Fix Issues
- **Common problems:** [TROUBLESHOOTING.md](TROUBLESHOOTING.md#common-issues-and-solutions)
- **Windows issues:** [WINDOWS.md](WINDOWS.md#common-issues)
- **Debug:** [TROUBLESHOOTING.md](TROUBLESHOOTING.md#debugging-commands)

### Understand Structure
- **Files:** [FILES.md](FILES.md#file-descriptions)
- **Test flow:** [FILES.md](FILES.md#test-flow)
- **Architecture:** [README.md](README.md#test-environment)

### Customize Tests
- **Modify VM:** [FILES.md](FILES.md#maintenance)
- **Add tests:** [README.md](README.md#manual-testing)
- **CI/CD:** [README.md](README.md#cicd-integration)

## 🔍 Find Information

### By Topic

**Installation:**
- Prerequisites: [README.md](README.md#prerequisites)
- Windows setup: [WINDOWS.md](WINDOWS.md#prerequisites)

**Running Tests:**
- Quick commands: [QUICKSTART.md](QUICKSTART.md)
- Full guide: [README.md](README.md#running-tests)
- PowerShell: [WINDOWS.md](WINDOWS.md#using-powershell-script-recommended)

**Troubleshooting:**
- Common issues: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)
- Windows issues: [WINDOWS.md](WINDOWS.md#common-issues)
- Debug commands: [TROUBLESHOOTING.md](TROUBLESHOOTING.md#debugging-commands)

**Understanding:**
- What gets tested: [README.md](README.md#what-gets-tested)
- File structure: [FILES.md](FILES.md)
- Test flow: [FILES.md](FILES.md#test-flow)

**Advanced:**
- Manual testing: [README.md](README.md#manual-testing)
- CI/CD integration: [README.md](README.md#cicd-integration)
- Customization: [FILES.md](FILES.md#maintenance)

### By Platform

**Linux/macOS:**
- [QUICKSTART.md](QUICKSTART.md) - Commands
- [README.md](README.md) - Full guide
- `./test.sh` - Helper script

**Windows:**
- [WINDOWS.md](WINDOWS.md) - Complete Windows guide
- [QUICKSTART.md](QUICKSTART.md) - Quick commands
- `.\test.ps1` - PowerShell script

### By Experience Level

**Beginner:**
1. [QUICKSTART.md](QUICKSTART.md)
2. Run: `./test.sh` or `.\test.ps1`
3. If issues: [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

**Intermediate:**
1. [README.md](README.md)
2. [FILES.md](FILES.md)
3. Customize `Vagrantfile`

**Advanced:**
1. All documentation
2. Modify tests
3. CI/CD integration

## 🛠️ Helper Scripts

### test.sh (Linux/macOS)
```bash
./test.sh [--destroy] [--provision] [--ssh] [--help]
```
See: [QUICKSTART.md](QUICKSTART.md#run-tests)

### test.ps1 (Windows)
```powershell
.\test.ps1 [-Destroy] [-Provision] [-SSH] [-Help]
```
See: [WINDOWS.md](WINDOWS.md#using-powershell-script-recommended)

## 📞 Getting Help

1. **Check documentation:**
   - Start with [QUICKSTART.md](QUICKSTART.md)
   - Check [TROUBLESHOOTING.md](TROUBLESHOOTING.md)

2. **Try common fixes:**
   ```bash
   vagrant destroy -f
   vagrant up
   ```

3. **Get debug info:**
   ```bash
   vagrant up --debug > debug.log 2>&1
   ```

4. **Report issue:**
   - Include OS and versions
   - Attach debug logs
   - Describe steps to reproduce

## 🎓 Learning Path

### Day 1: Getting Started
1. Read [QUICKSTART.md](QUICKSTART.md)
2. Install prerequisites
3. Run first test: `./test.sh` or `.\test.ps1`

### Day 2: Understanding
1. Read [README.md](README.md)
2. Read [FILES.md](FILES.md)
3. SSH into VM: `vagrant ssh`
4. Explore: `ls -la /home/vagrant/diffusion`

### Day 3: Customizing
1. Modify `Vagrantfile`
2. Add custom tests
3. Try manual testing

### Day 4: Advanced
1. Set up CI/CD
2. Integrate with pipeline
3. Optimize performance

## 📊 Documentation Stats

- **Total files:** 8 documentation files
- **Total scripts:** 2 helper scripts (bash + PowerShell)
- **Total pages:** ~50 pages of documentation
- **Platforms covered:** Linux, macOS, Windows
- **Languages:** Bash, PowerShell, Ruby (Vagrantfile)

## ✅ Documentation Checklist

- [x] Quick start guide
- [x] Complete README
- [x] Windows-specific guide
- [x] Troubleshooting guide
- [x] File reference
- [x] Helper scripts (bash + PowerShell)
- [x] CI/CD examples
- [x] Manual testing guide
- [x] Index/navigation

## 🔗 External Resources

- [Vagrant Documentation](https://www.vagrantup.com/docs)
- [VirtualBox Documentation](https://www.virtualbox.org/wiki/Documentation)
- [VMware Vagrant Plugin](https://www.vagrantup.com/docs/providers/vmware)
- [Docker Documentation](https://docs.docker.com/)
- [Go Documentation](https://go.dev/doc/)

---

**Need help?** Start with [QUICKSTART.md](QUICKSTART.md) or [TROUBLESHOOTING.md](TROUBLESHOOTING.md)!
