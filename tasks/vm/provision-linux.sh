#!/bin/bash
# Runs once inside a fresh Ubuntu guest, piped over SSH by vm.sh. Turns the
# server cloud image into a desktop that logs in by itself and never locks,
# and installs what building Zen needs.

set -euo pipefail

# The script arrives on stdin. Wrapping it in a function makes bash read all of
# it before running anything, so a command that reads stdin cannot swallow the rest.
main() {
  go_version=$1
  node_version=$2
  wails_version=$3

  export DEBIAN_FRONTEND=noninteractive

  # Background upgrades hold the apt lock at unpredictable times and would
  # change the guest between two runs of the same test.
  sudo systemctl disable --now unattended-upgrades.service apt-daily.timer apt-daily-upgrade.timer

  # Ubuntu's firefox package only installs a snap, and it fails the whole apt
  # run when the snap store cannot be reached. Mozilla's repository has a
  # regular package; the pin makes apt prefer it.
  sudo install -d -m 0755 /etc/apt/keyrings
  curl -fsSL https://packages.mozilla.org/apt/repo-signing-key.gpg |
    sudo tee /etc/apt/keyrings/packages.mozilla.org.asc >/dev/null
  echo 'deb [signed-by=/etc/apt/keyrings/packages.mozilla.org.asc] https://packages.mozilla.org/apt mozilla main' |
    sudo tee /etc/apt/sources.list.d/mozilla.list >/dev/null
  printf 'Package: *\nPin: origin packages.mozilla.org\nPin-Priority: 1000\n' |
    sudo tee /etc/apt/preferences.d/mozilla >/dev/null

  sudo apt-get update --quiet
  # The build packages mirror the deps task in tasks/build/Taskfile-linux.yml.
  # libnss3-tools provides certutil, which Zen needs to add its CA to Firefox.
  sudo apt-get install --yes --quiet ubuntu-desktop-minimal firefox libnss3-tools \
    gcc pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev libxapp-dev
  # Its first-login wizard would cover the desktop.
  sudo apt-get purge --yes --quiet gnome-initial-setup

  sudo tee /etc/gdm3/custom.conf >/dev/null <<EOF
[daemon]
AutomaticLoginEnable=true
AutomaticLogin=$USER
EOF

  # System-wide defaults rather than gsettings: no desktop session exists yet
  # to take the settings.
  sudo mkdir -p /etc/dconf/profile /etc/dconf/db/local.d
  printf 'user-db:user\nsystem-db:local\n' | sudo tee /etc/dconf/profile/user >/dev/null
  sudo tee /etc/dconf/db/local.d/00-zen-vm >/dev/null <<EOF
[org/gnome/desktop/session]
idle-delay=uint32 0

[org/gnome/desktop/screensaver]
lock-enabled=false

# "Updates available" pop-ups would cover whatever is being tested.
[com/ubuntu/update-notifier]
no-show-notifications=true
EOF
  sudo dconf update

  # /usr/local/bin is on the default PATH of SSH commands and of the desktop
  # session alike, so the tools are linked there instead of extending PATH.
  arch=$(dpkg --print-architecture)
  curl -fsSL "https://go.dev/dl/go$go_version.linux-$arch.tar.gz" | sudo tar -C /usr/local -xz
  sudo ln -sf /usr/local/go/bin/go /usr/local/go/bin/gofmt /usr/local/bin/

  # The newest release of the major version that frontend/package.json asks for.
  # Node names the architecture x64 where Debian says amd64.
  node_dist="https://nodejs.org/dist/latest-v${node_version%%.*}.x"
  node_archive=$(curl -fsSL "$node_dist/SHASUMS256.txt" | awk -v a="${arch/amd64/x64}" '$2 ~ "linux-" a "\\.tar\\.xz$" { print $2 }')
  sudo mkdir -p /usr/local/node
  curl -fsSL "$node_dist/$node_archive" | sudo tar -C /usr/local/node -xJ --strip-components 1
  sudo ln -sf /usr/local/node/bin/node /usr/local/node/bin/npm /usr/local/node/bin/npx /usr/local/bin/

  go install "github.com/wailsapp/wails/v2/cmd/wails@$wails_version"
  go install github.com/go-task/task/v3/cmd/task@latest
  sudo ln -sf ~/go/bin/wails ~/go/bin/task /usr/local/bin/

  # The seed disk is only attached for this first boot. Without it cloud-init
  # would search for a configuration source on every later boot.
  sudo touch /etc/cloud/cloud-init.disabled

  echo "provision: done ($(go version | cut -d' ' -f3), node $(node --version), wails $wails_version)"
}

main "$@"
