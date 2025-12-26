#!/bin/bash
# Test script to verify Python version compatibility checking

echo "=== Testing Python Version Compatibility System ==="
echo ""

# Build the binary
echo "Building diffusion..."
cd ..
go build -o diffusion.exe .
if [ $? -ne 0 ]; then
    echo "❌ Build failed"
    exit 1
fi
echo "✅ Build successful"
echo ""

# Test 1: Create a test role with Python 3.9
echo "Test 1: Creating test role with Python 3.9 configuration"
echo "-------------------------------------------------------"
mkdir -p test-role-py39
cd test-role-py39

cat > diffusion.toml << 'EOF'
[role]
name = "test-role"
namespace = "test"

[dependency]
[dependency.python]
min = "3.9"
max = "3.13"
default = "3.9"

ansible = ">=13.0.0"
molecule = ">=25.0.0"
ansible_lint = ">=24.0.0"
yamllint = ">=1.35.0"

[[dependency.collections]]
name = "community.general"
version = "latest"
EOF

echo "✅ Created diffusion.toml with Python 3.9 and latest tools"
echo ""

# Initialize dependencies
echo "Running: diffusion deps init"
../diffusion.exe deps init
echo ""

# Resolve dependencies (should show warnings)
echo "Running: diffusion deps resolve"
echo "Expected: Warnings about tool version adjustments for Python 3.9"
../diffusion.exe deps resolve
echo ""

# Check lock file
if [ -f "diffusion.lock" ]; then
    echo "✅ Lock file created"
    echo "Lock file contents:"
    cat diffusion.lock
else
    echo "❌ Lock file not created"
fi

# Cleanup
cd ..
rm -rf test-role-py39

echo ""
echo "=== Test completed ==="
