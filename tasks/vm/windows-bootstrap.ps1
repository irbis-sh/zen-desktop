# Runs once at the first logon of a fresh Windows install, started by the
# answer file. Does only what the host needs to reach the guest over SSH;
# provision-windows.ps1 does the rest from there, where its output is visible.
# Output of this script goes to C:\zen-bootstrap.log.

$ErrorActionPreference = 'Stop'

# Windows has no driver for the virtual machine's network card. The virtio-win
# CD has one, under whichever drive letter the CD got.
$driver = Get-PSDrive -PSProvider FileSystem |
  ForEach-Object { Join-Path $_.Root 'NetKVM\w11\ARM64\netkvm.inf' } |
  Where-Object { Test-Path $_ } |
  Select-Object -First 1
pnputil /add-driver $driver /install

while (-not (Get-NetRoute -DestinationPrefix 0.0.0.0/0 -ErrorAction SilentlyContinue)) {
  Start-Sleep -Seconds 2
}

# The server half of OpenSSH is an optional Windows feature, fetched from Windows Update.
Add-WindowsCapability -Online -Name OpenSSH.Server~~~~0.0.1.0

# Starting the service once creates C:\ProgramData\ssh.
Start-Service sshd
Stop-Service sshd

# For members of Administrators, sshd reads this file instead of the one in the
# user's home, and ignores it unless only administrators and SYSTEM can write to it.
$keys = 'C:\ProgramData\ssh\administrators_authorized_keys'
Copy-Item (Join-Path $PSScriptRoot 'authorized_keys') $keys
icacls $keys /inheritance:r /grant '*S-1-5-32-544:F' /grant '*S-1-5-18:F'

# The rule that comes with the feature does not cover networks Windows classes as public.
New-NetFirewallRule -DisplayName 'SSH for the Zen test rig' -Direction Inbound -Protocol TCP -LocalPort 22 -Action Allow

Set-Service sshd -StartupType Automatic
Start-Service sshd
