# Building Diffusion

This guide explains how to build Diffusion from source for different platforms.

## Prerequisites

- **Go 1.21+**: [Download Go](https://golang.org/dl/)
- **Make**: Build automation tool
  - Linux/macOS: Usually pre-installed
  - Windows: Install via [Chocolatey](https://chocolatey.org/) (`choco install make`) or use Git Bash

## Quick Start

### Build for Current Platform

```bash
make build
```

This builds a binary for your current OS and architecture in the `bin/` directory.

### Build for All Platforms

```bash
make dist
```

This creates binaries for all supported platforms:
- Linux: AMD64, ARM64, ARM
- macOS: AMD64 (Intel), ARM64 (Apple Silicon)
- Windows: AMD64, ARM64, ARM

## Available Commands

### Main Targets

| Command | Description |
|---------|-------------|
| `make build` | Build for current platform |
| `make dist` | Build for all platforms |
| `make linux` | Build for all Linux architectures |
| `make darwin` | Build for all macOS architectures |
| `make windows` | Build for all Windows architectures |
| `make test` | Run unit tests |
| `make clean` | Remove build artifacts |
| `make help` | Show all available commands |

### Platform-Specific Targets

#### Linux
```bash
make linux-amd64    # Ubuntu, Debian, RHEL, AlmaLinux (x86_64)
make linux-arm64    # ARM64 servers, Raspberry Pi 4+
make linux-arm      # Raspberry Pi 3, older ARM devices
```

#### macOS
```bash
make darwin-amd64   # Intel Macs
make darwin-arm64   # Apple Silicon (M1, M2, M3)
```

#### Windows
```bash
make windows-amd64  # Standard Windows PCs
make windows-arm64  # Windows on ARM (Surface Pro X, etc.)
make windows-arm    # Older Windows ARM devices
```

## Output Files

All binaries are placed in the `bin/` directory with descriptive names:

```
bin/
├── diffusion-linux-amd64
├── diffusion-linux-arm64
├── diffusion-linux-arm
├── diffusion-darwin-amd64
├── diffusion-darwin-arm64
├── diffusion-windows-amd64.exe
├── diffusion-windows-arm64.exe
└── diffusion-windows-arm.exe
```

## Build with Version

You can specify a version during build:

```bash
make VERSION=1.0.0 dist
```

If not specified, the version is automatically detected from:
1. Git tags (`git describe --tags`)
2. Falls back to `"dev"` if no tags exist

## Examples

### Build for Linux Servers
```bash
# Build for all Linux architectures
make linux

# Or build specific architecture
make linux-amd64
```

### Build for macOS
```bash
# Build for both Intel and Apple Silicon
make darwin

# Or build specific architecture
make darwin-arm64  # For M1/M2/M3 Macs
```

### Build for Windows
```bash
# Build for all Windows architectures
make windows

# Or build specific architecture
make windows-amd64
```

### Build Everything
```bash
# Clean previous builds and build for all platforms
make clean dist
```

### Run Tests Before Building
```bash
make test && make dist
```

## Cross-Compilation

Go supports cross-compilation out of the box. You can build for any platform from any platform:

- Build Windows binaries on Linux/macOS
- Build Linux binaries on Windows/macOS
- Build macOS binaries on Linux/Windows

Example from Windows:
```bash
make linux-amd64    # Creates Linux binary on Windows
```

## Build Flags

The Makefile uses optimized build flags:

- `-s`: Strip symbol table
- `-w`: Strip DWARF debugging information
- `-X main.Version`: Inject version information

These flags reduce binary size significantly.

## Troubleshooting

### Make Command Not Found (Windows)

**Option 1: Install Make**
```powershell
choco install make
```

**Option 2: Use Git Bash**
Git for Windows includes Make. Run commands in Git Bash instead of PowerShell.

**Option 3: Use Go Directly**
```bash
go build -o bin/diffusion.exe .
```

### Permission Denied (Linux/macOS)

Make the binary executable:
```bash
chmod +x bin/diffusion-linux-amd64
```

### Build Fails

1. **Check Go version:**
   ```bash
   go version  # Should be 1.21 or higher
   ```

2. **Update dependencies:**
   ```bash
   go mod download
   go mod tidy
   ```

3. **Clean and rebuild:**
   ```bash
   make clean
   make build
   ```

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Build

on: [push, pull_request]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.21'
      - name: Build
        run: make dist
      - name: Upload artifacts
        uses: actions/upload-artifact@v3
        with:
          name: binaries
          path: bin/
```

### GitLab CI Example

```yaml
build:
  image: golang:1.21
  script:
    - make dist
  artifacts:
    paths:
      - bin/
```

## Manual Build (Without Make)

If you can't use Make, build manually:

### Linux AMD64
```bash
GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o bin/diffusion-linux-amd64 .
```

### macOS ARM64
```bash
GOOS=darwin GOARCH=arm64 go build -ldflags "-s -w" -o bin/diffusion-darwin-arm64 .
```

### Windows AMD64
```bash
GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o bin/diffusion-windows-amd64.exe .
```

## See Also

- [Main README](../README.md)
- [Development Guide](../CONTRIBUTING.md) (if exists)
- [Go Cross Compilation](https://golang.org/doc/install/source#environment)

---

<div align="center">
Made with ❤️ by Polar-Team
</div>
