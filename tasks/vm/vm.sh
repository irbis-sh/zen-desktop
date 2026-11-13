#!/usr/bin/env bash
# Manages local virtual machines for testing Zen on other operating systems.
# Workflow and pitfalls: .agents/skills/testing-in-vms/SKILL.md
#
# A guest is named <os>:<name>. All guests of one OS start as copies of that
# OS's base, which is built once.
#
# Each guest OS has a backend file next to this one, <os>.sh, which defines:
#   <os>_base_exists, <os>_build_base
#   <os>_up            creates the guest from the base if needed and starts it
#   <os>_down
#   <os>_discard       deletes the guest's disk; the guest is not running
#   <os>_running
#   <os>_connect       sets the GUEST_* variables below, or fails if the guest is not running
#   <os>_gui_command   prints the guest command line that runs "$1" in the desktop session
#   <os>_sync          optional, replaces guest_sync for guests without rsync

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

usage() {
  cat >&2 <<EOF
Usage: vm.sh <command> <guest> [args]

  up <guest>              Create the guest if needed, start it, wait for SSH.
  down <guest>            Shut the guest down. Its disk is kept.
  reset <guest>           Discard the guest's disk and start it again from the clean base.
  rm <guest>              Shut the guest down and delete it.
  list                    List the guests with their state and checkout.
  ssh <guest> [args]      Open a shell, run a command, or pass ssh options (-L, -N).
  gui <guest> <command>   Run a command in the guest's desktop session, where it can show dialogs.
  sync <guest>            Copy this checkout to ~/zen in the guest.
  shot <guest> [file]     Save a screenshot and print its path.
  vnc <guest> <actions>   Send vncdo actions, e.g. move 512 384 click 1, type hello, key enter.

<guest> is macos, linux or windows, which means the guest of this checkout,
or <os>:<name> for any other guest.
EOF
  exit 2
}

die() {
  echo "vm: $*" >&2
  exit 1
}

# Reads the versions of the build tools from the same files CI reads them from.
read_tool_versions() {
  GO_VERSION=$(sed -n 's/^go //p' "$REPO_ROOT/go.mod")
  WAILS_VERSION=$(sed -n 's|.*github.com/wailsapp/wails/v2 \(v[^ ]*\).*|\1|p' "$REPO_ROOT/go.mod")
  NODE_VERSION=$(sed -n 's/.*"node": *">=\([0-9.]*\)".*/\1/p' "$REPO_ROOT/frontend/package.json")
}

# Runs a provisioning script inside a guest. The remaining arguments say how to
# reach it, in ssh's own syntax.
provision() {
  local script=$1
  shift
  read_tool_versions
  ssh "${SSH_OPTS[@]}" "$@" bash -s -- "$GO_VERSION" "$NODE_VERSION" "$WAILS_VERSION" <"$SCRIPT_DIR/$script"
}

# Builds the base of this OS unless it exists. Sessions that ask for a guest
# at the same time would otherwise each start a build, and the later one would
# shut the earlier one's build down as a leftover.
ensure_base() {
  local lock="$VM_HOME/$os/base.lock" waiting=
  ! "${os}_base_exists" || return 0
  # A symbolic link is created in one step, unlike a file and its content, so
  # the lock never exists without the ID of the process that holds it.
  until ln -s "$$" "$lock" 2>/dev/null; do
    if ! kill -0 "$(readlink "$lock")" 2>/dev/null; then
      rm -f "$lock"
      continue
    fi
    [ -n "$waiting" ] || echo "Waiting for another session to finish building the $os base." >&2
    waiting=1
    sleep 5
  done
  trap 'rm -f "$VM_HOME/$os/base.lock"' EXIT
  "${os}_base_exists" || "${os}_build_base"
  rm -f "$lock"
  trap - EXIT
}

guest_ssh() {
  "${os}_connect"
  ssh "${SSH_OPTS[@]}" -p "$GUEST_SSH_PORT" "$GUEST_USER@$GUEST_HOST" "$@"
}

