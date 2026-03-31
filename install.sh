#!/bin/sh
set -e

# Detect OS and Architecture
OS=$(uname -s)
ARCH=$(uname -m)

if [ "$OS" != "Linux" ]; then
    echo "Error: paruz is designed for Arch Linux and does not support $OS."
    exit 1
fi

case "$ARCH" in
    x86_64) ARCH="x86_64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    i386|i686) ARCH="i386" ;;
    armv6l) ARCH="v6" ;;
    armv7l) ARCH="v7" ;;
    *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

OS="linux"

REPO="vyogami/paruz"
# Get the latest release tag from GitHub API
LATEST_TAG=$(curl -s https://api.github.com/repos/$REPO/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')

if [ -z "$LATEST_TAG" ]; then
    echo "Could not fetch latest release tag. Falling back to v1.1.0-alpha"
    LATEST_TAG="v1.1.0-alpha"
fi

# Clean 'v' from tag if present for the filename part (GoReleaser format)
VERSION=$(echo $LATEST_TAG | sed 's/^v//')

# Construct download URL (matches GoReleaser template)
FILENAME="paruz_${OS}_${ARCH}.tar.gz"
URL="https://github.com/$REPO/releases/download/$LATEST_TAG/$FILENAME"

echo "Downloading paruz $LATEST_TAG for ${OS} ${ARCH}..."
TEMP_DIR=$(mktemp -d)
curl -sL "$URL" -o "$TEMP_DIR/$FILENAME"

echo "Extracting..."
tar -xzf "$TEMP_DIR/$FILENAME" -C "$TEMP_DIR"

echo "Installing to /usr/local/bin/paruz..."
sudo install -m 755 "$TEMP_DIR/paruz" /usr/local/bin/paruz

echo "Cleaning up..."
rm -rf "$TEMP_DIR"

echo "Successfully installed paruz!"
paruz --version || echo "Done!"
