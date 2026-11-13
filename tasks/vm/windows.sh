# Windows guests: Windows 11 on ARM under QEMU. Sourced by vm.sh.
# shellcheck shell=bash disable=SC2034,SC2154 # vm.sh reads the GUEST_* variables and sets os, name and guest.

# Microsoft offers no download a script can use, so the person installing
# fetches the ISO by hand.
WINDOWS_ISO="${ZEN_VM_WINDOWS_ISO:-$VM_HOME/images/windows-arm64.iso}"
# Windows has no driver for any network card QEMU can emulate on ARM. This CD
# of drivers has one for the virtio card.
WINDOWS_DRIVERS_VERSION=0.1.302
WINDOWS_DRIVERS_URL="https://fedorapeople.org/groups/virt/virtio-win/direct-downloads/archive-virtio/virtio-win-$WINDOWS_DRIVERS_VERSION-1/virtio-win-$WINDOWS_DRIVERS_VERSION.iso"
WINDOWS_DRIVERS_SHA256=303f7ae40dad495d6ae474fdc571df58958a4dbc5c37a522d80f9a203867949d
WINDOWS_DRIVERS="$VM_HOME/images/virtio-win-$WINDOWS_DRIVERS_VERSION.iso"
WINDOWS_USER=zen
WINDOWS_BASE="$VM_HOME/windows/base.qcow2"
# The machine that the functions below act on, see qemu.sh. windows_build_base
# points it at the machine that builds the base.
WINDOWS_DIR=$GUEST_DIR
WINDOWS_QEMU=qemu-system-aarch64

# shellcheck source=/dev/null
. "$SCRIPT_DIR/qemu.sh"

windows_running() {
  qemu_running "$WINDOWS_DIR"
}

windows_ssh() {
  local port
  read -r port _ <"$WINDOWS_DIR/ports"
  ssh "${SSH_OPTS[@]}" -p "$port" "$WINDOWS_USER@127.0.0.1" "$@"
}

# Starts QEMU on the machine's disk. The arguments are passed to QEMU. The
# disk has a file next to it, disk.qcow2.vars, in which the firmware keeps its
# settings: the screen resolution and the boot entry Windows adds for itself.
windows_start() {
  local disk="$WINDOWS_DIR/disk.qcow2" accel=kvm firmware ssh_port vnc_port
  [ "$(uname -s)" != Darwin ] || accel=hvf
  firmware="$(dirname "$(realpath "$(command -v "$WINDOWS_QEMU")")")/../share/qemu"
  [ -f "$disk.vars" ] || cp "$firmware/edk2-arm-vars.fd" "$disk.vars"
  qemu_pick_ports "$WINDOWS_DIR"
  read -r ssh_port vnc_port <"$WINDOWS_DIR/ports"
  # Windows only has drivers for a few of QEMU's devices: NVMe for the disk,
  # USB for input and CDs, and ramfb, a plain framebuffer, for the screen.
  # QEMU counts VNC displays from TCP port 5900.
  "$WINDOWS_QEMU" -M virt -accel "$accel" -cpu host -smp 4 -m 8G \
    -drive "if=pflash,format=raw,readonly=on,file=$firmware/edk2-aarch64-code.fd" \
    -drive "if=pflash,format=raw,file=$disk.vars" \
    -device ramfb -device qemu-xhci -device usb-kbd -device usb-tablet \
    -drive "if=none,id=disk,format=qcow2,file=$disk" -device nvme,drive=disk,serial=zen-windows \
    -nic "user,model=virtio-net-pci,hostfwd=tcp:127.0.0.1:$ssh_port-:22" \
    -display none -vnc "127.0.0.1:$((vnc_port - 5900))" \
    -serial "file:$WINDOWS_DIR/serial.log" -pidfile "$WINDOWS_DIR/pid" -daemonize "$@"
}

# Waits until SSH answers, for at most the given number of seconds.
windows_wait_ssh() {
  local deadline=$((SECONDS + $1)) machine port
  machine="windows:$(basename "$WINDOWS_DIR")"
  read -r port _ <"$WINDOWS_DIR/ports"
  while [ "$SECONDS" -lt "$deadline" ]; do
    windows_running || die "$machine stopped while waiting for SSH"
    # ssh uses the first value it is given for an option, so the shorter
    # timeout has to come before the one in SSH_OPTS.
    ! ssh -o ConnectTimeout=3 "${SSH_OPTS[@]}" -p "$port" "$WINDOWS_USER@127.0.0.1" exit 2>/dev/null || return 0
    sleep 3
  done
  die "$machine is running but SSH on port $port does not answer. Look at the screen: vm.sh shot $machine"
}

windows_down() {
  windows_running || return 0
  windows_ssh shutdown -s -t 0 >/dev/null 2>&1 || true
  qemu_wait_exit "$WINDOWS_DIR" 120
}

