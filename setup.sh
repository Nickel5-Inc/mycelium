#!/bin/bash

set -e  # Exit on any error

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}Starting Mycelium setup...${NC}"

# Check OS
if [[ "$OSTYPE" == "darwin"* ]]; then
    PACKAGE_MANAGER="brew"
    echo -e "${YELLOW}Detected macOS${NC}"
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    if command -v apt-get &> /dev/null; then
        PACKAGE_MANAGER="apt-get"
        echo -e "${YELLOW}Detected Debian/Ubuntu${NC}"
    elif command -v yum &> /dev/null; then
        PACKAGE_MANAGER="yum"
        echo -e "${YELLOW}Detected RHEL/CentOS${NC}"
    else
        echo -e "${RED}Unsupported Linux distribution${NC}"
        exit 1
    fi
else
    echo -e "${RED}Unsupported operating system: $OSTYPE${NC}"
    exit 1
fi

# Install package manager if needed (macOS)
if [[ "$PACKAGE_MANAGER" == "brew" ]] && ! command -v brew &> /dev/null; then
    echo -e "${YELLOW}Installing Homebrew...${NC}"
    /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"
fi

# Function to install packages based on package manager
install_package() {
    local package=$1
    echo -e "${YELLOW}Installing $package...${NC}"
    case $PACKAGE_MANAGER in
        "brew")
            brew install $package
            ;;
        "apt-get")
            sudo apt-get install -y $package
            ;;
        "yum")
            sudo yum install -y $package
            ;;
    esac
}

# Install Go if not present
if ! command -v go &> /dev/null; then
    echo -e "${YELLOW}Installing Go...${NC}"
    install_package "go"
fi

# Install PostgreSQL if not present
if ! command -v psql &> /dev/null; then
    echo -e "${YELLOW}Installing PostgreSQL...${NC}"
    install_package "postgresql"
fi

# Start PostgreSQL service
echo -e "${YELLOW}Starting PostgreSQL service...${NC}"
if [[ "$PACKAGE_MANAGER" == "brew" ]]; then
    brew services start postgresql
elif [[ "$PACKAGE_MANAGER" == "apt-get" ]]; then
    sudo systemctl start postgresql
    sudo systemctl enable postgresql
elif [[ "$PACKAGE_MANAGER" == "yum" ]]; then
    sudo systemctl start postgresql
    sudo systemctl enable postgresql
fi

# Wait for PostgreSQL to start
echo -e "${YELLOW}Waiting for PostgreSQL to start...${NC}"
sleep 5

# Create PostgreSQL user and database
echo -e "${YELLOW}Setting up PostgreSQL database...${NC}"
if [[ "$PACKAGE_MANAGER" == "brew" ]]; then
    # macOS default user is current user
    createdb mycelium 2>/dev/null || true
    psql -d mycelium -c "CREATE USER postgres SUPERUSER PASSWORD 'postgres'" 2>/dev/null || true
else
    # Linux needs to run as postgres user
    sudo -u postgres psql -c "CREATE DATABASE mycelium;" 2>/dev/null || true
    sudo -u postgres psql -c "CREATE USER postgres WITH SUPERUSER PASSWORD 'postgres';" 2>/dev/null || true
fi

# Install golangci-lint if not present
if ! command -v golangci-lint &> /dev/null; then
    echo -e "${YELLOW}Installing golangci-lint...${NC}"
    if [[ "$PACKAGE_MANAGER" == "brew" ]]; then
        brew install golangci-lint
    else
        # Install from official script
        curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
    fi
fi

# Set up Go environment
echo -e "${YELLOW}Setting up Go environment...${NC}"
go mod download
go mod tidy

# Run tests to verify setup
echo -e "${YELLOW}Running tests to verify setup...${NC}"
go test ./... -v

echo -e "${GREEN}Setup complete!${NC}"
echo -e "${YELLOW}Note: Bittensor wallet creation and registration must be done manually.${NC}"
echo -e "${YELLOW}To create and register a wallet, please follow the Bittensor documentation.${NC}" 