#!/usr/bin/env bash
# Manages local virtual machines for testing Zen on other operating systems.
# Workflow and pitfalls: .agents/skills/testing-in-vms/SKILL.md

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Disk images stay in the VM tool's own storage. This directory holds the SSH
# key, logs, screenshots and the vncdotool install.
VM_HOME="${ZEN_VM_HOME:-$HOME/.zen-vms}"
SSH_KEY="$VM_HOME/id_ed25519"
# Guests are recreated often and are only reachable from this machine, so
# their host keys are not worth tracking.
SSH_OPTS=(-i "$SSH_KEY" -o IdentitiesOnly=yes -o StrictHostKeyChecking=no
  -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=10)

MACOS_IMAGE="${ZEN_VM_MACOS_IMAGE:-ghcr.io/cirruslabs/macos-tahoe-vanilla:latest}"
MACOS_BASE=zen-macos-base
MACOS_VM=zen-macos
# Set by the image.
MACOS_USER="admin"
MACOS_PASSWORD="admin"

usage() {
  cat >&2 <<EOF
Usage: vm.sh <command> <os> [args]

  up <os>              Create the guest if needed, start it, wait for SSH.
  down <os>            Shut the guest down.
  reset <os>           Discard the guest and recreate it from the clean base.
  ssh <os> [args]      Open a shell, run a command, or pass ssh options (-L, -N).
  gui <os> <command>   Run a command in the guest's desktop session, where it can show dialogs.
  sync <os>            Copy this checkout to ~/zen in the guest.
  shot <os> [file]     Save a screenshot and print its path.
  vnc <os> <actions>   Send vncdo actions, e.g. move 512 384 click 1, type hello, key enter.

Supported: macos.
EOF
  exit 2
}

die() {
  echo "vm: $*" >&2
  exit 1
}

init_home() {
  mkdir -p "$VM_HOME/shots"
  [ -f "$SSH_KEY" ] || ssh-keygen -q -t ed25519 -N '' -C zen-vms -f "$SSH_KEY"
}

# These two capture the output instead of piping it into grep -q: grep exits on
# the first match, and under pipefail the writer's SIGPIPE would read as "no".
macos_exists() {
  local names
  names=$(tart list --quiet --source local)
  grep -qx "$1" <<<"$names"
}

macos_running() {
  local config
  config=$(tart get "$1" --format json 2>/dev/null) || return 1
  grep -q '"Running" : true' <<<"$config"
}

macos_ip() {
  macos_running "$MACOS_VM" || die "macos is not running. Run: vm.sh up macos"
  tart ip "$MACOS_VM"
}

# Starts a VM headless and waits until SSH answers.
macos_start() {
  local name=$1 log="$VM_HOME/$1.log" ip
  # Tart's VNC server works below the guest OS, so it also shows the login
  # window and password dialogs. It picks a new port and password on each start
  # and prints them to the log.
  nohup tart run "$name" --no-graphics --vnc-experimental >"$log" 2>&1 &
  for _ in $(seq 30); do
    grep -q 'vnc://' "$log" && break
    sleep 1
  done
  grep -q 'vnc://' "$log" || die "$name did not start: $(cat "$log")"
  ip=$(tart ip "$name" --wait 120)
  for _ in $(seq 60); do
    nc -z -G 2 "$ip" 22 2>/dev/null && return
    sleep 2
  done
  die "$name is running but SSH at $ip is unreachable. If the error is 'No route to host'," \
    "allow your terminal app under System Settings > Privacy & Security > Local Network."
}

macos_stop() {
  local ip
  macos_running "$1" || return 0
  # Shut down from inside the guest. `tart stop` cuts the power at once, and
  # files written in the last seconds before it are lost.
  if ip=$(tart ip "$1" 2>/dev/null); then
    ssh "${SSH_OPTS[@]}" "$MACOS_USER@$ip" sudo shutdown -h now >/dev/null 2>&1 || true
    for _ in $(seq 60); do
      macos_running "$1" || return 0
      sleep 1
    done
  fi
  tart stop "$1"
  # A clone or delete right after stop can fail while the VM still holds its lock.
  for _ in $(seq 15); do
    macos_running "$1" || return 0
    sleep 1
  done
}

# Builds the base VM: the public image plus our SSH key and the build tools.
# Guests are cheap copy-on-write clones of it.
macos_build_base() {
  local building="$MACOS_BASE-building" askpass="$VM_HOME/askpass" ip go_version node_version wails_version
  echo "Creating $MACOS_BASE from $MACOS_IMAGE. The first run downloads about 24 GB." >&2
  # Build under a temporary name so that a failed run is never mistaken for a finished base.
  if macos_exists "$building"; then
    macos_stop "$building"
    tart delete "$building"
  fi
  tart clone "$MACOS_IMAGE" "$building"
  # The image's 1024x768 display is too short for Zen's window. Pixels rather than
  # points turn off 2x scaling, which keeps screenshots small.
  tart set "$building" --display 1440x900px
  macos_start "$building"
  ip=$(tart ip "$building")

  # The image only accepts its default password, so the first login uses it to install the key.
  printf '#!/bin/sh\necho %s\n' "$MACOS_PASSWORD" >"$askpass"
  chmod 700 "$askpass"
  SSH_ASKPASS="$askpass" SSH_ASKPASS_REQUIRE=force ssh "${SSH_OPTS[@]}" \
    -o PubkeyAuthentication=no -o PreferredAuthentications=password,keyboard-interactive \
    "$MACOS_USER@$ip" 'mkdir -p ~/.ssh && chmod 700 ~/.ssh && cat >>~/.ssh/authorized_keys' <"$SSH_KEY.pub"

  # Same sources of truth as CI.
  go_version=$(sed -n 's/^go //p' "$REPO_ROOT/go.mod")
  wails_version=$(sed -n 's|.*github.com/wailsapp/wails/v2 \(v[^ ]*\).*|\1|p' "$REPO_ROOT/go.mod")
  node_version=$(sed -n 's/.*"node": *">=\([0-9.]*\)".*/\1/p' "$REPO_ROOT/frontend/package.json")
  ssh "${SSH_OPTS[@]}" "$MACOS_USER@$ip" bash -s -- "$go_version" "$node_version" "$wails_version" \
    <"$SCRIPT_DIR/provision-macos.sh"

  macos_stop "$building"
  tart rename "$building" "$MACOS_BASE"
}

