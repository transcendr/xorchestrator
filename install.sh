#!/bin/bash
#
# Perles Install Script
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/zjrosen/perles/main/install.sh | bash
#
# Environment Variables:
#   INSTALL_DIR - Installation directory (default: $HOME/.local/bin)
#   VERSION     - Specific version to install (default: latest)
#
set -e

# Configuration
OWNER="${OWNER:-zjrosen}"
REPO="perles"
# Default to a per-user install path so root is not required.
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

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
    local os
    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        linux)
            echo "linux"
            ;;
        darwin)
            echo "darwin"
            ;;
        *)
            error "Unsupported operating system: $os"
            ;;
    esac
}

# Detect architecture
detect_arch() {
    local arch
    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)
            echo "amd64"
            ;;
        aarch64|arm64)
            echo "arm64"
            ;;
        *)
            error "Unsupported architecture: $arch"
            ;;
    esac
}

# Get latest version from GitHub API
get_latest_version() {
    local version
    version=$(curl -sS "https://api.github.com/repos/$OWNER/$REPO/releases/latest" 2>/dev/null | \
        grep '"tag_name":' | \
        sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$version" ]; then
        error "Failed to fetch latest version. Check your network connection and GitHub repository."
    fi

    echo "$version"
}

# Download and install binary
install_binary() {
    local version="$1"
    local os="$2"
    local arch="$3"

    # Remove 'v' prefix for filename if present
    local version_num="${version#v}"
    local filename="${REPO}_${version_num}_${os}_${arch}.tar.gz"
    local url="https://github.com/$OWNER/$REPO/releases/download/$version/$filename"

    info "Downloading $REPO $version for $os/$arch..."

    # Create temporary directory
    local tmpdir
    tmpdir=$(mktemp -d)
    trap "rm -rf $tmpdir" EXIT

    # Download archive
    if ! curl -sS -L "$url" -o "$tmpdir/$filename" 2>/dev/null; then
        error "Failed to download $url"
    fi

    # Verify download succeeded and file has content
    if [ ! -s "$tmpdir/$filename" ]; then
        error "Downloaded file is empty. Release may not exist for $os/$arch."
    fi

    # Extract archive
    info "Extracting archive..."
    if ! tar -xzf "$tmpdir/$filename" -C "$tmpdir" 2>/dev/null; then
        error "Failed to extract archive. The download may be corrupted."
    fi

    # Verify binary exists
    if [ ! -f "$tmpdir/$REPO" ]; then
        error "Binary not found in archive."
    fi

    # Install binary
    info "Installing to $INSTALL_DIR..."

    # Create install directory if needed
    if [ ! -d "$INSTALL_DIR" ]; then
        if ! mkdir -p "$INSTALL_DIR" 2>/dev/null; then
            warn "Cannot create $INSTALL_DIR. Trying with sudo..."
            sudo mkdir -p "$INSTALL_DIR"
        fi
    fi

    # Move binary to install directory
    if ! mv "$tmpdir/$REPO" "$INSTALL_DIR/" 2>/dev/null; then
        warn "Cannot write to $INSTALL_DIR. Trying with sudo..."
        sudo mv "$tmpdir/$REPO" "$INSTALL_DIR/"
    fi

    # Make executable
    if ! chmod +x "$INSTALL_DIR/$REPO" 2>/dev/null; then
        sudo chmod +x "$INSTALL_DIR/$REPO"
    fi
}

# Check if install directory is in PATH and warn if not
check_path() {
    if ! printf "%s" "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
        echo ""
        warn "$INSTALL_DIR is not in your PATH."
        echo ""
        echo "  Add the following to your shell profile (~/.bashrc, ~/.zshrc, or ~/.profile):"
        echo ""
        echo "    export PATH=\"\$HOME/.local/bin:\$PATH\""
        echo ""
        echo "  Then reload your shell or run:"
        echo ""
        echo "    source ~/.profile  # or ~/.bashrc, ~/.zshrc"
        echo ""
    fi
}

# Verify installation
verify_install() {
    if command -v "$REPO" &>/dev/null; then
        local installed_version
        installed_version=$("$REPO" --version 2>/dev/null | head -1)
        info "Successfully installed: $installed_version"
    elif [ -x "$INSTALL_DIR/$REPO" ]; then
        local installed_version
        installed_version=$("$INSTALL_DIR/$REPO" --version 2>/dev/null | head -1)
        info "Successfully installed: $installed_version"
        check_path
    else
        error "Installation verification failed."
    fi
}

main() {
    echo ""
    echo "Perles Installer"
    echo "================"
    echo ""

    # Detect platform
    local os arch version
    os=$(detect_os)
    arch=$(detect_arch)

    info "Detected platform: $os/$arch"

    # Get version
    if [ -n "$VERSION" ]; then
        version="$VERSION"
        info "Using specified version: $version"
    else
        info "Fetching latest version..."
        version=$(get_latest_version)
        info "Latest version: $version"
    fi

    # Install
    install_binary "$version" "$os" "$arch"

    # Verify
    verify_install

    echo ""
    info "Installation complete!"
    echo ""
}

main "$@"
