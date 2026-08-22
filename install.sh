#!/usr/bin/env bash
set -euo pipefail

GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'
REPO="Sheldonsix/oracle_arm_go"

echo -e "${BLUE}==> Starting oracle-arm-go installation...${NC}"

for cmd in curl tar find; do
    if ! command -v "$cmd" >/dev/null 2>&1; then
        echo -e "${RED}Error: $cmd is required.${NC}"
        exit 1
    fi
done

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed -e 's/x86_64/amd64/' -e 's/aarch64/arm64/' -e 's/armv8l/arm64/')

echo -e "${BLUE}==> Detected OS: ${OS}, Architecture: ${ARCH}${NC}"

# Fetch latest release URL
echo -e "${BLUE}==> Fetching latest release information...${NC}"
DL_URL=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep browser_download_url | grep "${OS}_${ARCH}\.tar\.gz" | cut -d '"' -f 4 | head -n 1 || true)

if [ -z "$DL_URL" ]; then
    echo -e "${RED}Error: Could not find a suitable release for ${OS}_${ARCH}.${NC}"
    exit 1
fi

echo -e "${BLUE}==> Downloading latest release...${NC}"
curl -fsSL "$DL_URL" -o "$TMP_DIR/oracle-arm.tar.gz"

echo -e "${BLUE}==> Extracting files...${NC}"
tar -xzf "$TMP_DIR/oracle-arm.tar.gz" -C "$TMP_DIR"

# Find the binary
BIN_PATH=$(find "$TMP_DIR" -type f -name "oracle-arm" | head -n 1)
if [ -z "$BIN_PATH" ]; then
    # For windows archives which might be .exe, although we checked tar.gz
    BIN_PATH=$(find "$TMP_DIR" -type f -name "oracle-arm.exe" | head -n 1)
fi

if [ -z "$BIN_PATH" ]; then
    echo -e "${RED}Error: Could not find oracle-arm binary in the archive.${NC}"
    exit 1
fi

mv "$BIN_PATH" ./oracle-arm
chmod +x ./oracle-arm

# Try to find .env.example
ENV_EXAMPLE=$(find "$TMP_DIR" -type f -name ".env.example" | head -n 1)
if [ -n "$ENV_EXAMPLE" ]; then
    cp "$ENV_EXAMPLE" ./.env.example
else
    if curl -fsSL "https://raw.githubusercontent.com/${REPO}/main/.env.example" -o "$TMP_DIR/.env.example"; then
        cp "$TMP_DIR/.env.example" ./.env.example
    else
        echo -e "${RED}Warning: Could not download .env.example.${NC}"
    fi
fi

if [ -f .env.example ] && [ ! -f .env ]; then
    cp .env.example .env
    echo -e "${GREEN}==> Created default .env file.${NC}"
fi

echo -e "${GREEN}==> Installation successful!${NC}"
echo -e "Executable is located at: $(pwd)/oracle-arm"
if [ -f .env ]; then
    echo -e "Please edit the ${BLUE}.env${NC} file with your configuration."
fi
echo -e "To run the program, use: ${BLUE}./oracle-arm${NC}"
