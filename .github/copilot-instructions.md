# dumbs — Agent Instructions

`dumbs` is a deliberately misbehaving Go HTTP server used to teach sysadmin/devops skills. It simulates realistic production behaviour and exposes intentional chaos modes. Keep all code consistent with this purpose: production-grade idioms, no shortcuts, no frameworks beyond those already in use.

## Module & Build

- Module: `github.com/thordendal/dumbs`
- Go 1.26.1 — use modern Go idioms (`any`, `math/rand/v2`, range-over-int)
- `go build ./...` and `go vet ./...` must pass clean before any change is considered done
- No code generation, no Makefile — the binary is just `go build .`

## Dependencies (do not add new ones without discussion)

| Package | Purpose |
|---------|---------|
| `github.com/labstack/echo/v4` | HTTP framework |
| `github.com/labstack/echo-contrib/echoprometheus` | Prometheus middleware — always use `NewMiddlewareWithConfig` with `Registerer: m.Registry` |
| `github.com/rs/zerolog v1.35.0` | Structured logging — `logger.Get()` returns `*zerolog.Logger` (pointer receiver, required by v1.35) |
| `github.com/jackc/pgx/v5` | Postgres — raw SQL, no ORM |
| `github.com/prometheus/client_golang` | Private registry (`metrics.New()`); never use the default global registry |
| `gopkg.in/yaml.v3` | Config parsing with `KnownFields(true)` — unknown keys are errors |

## Project Layout

```
main.go                          # CLI flag, wiring, signal loop — keep thin
internal/config/config.go        # Nested YAML config, hot-reload, mutex
internal/logger/logger.go        # zerolog init + hot-swap; Init(cfg) once, Apply(cfg) on reload
internal/database/db.go          # pgxpool wrapper; Connect/InitSchema/ResetSchema/Ping/Close
internal/health/health.go        # IsLive() bool, IsReady() (bool, error)
internal/metrics/metrics.go      # Private Prometheus registry + chaos gauges + config_reloads counter
internal/chaos/types.go          # Resource doc structs (LogsConfig, DatadirConfig, …) + Patch structs
internal/chaos/manager.go        # ChaosManager: Apply*/Patch*/Activate*/Reset*/GetAll/ResetAll/StopAll
internal/chaos/logs.go           # Log-flood worker (~100 MB/min)
internal/chaos/datadir.go        # Disk-fill worker (sequential .bin files)
internal/chaos/database.go       # DB chaos: rapidInserts / hangingTxns / badQueries
internal/chaos/memory.go         # Memory-leak worker (~50 MB/min default)
internal/server/server.go        # Echo instance, middleware stack, route registration
internal/server/handlers.go      # All HTTP handlers
example.yaml                     # Annotated config reference
docker-compose.yaml              # postgres:17 for local dev
```

## Config Shape

```yaml
log:
  level: info        # debug | info | warn | error
  format: json       # json | plain (ConsoleWriter — for grep/awk teaching)
  output: stdout     # "stdout" or absolute file path
app:
  data_dir: /var/lib/dumbs
  database:
    dsn: "postgres://..."
  listen: ":8080"
```

Config is loaded once at startup via `--config` flag and hot-reloaded on `SIGHUP` or `POST /config/reload`.

## Signal Handling

| Signal | Behaviour |
|--------|-----------|
| `SIGHUP` | Reload config + apply logger |
| `SIGUSR1` | `StopAll()` chaos workers (server stays up) |
| `SIGQUIT` | Graceful shutdown: StopAll → server.Shutdown(30s) → db.Close → exit 0 |
| `SIGTERM` / `SIGINT` | Hard exit: log "hard shutdown, not cleaning up" → `os.Exit(1)` — DB connections left dangling intentionally |

