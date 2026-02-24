# Cross-compile, bundle ZIPs, and create a GitHub release.
# Usage:  .\scripts\release.ps1 v0.2.0

param(
    [Parameter(Position=0)]
    [string]$Tag
)

$ErrorActionPreference = "Stop"

if (-not $Tag) {
    $latest = (gh release list --limit 1 --json tagName 2>$null | ConvertFrom-Json)
    if ($latest -and $latest.Count -gt 0) {
        Write-Host "Latest release: $($latest[0].tagName)"
    }
    $Tag = Read-Host "Enter version tag (e.g. v0.3.0)"
    if (-not $Tag) { throw "Version tag is required" }
}
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$dist = Join-Path $root "dist"

if (!(Test-Path $dist)) { New-Item -ItemType Directory -Path $dist | Out-Null }

$targets = @(
    @{ GOOS="windows"; GOARCH="amd64"; Out="golinx-windows-amd64.exe"; Zip="golinx-windows-amd64.zip"; ZipBin="golinx.exe" },
    @{ GOOS="windows"; GOARCH="arm64"; Out="golinx-windows-arm64.exe"; Zip="golinx-windows-arm64.zip"; ZipBin="golinx.exe" },
    @{ GOOS="linux";   GOARCH="amd64"; Out="golinx-linux-amd64";      Zip="golinx-linux-amd64.zip";   ZipBin="golinx" },
    @{ GOOS="linux";   GOARCH="arm64"; Out="golinx-linux-arm64";      Zip="golinx-linux-arm64.zip";   ZipBin="golinx" },
    @{ GOOS="darwin";  GOARCH="arm64"; Out="golinx-darwin-arm64";      Zip="golinx-darwin-arm64.zip";  ZipBin="golinx" }
)

# Build binaries
Write-Host "Building $($targets.Count) targets ..."
Push-Location $root
try {
    foreach ($t in $targets) {
        $env:GOOS   = $t.GOOS
        $env:GOARCH = $t.GOARCH
        $out = Join-Path $dist $t.Out
        Write-Host "  $($t.GOOS)/$($t.GOARCH) -> $($t.Out)"
        go build -ldflags "-s -w -X main.Version=$Tag" -o $out .
        if ($LASTEXITCODE -ne 0) { throw "Build failed for $($t.GOOS)/$($t.GOARCH)" }
    }
} finally {
    Remove-Item Env:\GOOS  -ErrorAction SilentlyContinue
    Remove-Item Env:\GOARCH -ErrorAction SilentlyContinue
    Pop-Location
}

# Run tests
Write-Host ""
Write-Host "Running tests ..."
Push-Location $root
try {
    go test -count=1 ./...
    if ($LASTEXITCODE -ne 0) { throw "Tests failed" }
} finally { Pop-Location }

# Create ZIP bundles
Write-Host ""
Write-Host "Creating ZIP bundles ..."
$readme = Join-Path $dist "README.txt"
@"
GoLinx - URL shortener and people directory

Quick Start:
  1. Copy golinx.example.toml to golinx.toml
  2. Edit golinx.toml - add at least one listener (e.g. http://:8080)
  3. Run:  ./golinx
  4. Open: http://localhost:8080

Full documentation: https://github.com/staceyw/GoLinx
"@ | Set-Content -Path $readme -Encoding UTF8

$example = Join-Path $root "golinx.example.toml"

foreach ($t in $targets) {
    $zipPath = Join-Path $dist $t.Zip
    $binPath = Join-Path $dist $t.Out
    # Create a temp staging folder
    $stage = Join-Path $dist "stage"
    if (Test-Path $stage) { Remove-Item $stage -Recurse -Force }
    New-Item -ItemType Directory -Path $stage | Out-Null

    Copy-Item $binPath (Join-Path $stage $t.ZipBin)
    Copy-Item $example $stage
    Copy-Item $readme  $stage

    if (Test-Path $zipPath) { Remove-Item $zipPath -Force }
    Compress-Archive -Path "$stage\*" -DestinationPath $zipPath
    Remove-Item $stage -Recurse -Force
    Write-Host "  $($t.Zip)"
}

Remove-Item $readme -Force

# Collect all assets (binaries + ZIPs)
$assets = @()
foreach ($t in $targets) {
    $assets += Join-Path $dist $t.Out
    $assets += Join-Path $dist $t.Zip
}

# Create release
Write-Host ""
Write-Host "Creating release $Tag ..."
$notes = @"
## Downloads

| File | Description |
|------|-------------|
| ``golinx-windows-amd64.zip`` | Windows x64 (zip with config template) |
| ``golinx-windows-arm64.zip`` | Windows ARM64 (zip with config template) |
| ``golinx-linux-amd64.zip`` | Linux x64 (zip with config template) |
| ``golinx-linux-arm64.zip`` | Linux ARM64 / Raspberry Pi (zip with config template) |
| ``golinx-darwin-arm64.zip`` | macOS Apple Silicon (zip with config template) |
| ``golinx-windows-amd64.exe`` | Windows x64 (binary only) |
| ``golinx-windows-arm64.exe`` | Windows ARM64 (binary only) |
| ``golinx-linux-amd64`` | Linux x64 (binary only) |
| ``golinx-linux-arm64`` | Linux ARM64 / Raspberry Pi (binary only) |
| ``golinx-darwin-arm64`` | macOS Apple Silicon (binary only) |

> **Tip:** ZIP bundles include ``golinx.example.toml`` and a quick-start README.
"@
gh release create $Tag @assets --title "GoLinx $Tag" --generate-notes --notes $notes
if ($LASTEXITCODE -ne 0) { throw "gh release create failed" }

Write-Host ""
Write-Host "Done: https://github.com/staceyw/GoLinx/releases/tag/$Tag"
