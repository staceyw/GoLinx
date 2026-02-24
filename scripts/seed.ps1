# Seed GoLinx with cards from a JSON file.
# Usage:
#   Dev mode:  .\scripts\seed.ps1 http://localhost:8080
#   Tailnet:   .\scripts\seed.ps1 https://go.example.ts.net
#   Custom:    .\scripts\seed.ps1 http://localhost:8080 .\my-cards.json

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

$cards = Get-Content $File -Raw | ConvertFrom-Json
$API = "$Base/api/cards"

Write-Host "Seeding $($cards.Count) cards from $(Split-Path -Leaf $File) to $Base ..."

foreach ($card in $cards) {
    $json = $card | ConvertTo-Json -Compress
    $name = $card.shortName
    try {
        Invoke-RestMethod -Uri $API -Method Post -ContentType "application/json" -Body $json -ErrorAction Stop | Out-Null
        Write-Host "  + $name"
    } catch {
        Write-Host "  ! $name FAILED: $_"
    }
}

Write-Host ""
Write-Host "Done. Refresh browser to see cards."
