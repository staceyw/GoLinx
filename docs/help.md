# GoLinx Help

## Card Types

Everything in GoLinx is a **Card**. The card type determines its behavior:

| Type | Badge | What happens at `/{name}` |
|------|-------|---------------------------|
| Link | — | Redirects to the destination URL |
| Employee | Emp | Shows a profile page |
| Customer | Cus | Shows a profile page |
| Vendor | Ven | Shows a profile page |

## Creating Cards

- **+** button — creates a new Link card (short name + destination URL)
- **Person icon** button — creates a new person card (Employee, Customer, or Vendor) with name, title, email, phone, and social links

Short names must start with a letter, digit, or underscore, followed by letters, digits, dashes, underscores, or periods. They are unique and case-insensitive.

## Search

The search box filters cards in real time using substring matching with fuzzy fallback.

**Type prefix filters** narrow results to a single card type before searching:

| Prefix | Shows | Example |
|--------|-------|---------|
| `:e` | Employees | `:e john` |
| `:c` | Customers | `:c acme` |
| `:v` | Vendors | `:v cal` |
| `:l` | Links | `:l git` |

Use the prefix alone (e.g. `:e`) to show all cards of that type.

If your search narrows to exactly one card, press **Enter** to open it in a new tab.

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `Tab` | Cycle focus between search box and card links |
| `Shift+Tab` | Cycle focus in reverse |
| `Enter` | Open focused card, or single search result |
| `Escape` | Close any open modal or context menu |
| `Ctrl+S` | Save the active modal |
| `Double-click` | Open Edit modal (or Linx Info if not owner) |
| `Right-click` | Open context menu (Edit/Delete or View) |
| `F1` | Open this help page |

Focus is trapped inside open modals — Tab and Shift+Tab cycle through fields without leaving the dialog.

## Sorting

- **A-Z** — alphabetical by short name (links) or first name (people)
- **Popular** — by click count, descending
- **Recent** — by last clicked (links) or date created (people)

## Views

Toggle between **Grid** and **List** layout using the view buttons in the toolbar. Your preference is saved.

## Themes

12 themes available from the dropdown in the header: Catppuccin Mocha, Dracula, Nord, Solarized Dark, Solarized Light, One Dark, Gruvbox, Monokai Dimmed, Abyss, Catppuccin Latte, GitHub Light, and IBM 3278 Retro. Your selection is saved.

## Short Links

Every card has a short URL at `/{name}`. For links, this redirects to the destination URL and tracks the click. For people, it shows their profile page with contact info and social links.

## Detail Pages

Append `+` to any short name to view its detail page instead of redirecting. For example, `/mylink+` shows the link's destination, owner, click count, and creation date. For people cards, it shows the profile page.

## Path Passthrough

Links automatically pass through extra path segments. For example, if `/github` points to `https://github.com`, then `/github/anthropics/claude` redirects to `https://github.com/anthropics/claude`. Query parameters are also forwarded.

Advanced users can use Go template syntax in destination URLs: `{{.Path}}`, `{{.User}}`, `{{.Query}}`, and functions like `PathEscape`, `QueryEscape`, `ToLower`, `ToUpper`, `TrimPrefix`, `TrimSuffix`, `Match`.

## Local Aliases

Destination URLs can be a short name instead of a full URL. This creates a local alias — the server follows the chain internally without an extra redirect.

| Destination | Meaning |
|-------------|---------|
| `docs` | Alias to the `docs` short name (chain-followed server-side) |
| `http://go2/sometag` | Link to another server on the tailnet (scheme required) |
| `https://example.com` | External URL |

**Important:** Use just the short name for local aliases, not `go/docs`. The `go/` prefix is the server hostname — inside GoLinx, everything is referenced by short name alone. For links to other servers on your tailnet, include the `http://` scheme so GoLinx knows it's a different host.

## Permissions

When running on Tailscale, GoLinx enforces owner-based permissions using your Tailscale identity:

| Situation | Edit | Delete | Context Menu |
|-----------|------|--------|-------------|
| You own the card | Yes | Yes | Edit + Delete |
| Card has no owner | Yes (claims it) | Yes | Edit + Delete |
| Someone else owns it | No | No | View (readonly) |

- **Ownership** is set automatically when you create a card — your Tailscale login becomes the owner.
- **Unowned cards** (empty owner) can be claimed by anyone — editing an unowned card sets you as the owner.
- **Owners** can clear the owner field to make a card unowned, or change it to transfer ownership.
- **Non-owners** see a readonly "Linx Info" modal — same fields, but disabled with no Save button.
- **Double-click** on a card opens Edit if you own it, or Linx Info if you don't.
- **Admin mode** — users listed in the `admins` config can toggle admin mode via the header switch to bypass ownership checks.
- **Local mode** (no Tailscale) has no restrictions — all users share the same identity.

Permissions are enforced server-side — the API returns 403 Forbidden if you try to update or delete a card you don't own.

## Avatars

Person cards support avatar images. Upload via the Edit modal — pick a file and the preview updates immediately. Maximum file size is 5 MB.

## Settings

Theme, view mode, and sort mode are automatically saved and restored on your next visit.

## Listener URIs

Use `--listen` (repeatable) with a URI to configure listeners:

- `--listen "http://:8080"` — plain HTTP
- `--listen "https://:443;cert=c.pem;key=k.pem"` — HTTPS with your own certificates
- `--listen "ts+https://:443"` — Tailscale HTTPS (auto certs, requires `--ts-hostname`)
- `--listen "ts+http://:80"` — Tailscale plain HTTP (requires `--ts-hostname`)

Combine multiple `--listen` flags for mixed access.

## HTTPS Redirect

When an HTTPS listener exists (`https://` or `ts+https://`), its corresponding HTTP listener (`http://` or `ts+http://`) automatically redirects requests to HTTPS.

**Note:** If you use `curl` to interact with the API over HTTP when HTTPS is enabled, use the `-L` flag to follow redirects, or your request will return an empty response. We recommend always using `-L` regardless of current HTTPS status.

## Config File

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
```

Command-line flags override config file values (with a warning).

## Export & Import

- **Export** — visit `/.export` to download all cards as `links.json`
- **Import** — run `golinx --import links.json` to load cards from a backup (skips existing short names)
- **Resolve** — run `golinx --resolve links.json shortname/path` to test link resolution from a backup without starting the server

## API

```
GET    /api/cards              List cards (optional ?type= filter)
POST   /api/cards              Create card
PUT    /api/cards/{id}         Update card
DELETE /api/cards/{id}         Delete card
POST   /api/cards/{id}/avatar  Upload avatar
GET    /api/cards/{id}/avatar  Serve avatar
GET    /api/settings           Get setting (?key=)
PUT    /api/settings           Save setting
GET    /api/whoami             Current user, hostname, and Tailscale mode
GET    /.addlinx               Open the New Linx dialog
GET    /.help                  This help page
GET    /.export                Export all cards as JSON
GET    /.ping/{host}           TCP ping (host or host:port)
GET    /.whoami                WhoIs terminal (Tailscale user/node info)
GET    /{shortname}            Redirect or profile page
```
