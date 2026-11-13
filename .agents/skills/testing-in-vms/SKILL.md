---
name: testing-in-vms
description: >-
  Tests Zen inside a local virtual machine running another operating system.
  Use when a change needs checking on macOS, Linux or Windows, when
  reproducing an OS-specific bug, or when a test needs a clean machine or a
  real desktop session.
---

# Testing in VMs

`task vm -- <command> <guest>` runs a guest, copies the checkout into it, and gives SSH and screen access. `<guest>` is `macos`, `linux` or `windows`, which means this checkout's own guest of that OS, see "Several guests". The script behind it is `tasks/vm/vm.sh`, with one backend file per guest OS next to it.

Use SSH for everything a shell can do: building, starting the app, reading logs, checking system state. Use the screen only for what has no other route, such as system dialogs and the app's own window.

## Host requirements

Only Apple Silicon Macs have been tested as hosts.

| Guest | Needs | Disk | First `up` |
| --- | --- | --- | --- |
| `macos` | Tart, see below | about 60 GB | 24 GB download, then 7 to 10 minutes |
| `linux` | QEMU: `brew install qemu` | about 10 GB | 1 GB download, about 8 minutes in total |
| `windows` | QEMU and a Windows ISO, see below | about 22 GB | 0.9 GB download, then about 17 minutes |

For Tart: `brew install openai/tools/tart`. Homebrew refuses untrusted taps, so the person installing must first run `brew trust --formula openai/tools/tart openai/tools/softnet`. The terminal app also needs Local Network permission (System Settings > Privacy & Security > Local Network); without it SSH to the macOS guest fails with "No route to host".

For Windows: Microsoft has no download that a script can use, so a person must fetch the Windows 11 ARM64 ISO, in English (United States), from https://www.microsoft.com/software-download/windows11arm64 and save it as `~/.zen-vms/images/windows-arm64.iso`, or set `ZEN_VM_WINDOWS_ISO` to its path. The install accepts Microsoft's licence terms on that person's behalf and leaves Windows unactivated. Setup differs between Windows releases and editions, and the recipe has not been run with all of them. If `up` is still waiting after half an hour, look at the screen with `shot windows:base-build`: Setup may have stopped at a question the answer file does not cover.

The Linux guest runs at the host's architecture, so on Apple Silicon it is an ARM build. The Windows guest is always Windows on ARM and needs an ARM host. Linux hosts have not been tried. The scripts use KVM there. On an ARM host they also need QEMU's ARM firmware, which they only know how to find on Debian and Ubuntu (package `qemu-efi-aarch64`). macOS guests need a Mac.

## Commands

