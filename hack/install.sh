#!/bin/sh
set -euo pipefail

# Keeper Install Script - One-line install for AI Agent Runtime
# Supports: Linux amd64/arm64, auto-detects install location, verifies binary

REPO="SuiJQ/Keeper"
BIN_NAME="keeper"
DEFAULT_INSTALL_DIR="/usr/local/bin"
USER_INSTALL_DIR="$HOME/.local/bin"
VERSION="${KEEPER_VERSION:-latest}"
FORCE="${KEEPER_FORCE:-0}"

# Colors for terminal output (disabled if not a terminal)
if [ -t 1 ]; then
  RED='\033[0;31m'
  GREEN='\033[0;32m'
  YELLOW='\033[1;33m'
  BLUE='\033[0;34m'
  NC='\033[0m' # No Color
else
  RED=''
  GREEN=''
  YELLOW=''
  BLUE=''
  NC=''
fi

info() {
  printf "${BLUE}[INFO]${NC} %s\n" "$1"
}

success() {
  printf "${GREEN}[SUCCESS]${NC} %s\n" "$1"
}

warn() {
  printf "${YELLOW}[WARN]%s %s\n" "" "$1"
}

error() {
  printf "${RED}[ERROR]${NC} %s\n" "$1" >&2
}

# Check if running on Linux
check_os() {
  if [ "$(uname -s)" != "Linux" ]; then
    error "Keeper currently only supports Linux. Detected: $(uname -s)"
    error "Please run on a Linux machine with kernel 5.11+ for bwrap backend,"
    error "or Docker Engine for docker backend."
    exit 1
  fi
}

# Detect architecture
detect_arch() {
  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64|amd64) GOARCH="amd64" ;;
    aarch64|arm64) GOARCH="arm64" ;;
    *)
      error "Unsupported architecture: $ARCH"
      error "Supported architectures: amd64 (x86_64), arm64 (aarch64)"
      exit 1
      ;;
  esac
  info "Detected architecture: ${GOARCH}"
}

# Check required tools
check_dependencies() {
  local missing=0

  if ! command -v curl >/dev/null 2>&1; then
    error "curl is required but not installed. Please install curl first."
    missing=1
  fi

  if ! command -v tar >/dev/null 2>&1; then
    error "tar is required but not installed. Please install tar first."
    missing=1
  fi

  if [ "$missing" -eq 1 ]; then
    exit 1
  fi

  info "Dependencies check passed"
}

# Determine installation directory
detect_install_dir() {
  # If KEEPER_INSTALL_DIR is set, use it
  if [ -n "${KEEPER_INSTALL_DIR:-}" ]; then
    INSTALL_DIR="$KEEPER_INSTALL_DIR"
    info "Using custom install directory: $INSTALL_DIR"
    return
  fi

  # Check if we can write to default system directory
  if [ -w "$DEFAULT_INSTALL_DIR" ]; then
    INSTALL_DIR="$DEFAULT_INSTALL_DIR"
    info "System-wide install: $INSTALL_DIR"
  else
    # Fall back to user local directory
    INSTALL_DIR="$USER_INSTALL_DIR"
    warn "Cannot write to $DEFAULT_INSTALL_DIR (permission denied)"
    warn "Falling back to user-local install: $INSTALL_DIR"
    warn "Make sure $INSTALL_DIR is in your PATH"
  fi
}

# Check if binary already exists
check_existing_binary() {
  if [ -f "${INSTALL_DIR}/${BIN_NAME}" ]; then
    if [ "$FORCE" = "1" ]; then
      warn "Overwriting existing ${INSTALL_DIR}/${BIN_NAME}"
      return
    fi

    info "Found existing ${INSTALL_DIR}/${BIN_NAME}"
    CURRENT_VERSION="$("${INSTALL_DIR}/${BIN_NAME}" version 2>/dev/null || echo "unknown")"
    info "Current version: $CURRENT_VERSION"

    printf "Do you want to overwrite? [y/N] "
    read -r RESPONSE
    case "$RESPONSE" in
      [yY][eE][sS]|[yY])
        info "Overwriting existing binary"
        ;;
      *)
        info "Installation cancelled by user"
        exit 0
        ;;
    esac
  fi
}

# Resolve release tag
resolve_tag() {
  if [ "$VERSION" = "latest" ]; then
    info "Resolving latest release tag..."
    TAG=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | head -1 | sed -E 's/.*"([^"]+)".*/\1/')
    if [ -z "$TAG" ]; then
      error "Failed to resolve latest release tag from GitHub API"
      error "Possible reasons:"
      error "  - Network connectivity issues"
      error "  - GitHub API rate limit exceeded"
      error "  - Repository does not exist or has no releases"
      error ""
      error "You can try specifying a specific version:"
      error "  curl -fsSL https://raw.githubusercontent.com/SuiJQ/Keeper/main/hack/install.sh | KEEPER_VERSION=v0.1.0 sh"
      exit 1
    fi
    info "Latest release: $TAG"
  else
    TAG="$VERSION"
    info "Using specified version: $TAG"
  fi
}

