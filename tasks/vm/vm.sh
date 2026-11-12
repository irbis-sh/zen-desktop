#!/usr/bin/env bash
# Manages local virtual machines for testing Zen on other operating systems.
# Workflow and pitfalls: .agents/skills/testing-in-vms/SKILL.md
#
# Each guest OS has a backend file next to this one, <os>.sh, which defines:
#   <os>_up, <os>_down, <os>_reset
#   <os>_connect       sets the GUEST_* variables below, or fails if the guest is not running
#   <os>_gui_command   prints the guest command line that runs "$1" in the desktop session

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Holds the SSH key, logs, screenshots, the vncdotool install, and the disk
# images of the guests that run under QEMU.
VM_HOME="${ZEN_VM_HOME:-$HOME/.zen-vms}"
SSH_KEY="$VM_HOME/id_ed25519"
# Guests are recreated often and are only reachable from this machine, so
# their host keys are not worth tracking.
SSH_OPTS=(-i "$SSH_KEY" -o IdentitiesOnly=yes -o StrictHostKeyChecking=no
  -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR -o ConnectTimeout=10)

GUEST_USER=
GUEST_HOST=
GUEST_SSH_PORT=
GUEST_VNC_PORT=
GUEST_VNC_PASSWORD=
# Non-empty for VNC servers that ignore shift unless it is sent as its own key press.
GUEST_VNC_FORCE_CAPS=

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

Supported: macos, linux.
EOF
  exit 2
}

die() {
  echo "vm: $*" >&2
  exit 1
}

# Runs a provisioning script inside a guest. The remaining arguments say how to
# reach it, in ssh's own syntax. The tool versions come from the same files CI
# reads them from.
provision() {
  local script=$1 go_version node_version wails_version
  shift
  go_version=$(sed -n 's/^go //p' "$REPO_ROOT/go.mod")
  wails_version=$(sed -n 's|.*github.com/wailsapp/wails/v2 \(v[^ ]*\).*|\1|p' "$REPO_ROOT/go.mod")
  node_version=$(sed -n 's/.*"node": *">=\([0-9.]*\)".*/\1/p' "$REPO_ROOT/frontend/package.json")
  ssh "${SSH_OPTS[@]}" "$@" bash -s -- "$go_version" "$node_version" "$wails_version" <"$SCRIPT_DIR/$script"
}

guest_ssh() {
  "${os}_connect"
  ssh "${SSH_OPTS[@]}" -p "$GUEST_SSH_PORT" "$GUEST_USER@$GUEST_HOST" "$@"
}

guest_gui() {
  local quoted
  [ $# -gt 0 ] || usage
  printf -v quoted '%q' "$*"
  guest_ssh "$("${os}_gui_command" "$quoted")"
}

guest_sync() {
  local tag
  "${os}_connect"
  # node_modules holds platform-specific binaries, so each guest installs its own.
  rsync -a --delete --exclude .git --exclude node_modules --exclude /.claude --exclude /build/bin \
    -e "ssh ${SSH_OPTS[*]} -p $GUEST_SSH_PORT" "$REPO_ROOT/" "$GUEST_USER@$GUEST_HOST:zen/"
  # The build Taskfiles run `git describe`, but .git is not copied: in a worktree
  # it is a file that points to a path on the host. A stub repository that
  # carries the host's tag is enough for them.
  tag=$(git -C "$REPO_ROOT" describe --tags --always --abbrev=0)
  guest_ssh "cd zen && { [ -d .git ] || { git init -q && git -c user.name=vm -c user.email=vm@localhost \
    commit -q --allow-empty -m 'Stub for git describe'; }; } && git tag -f '$tag' >/dev/null"
}

guest_vnc() {
  local args
  "${os}_connect"
  if [ ! -x "$VM_HOME/vnc-venv/bin/vncdo" ]; then
    python3 -m venv "$VM_HOME/vnc-venv"
    "$VM_HOME/vnc-venv/bin/pip" install --quiet --disable-pip-version-check vncdotool==1.4.2 >&2
  fi
  # The VNC server belongs to the hypervisor, so it listens on the host, not in the guest.
  args=(-s "127.0.0.1::$GUEST_VNC_PORT")
  [ -z "$GUEST_VNC_PASSWORD" ] || args+=(-p "$GUEST_VNC_PASSWORD")
  [ -z "$GUEST_VNC_FORCE_CAPS" ] || args+=(--force-caps)
  PYTHONWARNINGS=ignore "$VM_HOME/vnc-venv/bin/vncdo" "${args[@]}" "$@"
}

guest_shot() {
  local file=${1:-$VM_HOME/shots/$os-$(date +%Y%m%d-%H%M%S).png}
  guest_vnc capture "$file"
  echo "$file"
}

[ $# -ge 2 ] || usage
command=$1 os=$2
shift 2

case "$os" in
  macos | linux) ;;
  *) die "unsupported OS '$os'. Supported: macos, linux" ;;
esac
# shellcheck source=/dev/null
. "$SCRIPT_DIR/$os.sh"

mkdir -p "$VM_HOME/shots"
[ -f "$SSH_KEY" ] || ssh-keygen -q -t ed25519 -N '' -C zen-vms -f "$SSH_KEY"

case "$command" in
  up) "${os}_up" ;;
  down) "${os}_down" ;;
  reset) "${os}_reset" ;;
  ssh) guest_ssh "$@" ;;
  gui) guest_gui "$@" ;;
  sync) guest_sync ;;
  shot) guest_shot "$@" ;;
  vnc) guest_vnc "$@" ;;
  *) usage ;;
esac
