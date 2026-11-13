# Shared by the backends whose guests run under QEMU. Sourced by them.
# shellcheck shell=bash disable=SC2034,SC2154 # The backends read ARM_FIRMWARE*, vm.sh sets os.
#
# Each machine, a guest or a base being built, has a directory with its disk
# and three files written here and by QEMU: pid, ports and serial.log.

# QEMU keeps its process ID in <dir>/pid. The file stays behind when QEMU is
# killed or the host restarts, and the ID may then belong to another program.
qemu_running() {
  local pid
  pid=$(cat "$1/pid" 2>/dev/null) && [[ "$(ps -p "$pid" -o comm= 2>/dev/null)" == *qemu-system* ]]
}

# Waits for a guest that was told to shut down, for at most the given number of
# seconds, and cuts its power after that. Fails in that case: a power cut can
# lose what the guest wrote last, which a disk about to become a base must not.
qemu_wait_exit() {
  local dir=$1 signal
  for _ in $(seq "$2"); do
    qemu_running "$dir" || return 0
    sleep 1
  done
  qemu_running "$dir" || return 0
  # QEMU needs a moment to end, and a start that follows at once would still
  # find the machine running. A QEMU that hangs does not act on the first
  # signal at all; no process can ignore the second.
  for signal in TERM KILL; do
    kill -s "$signal" "$(cat "$dir/pid")" 2>/dev/null || true
    for _ in $(seq 50); do
      qemu_running "$dir" || break 2
      sleep 0.1
    done
  done
  echo "vm: the guest did not shut down within $2 seconds and was powered off" >&2
  return 1
}

# Sets ARM_FIRMWARE and ARM_FIRMWARE_VARS, the firmware that ARM machines need
# and the empty file for its settings. QEMU's own build lies next to its
# binary, as with Homebrew. Debian and Ubuntu leave it out of their QEMU
# packages and have their own build in the package qemu-efi-aarch64.
qemu_find_arm_firmware() {
  local dir
  dir="$(dirname "$(realpath "$(command -v qemu-system-aarch64)")")/../share/qemu"
  if [ -f "$dir/edk2-aarch64-code.fd" ]; then
    ARM_FIRMWARE="$dir/edk2-aarch64-code.fd"
    ARM_FIRMWARE_VARS="$dir/edk2-arm-vars.fd"
  elif [ -f /usr/share/AAVMF/AAVMF_CODE.fd ]; then
    ARM_FIRMWARE=/usr/share/AAVMF/AAVMF_CODE.fd
    ARM_FIRMWARE_VARS=/usr/share/AAVMF/AAVMF_VARS.fd
  else
    die "no firmware for ARM machines at $dir/edk2-aarch64-code.fd or /usr/share/AAVMF/AAVMF_CODE.fd." \
      "Install your system's package of it, or add its location to qemu_find_arm_firmware."
  fi
}

# Writes two free loopback ports to <dir>/ports: the one that forwards to the
# machine's SSH, then the one for its screen. Machines that run side by side
# cannot share fixed ports.
qemu_pick_ports() {
  python3 -c '
import socket
sockets = [socket.socket() for _ in range(2)]
for s in sockets:
    s.bind(("127.0.0.1", 0))
print(*(s.getsockname()[1] for s in sockets))' >"$1/ports"
}

# The disk of a guest only stores its differences from the base it was made
# from, so it is worthless once that base is replaced.
qemu_discard_guests() {
  local dir
  for dir in "$VM_HOME/$os/guests"/*/; do
    [ -d "$dir" ] || continue
    ! qemu_running "$dir" ||
      die "$os:$(basename "$dir") still runs on the base that is to be replaced. Shut it down first: vm.sh down $os:$(basename "$dir")"
  done
  rm -f "$VM_HOME/$os/guests"/*/disk.qcow2*
}
