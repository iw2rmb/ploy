#!/bin/bash
# Ploy CLI Installation Script
# Usage: curl -fsSL https://raw.githubusercontent.com/iw2rmb/ploy/main/scripts/install.sh | bash

set -e

# Configuration
REPO="iw2rmb/ploy"
BINARY_NAME="ploy"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
GITHUB_API="https://api.github.com/repos/${REPO}"
GITHUB_DOWNLOAD="https://github.com/${REPO}/releases/download"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Helper functions
info() {
  echo -e "${GREEN}[INFO]${NC} $1"
}

warn() {
  echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
  echo -e "${RED}[ERROR]${NC} $1"
  exit 1
}

# Detect OS
detect_os() {
  case "$(uname -s)" in
    Darwin*)
      echo "darwin"
      ;;
    Linux*)
      echo "linux"
      ;;
    MINGW*|MSYS*|CYGWIN*)
      echo "windows"
      ;;
    *)
      error "Unsupported operating system: $(uname -s)"
      ;;
  esac
}

# Detect architecture
detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64)
      echo "amd64"
      ;;
    aarch64|arm64)
      echo "arm64"
      ;;
    armv7l)
      echo "arm"
      ;;
    i386|i686)
      echo "386"
      ;;
    *)
      error "Unsupported architecture: $(uname -m)"
      ;;
  esac
}

# Get latest release version
get_latest_version() {
  if command -v curl &> /dev/null; then
    curl -sSL "${GITHUB_API}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'
  elif command -v wget &> /dev/null; then
    wget -qO- "${GITHUB_API}/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/'
  else
    error "Neither curl nor wget found. Please install one of them."
  fi
}

# Download file
download_file() {
  local url=$1
  local output=$2

  if command -v curl &> /dev/null; then
    curl -sSL -o "$output" "$url"
  elif command -v wget &> /dev/null; then
    wget -qO "$output" "$url"
  else
    error "Neither curl nor wget found. Please install one of them."
  fi
}

# Verify checksum
verify_checksum() {
  local file=$1
  local checksum_file=$2
  local binary_name=$3

  info "Verifying checksum..."

  # Extract the checksum for our specific file
  local expected_checksum
  expected_checksum=$(grep "${binary_name}" "$checksum_file" | awk '{print $1}')

  if [ -z "$expected_checksum" ]; then
    warn "Could not find checksum for ${binary_name} in checksums.txt"
    return 0
  fi

  # Calculate actual checksum
  local actual_checksum
  if command -v sha256sum &> /dev/null; then
    actual_checksum=$(sha256sum "$file" | awk '{print $1}')
  elif command -v shasum &> /dev/null; then
    actual_checksum=$(shasum -a 256 "$file" | awk '{print $1}')
  else
    warn "sha256sum/shasum not found, skipping checksum verification"
    return 0
  fi

  if [ "$expected_checksum" != "$actual_checksum" ]; then
    error "Checksum verification failed!\nExpected: $expected_checksum\nActual: $actual_checksum"
  fi

  info "Checksum verified successfully"
}

# Main installation function
install_ploy() {
  local os
  local arch
  local version
  local archive_name
  local download_url
  local checksum_url
  local tmp_dir

  info "Detecting system..."
  os=$(detect_os)
  arch=$(detect_arch)
  info "Detected OS: $os, Architecture: $arch"

  info "Fetching latest release..."
  version=$(get_latest_version)
  if [ -z "$version" ]; then
    error "Failed to fetch latest version"
  fi
  info "Latest version: $version"

  # Construct archive name
  if [ "$os" = "windows" ]; then
    archive_name="${BINARY_NAME}_${version#v}_${os}_${arch}.zip"
  else
    archive_name="${BINARY_NAME}_${version#v}_${os}_${arch}.tar.gz"
  fi

  download_url="${GITHUB_DOWNLOAD}/${version}/${archive_name}"
  checksum_url="${GITHUB_DOWNLOAD}/${version}/checksums.txt"

  # Create temporary directory
  tmp_dir=$(mktemp -d)
  trap 'rm -rf "$tmp_dir"' EXIT

  info "Downloading ploy $version..."
  download_file "$download_url" "$tmp_dir/$archive_name"

  info "Downloading checksums..."
  download_file "$checksum_url" "$tmp_dir/checksums.txt"

  verify_checksum "$tmp_dir/$archive_name" "$tmp_dir/checksums.txt" "$archive_name"

  info "Extracting archive..."
  cd "$tmp_dir"
  if [ "$os" = "windows" ]; then
    if command -v unzip &> /dev/null; then
      unzip -q "$archive_name"
    else
      error "unzip not found. Please install unzip to continue."
    fi
  else
    tar -xzf "$archive_name"
  fi

  # Find the binary (might be in a subdirectory)
  local binary_path
  if [ "$os" = "windows" ]; then
    binary_path=$(find . -name "${BINARY_NAME}.exe" -type f | head -n 1)
  else
    binary_path=$(find . -name "${BINARY_NAME}" -type f | head -n 1)
  fi

  if [ -z "$binary_path" ]; then
    error "Could not find ${BINARY_NAME} binary in archive"
  fi

  info "Installing to ${INSTALL_DIR}..."

  # Check if install directory exists and is writable
  if [ ! -d "$INSTALL_DIR" ]; then
    warn "${INSTALL_DIR} does not exist. Attempting to create it..."
    if ! mkdir -p "$INSTALL_DIR" 2>/dev/null; then
      error "${INSTALL_DIR} does not exist and could not be created. Try running with sudo or set INSTALL_DIR to a writable location."
    fi
  fi

  # Try to install, use sudo if permission denied
  if ! cp "$binary_path" "${INSTALL_DIR}/${BINARY_NAME}" 2>/dev/null; then
    warn "Permission denied. Attempting with sudo..."
    if ! sudo cp "$binary_path" "${INSTALL_DIR}/${BINARY_NAME}"; then
      error "Failed to install ${BINARY_NAME} to ${INSTALL_DIR}"
    fi
    sudo chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
  else
    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"
  fi

  info "Installation complete!"
  echo ""
  info "Ploy ${version} has been installed to ${INSTALL_DIR}/${BINARY_NAME}"
  echo ""

  # Verify installation
  if command -v ploy &> /dev/null; then
    info "Verifying installation..."
    ploy version
    echo ""
    info "You can now use 'ploy' from anywhere in your terminal!"
  else
    warn "${INSTALL_DIR} is not in your PATH."
    echo ""
    echo "Add it to your PATH by adding this line to your shell configuration file:"
    echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    echo ""
    echo "Then reload your shell configuration:"
    echo "  source ~/.bashrc  # for bash"
    echo "  source ~/.zshrc   # for zsh"
  fi
}

# Run installation
main() {
  echo ""
  info "Ploy CLI Installation Script"
  echo ""

  # Check for custom version (optional future enhancement)
  if [ -n "$1" ]; then
    warn "Custom version installation not yet supported. Installing latest version."
  fi

  install_ploy
}

main "$@"
