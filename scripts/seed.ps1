# Seed GoLinx with linx from a JSON file.
# Usage:
#   Dev mode:  .\scripts\seed.ps1 http://localhost:8080
#   Tailnet:   .\scripts\seed.ps1 https://go.example.ts.net
#   Custom:    .\scripts\seed.ps1 http://localhost:8080 .\my-linx.json

param(
    [Parameter(Mandatory=$true, Position=0)]
    [string]$Base,

    [Parameter(Position=1)]
    [string]$File
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)

if (!$File) {
    $File = Join-Path $root "dev\seed.json"
}

if (!(Test-Path $File)) {
    Write-Host "Seed file not found: $File" -ForegroundColor Red
    exit 1
}

$linx = Get-Content $File -Raw | ConvertFrom-Json
$API = "$Base/api/linx"

Write-Host "Seeding $($linx.Count) linx from $(Split-Path -Leaf $File) to $Base ..."

foreach ($lnx in $linx) {
    $json = $lnx | ConvertTo-Json -Compress
    $name = $lnx.shortName
    try {
        Invoke-RestMethod -Uri $API -Method Post -ContentType "application/json" -Body $json -ErrorAction Stop | Out-Null
        Write-Host "  + $name"
    } catch {
        Write-Host "  ! $name FAILED: $_"
    }
}

Write-Host ""
Write-Host "Done. Refresh browser to see linx."
