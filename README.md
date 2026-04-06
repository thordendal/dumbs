# dumbs

A small HTTP server that behaves like a real production application: it exposes
health probes, Prometheus metrics, structured logs, and a config reload endpoint.
It also ships a set of **chaos endpoints** that deliberately misbehave — filling
disks, flooding logs, abusing the database, and leaking memory — so you can
practise diagnosing and fixing problems the way you would on an actual server.

---

## Prerequisites

| Requirement | Minimum version |
|-------------|-----------------|
| Go toolchain | 1.21 |
| PostgreSQL | 14 |

The app runs fine without a database (all endpoints except `/health/ready` and
the database chaos will work), but most exercises require one.

---

## Building

```sh
git clone <repo-url>
cd dumbs
go build -o dumbs .
```

The resulting `dumbs` binary has no runtime dependencies beyond a libc.

---

## PostgreSQL setup

Create a dedicated role and database before the first run:

```sql
-- run as the postgres superuser
CREATE USER dumbs WITH PASSWORD 'secret';
CREATE DATABASE dumbs OWNER dumbs;
```

The app will create the required tables automatically on startup.

---

## Configuration

Configuration is a YAML file. Pass its path with the `-config` flag.

```
./dumbs -config /etc/dumbs/dumbs.yaml
```

A fully annotated example is provided in [`example.yaml`](example.yaml).

### Reference

```yaml
---
log:
  level: info          # debug | info | warn | error
  format: json         # json  — one JSON object per line (pipe to jq)
                       # plain — human-readable key=val lines (grep/awk friendly)
  output: stdout       # stdout, or an absolute path to a log file

app:
  data_dir: /var/lib/dumbs   # must be writable by the process
  database:
    dsn: "postgres://dumbs:secret@localhost:5432/dumbs?sslmode=disable"
  listen: ":8080"      # host:port or a Unix socket path
```

### Hot reload

Send `SIGHUP` or call `POST /config/reload` to apply a new `log.level`,
`log.format`, or `log.output` without restarting the process.
`data_dir`, `listen`, and `database.dsn` are only read at startup.

---

## Running

```sh
./dumbs -config ./example.yaml
```

### Under systemd

An example unit is in [systemd.example](systemd.example)


## Signal handling

| Signal | Effect |
|--------|--------|
| `SIGHUP` (1) | Reload config file; apply new log level / format / output |
| `SIGQUIT` (3) | **Graceful stop**: finish in-flight requests, close DB connections cleanly |
| `SIGUSR1` (10) | Stop all active chaos workers; server keeps running |
| `SIGTERM` (15) / `SIGINT` (2) | **Hard stop**: process exits immediately, DB connections are abandoned |

```sh
kill -HUP  $(pidof dumbs)   # reload config
kill -QUIT $(pidof dumbs)   # graceful shutdown
kill -USR1 $(pidof dumbs)   # stop all chaos
```

---

## HTTP API

### General

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Returns `200 foobar`. Simple smoke-test. |
| `GET` | `/health/live` | Liveness probe — `200` as long as the process is running. |
| `GET` | `/health/ready` | Readiness probe — `200` if the database is reachable, `503` otherwise. |
| `GET` | `/metrics` | Prometheus metrics (text exposition format). |
| `POST` | `/config/reload` | Re-read the config file and apply log settings. |
| `POST` | `/database/reset` | Drop and recreate the `events` table. |

### Chaos

All chaos endpoints return `200` on success, `409` if the worker is already in
the requested state, and `503` if a required resource (e.g. database) is
unavailable.

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/chaos/logs/start` | Start flooding the log output. |
| `POST` | `/chaos/logs/stop` | Stop log flooding. |
| `POST` | `/chaos/datadir/start` | Start writing files into `data_dir`. |
| `POST` | `/chaos/datadir/stop` | Stop writing files. |
| `POST` | `/chaos/database/start` | Start database chaos. |
| `POST` | `/chaos/database/stop` | Stop database chaos. |
| `POST` | `/chaos/memory/start` | Start leaking memory (~50 MB/min). |
| `POST` | `/chaos/memory/stop` | Stop memory leak. |
| `GET` | `/chaos/status` | JSON object showing which workers are active. |

#### Example

```sh
# Start log flooding
curl -s -X POST http://localhost:8080/chaos/logs/start

# Check what's running
curl -s http://localhost:8080/chaos/status | jq .

# Stop everything
kill -USR1 $(pidof dumbs)
```

---

## Filesystem layout

```
/etc/dumbs/dumbs.yaml        config file
/var/lib/dumbs/              data directory (must be writable by the process)
/var/lib/dumbs/chaos/        chaos files are written here
/var/log/dumbs/dumbs.log     log file (if log.output points here)
```

Permissions recommended for a systemd setup:

```sh
useradd --system --no-create-home --shell /usr/sbin/nologin dumbs
install -d -o dumbs -g dumbs -m 750 /var/lib/dumbs
install -d -o dumbs -g dumbs -m 750 /var/log/dumbs
install -d -o root  -g root  -m 755 /etc/dumbs
```

---

## Metrics

The `/metrics` endpoint exposes standard Go runtime and process metrics plus:

| Metric | Type | Description |
|--------|------|-------------|
| `dumbs_chaos_logs_active` | Gauge | `1` while log-flood chaos is running |
| `dumbs_chaos_datadir_active` | Gauge | `1` while datadir chaos is running |
| `dumbs_chaos_database_active` | Gauge | `1` while database chaos is running |
| `dumbs_chaos_memory_active` | Gauge | `1` while memory-leak chaos is running |
| `dumbs_config_reloads_total` | Counter | Total successful config reloads |
| `dumbs_request_duration_seconds` | Histogram | HTTP request latency by method/path/status |
</content>
</invoke>