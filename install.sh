#!/bin/sh
set -e

# This script installs the Clara binary to /usr/local/bin using standard shell tools.

# --- Configuration ---
REPO="thecodecapo/Clara"

# --- Script Logic ---
echo "Fetching the latest version of Clara..."
API_URL="https://api.github.com/repos/${REPO}/releases/latest"

# Determine the OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/')
ASSET_NAME="clara-${OS}-${ARCH}"

echo "Looking for asset: ${ASSET_NAME}"

# Get the latest release data from GitHub API
RELEASE_DATA=$(curl -s "$API_URL")

# Extract version first
VERSION=$(echo "$RELEASE_DATA" | grep -o '"tag_name": *"[^"]*"' | sed 's/"tag_name": *"\([^"]*\)"/\1/')

# Find the download URL for the correct asset
# Using a more robust grep pattern
DOWNLOAD_URL=$(echo "$RELEASE_DATA" | grep -o '"browser_download_url": *"[^"]*'"${ASSET_NAME}"'"' | sed 's/"browser_download_url": *"\([^"]*\)"/\1/')

# Alternative method if the above doesn't work
if [ -z "$DOWNLOAD_URL" ]; then
    echo "Trying alternative parsing method..."
    DOWNLOAD_URL=$(echo "$RELEASE_DATA" | grep "browser_download_url" | grep "${ASSET_NAME}" | head -1 | sed 's/.*"browser_download_url": *"\([^"]*\)".*/\1/')
fi

# Debug information
echo "OS: ${OS}"
echo "ARCH: ${ARCH}"
echo "Asset name: ${ASSET_NAME}"
echo "Version: ${VERSION}"

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Error: Could not find a binary for your system (${OS}/${ARCH})."
    echo "Available assets:"
    echo "$RELEASE_DATA" | grep "browser_download_url" | sed 's/.*"browser_download_url": *"\([^"]*\)".*/\1/'
    echo "Please check the releases page at https://github.com/${REPO}/releases"
    exit 1
fi

echo "Download URL: ${DOWNLOAD_URL}"

# Download and install
echo "Downloading Clara ${VERSION}..."
curl -L -o /tmp/clara "${DOWNLOAD_URL}"

if [ ! -f /tmp/clara ]; then
    echo "Error: Failed to download Clara binary"
    exit 1
fi

chmod +x /tmp/clara

echo "Installing to /usr/local/bin/clara..."
sudo mv /tmp/clara /usr/local/bin/clara

echo "Clara ${VERSION} has been installed successfully!"
echo "You can now run 'clara --help' to get started."