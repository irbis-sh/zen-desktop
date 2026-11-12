# Linux guests: Ubuntu desktop under QEMU. Sourced by vm.sh.
# shellcheck shell=bash disable=SC2034 # The GUEST_* variables are read by vm.sh.

LINUX_RELEASE=26.04
LINUX_USER=zen
LINUX_SSH_PORT="${ZEN_VM_LINUX_SSH_PORT:-2231}"
# QEMU counts VNC displays from TCP port 5900.
LINUX_VNC_DISPLAY="${ZEN_VM_LINUX_VNC_DISPLAY:-31}"
LINUX_DIR="$VM_HOME/linux"

# The guest runs at the host's own architecture, since emulating another one
# is too slow for a desktop. Only Apple Silicon hosts have been tested.
case "$(uname -m)" in
  arm64 | aarch64)
    LINUX_QEMU=qemu-system-aarch64
    LINUX_IMAGE_ARCH=arm64
    LINUX_MACHINE=(-M virt -device "virtio-gpu-pci,xres=1440,yres=900")
    ;;
  x86_64)
    LINUX_QEMU=qemu-system-x86_64
    LINUX_IMAGE_ARCH=amd64
    LINUX_MACHINE=(-M q35 -vga none -device "virtio-vga,xres=1440,yres=900")
    ;;
  *) die "unsupported host architecture $(uname -m)" ;;
esac
LINUX_IMAGE="$VM_HOME/images/ubuntu-$LINUX_RELEASE-$LINUX_IMAGE_ARCH.img"

linux_running() {
  [ -f "$LINUX_DIR/pid" ] && kill -0 "$(cat "$LINUX_DIR/pid")" 2>/dev/null
}

linux_ssh() {
  ssh "${SSH_OPTS[@]}" -p "$LINUX_SSH_PORT" "$LINUX_USER@127.0.0.1" "$@"
}

# Starts QEMU on the given disk and waits until SSH answers. Further arguments
# are passed to QEMU.
linux_start() {
  local disk=$1 accel=kvm firmware=()
  shift
  [ "$(uname -s)" != Darwin ] || accel=hvf
  if [ "$LINUX_IMAGE_ARCH" = arm64 ]; then
    # ARM machines have no built-in firmware. QEMU ships one next to its binary.
    firmware=(-bios "$(dirname "$(realpath "$(command -v "$LINUX_QEMU")")")/../share/qemu/edk2-aarch64-code.fd")
  fi
  # User-mode networking needs no privileges and no bridge, and reaches the
  # guest's SSH through a forwarded loopback port.
  "$LINUX_QEMU" "${LINUX_MACHINE[@]}" -accel "$accel" -cpu host -smp 4 -m 8G ${firmware[@]+"${firmware[@]}"} \
    -drive "if=virtio,format=qcow2,file=$disk" \
    -nic "user,model=virtio-net-pci,hostfwd=tcp:127.0.0.1:$LINUX_SSH_PORT-:22" \
    -device qemu-xhci -device usb-kbd -device usb-tablet \
    -display none -vnc "127.0.0.1:$LINUX_VNC_DISPLAY" \
    -serial "file:$LINUX_DIR/serial.log" -pidfile "$LINUX_DIR/pid" -daemonize "$@"
  for _ in $(seq 60); do
    linux_ssh -o ConnectTimeout=3 true 2>/dev/null && return
    sleep 2
  done
  die "linux is running but SSH on port $LINUX_SSH_PORT does not answer. Boot log: $LINUX_DIR/serial.log"
}

linux_down() {
  local pid
  linux_running || return 0
  pid=$(cat "$LINUX_DIR/pid")
  linux_ssh sudo poweroff >/dev/null 2>&1 || true
  for _ in $(seq 60); do
    kill -0 "$pid" 2>/dev/null || return 0
    sleep 1
  done
  kill "$pid" 2>/dev/null || true
}

