# Builds the Vue frontend (if needed) and runs the Web App BFF.
#
#   ./Run-WebApp.ps1            # build frontend then serve
#   ./Run-WebApp.ps1 -SkipBuild # serve existing dist only

param([switch]$SkipBuild)

$ErrorActionPreference = "Stop"
$serverDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$root = Split-Path -Parent $serverDir   # the "Web App" folder
$webDir = Join-Path $root "web"

$goBin = "C:\Program Files\Go\bin"
if (Test-Path $goBin) { $env:Path = "$goBin;$env:Path" }

if (-not $SkipBuild) {
    Write-Host "Building frontend..." -ForegroundColor Cyan
    Push-Location $webDir
    try {
        if (-not (Test-Path "node_modules")) { npm install }
        npm run build
    } finally { Pop-Location }
}

Push-Location $serverDir
try {
    Write-Host "Starting Web App on :8090 (Ctrl+C to stop)..." -ForegroundColor Cyan
    go run .
} finally { Pop-Location }
