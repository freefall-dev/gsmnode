# Runs the SMS Gateway API Server.
# Loads .env (if present), then starts the Go server.
#
#   ./scripts/Run-ApiServer.ps1

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
$root = Split-Path -Parent $here   # the "API Server" folder

Push-Location $root
try {
    # Ensure Go is on PATH for this session.
    $goBin = "C:\Program Files\Go\bin"
    if (Test-Path $goBin) { $env:Path = "$goBin;$env:Path" }

    if (-not (Get-Command go -ErrorAction SilentlyContinue)) {
        throw "Go is not installed or not on PATH."
    }

    Write-Host "Starting API Server (Ctrl+C to stop)..." -ForegroundColor Cyan
    go run ./cmd/server
}
finally {
    Pop-Location
}