## HTTP API

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Always 200 "foobar\n" |
| `GET` | `/health/live` | Liveness — always 200 |
| `GET` | `/health/ready` | Readiness — 200 if DB ping OK, 503 otherwise |
| `GET` | `/metrics` | Prometheus metrics (private registry) |
| `POST` | `/config/reload` | Hot-reload config file |
| `GET` | `/chaos` | List all four chaos resource documents |
| `DELETE` | `/chaos` | Stop all + reset all to defaults |
| `GET` | `/chaos/:kind` | Read config + active state |
| `PUT` | `/chaos/:kind` | Replace full document; starts/stops/restarts worker |
| `PATCH` | `/chaos/:kind` | Partial update; omitted JSON fields unchanged |
| `POST` | `/chaos/:kind` | Activate with current config; 409 if already active |
| `DELETE` | `/chaos/:kind` | Stop + reset to defaults |
| `POST` | `/database/reset` | Drop + recreate the `events` table |

Valid `:kind` values: `logs`, `datadir`, `database`, `memory`.

## Chaos Resource Documents

Each worker is a REST resource. The document shape contains both config **and** active state. Example:

```json
{ "kind": "logs", "active": true, "rate_per_sec": 1520, "payload_bytes": 512 }
```

`active: true` = worker running. Setting `false` stops it. A config change while running = stop + restart (not hot-swap). All Patch structs use pointer fields (`*int`, `*bool`) so `nil` means "leave unchanged".

| Worker | Configurable fields | Defaults |
|--------|--------------------|-|
| `logs` | `rate_per_sec`, `payload_bytes` | 1520, 512 |
| `datadir` | `file_size_bytes` | 10 MiB |
| `database` | `insert_batch_size`, `bad_query_interval_ms`, `hanging_txn_interval_ms` | 1000, 200, 2000 |
| `memory` | `chunk_size_bytes`, `interval_ms` | 870 KiB, 1000 |

## Middleware Stack (order is fixed)

1. `Recover` — panic → 500, never crash the server
2. `RequestID` — injects `X-Request-ID`
3. `RequestLoggerWithConfig` — one structured zerolog line per request
4. `BodyLimit("1MB")`
5. `TimeoutWithConfig(30s)`
6. `echoprometheus.NewMiddlewareWithConfig` — uses private registry

## Coding Conventions

- **No global Prometheus registry.** Always pass `Registerer: m.Registry`.
- **zerolog pointer receiver.** `logger.Get()` returns `*zerolog.Logger`. Chain `.Info()`, `.Warn()`, `.Error()` on it directly.
- **Context cancellation for workers.** All chaos goroutines stop by `ctx.Done()`. No channels for stop signalling.
- **`sync.Mutex` on Manager.** All reads/writes to `cfgLogs`, `cfgDatadir`, etc. and the `leak` slice must be under `m.mu`.
- **Config snapshot at goroutine start.** Workers copy their config struct under the lock at entry, then use the snapshot for the whole run. Restart is the change mechanism.
- **Raw SQL.** Use `pgx/v5` directly. No GORM, sqlx, or query builders.
- **No ORM-style wrappers.** The `database.DB` type wraps the pool; handlers call `db.ResetSchema`, `db.Ping`, etc.
- **Error logging on 503.** `IsReady()` returns `(bool, error)`; the readiness handler logs `ERROR` with the actual DB error before returning 503.
- **Chaos sentinel errors are exported.** `chaos.ErrNoDB` and `chaos.ErrAlreadyActive` — map to HTTP 503 and 409 respectively in `chaosHTTPError`.
- **No versioning prefix** (`/v1/`). Paths start directly with the resource (`/chaos`, `/health`, …).
- **Startup log** includes `pid` and `go` version fields.

## Testing

There are currently no automated tests. If adding tests:
- Put them in `_test.go` files in the same package (`package chaos`, not `package chaos_test`), unless testing the public API surface.
- Use standard library `testing` package only — no testify.
- For HTTP handler tests, use `net/http/httptest` with a real Echo instance.
