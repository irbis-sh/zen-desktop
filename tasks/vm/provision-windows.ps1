# Runs once inside a fresh Windows guest, copied there and started over SSH by
# windows.sh. Installs what building Zen needs and keeps Windows from changing
# or locking itself between tests.

param(
  [Parameter(Mandatory)] [string] $GoVersion,
  [Parameter(Mandatory)] [string] $NodeVersion,
  [Parameter(Mandatory)] [string] $WailsVersion
)

$ErrorActionPreference = 'Stop'

# Any version works: Git is only needed for its shell and for `git describe`.
$gitVersion = '2.56.1'

function Get-Download([string] $Url) {
  $file = Join-Path $env:TEMP (Split-Path -Leaf $Url)
  # curl.exe ships with Windows and is several times faster than Invoke-WebRequest.
  # The retries are for failures that pass, such as a name that does not
  # resolve for a moment: one of them would otherwise undo a 20-minute install.
  curl.exe -fsSL --retry 5 --retry-all-errors -o $file $Url
  if ($LASTEXITCODE -ne 0) { throw "download failed: $Url" }
  $file
}

function Invoke-Installer([string] $File, [string[]] $Arguments) {
  $process = Start-Process -FilePath $File -ArgumentList $Arguments -Wait -PassThru
  if ($process.ExitCode -ne 0) { throw "$File exited with $($process.ExitCode)" }
}

# Background updates would change the guest between two runs of the same test
# and compete with builds for the processor.
$policy = 'HKLM:\SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate\AU'
New-Item -Path $policy -Force | Out-Null
Set-ItemProperty -Path $policy -Name NoAutoUpdate -Value 1 -Type DWord

# A screen that turns off, or a machine that sleeps, cannot be driven over VNC.
powercfg /change monitor-timeout-ac 0
powercfg /change standby-timeout-ac 0
# With hibernation on, "shut down" saves the running kernel to disk for a
# faster next start. Every guest would then resume the base's session.
powercfg /hibernate off

Invoke-Installer msiexec.exe '/i', (Get-Download "https://go.dev/dl/go$GoVersion.windows-arm64.msi"), '/qn', '/norestart'

# The newest release of the major version that frontend/package.json asks for.
$nodeDist = "https://nodejs.org/dist/latest-v$($NodeVersion.Split('.')[0]).x"
$nodeMsi = (curl.exe -fsSL "$nodeDist/SHASUMS256.txt") -match 'arm64\.msi$' -replace '^\S+\s+', ''
Invoke-Installer msiexec.exe '/i', (Get-Download "$nodeDist/$nodeMsi"), '/qn', '/norestart'

Invoke-Installer (Get-Download "https://github.com/git-for-windows/git/releases/download/v$gitVersion.windows.1/Git-$gitVersion-arm64.exe") '/VERYSILENT', '/NORESTART'

# The installers changed PATH in the registry; this process still has the old one.
$env:Path = [Environment]::GetEnvironmentVariable('Path', 'Machine') + ';' + [Environment]::GetEnvironmentVariable('Path', 'User')

go install "github.com/wailsapp/wails/v2/cmd/wails@$WailsVersion"
if ($LASTEXITCODE -ne 0) { throw 'go install wails failed' }
go install github.com/go-task/task/v3/cmd/task@latest
if ($LASTEXITCODE -ne 0) { throw 'go install task failed' }

# SSH sessions start cmd.exe by default. With Git's bash instead, the commands
# that vm.sh and the skill use on the other guests work here unchanged.
New-Item -Path 'HKLM:\SOFTWARE\OpenSSH' -Force | Out-Null
Set-ItemProperty -Path 'HKLM:\SOFTWARE\OpenSSH' -Name DefaultShell -Value 'C:\Program Files\Git\bin\bash.exe'

Write-Output "provision: done ($((go version).Split(' ')[2]), node $(node --version), wails $WailsVersion)"