windows_download_drivers() {
  echo "Downloading virtio-win drivers, about 900 MB." >&2
  mkdir -p "$VM_HOME/images"
  curl -fL --progress-bar -o "$WINDOWS_DRIVERS.part" "$WINDOWS_DRIVERS_URL"
  [ "$(shasum -a 256 "$WINDOWS_DRIVERS.part" | cut -d' ' -f1)" = "$WINDOWS_DRIVERS_SHA256" ] ||
    die "checksum mismatch for $WINDOWS_DRIVERS_URL"
  mv "$WINDOWS_DRIVERS.part" "$WINDOWS_DRIVERS"
}

# The firmware starts the screen at 800x600, too low for Zen's window, and
# Windows cannot change it afterwards. The firmware reads its preferred size
# from a variable, which its own shell can set: with nothing to boot from, the
# firmware falls into that shell and runs startup.nsh. 1024x768 is the largest
# size it offers for ramfb.
windows_set_resolution() {
  local seed="$WINDOWS_DIR/seed-firmware"
  mkdir -p "$seed"
  # Two little-endian 32-bit numbers: 0x400 = 1024 and 0x300 = 768.
  printf 'setvar PlatformConfig -guid 7235c51c-0c80-4cab-87ac-3b084a6304b1 -nv -bs =0004000000030000\r\nreset -s\r\n' >"$seed/startup.nsh"
  windows_start \
    -blockdev "driver=vvfat,dir=$seed,read-only=on,node-name=seed" -device usb-storage,drive=seed
  # A firmware that never reached `reset -s` is reported by the check below.
  qemu_wait_exit "$WINDOWS_DIR" 120 2>/dev/null || true
  grep -q 'setvar PlatformConfig' "$WINDOWS_DIR/serial.log" ||
    die "the firmware did not run the script that sets the screen resolution. Its output: $WINDOWS_DIR/serial.log"
}

# Builds the base disk: an unattended Windows install plus the build tools.
# Guests are copy-on-write overlays of it.
windows_build_base() {
  # A local variable is also seen by the functions called from here, so they
  # act on the machine that builds the base, not on the guest.
  local WINDOWS_DIR="$VM_HOME/windows/guests/base-build"
  local seed="$WINDOWS_DIR/seed" port deadline
  [ -f "$WINDOWS_ISO" ] || die "no Windows ISO at $WINDOWS_ISO.
Download the Windows 11 ARM64 ISO in English (United States) from
https://www.microsoft.com/software-download/windows11arm64
and save it there, or set ZEN_VM_WINDOWS_ISO to where it is.
The install accepts Microsoft's licence terms on your behalf and leaves Windows unactivated."
  [ -f "$WINDOWS_DRIVERS" ] || windows_download_drivers
  echo "Creating the Windows base. This installs Windows and takes about 20 minutes." >&2
  # A build that failed is left running, so that it can be looked at.
  windows_down || true
  qemu_discard_guests
  rm -rf "$WINDOWS_DIR"
  mkdir -p "$seed"
  qemu-img create -q -f qcow2 "$WINDOWS_DIR/disk.qcow2" 64G
  windows_set_resolution

  # Windows Setup looks for autounattend.xml in the root of every removable
  # disk, and QEMU can make a disk out of a plain directory.
  cp "$SCRIPT_DIR/windows-autounattend.xml" "$seed/autounattend.xml"
  cp "$SCRIPT_DIR/windows-bootstrap.ps1" "$seed/"
  cp "$SSH_KEY.pub" "$seed/authorized_keys"

  # The CD asks "Press any key to boot from CD or DVD" for five seconds and
  # gives up otherwise, which leaves no time to install vncdotool after QEMU
  # has started.
  vnc_install
  windows_start \
    -drive "if=none,id=install,media=cdrom,format=raw,readonly=on,file=$WINDOWS_ISO" \
    -device usb-storage,drive=install \
    -drive "if=none,id=drivers,media=cdrom,format=raw,readonly=on,file=$WINDOWS_DRIVERS" \
    -device usb-storage,drive=drivers \
    -blockdev "driver=vvfat,dir=$seed,read-only=on,node-name=seed" \
    -device usb-storage,drive=seed,removable=on
  # The firmware says on the serial port when it hands over to the CD, the
  # first USB device. The key presses start then and end within the CD's five
  # seconds: about 15 seconds later Windows Setup is on screen, where space
  # presses its Cancel button. The CD has files that start the installer
  # without asking, but started from the firmware's shell they fail, since
  # they cannot tell which device they came from.
  deadline=$((SECONDS + 60))
  until grep -aq 'BdsDxe: starting Boot.*/USB(' "$WINDOWS_DIR/serial.log"; do
    [ "$SECONDS" -lt "$deadline" ] ||
      die "the firmware did not report that it started the install CD. Its output, whose wording may have changed: $WINDOWS_DIR/serial.log"
    sleep 0.2
  done
  for _ in $(seq 4); do
    guest_vnc key space
  done
  windows_wait_ssh 3600

  read_tool_versions
  read -r port _ <"$WINDOWS_DIR/ports"
  scp "${SSH_OPTS[@]}" -q -P "$port" "$SCRIPT_DIR/provision-windows.ps1" "$WINDOWS_USER@127.0.0.1:"
  windows_ssh powershell -NoProfile -ExecutionPolicy Bypass -File provision-windows.ps1 \
    "$GO_VERSION" "$NODE_VERSION" "$WAILS_VERSION"

  windows_down
  # Moved only now, so that a failed run is never mistaken for a finished base.
  mv "$WINDOWS_DIR/disk.qcow2.vars" "$WINDOWS_BASE.vars"
  mv "$WINDOWS_DIR/disk.qcow2" "$WINDOWS_BASE"
  rm -rf "$WINDOWS_DIR"
}

