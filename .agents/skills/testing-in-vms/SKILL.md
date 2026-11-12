---
name: testing-in-vms
description: >-
  Tests Zen inside a local virtual machine running another operating system.
  Use when a change needs checking on macOS or Linux, when reproducing an
  OS-specific bug, or when a test needs a clean machine or a real desktop
  session.
---

# Testing in VMs

`task vm -- <command> <os>` runs a guest, copies the checkout into it, and gives SSH and screen access. `<os>` is `macos` or `linux`. The script behind it is `tasks/vm/vm.sh`, with one backend file per guest OS next to it.

Use SSH for everything a shell can do: building, starting the app, reading logs, checking system state. Use the screen only for what has no other route, such as password dialogs and the app's own window.

## Host requirements

Only Apple Silicon Macs have been tested as hosts.

| Guest | Needs | Disk | First `up` |
| --- | --- | --- | --- |
| `macos` | Tart, see below | about 60 GB | 24 GB download, then 7 to 10 minutes |
| `linux` | QEMU: `brew install qemu` | about 10 GB | 1 GB download, about 8 minutes in total |

For Tart: `brew install openai/tools/tart`. Homebrew refuses untrusted taps, so the person installing must first run `brew trust --formula openai/tools/tart openai/tools/softnet`. The terminal app also needs Local Network permission (System Settings > Privacy & Security > Local Network); without it SSH to the macOS guest fails with "No route to host".

The Linux guest runs at the host's architecture. On a Linux host it should work with KVM, but that has not been tried. macOS guests need a Mac.

## Commands

| Command | Effect |
| --- | --- |
| `up <os>` | Create the guest if needed, start it, wait for SSH and the desktop. |
| `sync <os>` | Copy this checkout, uncommitted changes included, to `~/zen` in the guest. |
| `ssh <os> [args]` | Run a command, or pass ssh options such as `-L`. |
| `gui <os> <command>` | Run a command inside the desktop session. |
| `shot <os> [file]` | Save a screenshot and print its path. |
| `vnc <os> <actions>` | Send [vncdo](https://vncdotool.readthedocs.io/en/latest/commands.html) actions: `move 720 394 click 1`, `type admin`, `key enter`, `pause 2`. |
| `reset <os>` | Replace the guest with a clean copy of the base. Takes under a minute. |
| `down <os>` | Shut the guest down. Its disk is kept. |

**`ssh` or `gui`.** An SSH session is outside the desktop session. On macOS a program started from it can open windows, but any password dialog it triggers fails; on Linux it cannot reach the display at all. Start GUI programs with `gui`. It stays attached until the program exits, so run it in the background for anything long-lived.

## A typical run

```sh
task vm -- up linux
task vm -- sync linux
task vm -- ssh linux 'cd zen && task build-dev'           # 2 to 3 minutes the first time
task vm -- gui linux '~/zen/build/bin/Zen' &              # macOS: ssh macos 'open ~/zen/build/bin/Zen.app'
task vm -- shot linux                                     # then read the image
task vm -- vnc linux move 753 800 click 1                 # click where the screenshot shows the Start button
```

The screen is 1440x900 and a pixel in the screenshot is the coordinate to click. Windows do not open in the same place every time, so read coordinates from a fresh screenshot, and take another after each action rather than assuming the result.

Check outcomes over SSH instead of reading them off the screen:

| | macOS | Linux |
| --- | --- | --- |
| Zen's log | `~/Library/Logs/Zen/application.log` | `~/.local/share/Zen/logs/application.log` |
| System proxy | `scutil --proxy` | `gsettings get org.gnome.system.proxy mode` |
| Trusted CA | `security dump-trust-settings -d` | `ls /usr/local/share/ca-certificates` |

For a request through the proxy on either: `curl -x http://127.0.0.1:<port> -sI https://<host>`, with the port from the log line `proxy listening on port`. A blocked request returns 403.

## Dev mode

```sh
task vm -- gui linux 'cd zen && task dev' &
task vm -- ssh linux -f -N -L 34116:localhost:34115
```

`http://localhost:34116` on the host then serves the guest's UI. Forward 34115, the Wails dev server: it serves the UI together with the script that connects it to the Go side. The Vite port, 5173, serves the UI without that script.

## macOS specifics

- **Login.** User `admin`, password `admin`.
- **First proxy start shows three password dialogs in a row.** The first, "Authorize Zen to install the root CA certificate", has its password field focused: type the password and press enter. The next two, "You are making changes to the System Certificate Trust Settings", look identical and do not focus the field: click the field, type, then click "Update Settings".
- **The guest is not a typical user's Mac.** The image turns Gatekeeper off, logs in automatically and allows `sudo` without a password. Do not use it to judge first-launch warnings for unsigned builds.
- **"Connection was closed cleanly" from `shot` or `vnc` means the VM has crashed.** Tart's VNC server crashes on the first connection after a minute or two without any client. `up` keeps one `vncdo` connection open in the background to prevent this, so do not kill that process. If it happens anyway, run `up` again; the guest's disk is intact.
- **DNS.** On some hosts the DNS server offered to VMs does not answer. Provisioning detects this and sets public DNS servers in the guest.

## Linux specifics

- **The guest** is Ubuntu 26.04 with the GNOME desktop on Wayland. User `zen`, password `zen`, `sudo` without a password. SSH is on host port 2231 and VNC on 5931, both on the loopback address only.
- **First proxy start shows two password dialogs in a row**, one to write the CA file and one to run `update-ca-certificates`. Both have the password field focused: type the password and press enter.
- **Run Firefox once before Zen's first proxy start.** Zen adds its CA to Firefox profiles only at the moment it installs the CA, and a profile exists only after Firefox has run. Otherwise Firefox rejects every page behind the proxy.
- **Firefox's first run shows a "Welcome to Firefox" dialog** whose Continue button accepts Mozilla's terms of use. Pages load behind it, and Zen lists what it blocks, so most tests do not need it dismissed. Do not click Continue unless the person you work for has said to.
- **Firefox comes from Mozilla's package repository, not from the snap** that Ubuntu installs by default. The snap store was unreachable when the recipe was written, and Ubuntu's package fails the whole install in that case. Profiles are therefore in `~/.config/mozilla/firefox`, not under `~/snap`.
- **`certutil` is installed** (`libnss3-tools`), which a stock Ubuntu desktop lacks. Remove the package to test how Zen behaves without it.

## Storage and rebuilding

`~/.zen-vms` holds the SSH key, logs and screenshots, and for Linux also the disks: `images/` has the downloaded Ubuntu image, `linux/base.qcow2` the provisioned base, and `linux/disk.qcow2` the guest, which only stores its differences from the base. macOS disks live in Tart's storage (`tart list`) as `zen-macos-base` and `zen-macos`.

A base installs Go, Node and Wails at the versions the repo asks for at the time. To rebuild one, after a Node major version bump or to pick up a newer image, delete it and run `reset`: `tart delete zen-macos-base` for macOS, `rm ~/.zen-vms/linux/base.qcow2` for Linux.