# Create install directory if needed
prepare_install_dir() {
  if [ ! -d "$INSTALL_DIR" ]; then
    info "Creating directory: $INSTALL_DIR"
    mkdir -p "$INSTALL_DIR"
  fi

  # Check if we can write to the install directory
  if [ ! -w "$INSTALL_DIR" ]; then
    error "Cannot write to $INSTALL_DIR"
    error "Please run with sudo, or set KEEPER_INSTALL_DIR to a writable directory:"
    error "  sudo curl -fsSL https://raw.githubusercontent.com/SuiJQ/Keeper/main/hack/install.sh | sh"
    error "  # OR"
    error "  curl -fsSL https://raw.githubusercontent.com/SuiJQ/Keeper/main/hack/install.sh | KEEPER_INSTALL_DIR=\$HOME/.local/bin sh"
    exit 1
  fi
}

# Download and install binary
download_and_install() {
  URL="https://github.com/${REPO}/releases/download/${TAG}/${BIN_NAME}-${GOARCH}"

  info "Downloading ${BIN_NAME} ${TAG} for ${GOARCH}..."
  info "URL: $URL"

  TMP_FILE=$(mktemp)
  trap "rm -f $TMP_FILE" EXIT

  # Download with progress bar
  HTTP_CODE=$(curl -fsSL --progress-bar -w "%{http_code}" -o "$TMP_FILE" "$URL" 2>&1) || {
    HTTP_CODE=$?
  }

  if [ "$HTTP_CODE" != "200" ]; then
    error "Failed to download binary (HTTP $HTTP_CODE)"
    error "URL: $URL"
    error ""
    error "Please check:"
    error "  1. Network connectivity"
    error "  2. Release tag exists: https://github.com/${REPO}/releases/tag/${TAG}"
    error "  3. Architecture asset exists in the release"
    exit 1
  fi

  # Verify it's actually a binary (not an HTML error page)
  if ! file "$TMP_FILE" | grep -q "ELF\|executable"; then
    error "Downloaded file does not appear to be a valid Linux binary"
    error "File type: $(file "$TMP_FILE")"
    error "This might mean the release asset is missing for ${GOARCH}"
    exit 1
  fi

  # Check file size (should be reasonable)
  FILE_SIZE=$(stat -c%s "$TMP_FILE" 2>/dev/null || stat -f%z "$TMP_FILE" 2>/dev/null || echo "0")
  if [ "$FILE_SIZE" -lt 1000000 ]; then
    error "Downloaded file is too small ($FILE_SIZE bytes). Expected ~20MB binary."
    error "The download may have failed or returned an error page."
    exit 1
  fi

  info "Download complete ($(numfmt --to=iec-i --suffix=B "$FILE_SIZE" 2>/dev/null || echo "${FILE_SIZE} bytes"))"

  # Install binary
  info "Installing to ${INSTALL_DIR}/${BIN_NAME}..."
  mv -f "$TMP_FILE" "${INSTALL_DIR}/${BIN_NAME}"
  chmod +x "${INSTALL_DIR}/${BIN_NAME}"
  trap - EXIT
}

# Verify installation
verify_installation() {
  info "Verifying installation..."

  if [ ! -x "${INSTALL_DIR}/${BIN_NAME}" ]; then
    error "Binary is not executable: ${INSTALL_DIR}/${BIN_NAME}"
    exit 1
  fi

  INSTALLED_VERSION="$("${INSTALL_DIR}/${BIN_NAME}" version 2>/dev/null || echo "unknown")"
  if [ "$INSTALLED_VERSION" = "unknown" ] || [ -z "$INSTALLED_VERSION" ]; then
    warn "Could not verify version (binary may have issues)"
  else
    success "Installed version: $INSTALLED_VERSION"
  fi

  # Check if in PATH
  if ! command -v "$BIN_NAME" >/dev/null 2>&1; then
    warn "$BIN_NAME is not in your PATH"
    if [ "$INSTALL_DIR" != "$DEFAULT_INSTALL_DIR" ]; then
      warn "Add this to your shell config (~/.bashrc, ~/.zshrc, etc.):"
      echo ""
      echo "  export PATH=\"\$PATH:${INSTALL_DIR}\""
      echo ""
    else
      warn "You may need to run: hash -r"
      warn "Or open a new terminal session"
    fi
  else
    PATH_DIR=$(dirname "$(command -v "$BIN_NAME")")
    if [ "$PATH_DIR" = "$INSTALL_DIR" ]; then
      success "$BIN_NAME is available in PATH at $PATH_DIR"
    else
      warn "$BIN_NAME found at $PATH_DIR, but installed to $INSTALL_DIR"
      warn "You may have another version in PATH. Adjust PATH or use full path."
    fi
  fi
}

# Print next steps
print_next_steps() {
  echo ""
  success "Keeper installed successfully!"
  echo ""
  info "Next steps:"
  echo "  1. Verify installation: keeper version"
  echo "  2. Create your first agent: keeper create my-agent"
  echo "  3. Start the agent: keeper start my-agent"
  echo "  4. Check status: keeper inspect my-agent"
  echo ""
  info "Documentation:"
  echo "  - User Guide: https://github.com/SuiJQ/Keeper/blob/main/README_USER.md"
  echo "  - MCP Guide:  https://github.com/SuiJQ/Keeper/blob/main/docs/MCP_GUIDE.md"
  echo "  - Full Docs:  https://github.com/SuiJQ/Keeper/tree/main/docs"
  echo ""
}

# Main installation flow
main() {
  echo ""
  info "Keeper Installer - AI Agent Linux Runtime"
  info "Repository: https://github.com/${REPO}"
  echo ""

  check_os
  detect_arch
  check_dependencies
  detect_install_dir
  check_existing_binary
  resolve_tag
  prepare_install_dir
  download_and_install
  verify_installation
  print_next_steps
}

main "$@"