macos_up() {
  command -v tart >/dev/null || die "tart not found. Install: brew install openai/tools/tart"
  if ! macos_exists "$MACOS_VM"; then
    macos_exists "$MACOS_BASE" || macos_build_base
    tart clone "$MACOS_BASE" "$MACOS_VM"
  fi
  if ! macos_running "$MACOS_VM"; then
    macos_start "$MACOS_VM"
    # The first VNC connection after a start only ever sees a black 1280x720
    # frame, so use it up here.
    macos_vnc pause 1
    # Tart's VNC server crashes, and takes the VM down with it, on the first
    # connection made after a minute or two without any client. Holding one
    # connection open for as long as the VM runs avoids that. It ends by itself
    # when the VM stops.
    nohup "$SCRIPT_DIR/vm.sh" vnc macos pause 10000000 >/dev/null 2>&1 &
  fi
  echo "macos: ssh $MACOS_USER@$(tart ip "$MACOS_VM"), $(grep -o 'vnc://.*' "$VM_HOME/$MACOS_VM.log")"
}

macos_reset() {
  macos_stop "$MACOS_VM"
  ! macos_exists "$MACOS_VM" || tart delete "$MACOS_VM"
  macos_up
}

macos_ssh() {
  local ip
  ip=$(macos_ip)
  ssh "${SSH_OPTS[@]}" "$MACOS_USER@$ip" "$@"
}

macos_gui() {
  local quoted
  [ $# -gt 0 ] || usage
  printf -v quoted '%q' "$*"
  # An SSH session is outside the desktop session. A program started from it can
  # open windows, but a password dialog it triggers fails with "no user
  # interaction was possible". launchctl asuser moves the command into the
  # desktop session, and the inner sudo drops back from root to the user.
  macos_ssh "sudo launchctl asuser \$(id -u) sudo -H -u $MACOS_USER zsh -c $quoted"
}

macos_sync() {
  local ip tag
  ip=$(macos_ip)
  # node_modules holds platform-specific binaries, so each guest installs its own.
  rsync -a --delete --exclude .git --exclude node_modules --exclude /.claude --exclude /build/bin \
    -e "ssh ${SSH_OPTS[*]}" "$REPO_ROOT/" "$MACOS_USER@$ip:zen/"
  # The build Taskfiles run `git describe`, but .git is not copied: in a worktree
  # it is a file that points to a path on the host. A stub repository that
  # carries the host's tag is enough for them.
  tag=$(git -C "$REPO_ROOT" describe --tags --always --abbrev=0)
  macos_ssh "cd zen && { [ -d .git ] || { git init -q && git -c user.name=vm -c user.email=vm@localhost \
    commit -q --allow-empty -m 'Stub for git describe'; }; } && git tag -f '$tag' >/dev/null"
}

macos_vnc() {
  local url port password
  macos_running "$MACOS_VM" || die "macos is not running. Run: vm.sh up macos"
  url=$(grep -o 'vnc://.*' "$VM_HOME/$MACOS_VM.log")
  password=${url#vnc://:}
  password=${password%@*}
  port=${url##*:}
  if [ ! -x "$VM_HOME/vnc-venv/bin/vncdo" ]; then
    python3 -m venv "$VM_HOME/vnc-venv"
    "$VM_HOME/vnc-venv/bin/pip" install --quiet --disable-pip-version-check vncdotool==1.4.2 >&2
  fi
  # --force-caps: without it Tart's VNC server ignores shift, so "A" arrives as "a" and "|" as "\".
  PYTHONWARNINGS=ignore "$VM_HOME/vnc-venv/bin/vncdo" --force-caps -s "127.0.0.1::$port" -p "$password" "$@"
}

macos_shot() {
  local file=${1:-$VM_HOME/shots/macos-$(date +%Y%m%d-%H%M%S).png}
  macos_vnc capture "$file"
  echo "$file"
}

[ $# -ge 2 ] || usage
command=$1 os=$2
shift 2

[ "$os" = macos ] || die "unsupported OS '$os'. Supported: macos"
init_home

case "$command" in
  up) macos_up ;;
  down) macos_stop "$MACOS_VM" ;;
  reset) macos_reset ;;
  ssh) macos_ssh "$@" ;;
  gui) macos_gui "$@" ;;
  sync) macos_sync ;;
  shot) macos_shot "$@" ;;
  vnc) macos_vnc "$@" ;;
  *) usage ;;
esac
