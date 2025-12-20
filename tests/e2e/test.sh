#!/bin/bash
# E2E Test Runner for Diffusion

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Available OS options
declare -A OS_OPTIONS=(
    ["ubuntu2204"]="Ubuntu 22.04 LTS"
    ["ubuntu2304"]="Ubuntu 23.04"
    ["ubuntu2404"]="Ubuntu 24.04 LTS"
    ["debian12"]="Debian 12 (Bookworm)"
    ["debian13"]="Debian 13 (Trixie)"
    ["windows11"]="Windows 11"
    ["windows10"]="Windows 10"
    ["macos15"]="macOS 15 Sequoia"
    ["macos14"]="macOS 14 Sonoma"
)

echo -e "${GREEN}=========================================="
echo "Diffusion E2E Test Runner"
echo -e "==========================================${NC}"

# Check if Vagrant is installed
if ! command -v vagrant &> /dev/null; then
    echo -e "${RED}Error: Vagrant is not installed${NC}"
    echo "Please install Vagrant from https://www.vagrantup.com/downloads"
    exit 1
fi

# Parse arguments
DESTROY=false
PROVISION=false
SSH=false
BOX_NAME="ubuntu2204"
LIST_OS=false

while [[ $# -gt 0 ]]; do
    case $1 in
        --destroy)
            DESTROY=true
            shift
            ;;
        --provision)
            PROVISION=true
            shift
            ;;
        --ssh)
            SSH=true
            shift
            ;;
        --os)
            BOX_NAME="$2"
            shift 2
            ;;
        --list-os)
            LIST_OS=true
            shift
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --destroy     Destroy VM after tests"
            echo "  --provision   Re-run provisioning only"
            echo "  --ssh         SSH into VM after tests"
            echo "  --os <name>   Select OS to test (default: ubuntu2204)"
            echo "  --list-os     List available OS options"
            echo "  --help        Show this help message"
            echo ""
            echo "Examples:"
            echo "  $0                           # Run tests on Ubuntu 22.04"
            echo "  $0 --os ubuntu2404           # Run tests on Ubuntu 24.04"
            echo "  $0 --os debian12 --destroy   # Test Debian 12 and cleanup"
            echo "  $0 --list-os                 # Show available OS options"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

# List OS options if requested
if [ "$LIST_OS" = true ]; then
    echo ""
    echo -e "${CYAN}Available OS options:${NC}"
    for key in "${!OS_OPTIONS[@]}"; do
        echo -e "  ${GREEN}$key${NC} - ${OS_OPTIONS[$key]}"
    done | sort
    echo ""
    echo "Usage: $0 --os <name>"
    exit 0
fi

# Validate OS selection
if [[ ! -v OS_OPTIONS[$BOX_NAME] ]]; then
    echo -e "${RED}Error: Unknown OS '$BOX_NAME'${NC}"
    echo "Use --list-os to see available options"
    exit 1
fi

echo -e "${CYAN}Selected OS: ${OS_OPTIONS[$BOX_NAME]}${NC}"
echo ""

# Change to e2e directory
cd "$(dirname "$0")"

# Export BOX_NAME for Vagrant
export BOX_NAME

if [ "$PROVISION" = true ]; then
    echo -e "${YELLOW}Re-running provisioning...${NC}"
    vagrant provision
else
    echo -e "${YELLOW}Starting VM and running tests...${NC}"
    vagrant up
fi

# Check if tests passed
if [ $? -eq 0 ]; then
    echo -e "${GREEN}=========================================="
    echo "✓ All tests passed!"
    echo -e "==========================================${NC}"
else
    echo -e "${RED}=========================================="
    echo "✗ Tests failed!"
    echo -e "==========================================${NC}"
    exit 1
fi

# SSH if requested
if [ "$SSH" = true ]; then
    echo -e "${YELLOW}Opening SSH session...${NC}"
    vagrant ssh
fi

# Destroy if requested
if [ "$DESTROY" = true ]; then
    echo -e "${YELLOW}Destroying VM...${NC}"
    vagrant destroy -f
    echo -e "${GREEN}VM destroyed${NC}"
fi

echo ""
echo -e "${GREEN}Done!${NC}"
echo ""
echo "Useful commands:"
echo "  vagrant ssh              - SSH into the VM"
echo "  vagrant provision        - Re-run tests"
echo "  vagrant destroy -f       - Destroy the VM"
echo ""
