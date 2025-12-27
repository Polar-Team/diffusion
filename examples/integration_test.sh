#!/bin/bash
# Integration test for dependency management system
# This script demonstrates the full workflow

set -e

echo "=== Diffusion Dependency Management Integration Test ==="
echo ""

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Create temporary test directory
TEST_DIR=$(mktemp -d)
echo -e "${YELLOW}Creating test directory: $TEST_DIR${NC}"
cd "$TEST_DIR"

# Initialize a test role
echo -e "${YELLOW}Step 1: Initializing test role...${NC}"
mkdir -p meta
cat > meta/main.yml <<EOF
galaxy_info:
  role_name: test_role
  author: Test Author
  description: Test role for dependency management
  license: MIT
  min_ansible_version: "2.10"
  platforms:
    - name: Ubuntu
      versions:
        - focal
        - jammy
  galaxy_tags:
    - test

collections:
  - community.general>=7.4.0
  - community.docker
EOF

# Create requirements.yml
mkdir -p scenarios/default
cat > scenarios/default/requirements.yml <<EOF
collections:
  - community.postgresql>=3.0.0
  - kubernetes.core

roles:
  - name: geerlingguy.docker
    version: 6.0.0
EOF

# Create diffusion.toml
cat > diffusion.toml <<EOF
[container_registry]
  registry_server = "ghcr.io"
  registry_provider = "Public"
  molecule_container_name = "polar-team/diffusion-molecule-container"
  molecule_container_tag = "latest-amd64"

[vault]
  enabled = false

[tests]
  type = "local"

[cache]
  enabled = true
EOF

echo -e "${GREEN}✓ Test role initialized${NC}"
echo ""

# Step 2: Initialize dependency configuration
echo -e "${YELLOW}Step 2: Initializing dependency configuration...${NC}"
diffusion deps init
echo -e "${GREEN}✓ Dependency configuration initialized${NC}"
echo ""

# Step 3: Add collections to config
echo -e "${YELLOW}Step 3: Adding collections to diffusion.toml...${NC}"
cat >> diffusion.toml <<EOF

[dependencies]
  ansible = ">=10.0.0"
  ansible_lint = ">=24.0.0"
  molecule = ">=24.0.0"
  yamllint = ">=1.35.0"

  [dependencies.python]
    min = "3.9"
    max = "3.13"
    default = "3.13.0"
    additional = []

  [[dependencies.collections]]
    name = "community.general"
    version = ">=7.4.0"

  [[dependencies.collections]]
    name = "community.docker"
    version = ">=3.4.0"
EOF
echo -e "${GREEN}✓ Collections added to configuration${NC}"
echo ""

# Step 4: Resolve dependencies
echo -e "${YELLOW}Step 4: Resolving dependencies...${NC}"
diffusion deps resolve
echo -e "${GREEN}✓ Dependencies resolved${NC}"
echo ""

# Step 5: Generate lock file
echo -e "${YELLOW}Step 5: Generating lock file...${NC}"
diffusion deps lock
if [ -f "diffusion.lock" ]; then
    echo -e "${GREEN}✓ Lock file generated${NC}"
    echo ""
    echo "Lock file contents:"
    head -20 diffusion.lock
else
    echo "ERROR: Lock file not generated"
    exit 1
fi
echo ""

# Step 6: Check lock file status
echo -e "${YELLOW}Step 6: Checking lock file status...${NC}"
if diffusion deps check; then
    echo -e "${GREEN}✓ Lock file is up-to-date${NC}"
else
    echo "ERROR: Lock file check failed"
    exit 1
fi
echo ""

# Step 7: Sync to container (dry run - create mock directory)
echo -e "${YELLOW}Step 7: Syncing to container project...${NC}"
CONTAINER_DIR="$TEST_DIR/container"
mkdir -p "$CONTAINER_DIR"
diffusion deps sync "$CONTAINER_DIR"
if [ -f "$CONTAINER_DIR/pyproject.toml" ]; then
    echo -e "${GREEN}✓ pyproject.toml generated${NC}"
    echo ""
    echo "pyproject.toml contents:"
    cat "$CONTAINER_DIR/pyproject.toml"
else
    echo "ERROR: pyproject.toml not generated"
    exit 1
fi
echo ""

# Step 8: Verify pyproject.toml contents
echo -e "${YELLOW}Step 8: Verifying pyproject.toml contents...${NC}"
PYPROJECT="$CONTAINER_DIR/pyproject.toml"

# Check for required dependencies
REQUIRED_DEPS=(
    "ansible"
    "molecule"
    "ansible-lint"
    "yamllint"
    "psycopg2-binary"  # From community.postgresql
    "docker"           # From community.docker
    "kubernetes"       # From kubernetes.core
)

ALL_FOUND=true
for dep in "${REQUIRED_DEPS[@]}"; do
    if grep -q "$dep" "$PYPROJECT"; then
        echo -e "${GREEN}✓ Found: $dep${NC}"
    else
        echo -e "ERROR: Missing: $dep"
        ALL_FOUND=false
    fi
done

if [ "$ALL_FOUND" = true ]; then
    echo -e "${GREEN}✓ All required dependencies found${NC}"
else
    echo "ERROR: Some dependencies missing"
    exit 1
fi
echo ""

# Step 9: Test lock file modification detection
echo -e "${YELLOW}Step 9: Testing lock file modification detection...${NC}"
# Modify a collection version
sed -i 's/community.general>=7.4.0/community.general>=8.0.0/' diffusion.toml
echo "Modified collection version in diffusion.toml"

# Check should now fail
if diffusion deps check 2>/dev/null; then
    echo "ERROR: Lock file check should have failed"
    exit 1
else
    echo -e "${GREEN}✓ Lock file correctly detected as out-of-date${NC}"
fi

# Update lock file
diffusion deps lock
echo -e "${GREEN}✓ Lock file updated${NC}"

# Check should now pass
if diffusion deps check; then
    echo -e "${GREEN}✓ Lock file is now up-to-date${NC}"
else
    echo "ERROR: Lock file check failed after update"
    exit 1
fi
echo ""

# Step 10: Verify hash changes
echo -e "${YELLOW}Step 10: Verifying hash changes...${NC}"
HASH1=$(grep "^hash:" diffusion.lock | cut -d' ' -f2)
echo "Current hash: $HASH1"

# Modify again
sed -i 's/community.general>=8.0.0/community.general>=7.4.0/' diffusion.toml
diffusion deps lock

HASH2=$(grep "^hash:" diffusion.lock | cut -d' ' -f2)
echo "New hash: $HASH2"

if [ "$HASH1" != "$HASH2" ]; then
    echo -e "${GREEN}✓ Hash changed correctly${NC}"
else
    echo "ERROR: Hash should have changed"
    exit 1
fi
echo ""

# Cleanup
echo -e "${YELLOW}Cleaning up test directory...${NC}"
cd /
rm -rf "$TEST_DIR"
echo -e "${GREEN}✓ Cleanup complete${NC}"
echo ""

echo -e "${GREEN}=== All Integration Tests Passed! ===${NC}"
echo ""
echo "Summary:"
echo "  ✓ Role initialization"
echo "  ✓ Dependency configuration"
echo "  ✓ Dependency resolution"
echo "  ✓ Lock file generation"
echo "  ✓ Lock file validation"
echo "  ✓ pyproject.toml generation"
echo "  ✓ Dependency verification"
echo "  ✓ Modification detection"
echo "  ✓ Hash computation"
echo ""
echo "The dependency management system is working correctly!"
