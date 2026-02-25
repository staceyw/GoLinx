# scripts/

Development and release scripts (PowerShell).

## Scripts

| Script | Description |
|--------|-------------|
| `run` | Build and run GoLinx from `dev/` |
| `seed` | Seed the database from `dev/seed.json` |
| `release` | Cross-compile, bundle ZIPs, and create a GitHub release |

## Usage

```powershell
.\scripts\run.ps1                                  # build & run
.\scripts\run.ps1 --verbose                        # with extra flags
.\scripts\seed.ps1 http://localhost:8080            # seed (local)
.\scripts\seed.ps1 https://go.example.ts.net       # seed (tailnet)
.\scripts\seed.ps1 http://localhost:8080 .\my.json  # seed (custom file)
.\scripts\release.ps1 v0.2.0                       # create release
```

## Notes

- All scripts resolve the repo root from their own path — run from anywhere
- `run` builds into `dev/` and runs from there so runtime files stay out of root
- `seed` defaults to `dev/seed.json` if no file is specified
- `release` builds into `dist/`, runs tests, creates ZIP bundles (binary + example config + README.txt), then calls `gh release create`

## Release Assets

Each release uploads 10 files — a raw binary and a ZIP bundle per platform:

| Platform | Binary | ZIP (binary + config + quickstart) |
|----------|--------|------------------------------------|
| Windows x64 | `golinx-windows-amd64.exe` | `golinx-windows-amd64.zip` |
| Windows ARM64 | `golinx-windows-arm64.exe` | `golinx-windows-arm64.zip` |
| Linux x64 | `golinx-linux-amd64` | `golinx-linux-amd64.zip` |
| Linux ARM64 (Pi) | `golinx-linux-arm64` | `golinx-linux-arm64.zip` |
| macOS Apple Silicon | `golinx-darwin-arm64` | `golinx-darwin-arm64.zip` |

ZIPs are for end users. Raw binaries are for developers who just want the executable.

## Building Manually

Prerequisites: [Go 1.23+](https://go.dev/dl/)

```bash
# Build for your current platform
go build -o golinx .

# Build with a version tag
go build -ldflags "-X main.Version=v0.2.0" -o golinx .

# Cross-compile for a specific platform
GOOS=linux GOARCH=arm64 go build -ldflags "-X main.Version=v0.2.0" -o golinx .
```

Common `GOOS`/`GOARCH` combinations:

| Target | GOOS | GOARCH |
|--------|------|--------|
| Windows x64 | `windows` | `amd64` |
| Windows ARM64 | `windows` | `arm64` |
| Linux x64 | `linux` | `amd64` |
| Linux ARM64 (Raspberry Pi) | `linux` | `arm64` |
| macOS Apple Silicon | `darwin` | `arm64` |

On Windows (PowerShell), set environment variables before the build:

```powershell
$env:GOOS = "linux"
$env:GOARCH = "arm64"
go build -ldflags "-X main.Version=v0.2.0" -o golinx .
Remove-Item Env:\GOOS, Env:\GOARCH
```
