# Dependency Management Examples

This directory contains examples and tests for the Diffusion dependency management system.

## Files

### diffusion-with-deps.toml

Complete example of `diffusion.toml` with dependency management configuration.

**Usage:**
```bash
# Copy to your project
cp diffusion-with-deps.toml /path/to/your/role/diffusion.toml

# Edit as needed
vim /path/to/your/role/diffusion.toml

# Generate lock file
cd /path/to/your/role
diffusion deps lock
```

**Features demonstrated:**
- Python version configuration
- Tool version management (ansible, molecule, etc.)
- Collection dependencies with versions
- Comments and best practices

### integration_test.sh

Comprehensive integration test that demonstrates the full dependency management workflow.

**Usage:**
```bash
# Make executable
chmod +x integration_test.sh

# Run test
./integration_test.sh
```

**What it tests:**
1. Role initialization
2. Dependency configuration
3. Dependency resolution
4. Lock file generation
5. Lock file validation
6. pyproject.toml generation
7. Dependency verification
8. Modification detection
9. Hash computation

**Expected output:**
```
=== Diffusion Dependency Management Integration Test ===

Step 1: Initializing test role...
✓ Test role initialized

Step 2: Initializing dependency configuration...
✓ Dependency configuration initialized

...

=== All Integration Tests Passed! ===

Summary:
  ✓ Role initialization
  ✓ Dependency configuration
  ✓ Dependency resolution
  ✓ Lock file generation
  ✓ Lock file validation
  ✓ pyproject.toml generation
  ✓ Dependency verification
  ✓ Modification detection
  ✓ Hash computation

The dependency management system is working correctly!
```

## Example Workflows

### Example 1: PostgreSQL Role

```toml
# diffusion.toml
[dependencies]
  ansible = ">=10.0.0"
  ansible_lint = ">=24.0.0"
  molecule = ">=24.0.0"
  yamllint = ">=1.35.0"

  [dependencies.python]
    min = "3.9"
    max = "3.13"
    default = "3.13.0"

  [[dependencies.collections]]
    name = "community.postgresql"
    version = ">=3.0.0"
```

**Result:**
- Automatically includes `psycopg2-binary>=2.9.0` in pyproject.toml
- Python 3.13.0 as default interpreter
- Ansible 10.0.0+ and Molecule 24.0.0+

### Example 2: Kubernetes Role

```toml
# diffusion.toml
[dependencies]
  ansible = ">=10.0.0"
  ansible_lint = ">=24.0.0"
  molecule = ">=24.0.0"
  yamllint = ">=1.35.0"

  [dependencies.python]
    min = "3.9"
    max = "3.13"
    default = "3.13.0"

  [[dependencies.collections]]
    name = "kubernetes.core"
    version = ">=2.4.0"

  [[dependencies.collections]]
    name = "community.docker"
    version = ">=3.4.0"
```

**Result:**
- Includes `kubernetes>=25.0.0`, `PyYAML>=6.0`, `docker>=6.0.0`
- Supports both Kubernetes and Docker testing

### Example 3: Multi-Cloud Role

```toml
# diffusion.toml
[dependencies]
  ansible = ">=10.0.0"
  ansible_lint = ">=24.0.0"
  molecule = ">=24.0.0"
  yamllint = ">=1.35.0"

  [dependencies.python]
    min = "3.9"
    max = "3.13"
    default = "3.13.0"

  [[dependencies.collections]]
    name = "amazon.aws"
    version = ">=6.0.0"

  [[dependencies.collections]]
    name = "google.cloud"
    version = ">=1.0.0"

  [[dependencies.collections]]
    name = "azure.azcollection"
    version = ">=1.0.0"
```

**Result:**
- Includes `boto3`, `botocore` for AWS
- Includes `google-auth` for GCP
- Includes `azure-cli-core` for Azure
- Multi-cloud testing support

### Example 4: Multiple Python Versions

```toml
# diffusion.toml
[dependencies]
  ansible = ">=10.0.0"
  ansible_lint = ">=24.0.0"
  molecule = ">=24.0.0"
  yamllint = ">=1.35.0"

  [dependencies.python]
    min = "3.9"
    max = "3.13"
    default = "3.13.0"
    additional = ["3.10.0", "3.11.0", "3.12.0"]

  [[dependencies.collections]]
    name = "community.general"
    version = ">=7.4.0"
```

**Result:**
- Installs Python 3.10, 3.11, 3.12, and 3.13
- Tests can run against multiple Python versions
- Ensures compatibility across Python versions

## Testing Your Configuration

### Step 1: Validate Configuration

```bash
# Check syntax
cat diffusion.toml

# Initialize if needed
diffusion deps init
```

