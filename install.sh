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

# Get the latest release data from GitHub API
RELEASE_DATA=$(curl -s "$API_URL")

# Find the download URL for the correct asset using grep, sed, and cut
# This is more complex than jq, but has no dependencies.
DOWNLOAD_URL=$(echo "$RELEASE_DATA" | grep "browser_download_url.*${ASSET_NAME}" | cut -d '"' -f 4)
VERSION=$(echo "$RELEASE_DATA" | grep -o '"tag_name": ".*"' | cut -d '"' -f 4)

if [ -z "$DOWNLOAD_URL" ]; then
    echo "Error: Could not find a binary for your system (${OS}/${ARCH}). Please check the releases page."
    exit 1
fi

# Download and install
echo "Downloading Clara ${VERSION}..."
curl -L -o /tmp/clara "${DOWNLOAD_URL}"
chmod +x /tmp/clara

echo "Installing to /usr/local/bin/clara..."
sudo mv /tmp/clara /usr/local/bin/clara

echo "Clara ${VERSION} has been installed successfully!"