linux_download_image() {
  local url="https://cloud-images.ubuntu.com/releases/$LINUX_RELEASE/release" expected actual
  local name="ubuntu-$LINUX_RELEASE-server-cloudimg-$LINUX_IMAGE_ARCH.img"
  mkdir -p "$VM_HOME/images"
  echo "Downloading $name, about 1 GB." >&2
  curl -fL --progress-bar -o "$LINUX_IMAGE.part" "$url/$name"
  expected=$(curl -fsSL "$url/SHA256SUMS" | awk -v n="*$name" '$2 == n { print $1 }')
  actual=$(shasum -a 256 "$LINUX_IMAGE.part" | cut -d' ' -f1)
  [ -n "$expected" ] && [ "$expected" = "$actual" ] || die "checksum mismatch for $name"
  mv "$LINUX_IMAGE.part" "$LINUX_IMAGE"
}

# Builds the base disk: the cloud image turned into a desktop with the build
# tools. Guests are copy-on-write overlays of it.
linux_build_base() {
  local building="$LINUX_DIR/base-building.qcow2" seed="$LINUX_DIR/seed"
  [ -f "$LINUX_IMAGE" ] || linux_download_image
  echo "Creating the Linux base. This installs a desktop and takes a while." >&2
  rm -rf "$building" "$seed"
  mkdir -p "$seed"
  # The cloud image is 3.5 GB, too small for a desktop and a build.
  qemu-img create -q -f qcow2 -F qcow2 -b "$LINUX_IMAGE" "$building" 40G

  # cloud-init creates the user on first boot. It reads these two files from
  # any disk labelled "cidata", which QEMU can make out of a plain directory.
  cat >"$seed/user-data" <<EOF
#cloud-config
users:
  - name: $LINUX_USER
    plain_text_passwd: $LINUX_USER
    lock_passwd: false
    shell: /bin/bash
    # Members of sudo are the administrators polkit accepts in password
    # dialogs. Without it the dialogs ask for root's password, which is not set.
    groups: [adm, sudo]
    sudo: ALL=(ALL) NOPASSWD:ALL
    ssh_authorized_keys:
      - $(cat "$SSH_KEY.pub")
EOF
  printf 'instance-id: zen-linux\nlocal-hostname: zen-linux\n' >"$seed/meta-data"
  linux_start "$building" \
    -blockdev "driver=vvfat,dir=$seed,label=cidata,read-only=on,node-name=seed" \
    -device virtio-blk-pci,drive=seed
  linux_ssh cloud-init status --wait >/dev/null

  provision provision-linux.sh -p "$LINUX_SSH_PORT" "$LINUX_USER@127.0.0.1"

  linux_down
  # Renamed only now, so that a failed run is never mistaken for a finished base.
  mv "$building" "$LINUX_DIR/base.qcow2"
}

linux_connect() {
  linux_running || die "linux is not running. Run: vm.sh up linux"
  GUEST_USER=$LINUX_USER
  GUEST_HOST=127.0.0.1
  GUEST_SSH_PORT=$LINUX_SSH_PORT
  GUEST_VNC_PORT=$((5900 + LINUX_VNC_DISPLAY))
}

linux_up() {
  command -v "$LINUX_QEMU" >/dev/null || die "$LINUX_QEMU not found. Install QEMU, e.g. brew install qemu"
  mkdir -p "$LINUX_DIR"
  if [ ! -f "$LINUX_DIR/disk.qcow2" ]; then
    [ -f "$LINUX_DIR/base.qcow2" ] || linux_build_base
    qemu-img create -q -f qcow2 -F qcow2 -b "$LINUX_DIR/base.qcow2" "$LINUX_DIR/disk.qcow2"
  fi
  if ! linux_running; then
    linux_start "$LINUX_DIR/disk.qcow2"
    # SSH answers a few seconds before the automatic login has finished, and
    # `gui` needs the desktop session.
    for _ in $(seq 60); do
      ! linux_ssh systemctl --user is-active --quiet graphical-session.target || break
      sleep 1
    done
  fi
  echo "linux: ssh -p $LINUX_SSH_PORT $LINUX_USER@127.0.0.1, vnc://127.0.0.1:$((5900 + LINUX_VNC_DISPLAY))"
}

linux_reset() {
  linux_down
  rm -f "$LINUX_DIR/disk.qcow2"
  linux_up
}

linux_gui_command() {
  # A command started over SSH has no access to the display and cannot show
  # password dialogs. The user's systemd instance carries the desktop session's
  # environment, so a transient unit there has both.
  echo "systemd-run --user --quiet --collect --pipe --wait --same-dir bash -c $1"
}
