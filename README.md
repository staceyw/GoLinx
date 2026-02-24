# GoLinx

URL shortener and people directory in a single Go binary. Short links redirect and people linx get automatic profile pages. Everything runs from one embedded SPA with SQLite storage. Supports HTTP, HTTPS, and Tailscale listeners. Inspired by Tailscale's [golink](https://github.com/tailscale/golink) with theme support, enhanced UX, and additional features.

![screenshot](docs/screenshot.svg)

## Quick Start

```bash
# HTTP only (local development or LAN)
go build -o golinx . && ./golinx --listen "http://:8080"

# Tailscale HTTPS (port 80 required for browser fallback from short name)
go build -o golinx . && ./golinx --ts-hostname go --listen "ts+https://:443" --listen "ts+http://:80"

# Tailscale HTTPS + local LAN access
go build -o golinx . && ./golinx --ts-hostname go --listen "ts+https://:443" --listen "ts+http://:80" --listen "http://:8080"
```

**Local mode** serves plain HTTP with `local@<hostname>` identity.

**Tailscale mode** joins your Tailscale tailnet via [tsnet](https://pkg.go.dev/tailscale.com/tsnet) with automatic HTTPS and user identification via WhoIs — per-user settings and auto-filled linx ownership come for free.

### Listener URIs

Each `--listen` flag takes a self-describing URI. Combine multiple `--listen` flags to run listeners together.

| Scheme | Format | Description |
|--------|--------|-------------|
| `http://` | `http://[addr]:port` | Plain HTTP |
| `https://` | `https://[addr]:port;cert=<path>;key=<path>` | HTTPS with your own certificates |
| `ts+https://` | `ts+https://:port` | Tailscale HTTPS (auto certs, requires `--ts-hostname`) |
| `ts+http://` | `ts+http://:port` | Tailscale plain HTTP (requires `--ts-hostname`) |

Host must be empty or an IP address — hostnames are not allowed in listener URIs.

### Global Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--listen` | (repeatable) | Listener URI — at least one required |
| `--verbose` | `false` | Verbose tsnet logging |
| `--ts-hostname` | — | Tailscale node hostname (required for `ts+*` listeners) |
| `--ts-dir` | OS config dir | Tailscale state directory (e.g. `~/.config/tsnet-golinx`) |
| `--max-resolve-depth` | `5` | Maximum link chain resolution depth |
| `--import <file>` | — | Import linx from a JSON backup and exit |
| `--resolve <file> <path>` | — | Resolve a short link from a JSON backup and exit |

### Configuration Matrix

| Scenario | Command |
|----------|---------|
| HTTP only on LAN | `./golinx --listen "http://:8080"` |
| HTTPS with own certs | `./golinx --listen "https://:443;cert=cert.pem;key=key.pem"` |
| HTTPS + HTTP redirect | `./golinx --listen "http://:80" --listen "https://:443;cert=cert.pem;key=key.pem"` |
| Tailscale HTTPS | `./golinx --ts-hostname go --listen "ts+https://:443" --listen "ts+http://:80"` |
| Tailscale + local LAN | `./golinx --ts-hostname go --listen "ts+https://:443" --listen "ts+http://:80" --listen "http://:8080"` |

**Identity:** Tailscale listeners use WhoIs login. Local listeners use `local@<hostname>`. Mixed mode falls back to local identity for non-tailnet requests.

### Config File

Place a `golinx.toml` in the working directory to avoid repeating flags:

```toml
listen = [
  "ts+https://:443",
  "ts+http://:80",
  "http://:8080",
]
ts-hostname = "go"
verbose = false
max-resolve-depth = 5
# ts-dir = "/data/tsnet"  # default: OS config dir (e.g. ~/.config/tsnet-golinx)
# admins = ["alice@example.com"]  # Tailscale logins that can toggle admin mode
```

With a config file, just run `./golinx` — no flags needed. Command-line flags override config file values (with a warning).

### Why You Need an HTTP Listener

Modern browsers try HTTPS first when you type a bare hostname like `go/cal`. If the HTTPS certificate doesn't match the short name (Tailscale certs are issued for the FQDN, e.g. `go.example.ts.net`, not `go`), the browser falls back to HTTP on port 80. Without an HTTP listener, that fallback times out.

**Always pair an HTTPS listener with its HTTP counterpart:**

| HTTPS listener | Required HTTP listener | Why |
|----------------|----------------------|-----|
| `ts+https://:443` | `ts+http://:80` | Catches browser fallback from `go/` short name |
| `https://:443;cert=...;key=...` | `http://:80` | Catches browser fallback from LAN hostname |

The HTTP listener automatically redirects to the HTTPS FQDN, so the final page loads with a valid certificate.

### HTTPS Redirect

When an HTTPS listener exists (`https://` or `ts+https://`), its corresponding HTTP listener (`http://` or `ts+http://`) automatically redirects requests to the HTTPS equivalent. If `ts+https://` is configured but the tailnet does not support HTTPS certificates, GoLinx exits with an error.

HSTS (`Strict-Transport-Security`) headers are set only for fully-qualified domain names (hostnames containing a dot). This avoids HSTS issues with `localhost`, bare hostnames like `go`, and IPv6 addresses.

> **Note:** If you use `curl` to interact with the API over HTTP when HTTPS is enabled, use the `-L` flag to follow redirects, or your request will return an empty response. We recommend always using `-L` regardless of current HTTPS status.

### Development Setup

Scripts keep the repo root clean by building and running from `dev/`:

```bash
# 1. Copy the example config into dev/ and edit for your environment
mkdir dev
cp golinx.example.toml dev/golinx.toml

# 2. Build and run (PowerShell: .\scripts\run.ps1)
./scripts/run.sh

# 3. Seed sample data while server is running (PowerShell: .\scripts\seed.ps1)
./scripts/seed.sh http://localhost:8080
```

Runtime files (`golinx.db`, config, binary) all live in `dev/` which is gitignored. Edit `dev/seed.json` to customize seed data.

### Seed Data

GoLinx starts with an empty database. Populate it with sample linx:

```bash
./scripts/seed.sh http://localhost:8080           # local
./scripts/seed.sh https://go.example.ts.net       # tailnet
```

## Permissions

When running on Tailscale, GoLinx enforces simple owner-based access control:

| Situation | Edit | Delete | UI |
|-----------|------|--------|-----|
| You own the linx | Yes | Yes | Edit + Delete |
| Linx has no owner | Yes (claims it) | Yes | Edit + Delete |
| Someone else owns it | No | No | View only |

- Linx are automatically owned by the Tailscale user who creates them
- Unowned linx can be claimed by anyone — editing sets you as owner
- Owners can clear or transfer ownership via the owner field
- Non-owners see a readonly "Linx Info" view
- **Admin mode** — users listed in `admins` config can toggle admin mode to bypass ownership checks
- Local mode (no Tailscale) has no restrictions
- Enforced server-side — API returns 403 for unauthorized edits/deletes

## Linx Types

| Type | Badge | `/{name}` behavior |
|------|-------|-----------------------|
| Link (default) | — | 302 redirect to destination URL |
| Employee | Emp | Profile page |
| Customer | Cus | Profile page |
| Vendor | Ven | Profile page |

## Features

- **Unified linx model** — links, employees, customers, and vendors in one grid
- **Fuzzy search** with type prefix filters (`:e`, `:c`, `:v`, `:l`)
- **12 themes** — Catppuccin Mocha, Dracula, Nord, Solarized Dark/Light, One Dark, Gruvbox, Monokai Dimmed, Abyss, Catppuccin Latte, GitHub Light, IBM 3278 Retro
- **Grid and list views** with sort by A-Z, Popular, or Recent
- **Profile pages** for people with avatar, contact info, and social links
- **Detail pages** — append `+` to any short name (`/name+`) to view metadata instead of redirecting
- **Path passthrough** — `/github/anthropics/claude` resolves through `/github` to `https://github.com/anthropics/claude`
- **Go template URLs** — `{{.Path}}`, `{{.User}}`, `{{.Query}}` and functions like `PathEscape`, `QueryEscape`, `TrimPrefix`
- **Recursive link resolution** — chains of local links are followed automatically (configurable depth)
- **Punctuation trimming** — trailing `.`, `,`, `()`, `[]`, `{}` stripped for copy-paste tolerance
- **Link loop detection** — creating a link that would form a cycle is rejected with a clear error message
- **Click tracking** with count and last-clicked timestamp
- **Owner-based permissions** — edit/delete your own linx, view others (Tailscale mode)
- **Admin mode** — configurable admin list with header toggle to bypass ownership checks
- **Right-click context menu** for Edit, Delete, or View
- **Keyboard navigation** — Tab cycles Linx's, Enter opens single search result, Ctrl+S saves, Escape closes modals, F1 opens help
- **Modal focus trapping** — Tab stays within open dialogs
- **Avatar upload** with live preview (5 MB max)
- **Graceful shutdown** on Ctrl+C
- **URI-based listeners** — `http://`, `https://`, `ts+http://`, and `ts+https://` schemes, composable via multiple `--listen` flags
- **Tailscale integration** — runs on your tailnet via tsnet with automatic HTTPS and user identification
- **Per-user settings** persisted to SQLite (theme, view mode, sort mode)
- **Single binary** — all HTML/CSS/JS/help embedded, no external assets
- **In-app help** at `/.help` — rendered from Markdown via [goldmark](https://github.com/yuin/goldmark)
- **JSON export** of all linx at `/.export`
- **CLI import** — `--import links.json` loads linx from a backup, skipping existing short names
- **CLI resolve** — `--resolve links.json github/org/repo` tests link resolution from a backup without starting the server

## Link Resolution

When a request arrives at `/{name}` (or `/{name}/extra/path`):

1. **Lookup** — the first path segment is used as the short name
2. **Punctuation trim** — if not found, trailing `.,()[]{}` are stripped and retried
3. **Expand** — the destination URL is expanded with any extra path/query via Go templates
4. **Chain follow** — if the expanded URL points to another local link, it is followed recursively (up to `max-resolve-depth` hops, default 5)
5. **Redirect** — the final URL is served as a 302 redirect; each hop increments its click count

### Detail Pages

Append `+` to any short name to view its detail page instead of redirecting:

- **Links** — `/github+` shows the destination URL, description, owner, click count, and creation date
- **People** — `/john+` shows the profile page with contact info and social links

### Path Passthrough

Extra path segments after the short name are appended to the destination URL:

```
/github                    → https://github.com
/github/anthropics/claude  → https://github.com/anthropics/claude
/github?tab=repositories   → https://github.com?tab=repositories
```

### Local Aliases

Destination URLs can be a short name instead of a full URL. This creates a local alias — the server follows the chain internally without an extra redirect.

```
docs2 → docs          Local alias (chain-followed server-side)
hub   → http://go2/x  Link to another tailnet server (scheme required)
```

Use just the short name for local aliases, not `go/docs`. The `go/` prefix is the server hostname — inside GoLinx, everything is referenced by short name alone. For links to other servers on your tailnet, include the `http://` scheme so GoLinx knows it's a different host.

### Go Template URLs

Destination URLs support Go `text/template` syntax for advanced routing:

| Variable/Function | Description |
|-------------------|-------------|
| `{{.Path}}` | Extra path after the short name |
| `{{.User}}` | Tailscale user login (or local identity) |
| `{{.Query}}` | Full query string |
| `{{.Now}}` | Current time (UTC) |
| `PathEscape` | URL path-escapes a string |
| `QueryEscape` | URL query-escapes a string |
| `TrimPrefix` | Trims a prefix from a string |
| `TrimSuffix` | Trims a suffix from a string |
| `ToLower` / `ToUpper` | Case conversion |
| `Match` | Regexp match (returns bool) |

Example: a link with destination `https://search.example.com/q={{QueryEscape .Query}}` redirects `/search?foo+bar` to `https://search.example.com/q=foo+bar`.

## Export & Import

### Export

Visit `/.export` to download all linx as `links.json`.

### Import

```bash
./golinx --import links.json
```

Loads linx from a JSON backup into the database. Existing short names are skipped — import is additive only.

### Resolve

```bash
./golinx --resolve links.json github/test
```

Tests link resolution from a JSON backup without starting the server. Loads data into an in-memory SQLite database and runs the same resolution logic as the live server. Useful for verifying redirects before importing.

## Architecture

```
main.go              Entry point
golinx.go            HTTP handlers + embedded SPA (HTML/CSS/JS as string literal)
db.go                SQLite data layer with mutex-protected CRUD
schema.sql           Embedded via //go:embed
docs/help.md         Help content (Markdown, embedded and rendered via goldmark)
static/favicon.svg   App icon (embedded via //go:embed)
scripts/seed.ps1     Seed script for populating sample linx
golinx.example.toml  Example configuration file
```

Pure Go with `modernc.org/sqlite`, `tailscale.com/tsnet`, and `github.com/yuin/goldmark` — no CGo, no Node, no build tools.

## API

```
GET    /api/linx              List linx (optional ?type= filter)
POST   /api/linx              Create linx
PUT    /api/linx/{id}         Update linx
DELETE /api/linx/{id}         Delete linx
POST   /api/linx/{id}/avatar  Upload avatar
GET    /api/linx/{id}/avatar  Serve avatar
GET    /api/settings           Get setting (?key=)
PUT    /api/settings           Save setting
GET    /api/whoami             Current user, hostname, and Tailscale mode
GET    /.addlinx               Open the New Linx dialog
GET    /.help                  In-app help page
GET    /.export                Export all linx as JSON
GET    /.ping/{host}           TCP ping (host or host:port)
GET    /.whoami                WhoIs terminal (Tailscale user/node info)
GET    /{shortname}            Redirect or profile page
GET    /{shortname}+           Detail page (link metadata or profile)
```

## License

BSD 3-Clause. See [LICENSE](LICENSE).
