# macOS guests, run with Tart. Sourced by vm.sh.
# shellcheck shell=bash disable=SC2034,SC2154 # vm.sh reads the GUEST_* variables and sets os, name and guest.

MACOS_IMAGE="${ZEN_VM_MACOS_IMAGE:-ghcr.io/cirruslabs/macos-tahoe-vanilla:latest}"
MACOS_BASE=zen-macos-base
# The VM that the functions below act on, and the directory with its log.
# macos_build_base points them at the VM that builds the base.
MACOS_VM="zen-macos-$name"
MACOS_DIR=$GUEST_DIR
# Set by the image.
MACOS_USER="admin"
MACOS_PASSWORD="admin"

# These two capture the output instead of piping it into grep -q: grep exits on
# the first match, and under pipefail the writer's SIGPIPE would read as "no".
macos_exists() {
  local names
  names=$(tart list --quiet --source local)
  # -F: a guest name can hold a dot, which would otherwise match any character.
  grep -qxF "$1" <<<"$names"
}

macos_running() {
  local config
  config=$(tart get "$MACOS_VM" --format json 2>/dev/null) || return 1
  grep -q '"Running" : true' <<<"$config"
}

# Starts the VM headless and waits until SSH answers.
macos_start() {
  local log="$MACOS_DIR/tart.log" ip
  # Tart's VNC server works below the guest OS, so it also shows the login
  # window and password dialogs. It picks a new port and password on each start
  # and prints them to the log.
  # --no-audio: Tart otherwise plays the guest's sound on the host's speakers,
  # including the alert beep macOS makes for every key press nothing accepts.
  nohup tart run "$MACOS_VM" --no-graphics --no-audio --vnc-experimental >"$log" 2>&1 &
  for _ in $(seq 30); do
    grep -q 'vnc://' "$log" && break
    sleep 1
  done
  grep -q 'vnc://' "$log" || die "$MACOS_VM did not start: $(cat "$log")"
  ip=$(tart ip "$MACOS_VM" --wait 120)
  for _ in $(seq 60); do
    nc -z -G 2 "$ip" 22 2>/dev/null && return
    sleep 2
  done
  die "$MACOS_VM is running but SSH at $ip is unreachable. If the error is 'No route to host'," \
    "allow your terminal app under System Settings > Privacy & Security > Local Network."
}

macos_down() {
  local ip
  macos_running || return 0
  # Shut down from inside the guest. `tart stop` cuts the power at once, and
  # files written in the last seconds before it are lost.
  if ip=$(tart ip "$MACOS_VM" 2>/dev/null); then
    ssh "${SSH_OPTS[@]}" "$MACOS_USER@$ip" sudo shutdown -h now >/dev/null 2>&1 || true
    for _ in $(seq 60); do
      macos_running || return 0
      sleep 1
    done
  fi
  tart stop "$MACOS_VM"
  # A clone or delete right after stop can fail while the VM still holds its lock.
  for _ in $(seq 15); do
    macos_running || return 0
    sleep 1
  done
}

macos_base_exists() {
  macos_exists "$MACOS_BASE"
}

# Builds the base VM: the public image plus our SSH key and the build tools.
# Guests are cheap copy-on-write clones of it.
macos_build_base() {
  # Local variables are also seen by the functions called from here, so they
  # act on the VM that builds the base, not on the guest. It has a name of its
  # own so that a failed run is never mistaken for a finished base.
  local MACOS_VM=zen-macos-base-build MACOS_DIR="$VM_HOME/macos/guests/base-build"
  local askpass="$VM_HOME/askpass" ip
  echo "Creating $MACOS_BASE from $MACOS_IMAGE. The first run downloads about 24 GB." >&2
  if macos_exists "$MACOS_VM"; then
    macos_down
    tart delete "$MACOS_VM"
  fi
  rm -rf "$MACOS_DIR"
  mkdir -p "$MACOS_DIR"
  tart clone "$MACOS_IMAGE" "$MACOS_VM"
  # The image's 1024x768 display is too short for Zen's window. Pixels rather than
  # points turn off 2x scaling, which keeps screenshots small.
  tart set "$MACOS_VM" --display 1440x900px
  macos_start
  ip=$(tart ip "$MACOS_VM")

  # The image only accepts its default password, so the first login uses it to install the key.
  printf '#!/bin/sh\necho %s\n' "$MACOS_PASSWORD" >"$askpass"
  chmod 700 "$askpass"
  SSH_ASKPASS="$askpass" SSH_ASKPASS_REQUIRE=force ssh "${SSH_OPTS[@]}" \
    -o PubkeyAuthentication=no -o PreferredAuthentications=password,keyboard-interactive \
    "$MACOS_USER@$ip" 'mkdir -p ~/.ssh && chmod 700 ~/.ssh && cat >>~/.ssh/authorized_keys' <"$SSH_KEY.pub"

  provision provision-macos.sh "$MACOS_USER@$ip"

  macos_down
  tart rename "$MACOS_VM" "$MACOS_BASE"
  rm -rf "$MACOS_DIR"
}

macos_connect() {
  local url
  macos_running || die "$guest is not running. Run: vm.sh up $guest"
  GUEST_USER=$MACOS_USER
  GUEST_HOST=$(tart ip "$MACOS_VM")
  GUEST_SSH_PORT=22
  url=$(grep -o 'vnc://.*' "$MACOS_DIR/tart.log")
  GUEST_VNC_PASSWORD=${url#vnc://:}
  GUEST_VNC_PASSWORD=${GUEST_VNC_PASSWORD%@*}
  GUEST_VNC_PORT=${url##*:}
}

macos_up() {
  command -v tart >/dev/null || die "tart not found. Install: brew install openai/tools/tart"
  ensure_base
  macos_exists "$MACOS_VM" || tart clone "$MACOS_BASE" "$MACOS_VM"
  if ! macos_running; then
    macos_start
    # The first VNC connection after a start only ever sees a black 1280x720
    # frame, so use it up here.
    guest_vnc pause 1
    # Tart's VNC server crashes, and takes the VM down with it, on the first
    # connection made after a minute or two without any client. Holding one
    # connection open for as long as the VM runs avoids that. It ends by itself
    # when the VM stops.
    nohup "$SCRIPT_DIR/vm.sh" vnc "macos:$name" pause 10000000 >/dev/null 2>&1 &
  fi
  macos_connect
  echo "$os:$name: ssh $GUEST_USER@$GUEST_HOST, vnc://:$GUEST_VNC_PASSWORD@127.0.0.1:$GUEST_VNC_PORT"
}

macos_discard() {
  ! macos_exists "$MACOS_VM" || tart delete "$MACOS_VM"
}

macos_gui_command() {
  # An SSH session is outside the desktop session. A program started from it can
  # open windows, but a password dialog it triggers fails with "no user
  # interaction was possible". launchctl asuser moves the command into the
  # desktop session, and the inner sudo drops back from root to the user.
  echo "sudo launchctl asuser \$(id -u) sudo -H -u $MACOS_USER zsh -c $1"
}