| Command | Effect |
| --- | --- |
| `up <guest>` | Create the guest if needed, start it, wait for SSH and the desktop. |
| `sync <guest>` | Copy this checkout, uncommitted changes included, to `~/zen` in the guest. |
| `ssh <guest> [args]` | Run a command, or pass ssh options such as `-L`. |
| `gui <guest> <command>` | Run a command inside the desktop session. |
| `shot <guest> [file]` | Save a screenshot and print its path. |
| `vnc <guest> <actions>` | Send [vncdo](https://vncdotool.readthedocs.io/en/latest/commands.html) actions: `move 720 394 click 1`, `type admin`, `key enter`, `pause 2`. |
| `reset <guest>` | Replace the guest's disk with a clean copy of the base. Takes under a minute. |
| `down <guest>` | Shut the guest down. Its disk is kept. |
| `rm <guest>` | Shut the guest down and delete it. |
| `list` | List all guests with their state and checkout. |

**`ssh` or `gui`.** An SSH session is outside the desktop session. On macOS a program started from it can open windows, but any password dialog it triggers fails; on Linux and Windows it cannot show a window at all. Start GUI programs with `gui`. It stays attached until the program exits, so run it in the background for anything long-lived. On macOS and Linux, ending that `gui` command also ends a program that prints output, as Zen does; on Windows the program keeps running.

## Several guests

Each checkout has its own guest per OS, named after the checkout's directory: in the worktree `fix-638`, `linux` means `linux:fix-638`. Sessions that work in different checkouts therefore never share a guest. Write the full name to reach any other guest: `task vm -- up linux:clean` creates a second one for this checkout.

- **A new guest is cheap.** All guests of an OS start as copies of one base, so creating one takes under a minute and little disk space. Only the first guest of an OS on a machine has to build the base, and a session that asks for a guest during that build waits for it.
- **A running guest is not cheap.** Each Linux or Windows guest takes 4 processor cores and up to 8 GB of memory, and macOS lets two macOS guests run at a time. Shut guests down when a test is done, and delete them with `rm` when the work is. `list` marks the guests whose checkout no longer exists.
- **`<os>:base-build` is the machine that builds a base.** While `up` waits for a Linux or Windows build, or after one failed, `ssh`, `shot` and `vnc` work on it.

## A typical run

```sh
task vm -- up linux
task vm -- sync linux
task vm -- ssh linux 'cd zen && task build-dev'           # 2 to 3 minutes the first time
task vm -- gui linux '~/zen/build/bin/Zen' &              # macOS: ssh macos 'open ~/zen/build/bin/Zen.app'
task vm -- shot linux                                     # then read the image
task vm -- vnc linux move 753 800 click 1                 # click where the screenshot shows the Start button
```

The screen is 1440x900, or 1024x768 on Windows, and a pixel in the screenshot is the coordinate to click. Windows do not open in the same place every time, so read coordinates from a fresh screenshot, and take another after each action rather than assuming the result.

Check outcomes over SSH instead of reading them off the screen:

| | Zen's log | System proxy | Trusted CA |
| --- | --- | --- | --- |
| macOS | `~/Library/Logs/Zen/application.log` | `scutil --proxy` | `security dump-trust-settings -d` |
| Linux | `~/.local/share/Zen/logs/application.log` | `gsettings get org.gnome.system.proxy mode` | `ls /usr/local/share/ca-certificates` |
| Windows | `$LOCALAPPDATA/Zen/Logs/application.log` | `reg query 'HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings' //v AutoConfigURL` | `certutil -user -store Root "Zen Personal CA"` |

For a request through the proxy: `curl -x http://127.0.0.1:<port> -sI https://<host>`, with the port from the log line `proxy listening on port`. A blocked request returns 403. On Windows use `/c/Windows/System32/curl.exe --ssl-no-revoke` in place of `curl`, see below.

## Dev mode

```sh
task vm -- gui linux 'cd zen && task dev' &
task vm -- ssh linux -f -N -L 34116:localhost:34115       # Windows: 34116:127.0.0.1:34115
```

`http://localhost:34116` on the host then serves the guest's UI. Use another host port if a second session already forwards this one. Forward 34115, the Wails dev server: it serves the UI together with the script that connects it to the Go side. The Vite port, 5173, serves the UI without that script.

## macOS specifics

- **Login.** User `admin`, password `admin`.
- **First proxy start shows three password dialogs in a row.** The first, "Authorize Zen to install the root CA certificate", has its password field focused: type the password and press enter. The next two, "You are making changes to the System Certificate Trust Settings", look identical and do not focus the field: click the field, type, then click "Update Settings".
- **The guest is not a typical user's Mac.** The image turns Gatekeeper off, logs in automatically and allows `sudo` without a password. Do not use it to judge first-launch warnings for unsigned builds.
- **"Connection was closed cleanly" from `shot` or `vnc` means the VM has crashed.** Tart's VNC server crashes on the first connection after a minute or two without any client. `up` keeps one `vncdo` connection open in the background to prevent this, so do not kill that process. If it happens anyway, run `up` again; the guest's disk is intact.
- **DNS.** On some hosts the DNS server offered to VMs does not answer. Provisioning detects this and sets public DNS servers in the guest.

## Linux specifics

- **The guest** is Ubuntu 26.04 with the GNOME desktop on Wayland. User `zen`, password `zen`, `sudo` without a password. `up` prints the host ports for SSH and VNC, which only listen on the loopback address.
- **First proxy start shows two password dialogs in a row**, one to write the CA file and one to run `update-ca-certificates`. Both have the password field focused: type the password and press enter.
- **Run Firefox once before Zen's first proxy start.** Zen adds its CA to Firefox profiles only at the moment it installs the CA, and a profile exists only after Firefox has run. Otherwise Firefox rejects every page behind the proxy.
- **Firefox does not show its first-run "Welcome" dialog**, whose only button accepts Mozilla's terms of use. Provisioning turns it off with the `SkipTermsOfUse` policy in `/etc/firefox/policies/policies.json`.
- **Firefox comes from Mozilla's package repository, not from the snap** that Ubuntu installs by default. Ubuntu's package installs the snap, and fails the whole install when the snap store is unreachable. Profiles are therefore in `~/.config/mozilla/firefox`, not under `~/snap`.
- **`certutil` is installed** (`libnss3-tools`), which a stock Ubuntu desktop lacks. Remove the package to test how Zen behaves without it.

## Windows specifics

- **The guest** is Windows 11 Pro on ARM. User `zen`, password `zen`, a member of Administrators. `up` prints the host ports for SSH and VNC, which only listen on the loopback address.
- **The remote shell is Git Bash**, so `cd zen && task build-dev`, `tail`, `grep` and `~` work as on the other guests. Two things differ:
  - Git Bash rewrites arguments that look like Unix paths, which breaks Windows options such as `/v`. Write them with two slashes, `//v`, or put `MSYS_NO_PATHCONV=1` in front of the command.
  - Its `curl` trusts its own list of CAs, not the Windows one that Zen installs into. `/c/Windows/System32/curl.exe` uses the Windows list; it needs `--ssl-no-revoke` because Zen's certificates name no revocation server.
- **SSH commands run with full administrator rights, `gui` commands with the rights of a program the user starts by hand.** Start Zen with `gui`, or it runs elevated, which no user does.
- **First proxy start shows one "Security Warning" dialog** asking whether to install the certificate. It opens behind Zen's window, so the screenshot only shows Zen with a spinner. Press alt+tab to bring it forward (`vnc windows keydown alt key tab keyup alt`), then click Yes.
- **Edge:** `gui windows '"/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe" --no-first-run <url>' &`. Without `--no-first-run` Edge opens its sign-in and data-sharing questions first.
- **The guest is not a typical user's PC.** Windows is not activated, automatic updates are off, the machine has no TPM, and the user logs in automatically.

## Storage and rebuilding

`~/.zen-vms` holds the SSH key, logs and screenshots, and for Linux and Windows also the disks: `images/` has the downloads, which are only needed to build a base, `<os>/base.qcow2` the provisioned base, and `<os>/guests/<name>/disk.qcow2` a guest, which only stores its differences from the base. macOS disks live in Tart's storage (`tart list`) as `zen-macos-base` and `zen-macos-<name>`.

A base installs Go, Node and Wails at the versions the repo asks for at the time. To rebuild one, for example after a Node major version bump, delete it and run `reset`: `tart delete zen-macos-base` for macOS, `rm ~/.zen-vms/<os>/base.qcow2` for Linux and Windows. Shut down the other guests of that OS first: a Linux or Windows rebuild deletes the disks of all of them, since they are of no use without the base they were made from.

The rebuild starts from the OS image that was downloaded the first time. For a newer one, run `tart pull ghcr.io/cirruslabs/macos-tahoe-vanilla:latest` for macOS, or delete `~/.zen-vms/images/ubuntu-*.img` for Linux, before the rebuild.
