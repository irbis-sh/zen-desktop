# Shared by the backends whose guests run under QEMU. Sourced by them.
# shellcheck shell=bash disable=SC2154 # vm.sh sets os.
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
  local dir=$1
  for _ in $(seq "$2"); do
    qemu_running "$dir" || return 0
    sleep 1
  done
  kill "$(cat "$dir/pid")" 2>/dev/null || true
  echo "vm: the guest did not shut down within $2 seconds and was powered off" >&2
  return 1
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
