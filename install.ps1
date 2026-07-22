#Requires -Version 5.1
param([switch]$BuildAll)
$ErrorActionPreference = 'Stop'
$binary = 'godot-tui.exe'
$installDir = Join-Path $env:LOCALAPPDATA 'Programs\godot-tui'
$build = Join-Path $PSScriptRoot "bin\$binary"
if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
  $build = Join-Path $PSScriptRoot 'releases\windows\godot-tui.exe'
  if (-not (Test-Path $build)) { throw 'Go is not installed and no Windows release binary exists.' }
} else {
  New-Item -ItemType Directory -Force (Split-Path $build) | Out-Null
  if ($BuildAll) {
    $targets = @(
      @{ OS='linux'; ARCH='amd64'; OUT='releases\linux\amd64\godot-tui' },
      @{ OS='linux'; ARCH='arm64'; OUT='releases\linux\arm64\godot-tui' },
      @{ OS='darwin'; ARCH='amd64'; OUT='releases\darwin\amd64\godot-tui' },
      @{ OS='darwin'; ARCH='arm64'; OUT='releases\darwin\arm64\godot-tui' },
      @{ OS='windows'; ARCH='amd64'; OUT='releases\windows\godot-tui.exe' }
    )
    foreach ($target in $targets) {
      $out = Join-Path $PSScriptRoot $target.OUT
      New-Item -ItemType Directory -Force (Split-Path $out) | Out-Null
      $env:GOOS = $target.OS; $env:GOARCH = $target.ARCH
      go build -ldflags="-s -w" -o $out .
    }
    $env:GOOS = $null; $env:GOARCH = $null
    Write-Host 'Release binaries written to releases\'
    exit 0
  }
  go build -ldflags="-s -w" -o $build .
}
New-Item -ItemType Directory -Force $installDir | Out-Null
Copy-Item $build (Join-Path $installDir $binary) -Force
$path = [Environment]::GetEnvironmentVariable('Path','User')
if (($path -split ';') -notcontains $installDir) { [Environment]::SetEnvironmentVariable('Path', (($path.TrimEnd(';')+';'+$installDir).Trim(';')), 'User') }
Write-Host "Installed $installDir\$binary"
