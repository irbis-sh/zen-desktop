---
name: testing-in-vms
description: >-
  Tests Zen inside a local virtual machine running another operating system.
  Use when a change needs checking on macOS, when reproducing an OS-specific
  bug, or when a test needs a clean machine or a real desktop session.
---

# Testing in VMs

`task vm -- <command> <os>` runs a guest, copies the checkout into it, and gives SSH and screen access. Only macOS guests are supported so far. The script behind it is `tasks/vm/vm.sh`.

Use SSH for everything a shell can do: building, starting the app, reading logs, checking system state. Use the screen only for what has no other route, such as password dialogs and the app's own window.

## Host requirements

- An Apple Silicon Mac with about 60 GB of free disk.
- Tart: `brew install openai/tools/tart`. Homebrew refuses untrusted taps, so the person installing must first run `brew trust --formula openai/tools/tart openai/tools/softnet`.
- Local Network permission for the terminal app (System Settings > Privacy & Security > Local Network). Without it SSH to the guest fails with "No route to host".

## Commands

| Command | Effect |
| --- | --- |
| `up macos` | Create the guest if needed, start it, wait for SSH. The first run downloads 24 GB and then takes 7 to 10 minutes. |
| `sync macos` | Copy this checkout, uncommitted changes included, to `~/zen` in the guest. |
| `ssh macos [args]` | Run a command, or pass ssh options such as `-L`. |
| `gui macos <command>` | Run a command inside the desktop session. |
| `shot macos [file]` | Save a screenshot and print its path. |
| `vnc macos <actions>` | Send [vncdo](https://vncdotool.readthedocs.io/en/latest/commands.html) actions: `move 720 394 click 1`, `type admin`, `key enter`, `pause 2`. |
| `reset macos` | Replace the guest with a clean copy of the base. Takes seconds. |
| `down macos` | Shut the guest down. Its disk is kept. |

## A typical run

```sh
task vm -- up macos
task vm -- sync macos
task vm -- ssh macos 'cd zen && task build-dev'          # about 2 minutes the first time
task vm -- ssh macos 'open ~/zen/build/bin/Zen.app'
task vm -- shot macos                                     # then read the image
task vm -- vnc macos move 720 735 click 1                 # click where the screenshot shows the Start button
```

The screen is 1440x900 and a pixel in the screenshot is the coordinate to click. Windows do not open in the same place every time, so read coordinates from a fresh screenshot, and take another after each action rather than assuming the result.

Check outcomes over SSH instead of reading them off the screen:

- Zen's log: `~/Library/Logs/Zen/application.log`
- System proxy: `scutil --proxy`
- Trusted CA: `security dump-trust-settings -d`
- A request through the proxy: `curl -x http://127.0.0.1:<port> -sI https://<host>`, with the port from the log line `proxy listening on port`. A blocked request returns 403.

## Dev mode

```sh
task vm -- gui macos 'cd zen && task dev'                 # keeps running, so run it in the background
task vm -- ssh macos -f -N -L 34116:localhost:34115
```

`http://localhost:34116` on the host then serves the guest's UI. Forward 34115, the Wails dev server: it serves the UI together with the script that connects it to the Go side. The Vite port, 5173, serves the UI without that script.

## macOS specifics

- **`ssh` or `gui`.** An SSH session is outside the desktop session. A program started from it can open windows, but any password dialog it triggers fails with "no user interaction was possible". Start such programs with `gui`, or with `open` for an app bundle.
- **First proxy start shows three password dialogs in a row.** The first, "Authorize Zen to install the root CA certificate", has its password field focused: type the password and press enter. The next two, "You are making changes to the System Certificate Trust Settings", look identical and do not focus the field: click the field, type, then click "Update Settings". The password is `admin`.
- **The guest is not a typical user's Mac.** The image turns Gatekeeper off, logs in automatically and allows `sudo` without a password. Do not use it to judge first-launch warnings for unsigned builds.
- **"Connection was closed cleanly" from `shot` or `vnc` means the VM has crashed.** Tart's VNC server crashes on the first connection after a minute or two without any client. `up` keeps one `vncdo` connection open in the background to prevent this, so do not kill that process. If it happens anyway, run `up` again; the guest's disk is intact.
- **DNS.** On some hosts the DNS server offered to VMs does not answer. Provisioning detects this and sets public DNS servers in the guest.

## Storage and rebuilding

Disk images live in Tart's storage (`tart list`): `zen-macos-base` holds the image plus the build tools, and `zen-macos` is a copy-on-write clone of it. The SSH key, logs and screenshots are in `~/.zen-vms`.

The base installs Go, Node and Wails at the versions the repo asks for at the time. After a Node major version bump, or to pick up a newer image, delete it with `tart delete zen-macos-base` and run `reset`.
