# macOS guests, run with Tart. Sourced by vm.sh.
# shellcheck shell=bash disable=SC2034 # The GUEST_* variables are read by vm.sh.

MACOS_IMAGE="${ZEN_VM_MACOS_IMAGE:-ghcr.io/cirruslabs/macos-tahoe-vanilla:latest}"
MACOS_BASE=zen-macos-base
MACOS_VM=zen-macos
# Set by the image.
MACOS_USER="admin"
MACOS_PASSWORD="admin"

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

# Starts a VM headless and waits until SSH answers.
macos_start() {
  local name=$1 log="$VM_HOME/$1.log" ip
  # Tart's VNC server works below the guest OS, so it also shows the login
  # window and password dialogs. It picks a new port and password on each start
  # and prints them to the log.
  # --no-audio: Tart otherwise plays the guest's sound on the host's speakers,
  # including the alert beep macOS makes for every key press nothing accepts.
  nohup tart run "$name" --no-graphics --no-audio --vnc-experimental >"$log" 2>&1 &
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
  local building="$MACOS_BASE-building" askpass="$VM_HOME/askpass" ip
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

  provision provision-macos.sh "$MACOS_USER@$ip"

  macos_stop "$building"
  tart rename "$building" "$MACOS_BASE"
}

macos_connect() {
  local url
  macos_running "$MACOS_VM" || die "macos is not running. Run: vm.sh up macos"
  GUEST_USER=$MACOS_USER
  GUEST_HOST=$(tart ip "$MACOS_VM")
  GUEST_SSH_PORT=22
  url=$(grep -o 'vnc://.*' "$VM_HOME/$MACOS_VM.log")
  GUEST_VNC_PASSWORD=${url#vnc://:}
  GUEST_VNC_PASSWORD=${GUEST_VNC_PASSWORD%@*}
  GUEST_VNC_PORT=${url##*:}
  # Without this Tart's VNC server ignores shift: "A" arrives as "a" and "|" as "\".
  GUEST_VNC_FORCE_CAPS=1
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
    guest_vnc pause 1
    # Tart's VNC server crashes, and takes the VM down with it, on the first
    # connection made after a minute or two without any client. Holding one
    # connection open for as long as the VM runs avoids that. It ends by itself
    # when the VM stops.
    nohup "$SCRIPT_DIR/vm.sh" vnc macos pause 10000000 >/dev/null 2>&1 &
  fi
  macos_connect
  echo "macos: ssh $GUEST_USER@$GUEST_HOST, vnc://:$GUEST_VNC_PASSWORD@127.0.0.1:$GUEST_VNC_PORT"
}

macos_down() {
  macos_stop "$MACOS_VM"
}

macos_reset() {
  macos_stop "$MACOS_VM"
  ! macos_exists "$MACOS_VM" || tart delete "$MACOS_VM"
  macos_up
}

macos_gui_command() {
  # An SSH session is outside the desktop session. A program started from it can
  # open windows, but a password dialog it triggers fails with "no user
  # interaction was possible". launchctl asuser moves the command into the
  # desktop session, and the inner sudo drops back from root to the user.
  echo "sudo launchctl asuser \$(id -u) sudo -H -u $MACOS_USER zsh -c $1"
}
