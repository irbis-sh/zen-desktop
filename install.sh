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

bindir="$HOME/.local/bin"
icondir="$HOME/.local/share/icons/hicolor/scalable/apps"
appsdir="$HOME/.local/share/applications"

main() {
    platform="$(uname -s)"
    arch="$(uname -m)"

    if [ "$platform" = "Linux" ]; then
        platform="linux"
    else
        echo "[ERROR] This script only supports Linux (detected: $platform)">&2
        exit 1
    fi

    if [ "${1:-}" = "--uninstall" ]; then
        uninstall
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

    mkdir -p "$bindir"
    cp "$tmpdir/extract/Zen" "$bindir/zen.new"
    chmod +x "$bindir/zen.new"
    mv "$bindir/zen.new" "$bindir/zen"

    echo "Installed Zen to $bindir/zen"

    mkdir -p "$icondir"
    icon_url="https://raw.githubusercontent.com/irbis-sh/zen-desktop/refs/tags/$version/assets/logo.png"
    if ! curl "$icon_url" > "$icondir/zen.png"; then
        echo "[WARN] Failed to download icon" >&2
        rm -f "$icondir/zen.png"
    fi

    mkdir -p "$appsdir"
    cat > "$appsdir/zen-adblocker.desktop" <<EOF
[Desktop Entry]
Type=Application
Name=Zen
Comment=System-wide ad-blocker and privacy guard
Exec="$bindir/zen"
Icon=zen
StartupWMClass=zen
Categories=Network;Security;Utility;
Keywords=adblock;ad-block;privacy;proxy;
EOF

    if command -v update-desktop-database >/dev/null 2>&1; then
        update-desktop-database "$appsdir" 2>/dev/null || true
    fi

    # Check runtime dependencies
    ldconfig_bin="$(command -v ldconfig || echo /sbin/ldconfig)"
    if [ -x "$ldconfig_bin" ]; then
        ldcache="$("$ldconfig_bin" -p 2>/dev/null)"

        if ! echo "$ldcache" | grep -qi webkit2gtk-4.1; then
            echo "[WARN] Zen requires webkit2gtk-4.1 to run, install it running:">&2
            if command -v pacman >/dev/null 2>&1; then
                echo "sudo pacman -S webkit2gtk-4.1">&2
            elif command -v apt >/dev/null 2>&1; then
                echo "sudo apt install libwebkit2gtk-4.1-0">&2
            elif command -v dnf >/dev/null 2>&1; then
                echo "sudo dnf install webkit2gtk4.1">&2
            else
                echo "please check your system's package manager">&2
            fi
        fi
        if ! echo "$ldcache" | grep -qi libgtk-3; then
            echo "[WARN] Zen requires gtk3 to run, install it running:">&2
            if command -v pacman >/dev/null 2>&1; then
                echo "sudo pacman -S gtk3">&2
            elif command -v apt >/dev/null 2>&1; then
                echo "sudo apt install libgtk-3-0 || sudo apt install libgtk-3-0t64">&2
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
        i=1
        while [ "$i" -le 10 ] && pgrep -x zen >/dev/null 2>&1 ; do
            sleep 1
            i=$((i + 1))
        done
        if pgrep -x zen >/dev/null 2>&1; then
            echo "[ERROR] Failed to stop Zen; aborting uninstallation" >&2
            exit 1
        fi
    fi

    if [ -x "$bindir/zen" ]; then
        if trust list | grep -qi "zen personal ca"; then
            if ! "$bindir/zen" --uninstall-ca; then
                echo "Aborted Uninstallation of Zen">&2
                exit 1
            fi
        fi
    fi

    rm -f "$bindir/zen"
    rm -f "$appsdir/zen-adblocker.desktop"
    rm -f "$icondir/zen.png"

    if command -v update-desktop-database >/dev/null 2>&1; then
        update-desktop-database "$appsdir" 2>/dev/null || true
    fi

    echo "Successfully Uninstalled Zen"
    exit 0
}

main "$@"
