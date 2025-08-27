#!/bin/sh
set -e

# This script installs the Clara binary to /usr/local/bin

# --- Configuration ---
REPO="thecodecapo/Clara"

# --- Script Logic ---
echo "Fetching the latest version of Clara..."
LATEST_RELEASE_URL="https://api.github.com/repos/${REPO}/releases/latest"

# Find the latest release version and construct the asset name
VERSION=$(curl -s "$LATEST_RELEASE_URL" | grep -Po '"tag_name": "\K.*?(?=")')
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
if [ "$ARCH" = "x86_64" ]; then
    ARCH="amd64"
fi
ASSET_NAME="clara-${OS}-${ARCH}"

# Find the download URL for the correct asset
DOWNLOAD_URL=$(curl -s "$LATEST_RELEASE_URL" | grep -o "browser_download_url.*${ASSET_NAME}" | cut -d '"' -f 4)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Error: Could not find a binary for your system (${OS}/${ARCH})."
    exit 1
fi

# Download and install
echo "Downloading Clara ${VERSION}..."
curl -L -o /tmp/clara "${DOWNLOAD_URL}"
chmod +x /tmp/clara
echo "Installing to /usr/local/bin/clara..."
sudo mv /tmp/clara /usr/local/bin/clara

echo "Clara ${VERSION} has been installed successfully!"