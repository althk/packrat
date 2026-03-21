#!/usr/bin/env bash
set -euo pipefail

# Packrat installer
# Usage: curl -sSL https://raw.githubusercontent.com/<user>/packrat/main/scripts/install.sh | bash

REPO="harish/packrat"
INSTALL_DIR="/usr/local/bin"

detect_os() {
    case "$(uname -s)" in
        Linux*)  echo "linux";;
        Darwin*) echo "darwin";;
        *)       echo "unsupported"; exit 1;;
    esac
}

detect_arch() {
    case "$(uname -m)" in
        x86_64)  echo "amd64";;
        aarch64) echo "arm64";;
        arm64)   echo "arm64";;
        *)       echo "unsupported"; exit 1;;
    esac
}

main() {
    local os=$(detect_os)
    local arch=$(detect_arch)

    echo "Detecting system: ${os}/${arch}"

    local latest=$(curl -sSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | sed -E 's/.*"([^"]+)".*/\1/')

    if [ -z "$latest" ]; then
        echo "Error: could not determine latest version"
        exit 1
    fi

    echo "Latest version: ${latest}"

    local url="https://github.com/${REPO}/releases/download/${latest}/packrat-${os}-${arch}.tar.gz"
    local tmp=$(mktemp -d)

    echo "Downloading ${url}..."
    curl -sSL "$url" -o "${tmp}/packrat.tar.gz"

    echo "Extracting..."
    tar xzf "${tmp}/packrat.tar.gz" -C "${tmp}"

    echo "Installing to ${INSTALL_DIR}/packrat..."
    sudo mv "${tmp}/packrat" "${INSTALL_DIR}/packrat"
    sudo chmod +x "${INSTALL_DIR}/packrat"

    rm -rf "${tmp}"

    echo "Done! Run 'packrat init' to get started."
}

main "$@"
