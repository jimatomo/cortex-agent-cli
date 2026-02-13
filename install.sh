#!/bin/sh
# coragent install script
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/jimatomo/cortex-agent-cli/main/install.sh | sh
#   curl -fsSL https://raw.githubusercontent.com/jimatomo/cortex-agent-cli/main/install.sh | sh -s -- --version v0.1.0
#   curl -fsSL https://raw.githubusercontent.com/jimatomo/cortex-agent-cli/main/install.sh | sh -s -- --install-dir ~/bin
#
# This script is compatible with bash, zsh, and sh.

set -e

# Configuration
REPO="jimatomo/cortex-agent-cli"
BINARY_NAME="coragent"
GITHUB_API="https://api.github.com"
GITHUB_RELEASES="https://github.com/${REPO}/releases"

# Default values
INSTALL_DIR="$HOME/.local/bin"
VERSION=""
VERBOSE=0
FORCE=0

# Colors (disabled if not interactive)
if [ -t 1 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[0;33m'
    BLUE='\033[0;34m'
    NC='\033[0m'
else
    RED=''
    GREEN=''
    YELLOW=''
    BLUE=''
    NC=''
fi

#######################################
# Print functions
#######################################
info() {
    printf "${BLUE}==>${NC} %s\n" "$1"
}

success() {
    printf "${GREEN}==>${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}Warning:${NC} %s\n" "$1" >&2
}

error() {
    printf "${RED}Error:${NC} %s\n" "$1" >&2
    exit 1
}

debug() {
    if [ "$VERBOSE" = "1" ]; then
        printf "${YELLOW}Debug:${NC} %s\n" "$1" >&2
    fi
}

#######################################
# Usage
#######################################
usage() {
    cat <<EOF
coragent installer

Usage:
    install.sh [options]

Options:
    -v, --version VERSION    Install a specific version (e.g., v0.1.0)
    -d, --install-dir DIR    Installation directory (default: ~/.local/bin)
    -f, --force              Force reinstall even if same version exists
    --verbose                Enable verbose output
    -h, --help               Show this help message

Examples:
    # Install latest version
    curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sh

    # Install specific version
    curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sh -s -- -v v0.1.0

    # Install to custom directory
    curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sh -s -- -d ~/bin
EOF
    exit 0
}

#######################################
# Parse arguments
#######################################
parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            -v|--version)
                if [ -z "$2" ] || [ "${2#-}" != "$2" ]; then
                    error "Option $1 requires a value"
                fi
                VERSION="$2"
                shift 2
                ;;
            -d|--install-dir)
                if [ -z "$2" ] || [ "${2#-}" != "$2" ]; then
                    error "Option $1 requires a value"
                fi
                INSTALL_DIR="$2"
                shift 2
                ;;
            --verbose)
                VERBOSE=1
                shift
                ;;
            -f|--force)
                FORCE=1
                shift
                ;;
            -h|--help)
                usage
                ;;
            *)
                error "Unknown option: $1"
                ;;
        esac
    done
}

#######################################
# Check required commands
#######################################
check_dependencies() {
    local missing=""

    for cmd in curl tar; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            missing="$missing $cmd"
        fi
    done

    if [ -n "$missing" ]; then
        error "Required commands not found:$missing"
    fi

    # Check for checksum command
    if command -v sha256sum >/dev/null 2>&1; then
        CHECKSUM_CMD="sha256sum"
    elif command -v shasum >/dev/null 2>&1; then
        CHECKSUM_CMD="shasum -a 256"
    else
        error "Neither sha256sum nor shasum found. Cannot verify checksum."
    fi

    debug "Using checksum command: $CHECKSUM_CMD"
}

#######################################
# Detect OS and architecture
#######################################
detect_platform() {
    OS="$(uname -s)"
    ARCH="$(uname -m)"

    case "$OS" in
        Linux)
            OS="linux"
            ;;
        Darwin)
            OS="darwin"
            ;;
        *)
            error "Unsupported operating system: $OS"
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        *)
            error "Unsupported architecture: $ARCH"
            ;;
    esac

    debug "Detected platform: ${OS}_${ARCH}"
}

#######################################
# Get latest version from GitHub
#######################################
validate_version() {
    local ver="$1"
    # Version should match v0.0.0 or 0.0.0 pattern (with optional pre-release suffix)
    case "$ver" in
        v[0-9]*.[0-9]*.[0-9]*|[0-9]*.[0-9]*.[0-9]*)
            return 0
            ;;
        *)
            return 1
            ;;
    esac
}

get_latest_version() {
    if [ -n "$VERSION" ]; then
        if ! validate_version "$VERSION"; then
            error "Invalid version format: $VERSION (expected: v0.0.0 or 0.0.0)"
        fi
        # Ensure version starts with 'v'
        case "$VERSION" in
            v*) ;;
            *)  VERSION="v${VERSION}" ;;
        esac
        debug "Using specified version: $VERSION"
        return
    fi

    info "Fetching latest version..."

    VERSION=$(curl -fsSL "${GITHUB_API}/repos/${REPO}/releases/latest" 2>/dev/null |
        grep '"tag_name":' |
        sed -E 's/.*"tag_name": *"([^"]+)".*/\1/')

    if [ -z "$VERSION" ]; then
        error "Failed to get latest version. Please specify a version with -v flag."
    fi

    if ! validate_version "$VERSION"; then
        error "Invalid version received from GitHub: $VERSION"
    fi

    debug "Latest version: $VERSION"
}

