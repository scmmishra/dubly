# Dubly

A single-user, SQLite-backed link shortener with in-memory analytics buffering, multiple custom domain support, and MaxMind geo lookup.

## Quick Start

```bash
go build ./cmd/server

DUBLY_PASSWORD=your-secret-key \
DUBLY_DOMAINS=short.io,go.example.com \
./server
```

## Configuration

All configuration is via environment variables.

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DUBLY_PORT` | No | `8080` | Server listen port |
| `DUBLY_DB_PATH` | No | `./dubly.db` | SQLite database file path |
| `DUBLY_PASSWORD` | Yes | — | API key for authenticating requests |
| `DUBLY_DOMAINS` | Yes | — | Comma-separated allowed domains |
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
curl http://localhost:8080/api/links?limit=25&offset=0&search=example \
  -H "X-API-Key: your-secret-key"
```

### Get a link

```bash
curl http://localhost:8080/api/links/1 \
  -H "X-API-Key: your-secret-key"
```

### Update a link

```bash
curl -X PUT http://localhost:8080/api/links/1 \
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

Any request not under `/api/` is treated as a redirect. The `Host` header determines the domain and the path determines the slug.

```
https://short.io/custom-slug → 302 → https://example.com/some/long/url
```

## Analytics

Click events are buffered in memory and flushed to SQLite in batches. Each click records:

- Timestamp, IP, referer
- Browser, OS, device type (parsed from User-Agent)
- Country, city, region, coordinates (from MaxMind GeoLite2, if configured)

## Project Structure

```
dubly/
├── cmd/server/main.go          # Entry point
├── internal/
│   ├── config/config.go        # Env var parsing
│   ├── db/
│   │   ├── db.go               # SQLite connection
│   │   └── migrations.go       # Schema migrations
│   ├── models/
│   │   ├── link.go             # Link CRUD
│   │   └── click.go            # Click batch insert
│   ├── analytics/collector.go  # Buffered analytics
│   ├── handlers/
│   │   ├── middleware.go       # Auth middleware
│   │   ├── links.go            # REST handlers
│   │   └── redirect.go         # Redirect handler
│   ├── geo/geo.go              # MaxMind reader
│   ├── cache/lru.go            # LRU cache
│   └── slug/slug.go            # Slug generation
├── go.mod
└── go.sum
```
