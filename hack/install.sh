#!/bin/sh
set -euo pipefail

REPO="SuiJQ/Keeper"
BIN_NAME="keeper"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${KEEPER_VERSION:-latest}"

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

# Resolve download URL
if [ "$VERSION" = "latest" ]; then
  TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | sed -E 's/.*"([^"]+)".*/\1/')
  if [ -z "$TAG" ]; then
    echo "Failed to resolve latest release tag" >&2
    exit 1
  fi
else
  TAG="$VERSION"
fi

URL="https://github.com/${REPO}/releases/download/${TAG}/${BIN_NAME}-${GOARCH}"

echo "Downloading ${BIN_NAME} ${TAG} for ${GOARCH}..."
curl -fsSL "$URL" -o "${INSTALL_DIR}/${BIN_NAME}"

chmod +x "${INSTALL_DIR}/${BIN_NAME}"

echo "Installed ${BIN_NAME} to ${INSTALL_DIR}/${BIN_NAME}"
"${INSTALL_DIR}/${BIN_NAME}" version
