# Build and run GoLinx from the dev/ directory.
# Usage:  .\scripts\run.ps1
# Extra args are passed through: .\scripts\run.ps1 --verbose
# Note: This script is for development purposes. For production use, build and run from the root directory.
 
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$dev = Join-Path $root "dev"

if (!(Test-Path $dev)) { New-Item -ItemType Directory -Path $dev | Out-Null }

Write-Host "Building golinx.exe ..."
Push-Location $root
try {
    go build -ldflags "-X main.Version=dev" -o "$dev\golinx.exe" .
    if ($LASTEXITCODE -ne 0) { throw "Build failed" }
} finally { Pop-Location }

Write-Host "Running from dev/ ..."
Push-Location $dev
try {
    & .\golinx.exe @args
} finally { Pop-Location }