guest_gui() {
  local quoted
  [ $# -gt 0 ] || usage
  printf -v quoted '%q' "$*"
  # -n: an ssh that can read the terminal is suspended once it is put in the background.
  guest_ssh -n "$("${os}_gui_command" "$quoted")"
}

guest_sync() {
  "${os}_connect"
  # node_modules holds platform-specific binaries, so each guest installs its
  # own. package.json.md5 is Wails' record of which package.json the
  # node_modules next to it were installed from, so it stays with them.
  rsync -a --delete --exclude .git --exclude node_modules --exclude /frontend/package.json.md5 \
    --exclude /.claude --exclude /build/bin \
    -e "ssh ${SSH_OPTS[*]} -p $GUEST_SSH_PORT" "$REPO_ROOT/" "$GUEST_USER@$GUEST_HOST:zen/"
  guest_sync_git_stub
}

# The build Taskfiles run `git describe`, but .git is not copied: in a worktree
# it is a file that points to a path on the host. A stub repository that
# carries the host's tag is enough for them. It is made anew each time: with
# the tags of earlier syncs on the same commit, `git describe` would pick
# among them by name.
guest_sync_git_stub() {
  local tag
  tag=$(git -C "$REPO_ROOT" describe --tags --always --abbrev=0)
  guest_ssh "cd zen && rm -rf .git && git init -q && git -c user.name=vm -c user.email=vm@localhost \
    commit -q --allow-empty -m 'Stub for git describe' && git tag '$tag'"
}

# Installs vncdotool on first use. Separate from guest_vnc for callers that
# have to send a key within seconds of starting a guest.
vnc_install() {
  [ ! -x "$VM_HOME/vnc-venv/bin/vncdo" ] || return 0
  python3 -m venv "$VM_HOME/vnc-venv"
  "$VM_HOME/vnc-venv/bin/pip" install --quiet --disable-pip-version-check vncdotool==1.4.2 >&2
  # The first start of a new install takes several seconds.
  "$VM_HOME/vnc-venv/bin/vncdo" --help >/dev/null
}

guest_vnc() {
  local args
  "${os}_connect"
  vnc_install
  # The VNC server belongs to the hypervisor, so it listens on the host, not in the guest.
  # Without --force-caps both Tart's and QEMU's VNC servers ignore shift: ":"
  # arrives as ";" and "|" as "\".
  args=(-s "127.0.0.1::$GUEST_VNC_PORT" --force-caps)
  [ -z "$GUEST_VNC_PASSWORD" ] || args+=(-p "$GUEST_VNC_PASSWORD")
  PYTHONWARNINGS=ignore "$VM_HOME/vnc-venv/bin/vncdo" "${args[@]}" "$@"
}

guest_shot() {
  local file=${1:-$VM_HOME/shots/$os-$name-$(date +%Y%m%d-%H%M%S).png}
  guest_vnc capture "$file"
  echo "$file"
}

guest_list() {
  local dir
  for dir in "$VM_HOME"/*/guests/*/; do
    [ -d "$dir" ] || continue
    # In a subshell, because each backend is written for one guest per run.
    (
      GUEST_DIR=${dir%/}
      name=$(basename "$GUEST_DIR")
      os=$(basename "$(dirname "$(dirname "$GUEST_DIR")")")
      # shellcheck source=/dev/null
      . "$SCRIPT_DIR/$os.sh"
      state=stopped
      ! "${os}_running" || state=running
      checkout=$(cat "$GUEST_DIR/checkout" 2>/dev/null || echo -)
      [ -d "$checkout" ] || [ "$checkout" = - ] || checkout="$checkout (deleted)"
      printf '%-32s %-8s %s\n' "$os:$name" "$state" "$checkout"
    )
  done
}

[ $# -ge 1 ] || usage
command=$1
if [ "$command" = list ]; then
  guest_list
  exit
fi
[ $# -ge 2 ] || usage
guest=$2
shift 2

os=${guest%%:*}
case "$os" in
  macos | linux | windows) ;;
  *) die "unsupported OS '$os'. Supported: macos, linux, windows" ;;
esac
# Sessions that work in different checkouts do not know of each other, and
# `sync` makes a guest a copy of one checkout. So unless told otherwise, each
# checkout gets its own guest, named after its directory.
name=$(basename "$REPO_ROOT")
[ "$guest" = "$os" ] || name=${guest#*:}
[[ "$name" =~ ^[A-Za-z0-9][A-Za-z0-9._-]*$ ]] ||
  die "'$name' cannot be used as a guest name. Give one made of letters, digits, '.', '_' and '-': vm.sh $command $os:<name>"
# The machine that builds a base is kept like a guest, so that a build that
# hangs or fails can be looked at with the commands below.
case "$name:$command" in
  base:* | base-build:up | base-build:reset)
    die "the names base and base-build belong to the base and the machine that builds it. Pick another: vm.sh $command $os:<name>"
    ;;
esac
GUEST_DIR="$VM_HOME/$os/guests/$name"
# Two checkouts can have directories of the same name.
if [ "$guest" = "$os" ] && [ -f "$GUEST_DIR/checkout" ] && [ "$(cat "$GUEST_DIR/checkout")" != "$REPO_ROOT" ]; then
  die "the guest $os:$name belongs to the checkout at $(cat "$GUEST_DIR/checkout"). Name it to use it from here, or pick another name: vm.sh $command $os:<name>"
fi
# shellcheck source=/dev/null
. "$SCRIPT_DIR/$os.sh"

mkdir -p "$VM_HOME/shots"
[ -f "$SSH_KEY" ] || ssh-keygen -q -t ed25519 -N '' -C zen-vms -f "$SSH_KEY"

case "$command" in
  up | reset)
    mkdir -p "$GUEST_DIR"
    [ -f "$GUEST_DIR/checkout" ] || echo "$REPO_ROOT" >"$GUEST_DIR/checkout"
    if [ "$command" = reset ]; then
      "${os}_down" || true
      "${os}_discard"
    fi
    "${os}_up"
    ;;
  down) "${os}_down" ;;
  rm)
    "${os}_down" || true
    "${os}_discard"
    rm -rf "$GUEST_DIR"
    ;;
  ssh) guest_ssh "$@" ;;
  gui) guest_gui "$@" ;;
  sync) if declare -F "${os}_sync" >/dev/null; then "${os}_sync"; else guest_sync; fi ;;
  shot) guest_shot "$@" ;;
  vnc) guest_vnc "$@" ;;
  *) usage ;;
esac
