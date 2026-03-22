#!/usr/bin/env bash
set -euo pipefail

# Packrat installer
# Usage: curl -sSL https://raw.githubusercontent.com/althk/packrat/main/scripts/install.sh | bash

REPO="althk/packrat"
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

    install_completions

    echo "Done! Run 'packrat init' to get started."
}

detect_shell() {
    local current_shell
    current_shell="$(basename "${SHELL:-}")"
    case "$current_shell" in
        bash|zsh|fish) echo "$current_shell";;
        *) echo "";;
    esac
}

install_completions() {
    local shell_name
    shell_name="$(detect_shell)"

    if [ -z "$shell_name" ]; then
        echo "Skipping shell completions: unsupported shell"
        return
    fi

    echo "Installing ${shell_name} completions..."

    case "$shell_name" in
        bash)
            local bash_comp_dir="/etc/bash_completion.d"
            if [ -d "$bash_comp_dir" ]; then
                packrat completion bash | sudo tee "${bash_comp_dir}/packrat" > /dev/null
                echo "Bash completions installed to ${bash_comp_dir}/packrat"
            else
                local user_comp="${HOME}/.local/share/bash-completion/completions"
                mkdir -p "$user_comp"
                packrat completion bash > "${user_comp}/packrat"
                echo "Bash completions installed to ${user_comp}/packrat"
            fi
            ;;
        zsh)
            local zsh_comp_dir="${HOME}/.zsh/completions"
            mkdir -p "$zsh_comp_dir"
            packrat completion zsh > "${zsh_comp_dir}/_packrat"
            echo "Zsh completions installed to ${zsh_comp_dir}/_packrat"
            echo "Ensure 'fpath=(~/.zsh/completions \$fpath)' is in your .zshrc (before compinit)"
            ;;
        fish)
            local fish_comp_dir="${HOME}/.config/fish/completions"
            mkdir -p "$fish_comp_dir"
            packrat completion fish > "${fish_comp_dir}/packrat.fish"
            echo "Fish completions installed to ${fish_comp_dir}/packrat.fish"
            ;;
    esac
}

main "$@"
