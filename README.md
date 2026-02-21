# Dubly

A single-user, SQLite-backed link shortener with a built-in admin UI, in-memory analytics buffering, multiple custom domain support, and MaxMind geo lookup.

## Installation

The install script sets up everything on a fresh Ubuntu/Debian server: Go, Caddy (reverse proxy with auto-HTTPS), systemd service, firewall rules, and optional Litestream S3 backups.

```bash
curl -fsSL https://raw.githubusercontent.com/scmmishra/dubly/main/scripts/install.sh | sudo bash
```

Or clone first and run locally:

```bash
git clone https://github.com/scmmishra/dubly.git
cd dubly
sudo bash scripts/install.sh
```

The script will interactively prompt for your domain(s), API password, and optional S3/GeoIP configuration.

To update an existing installation:

```bash
sudo /opt/dubly/scripts/install.sh --update
```

## Local Development

```bash
go build -o dubly ./cmd/server

DUBLY_PASSWORD=your-secret-key \
DUBLY_DOMAINS=short.io,go.example.com \
./dubly
```

## Configuration

All configuration is via environment variables.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DUBLY_PASSWORD` | Yes | — | API key for authenticating requests |
| `DUBLY_DOMAINS` | Yes | — | Comma-separated allowed domains |
| `DUBLY_PORT` | No | `8080` | Server listen port |
| `DUBLY_DB_PATH` | No | `./dubly.db` | SQLite database file path |
| `DUBLY_APP_NAME` | No | `Dubly` | Display name used in the admin UI |
| `DUBLY_GEOIP_PATH` | No | — | Path to GeoLite2-City.mmdb (geo disabled if unset) |
| `DUBLY_FLUSH_INTERVAL` | No | `30s` | Analytics flush interval |
| `DUBLY_BUFFER_SIZE` | No | `50000` | Analytics channel buffer capacity |
| `DUBLY_CACHE_SIZE` | No | `10000` | LRU cache max entries |

## API

All `/api/*` routes require the `X-API-Key` header.

### Create a link

```bash
curl -X POST http://localhost:8080/api/links \
  -H "X-API-Key: your-secret-key" \
  -H "Content-Type: application/json" \
  -d '{
    "domain": "short.io",
    "destination": "https://example.com/some/long/url",
    "title": "Example Link",
    "tags": "demo",
    "slug": "custom-slug"
  }'
```

`slug` is optional — a random 6-character Base62 slug is generated if omitted.

### List links

```bash
curl "http://localhost:8080/api/links?limit=25&offset=0&search=example" \
  -H "X-API-Key: your-secret-key"
```

### Get a link

```bash
curl http://localhost:8080/api/links/1 \
  -H "X-API-Key: your-secret-key"
```

### Update a link

```bash
curl -X PATCH http://localhost:8080/api/links/1 \
  -H "X-API-Key: your-secret-key" \
  -H "Content-Type: application/json" \
  -d '{"destination": "https://example.com/new-url"}'
```

### Delete a link (soft delete)

```bash
curl -X DELETE http://localhost:8080/api/links/1 \
  -H "X-API-Key: your-secret-key"
```

Soft-deleted links return `410 Gone` on redirect.

## Redirects

Any request not under `/api/` or `/admin/` is treated as a redirect. The `Host` header determines the domain and the path determines the slug.

```
https://short.io/custom-slug → 302 → https://example.com/some/long/url
```

## Analytics

Click events are buffered in memory and flushed to SQLite in batches. Each click records:

- Timestamp, IP, referer
- Browser, OS, device type (parsed from User-Agent)
- Country, city, region, coordinates (from MaxMind GeoLite2, if configured)
