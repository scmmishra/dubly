# Dubly

URL shortener backend — Go, SQLite, chi router.

## Quick Reference

```bash
go run ./cmd/server          # start server (needs DUBLY_PASSWORD + DUBLY_DOMAINS)
go test ./internal/... -v    # run all tests
```

## Project Structure

```
cmd/server/main.go           # entry point, router wiring, graceful shutdown
internal/
  config/                    # env-based configuration (Load, IsDomainAllowed)
  db/                        # SQLite connection, pragmas, migrations
  models/                    # Link and Click CRUD (plain sql.DB, no ORM)
  slug/                      # crypto/rand Base62 slug generation (6 chars)
  cache/                     # LRU cache wrapping hashicorp/golang-lru
  geo/                       # MaxMind GeoIP lookup with no-op fallback
  analytics/                 # Buffered click collector with background flush
  handlers/                  # HTTP handlers (links CRUD, redirect, auth middleware)
```

## Architecture Decisions

- **Single SQLite file** with WAL mode, FK enforcement, single-writer (`MaxOpenConns(1)`)
- **Soft deletes** — `is_active=0`, never hard delete. Redirect returns 410 for inactive links.
- **Slug uniqueness** is per-domain (`UNIQUE(slug, domain)`). `SlugExists` checks ALL rows including soft-deleted to prevent constraint collisions.
- **Cache invalidation** on update captures the old domain/slug key *before* mutation.
- **Analytics collector** is non-blocking — drops events when buffer is full rather than applying backpressure.
- **No-op geo reader** when GeoIP path is empty — graceful degradation, no error.

## Environment Variables

| Variable | Required | Default |
|---|---|---|
| `DUBLY_PASSWORD` | yes | — |
| `DUBLY_DOMAINS` | yes | — (comma-separated) |
| `DUBLY_PORT` | no | 8080 |
| `DUBLY_DB_PATH` | no | ./dubly.db |
| `DUBLY_GEOIP_PATH` | no | "" (geo disabled) |
| `DUBLY_FLUSH_INTERVAL` | no | 30s |
| `DUBLY_BUFFER_SIZE` | no | 50000 |
| `DUBLY_CACHE_SIZE` | no | 10000 |

## Testing Principles

### Use real infrastructure, not mocks
Every test uses a real in-memory SQLite database via `db.Open(":memory:")`. This gives real constraint checking, FK enforcement, and SQL behavior. The only stub is the geo reader (no-op when path is empty, which is the production fallback anyway).

### Black-box for handlers, white-box for packages
Handler tests live in `package handlers_test` and exercise the full HTTP stack through a real chi router wired identically to production. Package-level tests (config, models, slug, cache, geo, analytics) use same-package access where it simplifies setup.

### Test behavior, not implementation
Tests assert on observable outcomes: HTTP status codes, response bodies, database state. No testing of private helper functions in isolation unless they carry independent behavioral guarantees.

### Each test gets a fresh database
The `testDB(t)` helper creates an isolated in-memory SQLite per test. No shared state between tests, no ordering dependencies, no cleanup hassle.

### No testify — standard library only
Use `testing.T` with `t.Errorf` / `t.Fatalf`. Keep assertions explicit and readable without third-party matchers.

### Protect regression-prone boundaries
Three specific regression tests guard behaviors that are easy to accidentally break:
1. `GetLinkBySlugAndDomain` must return inactive links (otherwise redirect gives 404 instead of 410)
2. `SlugExists` must include soft-deleted rows (otherwise auto-generation can collide with UNIQUE constraint)
3. Update handler must invalidate cache using the *old* slug/domain key (otherwise stale redirects persist)

### Non-flaky timing tests
The analytics flush-on-ticker test uses a 50ms interval with a 200ms sleep — 4x margin. The non-blocking push test uses a 1-hour interval to guarantee zero auto-flushes during the test window.