#######################################
# Check if already installed
#######################################
check_existing_installation() {
    if [ ! -x "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        return 0
    fi

    local installed_version
    installed_version=$("${INSTALL_DIR}/${BINARY_NAME}" version 2>/dev/null || echo "")

    if [ -z "$installed_version" ]; then
        return 0
    fi

    # Normalize versions for comparison (remove 'v' prefix if present)
    local target_ver="${VERSION#v}"
    local installed_ver="${installed_version#v}"

    debug "Installed version: $installed_ver, Target version: $target_ver"

    if [ "$installed_ver" = "$target_ver" ]; then
        if [ "$FORCE" = "1" ]; then
            info "Reinstalling ${BINARY_NAME} ${VERSION} (--force specified)"
            return 0
        fi
        success "${BINARY_NAME} ${VERSION} is already installed"
        echo ""
        echo "Use --force to reinstall."
        exit 0
    fi

    info "Upgrading ${BINARY_NAME} from ${installed_version} to ${VERSION}"
}

#######################################
# Download and verify binary
#######################################
download_and_install() {
    local version_num="${VERSION#v}"
    local archive_name="${BINARY_NAME}_${version_num}_${OS}_${ARCH}.tar.gz"
    local download_url="${GITHUB_RELEASES}/download/${VERSION}/${archive_name}"
    local checksum_url="${GITHUB_RELEASES}/download/${VERSION}/checksums.txt"

    # Create temporary directory
    local tmp_dir
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT

    debug "Temporary directory: $tmp_dir"

    info "Downloading ${BINARY_NAME} ${VERSION} for ${OS}/${ARCH}..."
    debug "Download URL: $download_url"

    # Download archive
    if ! curl -fsSL -o "${tmp_dir}/${archive_name}" "$download_url"; then
        error "Failed to download ${archive_name}. Check if the version exists."
    fi

    # Download and verify checksum
    info "Verifying checksum..."
    if ! curl -fsSL -o "${tmp_dir}/checksums.txt" "$checksum_url"; then
        error "Failed to download checksums.txt"
    fi

    # Extract expected checksum (using awk for exact filename match)
    local expected_checksum
    local match_count
    expected_checksum=$(awk -v target="${archive_name}" '$2 == target {print $1}' "${tmp_dir}/checksums.txt")
    match_count=$(awk -v target="${archive_name}" '$2 == target {count++} END {print count+0}' "${tmp_dir}/checksums.txt")

    if [ "$match_count" -eq 0 ]; then
        error "Checksum not found for ${archive_name}"
    fi

    if [ "$match_count" -gt 1 ]; then
        error "Multiple checksums found for ${archive_name}. The checksums.txt file may be corrupted."
    fi

    # Validate checksum format (64 hex characters for SHA256)
    case "$expected_checksum" in
        [a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9][a-f0-9])
            ;;
        *)
            error "Invalid checksum format in checksums.txt"
            ;;
    esac

    # Calculate actual checksum
    local actual_checksum
    actual_checksum=$(cd "$tmp_dir" && $CHECKSUM_CMD "$archive_name" | awk '{print $1}')

    debug "Expected checksum: $expected_checksum"
    debug "Actual checksum:   $actual_checksum"

    if [ "$expected_checksum" != "$actual_checksum" ]; then
        error "Checksum verification failed! The downloaded file may be corrupted or tampered with."
    fi

    success "Checksum verified"

    # Extract archive
    info "Extracting archive..."
    tar -xzf "${tmp_dir}/${archive_name}" -C "$tmp_dir"

    # Create install directory if needed
    if [ ! -d "$INSTALL_DIR" ]; then
        info "Creating install directory: $INSTALL_DIR"
        mkdir -p "$INSTALL_DIR"
    fi

    # Install binary
    info "Installing to ${INSTALL_DIR}/${BINARY_NAME}..."

    if [ -f "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        debug "Removing existing binary"
        rm -f "${INSTALL_DIR}/${BINARY_NAME}"
    fi

    mv "${tmp_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
    chmod +x "${INSTALL_DIR}/${BINARY_NAME}"

    success "Successfully installed ${BINARY_NAME} ${VERSION}"
}

#######################################
# Check PATH and provide instructions
#######################################
check_path() {
    case ":${PATH}:" in
        *":${INSTALL_DIR}:"*)
            return
            ;;
    esac

    warn "${INSTALL_DIR} is not in your PATH"
    echo ""
    echo "Add it to your shell configuration:"
    echo ""

    # Detect shell
    local shell_name
    shell_name=$(basename "${SHELL:-/bin/sh}")

    case "$shell_name" in
        zsh)
            echo "  echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.zshrc"
            echo "  source ~/.zshrc"
            ;;
        bash)
            if [ "$(uname -s)" = "Darwin" ]; then
                echo "  echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.bash_profile"
                echo "  source ~/.bash_profile"
            else
                echo "  echo 'export PATH=\"${INSTALL_DIR}:\$PATH\"' >> ~/.bashrc"
                echo "  source ~/.bashrc"
            fi
            ;;
        *)
            echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
            ;;
    esac

    echo ""
}

#######################################
# Verify installation
#######################################
verify_installation() {
    if [ -x "${INSTALL_DIR}/${BINARY_NAME}" ]; then
        local installed_version
        installed_version=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null || echo "unknown")
        success "Installation complete: ${BINARY_NAME} ${installed_version}"
    else
        error "Installation verification failed"
    fi
}

#######################################
# Main
#######################################
main() {
    parse_args "$@"

    echo ""
    echo "  ${BINARY_NAME} installer"
    echo "  ========================"
    echo ""

    check_dependencies
    detect_platform
    get_latest_version
    check_existing_installation
    download_and_install
    check_path
    verify_installation

    echo ""
    echo "Run '${BINARY_NAME} --help' to get started."
    echo ""
}

main "$@"
