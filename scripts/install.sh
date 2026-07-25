#!/usr/bin/env sh
set -eu

# Downloads the latest Zen tarball and installs the binary into ~/.local/bin/
# If you'd prefer to do this manually, you can find the latest release at
# https://github.com/irbis-sh/zen-desktop/releases or alternatively at
# https://irbis.sh/zen/#downloads
# 
# This installer provides a prebuilt glibc binary and will not work
# on distributions that do not have a system-wide glibc by default.
# That includes NixOS and musl based distributions.

main() {
    if [ "${1:-}" = "--uninstall" ]; then
        uninstall
    fi

    platform="$(uname -s)"
    arch="$(uname -m)"

    if [ "$platform" = "Linux" ]; then
        platform="linux"
    else
        echo "[ERROR] This script only supports Linux (detected: $platform)">&2
        exit 1
    fi

    if [ "$arch" = "x86_64" ]; then
        arch="amd64"
    elif [ "$arch" = "aarch64" ]; then
        arch="arm64"
    else
        echo "[ERROR] Unsupported architecture $arch">&2
        exit 1
    fi

    # Support for both curl and wget
    # curl: quiet, for capturing output
    # download: shows progress, for the tarball
    if command -v curl >/dev/null 2>&1; then
        curl () {
            command curl -fsSL "$@"
        }
        download () {
            command curl -fL "$@"
        }
    elif command -v wget >/dev/null 2>&1; then
        curl () {
            wget -nv -O- "$@"
        }
        download () {
            wget -q --show-progress -O- "$@"
        }
    else
        echo "[ERROR] Could not find 'curl' or 'wget' in path">&2
        exit 1
    fi

    if ! manifest="$(curl "https://update-manifests.zenprivacy.net/stable/$platform/$arch/manifest.json")"; then
        echo "[ERROR] Failed to fetch update manifest for $platform: $arch" >&2
        exit 1
    fi

    asset_url="$(echo "$manifest" | sed -n 's/.*"assetURL"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
    sha256="$(echo "$manifest" | sed -n 's/.*"sha256"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"
    version="$(echo "$manifest" | sed -n 's/.*"version"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')"

    if [ -z "$asset_url" ] || [ -z "$sha256" ] || [ -z "$version" ]; then
        echo "[ERROR] Update manifest missing assetURL or sha256 or version" >&2
        exit 1
    fi

    tmpdir="$(mktemp -d)"
    trap 'rm -rf "$tmpdir"' EXIT INT TERM
    asset="$tmpdir/asset"

    echo "Installing Zen $version"
    if ! download "$asset_url" > "$asset"; then
        echo "[ERROR] Failed to download $asset_url" >&2
        exit 1
    fi

    actual_sha="$(sha256sum "$asset" | cut -d' ' -f1)"
    if [ "$actual_sha" != "$sha256" ]; then
        echo "[ERROR] Checksum mismatch for $asset_url" >&2
        exit 1
    fi
    echo

    mkdir "$tmpdir/extract"
    if ! tar -xzf "$asset" -C "$tmpdir/extract"; then
        echo "[ERROR] Failed to extract archive" >&2
        exit 1
    fi

    bindir="$HOME/.local/bin"
    mkdir -p "$bindir"
    cp "$tmpdir/extract/Zen" "$bindir/zen.new"
    chmod +x "$bindir/zen.new"
    mv "$bindir/zen.new" "$bindir/zen"

    echo "Installed Zen to $bindir/zen"

    icondir="$HOME/.local/share/pixmaps"
    mkdir -p "$icondir"
    icon_url="https://raw.githubusercontent.com/irbis-sh/zen-desktop/refs/tags/$version/assets/logo.png"
    if ! curl "$icon_url" > "$icondir/zen-adblocker.png"; then
        echo "[WARN] Failed to download icon" >&2
        rm -f "$icondir/zen-adblocker.png"
    fi

    appsdir="$HOME/.local/share/applications"
    cat > "$appsdir/zen-adblocker.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=Zen
Comment=An open-source system-wide ad-blocker and privacy guard
Exec=$bindir/zen
Icon=zen-adblocker
StartupWMClass=zen
EOF

    if command -v update-desktop-database >/dev/null 2>&1; then
        update-desktop-database "$appsdir" 2>/dev/null || true
    fi

    # Check runtime dependencies
    if command -v ldconfig >/dev/null 2>&1; then
        if ! ldconfig -p | grep -qi webkit2gtk-4.1; then
            echo "[WARN] Zen requires webkit2gtk-4.1 to run, install it running:">&2
            if command -v pacman >/dev/null 2>&1; then
                echo "sudo pacman -S webkit2gtk-4.1">&2
            elif command -v apt >/dev/null 2>&1; then
                echo "sudo apt install libwebkit2gtk-4.1-0">&2
            elif command -v dnf >/dev/null 2>&1; then
                echo "sudo dnf install webkit2gtk-4.1">&2
            else
                echo "please check your system's package manager">&2
            fi
        fi
        if ! ldconfig -p | grep -qi libgtk-3; then
            echo "[WARN] Zen requires gtk3 to run, install it running:">&2
            if command -v pacman >/dev/null 2>&1; then
                echo "sudo pacman -S gtk3">&2
            elif command -v apt >/dev/null 2>&1; then
                echo "sudo apt install libgtk-3-0t64">&2
            elif command -v dnf >/dev/null 2>&1; then
                echo "sudo dnf install gtk3">&2
            else
                echo "please check your system's package manager">&2
            fi
        fi
    fi
}

uninstall() {
    echo "Uninstalling Zen..."
    if pgrep -x zen >/dev/null 2>&1; then
        pkill -x zen 2>/dev/null || true
        sleep 1
    fi

    bindir="$HOME/.local/bin"
    icondir="$HOME/.local/share/pixmaps"
    appsdir="$HOME/.local/share/applications"

    if [ -x "$bindir/zen" ]; then
        "$bindir/zen" --uninstall-ca 2>/dev/null || true
    fi

    rm -f "$bindir/zen"
    rm -f "$appsdir/zen-adblocker.desktop"
    rm -f "$icondir/zen-adblocker.png"

    if command -v update-desktop-database >/dev/null 2>&1; then
        update-desktop-database "$appsdir" 2>/dev/null || true
    fi

    echo "Successfully Uninstalled Zen"
    exit 0
}

main "$@"
