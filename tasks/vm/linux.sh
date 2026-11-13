# Linux guests: Ubuntu desktop under QEMU. Sourced by vm.sh.
# shellcheck shell=bash disable=SC2034,SC2154 # vm.sh reads the GUEST_* variables and sets os, name and guest.

LINUX_RELEASE=26.04
LINUX_USER=zen
LINUX_BASE="$VM_HOME/linux/base.qcow2"
# The machine that the functions below act on, see qemu.sh. linux_build_base
# points it at the machine that builds the base.
LINUX_DIR=$GUEST_DIR

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

# shellcheck source=/dev/null
. "$SCRIPT_DIR/qemu.sh"

linux_running() {
  qemu_running "$LINUX_DIR"
}

linux_ssh() {
  local port
  read -r port _ <"$LINUX_DIR/ports"
  ssh "${SSH_OPTS[@]}" -p "$port" "$LINUX_USER@127.0.0.1" "$@"
}

# Starts QEMU on the machine's disk and waits until SSH answers. The arguments
# are passed to QEMU.
linux_start() {
  local accel=kvm firmware=() ssh_port vnc_port
  [ "$(uname -s)" != Darwin ] || accel=hvf
  if [ "$LINUX_IMAGE_ARCH" = arm64 ]; then
    # ARM machines have no built-in firmware. QEMU ships one next to its binary.
    firmware=(-bios "$(dirname "$(realpath "$(command -v "$LINUX_QEMU")")")/../share/qemu/edk2-aarch64-code.fd")
  fi
  qemu_pick_ports "$LINUX_DIR"
  read -r ssh_port vnc_port <"$LINUX_DIR/ports"
  # User-mode networking needs no privileges and no bridge, and reaches the
  # guest's SSH through a forwarded loopback port. QEMU counts VNC displays
  # from TCP port 5900.
  "$LINUX_QEMU" "${LINUX_MACHINE[@]}" -accel "$accel" -cpu host -smp 4 -m 8G ${firmware[@]+"${firmware[@]}"} \
    -drive "if=virtio,format=qcow2,file=$LINUX_DIR/disk.qcow2" \
    -nic "user,model=virtio-net-pci,hostfwd=tcp:127.0.0.1:$ssh_port-:22" \
    -device qemu-xhci -device usb-kbd -device usb-tablet \
    -display none -vnc "127.0.0.1:$((vnc_port - 5900))" \
    -serial "file:$LINUX_DIR/serial.log" -pidfile "$LINUX_DIR/pid" -daemonize "$@"
  for _ in $(seq 60); do
    # ssh uses the first value it is given for an option, so the shorter
    # timeout has to come before the one in SSH_OPTS.
    ssh -o ConnectTimeout=3 "${SSH_OPTS[@]}" -p "$ssh_port" "$LINUX_USER@127.0.0.1" true 2>/dev/null && return
    sleep 2
  done
  die "linux:$(basename "$LINUX_DIR") is running but SSH on port $ssh_port does not answer. Boot log: $LINUX_DIR/serial.log"
}

linux_down() {
  linux_running || return 0
  linux_ssh sudo poweroff >/dev/null 2>&1 || true
  qemu_wait_exit "$LINUX_DIR" 60
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

linux_base_exists() {
  [ -f "$LINUX_BASE" ]
}

# Builds the base disk: the cloud image turned into a desktop with the build
# tools. Guests are copy-on-write overlays of it.
linux_build_base() {
  # A local variable is also seen by the functions called from here, so they
  # act on the machine that builds the base, not on the guest.
  local LINUX_DIR="$VM_HOME/linux/guests/base-build"
  local seed="$LINUX_DIR/seed" port
  [ -f "$LINUX_IMAGE" ] || linux_download_image
  echo "Creating the Linux base. This installs a desktop and takes a while." >&2
  # A build that failed is left running, so that it can be looked at.
  linux_down || true
  qemu_discard_guests
  rm -rf "$LINUX_DIR"
  mkdir -p "$seed"
  # A copy, not an overlay, so that the base does not depend on the download.
  # The cloud image is 3.5 GB, too small for a desktop and a build.
  cp "$LINUX_IMAGE" "$LINUX_DIR/disk.qcow2"
  qemu-img resize -q -f qcow2 "$LINUX_DIR/disk.qcow2" 40G

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
  linux_start \
    -blockdev "driver=vvfat,dir=$seed,label=cidata,read-only=on,node-name=seed" \
    -device virtio-blk-pci,drive=seed
  linux_ssh cloud-init status --wait >/dev/null

  read -r port _ <"$LINUX_DIR/ports"
  provision provision-linux.sh -p "$port" "$LINUX_USER@127.0.0.1"

  linux_down
  # Moved only now, so that a failed run is never mistaken for a finished base.
  mv "$LINUX_DIR/disk.qcow2" "$LINUX_BASE"
  rm -rf "$LINUX_DIR"
}

linux_connect() {
  linux_running || die "$guest is not running. Run: vm.sh up $guest"
  GUEST_USER=$LINUX_USER
  GUEST_HOST=127.0.0.1
  read -r GUEST_SSH_PORT GUEST_VNC_PORT <"$LINUX_DIR/ports"
}

linux_up() {
  command -v "$LINUX_QEMU" >/dev/null || die "$LINUX_QEMU not found. Install QEMU, e.g. brew install qemu"
  ensure_base
  [ -f "$LINUX_DIR/disk.qcow2" ] ||
    qemu-img create -q -f qcow2 -F qcow2 -b "$LINUX_BASE" "$LINUX_DIR/disk.qcow2"
  if ! linux_running; then
    linux_start
    # SSH answers a few seconds before the automatic login has finished, and
    # `gui` needs the desktop session.
    for _ in $(seq 60); do
      ! linux_ssh systemctl --user is-active --quiet graphical-session.target || break
      sleep 1
    done
  fi
  linux_connect
  echo "$os:$name: ssh -p $GUEST_SSH_PORT $LINUX_USER@127.0.0.1, vnc://127.0.0.1:$GUEST_VNC_PORT"
}

linux_discard() {
  rm -f "$LINUX_DIR/disk.qcow2"
}

linux_gui_command() {
  # A command started over SSH has no access to the display and cannot show
  # password dialogs. The user's systemd instance carries the desktop session's
  # environment, so a transient unit there has both.
  echo "systemd-run --user --quiet --collect --pipe --wait --same-dir bash -c $1"
}