### Step 2: Resolve Dependencies

```bash
# See what will be installed
diffusion deps resolve
```

### Step 3: Generate Lock File

```bash
# Create lock file
diffusion deps lock

# Verify it was created
cat diffusion.lock
```

### Step 4: Sync to Container

```bash
# Generate pyproject.toml
diffusion deps sync

# Verify it was created
cat ../diffusion-molecule-container/pyproject.toml
```

### Step 5: Test Container Build

```bash
# Build container with new dependencies
cd ../diffusion-molecule-container
docker build -t test-molecule .

# Test it works
docker run --rm test-molecule molecule --version
```

## Common Patterns

### Pattern 1: Minimal Configuration

```toml
[dependencies]
  # Use defaults for tools
  ansible = ">=10.0.0"
  ansible_lint = ">=24.0.0"
  molecule = ">=24.0.0"
  yamllint = ">=1.35.0"

  [dependencies.python]
    min = "3.9"
    max = "3.13"
    default = "3.13.0"
```

### Pattern 2: Pinned Versions

```toml
[dependencies]
  ansible = "==10.5.0"
  ansible_lint = "==24.2.0"
  molecule = "==24.1.0"
  yamllint = "==1.35.1"

  [dependencies.python]
    min = "3.11"
    max = "3.11"
    default = "3.11.8"
```

### Pattern 3: Version Ranges

```toml
[dependencies]
  ansible = ">=10.0.0,<11.0.0"
  ansible_lint = ">=24.0.0,<25.0.0"
  molecule = ">=24.0.0"
  yamllint = ">=1.35.0"

  [dependencies.python]
    min = "3.9"
    max = "3.13"
    default = "3.13.0"
```

## Troubleshooting Examples

### Issue: Lock file out of date

```bash
# Check what changed
diffusion deps resolve

# Update lock file
diffusion deps lock

# Verify
diffusion deps check
```

### Issue: Missing Python dependency

```bash
# Check resolved dependencies
diffusion deps resolve

# Check generated pyproject.toml
cat ../diffusion-molecule-container/pyproject.toml

# Add missing dependency manually if needed
vim ../diffusion-molecule-container/pyproject.toml
```

### Issue: Container build fails

```bash
# Check Python version compatibility
diffusion deps resolve

# Verify pyproject.toml syntax
python -m toml ../diffusion-molecule-container/pyproject.toml

# Test build with verbose output
cd ../diffusion-molecule-container
docker build --progress=plain .
```

## CI/CD Examples

### GitHub Actions

```yaml
name: Test

on: [push, pull_request]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      
      - name: Install diffusion
        run: |
          curl -L https://github.com/your-org/diffusion/releases/latest/download/diffusion-linux-amd64 -o /usr/local/bin/diffusion
          chmod +x /usr/local/bin/diffusion
      
      - name: Check dependencies
        run: diffusion deps check
      
      - name: Run tests
        run: diffusion mol --verify
```

### GitLab CI

```yaml
test:
  image: ubuntu:latest
  before_script:
    - apt-get update && apt-get install -y curl
    - curl -L https://github.com/your-org/diffusion/releases/latest/download/diffusion-linux-amd64 -o /usr/local/bin/diffusion
    - chmod +x /usr/local/bin/diffusion
  script:
    - diffusion deps check
    - diffusion mol --verify
```

### Jenkins

```groovy
pipeline {
    agent any
    stages {
        stage('Check Dependencies') {
            steps {
                sh 'diffusion deps check'
            }
        }
        stage('Test') {
            steps {
                sh 'diffusion mol --verify'
            }
        }
    }
}
```

## Best Practices

1. **Always commit diffusion.lock**: Ensures reproducible builds
2. **Use version constraints**: Prefer `>=` over exact versions for flexibility
3. **Test locally first**: Run `diffusion deps resolve` before committing
4. **Review changes**: Check what changed in lock file before committing
5. **Sync after updates**: Always run `diffusion deps sync` after changes
6. **Run checks in CI**: Add `diffusion deps check` to your CI pipeline
7. **Document custom deps**: Add comments in diffusion.toml for custom dependencies

## Resources

- [Quick Start Guide](../docs/QUICKSTART_DEPENDENCY_MANAGEMENT.md)
- [Full Documentation](../docs/dependency-management.md)
- [Technical Guide](../docs/DEPENDENCY_MANAGEMENT_FEATURE.md)
- [Changelog](../docs/CHANGELOG_DEPENDENCY_MANAGEMENT.md)

## Support

- GitHub Issues: https://github.com/your-org/diffusion/issues
- Documentation: https://github.com/your-org/diffusion/docs
- Examples: https://github.com/your-org/diffusion/examples
