$ErrorActionPreference = "Stop"

$projectRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$serverPath = Join-Path $projectRoot "block-server.exe"

$targets = Get-Process | Where-Object {
  $_.Path -and [string]::Equals($_.Path, $serverPath, [System.StringComparison]::OrdinalIgnoreCase)
}

if (-not $targets) {
  Write-Host "block-server.exe is not running from this folder."
  exit 0
}

$targets | Stop-Process -Force

$stoppedIds = $targets | ForEach-Object { $_.Id }
Write-Host ("Stopped block-server.exe process: " + ($stoppedIds -join ", "))
