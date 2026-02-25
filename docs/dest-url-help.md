# Destination URL

The destination URL is where `/shortname` redirects to. It can be an external URL, a local alias, or a Go template.

## External URL

Standard URLs redirect as-is:

| Destination | `/mylink` goes to |
|-------------|-------------------|
| `https://github.com` | `https://github.com` |
| `https://example.com/docs` | `https://example.com/docs` |

## Path Passthrough

Extra path segments after the short name are appended automatically:

| Destination | Request | Redirects to |
|-------------|---------|-------------|
| `https://github.com` | `/gh/anthropics/claude` | `https://github.com/anthropics/claude` |
| `https://google.com/search?q=` | `/g?q=hello` | `https://google.com/search?q=hello` |

Query parameters are forwarded too.

## Local Aliases

Use just a short name (no scheme) to create a chain through another linx:

| Destination | Meaning |
|-------------|---------|
| `docs` | Redirect to whatever `/docs` points to |
| `team-wiki` | Chain through the `team-wiki` linx |

Chains are followed server-side (no extra redirect). Use `http://` or `https://` for links to other servers.

## Go Templates

Use `{{...}}` syntax for dynamic URLs. Available variables:

| Variable | Description | Example |
|----------|-------------|---------|
| `{{.Path}}` | Extra path after short name | `/search/hello` &rarr; `hello` |
| `{{.User}}` | Current user login | `alice@example.com` |
| `{{.Now.Format "2006-01-02"}}` | Current date/time | `2025-03-15` |
| `{{.Query.key}}` | Query parameter values | `/go?repo=foo` &rarr; `[foo]` |

### Template Functions

| Function | Description |
|----------|-------------|
| `PathEscape` | URL-encode a path segment |
| `QueryEscape` | URL-encode a query value |
| `ToLower` / `ToUpper` | Change case |
| `TrimPrefix` / `TrimSuffix` | Remove a prefix or suffix |

### Template Examples

| Destination URL | Request | Redirects to |
|-----------------|---------|-------------|
| `https://google.com/search?q={{QueryEscape .Path}}` | `/g/hello world` | `https://google.com/search?q=hello+world` |
| `https://github.com/{{.User}}` | `/my-gh` | `https://github.com/alice@example.com` |
| `https://cal.com/{{TrimSuffix .User "@example.com"}}` | `/cal` | `https://cal.com/alice` |
