#!/bin/bash
# Runs once inside a fresh macOS guest, piped over SSH by vm.sh, to install
# what building Zen needs. The image already logs in automatically and never
# sleeps or locks, so none of that is set up here.

set -euo pipefail

# The script arrives on stdin. Wrapping it in a function makes bash read all of
# it before running anything, so a command that reads stdin cannot swallow the rest.
main() {
  go_version=$1
  node_version=$2
  wails_version=$3

  # On some hosts the DNS server that macOS offers to VMs does not answer, which
  # leaves the guest with internet access but no name resolution.
  if [ -z "$(dig +short +time=2 +tries=2 example.com | grep -E '^[0-9.]+$')" ]; then
    echo "provision: the host's DNS does not answer, using public DNS servers instead"
    sudo networksetup -setdnsservers Ethernet 9.9.9.9 1.1.1.1
  fi

  # cgo needs Apple's compiler. Check the directory rather than run xcode-select
  # or clang: without the tools those open an install dialog on the guest's screen.
  if [ ! -d /Library/Developer/CommandLineTools/usr/bin ]; then
    # This marker makes softwareupdate list the Command Line Tools.
    marker=/tmp/.com.apple.dt.CommandLineTools.installondemand.in-progress
    touch "$marker"
    label=$(softwareupdate --list 2>/dev/null | sed -n 's/^\* Label: \(Command Line Tools.*\)$/\1/p' | sort -V | tail -1)
    [ -n "$label" ] || { echo "provision: no Command Line Tools offered by softwareupdate" >&2; exit 1; }
    sudo softwareupdate --install "$label"
    rm -f "$marker"
  fi

  curl -fsSL "https://go.dev/dl/go$go_version.darwin-arm64.tar.gz" | sudo tar -C /usr/local -xz

  # The newest release of the major version that frontend/package.json asks for.
  node_dist="https://nodejs.org/dist/latest-v${node_version%%.*}.x"
  node_archive=$(curl -fsSL "$node_dist/SHASUMS256.txt" | awk '/darwin-arm64\.tar\.gz$/ { print $2 }')
  sudo mkdir -p /usr/local/node
  curl -fsSL "$node_dist/$node_archive" | sudo tar -C /usr/local/node -xz --strip-components 1

  # Commands sent over SSH run in a non-interactive shell, which reads only ~/.zshenv.
  # shellcheck disable=SC2016 # $HOME and $PATH must expand in the guest's shell, not here.
  path_line='export PATH="/usr/local/go/bin:$HOME/go/bin:/usr/local/node/bin:$PATH"'
  grep -qxF "$path_line" ~/.zshenv 2>/dev/null || echo "$path_line" >>~/.zshenv
  eval "$path_line"

  go install "github.com/wailsapp/wails/v2/cmd/wails@$wails_version"
  go install github.com/go-task/task/v3/cmd/task@latest

  echo "provision: done ($(go version | cut -d' ' -f3), node $(node --version), wails $wails_version)"
}

main "$@"