windows_connect() {
  windows_running || die "$guest is not running. Run: vm.sh up $guest"
  GUEST_USER=$WINDOWS_USER
  GUEST_HOST=127.0.0.1
  read -r GUEST_SSH_PORT GUEST_VNC_PORT <"$WINDOWS_DIR/ports"
}

windows_base_exists() {
  [ -f "$WINDOWS_BASE" ]
}

windows_up() {
  local disk="$WINDOWS_DIR/disk.qcow2" tries=0
  case "$(uname -m)" in
    arm64 | aarch64) ;;
    *) die "the Windows guest needs an ARM host, since it runs Windows on ARM" ;;
  esac
  command -v "$WINDOWS_QEMU" >/dev/null || die "$WINDOWS_QEMU not found. Install QEMU, e.g. brew install qemu"
  ensure_base
  if [ ! -f "$disk" ]; then
    qemu-img create -q -f qcow2 -F qcow2 -b "$WINDOWS_BASE" "$disk"
    cp "$WINDOWS_BASE.vars" "$disk.vars"
  fi
  if ! windows_running; then
    windows_start
    windows_wait_ssh 300
    # SSH answers before the automatic login has finished, and `gui` needs the desktop.
    until windows_ssh 'tasklist | grep -q explorer.exe'; do
      tries=$((tries + 1))
      [ "$tries" -lt 60 ] ||
        die "$guest is running but its user is not logged in, so \`gui\` cannot work. Look at the screen: vm.sh shot $guest"
      sleep 1
    done
  fi
  windows_connect
  echo "$os:$name: ssh -p $GUEST_SSH_PORT $WINDOWS_USER@127.0.0.1, vnc://127.0.0.1:$GUEST_VNC_PORT"
}

windows_discard() {
  rm -f "$WINDOWS_DIR/disk.qcow2" "$WINDOWS_DIR/disk.qcow2.vars"
}

# rsync does not exist on Windows, so the checkout travels as a tar stream.
# Files are deleted first, because tar would leave behind those that are gone
# from the checkout.
windows_sync() {
  windows_connect
  # The excluded paths are the ones guest_sync leaves alone, for its reasons.
  COPYFILE_DISABLE=1 tar -C "$REPO_ROOT" --no-xattrs --exclude .git --exclude node_modules \
    --exclude ./frontend/package.json.md5 --exclude ./.claude --exclude ./build/bin -cf - . |
    windows_ssh "mkdir -p zen && cd zen &&
      find . \( -name node_modules -o -path ./.git -o -path ./build/bin -o -path ./frontend/package.json.md5 \) -prune \
        -o -type f -exec rm -f {} + &&
      tar -xf -"
  guest_sync_git_stub
}

windows_gui_command() {
  # A command started over SSH runs in a session without a desktop. A scheduled
  # task marked interactive runs in the desktop session of the logged-in user,
  # with the same rights as a program they start by hand. It has no link to
  # this SSH session, so output and exit status travel through files. Those of
  # finished commands are removed by the next call, since this one may be cut
  # off before its command ends. conhost --headless keeps the task from opening
  # a console window.
  cat <<EOF
mkdir -p ~/.zen-gui && cd ~/.zen-gui || exit 1
for old in zen-gui-*.status; do
  [ -e "\$old" ] || continue
  MSYS_NO_PATHCONV=1 schtasks /delete /f /tn "\${old%.status}" >/dev/null 2>&1
  rm -f "\${old%.status}".*
done
id=zen-gui-\$\$
printf 'cd ~\n%s\n' $1 >\$id.sh
: >\$id.log
MSYS_NO_PATHCONV=1 schtasks /create /f /tn \$id /sc once /st 00:00 /it /tr "conhost --headless \"C:\\Program Files\\Git\\bin\\bash.exe\" -c \"cd ~/.zen-gui; bash \$id.sh >\$id.log 2>&1; echo \\\$? >\$id.status\"" >/dev/null 2>&1 &&
  MSYS_NO_PATHCONV=1 schtasks /run /tn \$id >/dev/null ||
  { echo "gui: could not start a task in the desktop session" >&2; exit 1; }
tail -f \$id.log 2>/dev/null &
until [ -e \$id.status ]; do sleep 1; done
sleep 1
kill \$! 2>/dev/null
exit "\$(cat \$id.status)"
EOF
}
