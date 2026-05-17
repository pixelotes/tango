# Tango

A lightweight orchestrator that wraps [aria2c](https://aria2.github.io/) and turns it into a self-contained download daemon: a single Go binary, a REST API, an embedded web UI, automatic seeding control, and per-download target paths.

Designed to run comfortably on a Raspberry Pi 3B (1 GB RAM) and to be driven from other tools (e.g. [Reel](https://github.com/pixelotes/reel)) via HTTP.

> **Tango is not a fork of aria2.** It spawns `aria2c` as a child process and talks to it over its built-in JSON-RPC interface on `localhost:6800`. aria2c does the heavy lifting; Tango adds the orchestration that aria2c doesn't ship with.

## What it does

aria2c is a great multi-protocol downloader (BitTorrent, HTTP, FTP, Metalink) but it's a low-level tool. It doesn't move files after completion, doesn't enforce a global seeding policy across torrents, and its JSON-RPC API is not what most clients expect.

Tango fills those gaps:

- **Per-download target paths.** Every download can carry a `target_path`. When a torrent finishes, Tango moves the files there — hardlinking when possible, falling back to copy + delete across filesystems.
- **Seeding policy.** A background monitor enforces a configurable upload ratio and/or a maximum seed time, with optional auto-removal (and optional data deletion).
- **REST API.** A small, opinionated HTTP API (see [docs/api.md](docs/api.md)) — not a clone of qBittorrent's or Transmission's. It's shaped around what consumers actually need.
- **Embedded web UI.** Alpine.js + Bootstrap 5 served from the same binary via `go:embed`. No separate static files to ship.
- **Process supervision.** Tango watches the `aria2c` subprocess and restarts it if it crashes. The aria2c session file is persisted to disk so in-flight downloads survive a restart.
- **Tracker enrichment (optional).** Periodically fetches a public tracker list (default: [ngosang/trackerslist](https://github.com/ngosang/trackerslist)) and injects it into every new torrent.
- **Magnet metadata handling.** Magnet-link metadata downloads are followed transparently: the user-facing GID stays stable, metadata (target path, label) is inherited by the real content GID.

## Why a custom API (and not qBittorrent's)

This was a deliberate choice. qBittorrent's WebUI API is huge, partially undocumented, and shaped by its own internal model (categories, save paths, queue priorities, file priorities with magic integers, …). Most of it is irrelevant for a headless downloader being driven by another program.

Tango exposes a small JSON surface: add a download, list downloads, get one, pause/resume, delete. That's almost it. The response shape is flat, well-typed, and consistent across endpoints — no `null` vs `"-1"` vs empty-string disagreements, no separate "preferences" endpoint to discover where files actually went.

See [docs/api.md](docs/api.md) for the full reference.

## Quick start

### Docker (recommended)

```bash
docker compose up -d
```

Then open `http://localhost:7246` for the web UI, or hit `http://localhost:7246/api/health`.

Bind mounts in [docker-compose.yml](docker-compose.yml):
- `./downloads` → aria2c's working directory (where files land, plus `dht.dat`, `dht6.dat`, `aria2.session`)
- `session` (named volume) → SQLite database
- `./config.docker.yaml` → mounted read-only as `/tango/config.yaml`

Exposed ports:
- `7246/tcp` — API + web UI
- `6881/tcp` + `6881/udp` — BitTorrent peer/DHT port

### From source

```bash
go build -o tango .
./tango --config config.yaml
```

You'll need `aria2c` on `$PATH` (or set `aria2.bin_path` in the config).

## Configuration

See [config.yaml](config.yaml) for the full annotated example. Highlights:

```yaml
host: "0.0.0.0"
port: 7246
data_dir: "./data"               # aria2c's working dir
database_path: "./session/tango.db"
webui_enabled: true

aria2:
  rpc_url: "http://127.0.0.1:6800/jsonrpc"
  listen_port: 6881
  max_concurrent: 5
  dht_enabled: true
  pex_enabled: true
  disk_cache: 16777216           # 16 MiB — tuned for Pi

trackers:
  auto_add: false                # set true to enrich new torrents with public trackers
  list_url: "https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_all.txt"
  update_interval: "168h"

seeding:
  target_ratio: 1.5              # stop seeding at 1.5x
  max_seed_time_minutes: 0       # 0 = no time limit
  check_interval_seconds: 30
  auto_remove: false             # remove torrent from list after target ratio
  auto_delete_data: false        # …and also delete files
```

Override the port with `--port`:

```bash
./tango --config config.yaml --port 8080
```

## Seeding control

aria2c has a `--seed-ratio` flag, but it's per-download and aria2c doesn't restart-seed or globally enforce limits across the queue. Tango runs its own monitor goroutine that polls active torrents every `check_interval_seconds`:

1. For each torrent in seeding state, compute `uploaded / downloaded`.
2. Persist the current ratio in SQLite.
3. If the ratio crosses `target_ratio`:
   - `auto_remove: false` → pause the torrent.
   - `auto_remove: true` → force-remove from aria2c (optionally delete the files too).
4. If `max_seed_time_minutes > 0` and the elapsed time since completion exceeds it, pause.

## Completion handler

Every 10 seconds, the engine looks for stopped/complete downloads whose `target_path` differs from aria2c's `dir`. For each:

1. Try `os.Link` (hardlink) — instant, no extra disk space.
2. If that fails (cross-filesystem, etc.), fall back to copy + delete.
3. Mark the download as moved in SQLite so it isn't re-processed.

This lets you keep aria2c's `data_dir` on a fast disk and move finished files to bulk storage, all driven by the `target_path` you set when adding the download.

## REST API

Full reference: **[docs/api.md](docs/api.md)**.

Summary:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/health` | Liveness + version + torrent count |
| `GET` | `/api/stats` | Global speed and counters |
| `GET` | `/api/torrents` | List all downloads |
| `POST` | `/api/torrents` | Add a magnet/HTTP/FTP URI |
| `POST` | `/api/torrents/file` | Add a `.torrent` file (multipart) |
| `GET` | `/api/torrents/{id}` | One download with files/peers/trackers |
| `DELETE` | `/api/torrents/{id}?delete_data=true` | Remove |
| `POST` | `/api/torrents/{id}/start` | Resume |
| `POST` | `/api/torrents/{id}/stop` | Pause |
| `GET` | `/api/torrents/{id}/files` | File list with per-file progress |
| `GET` | `/api/torrents/{id}/peers` | Connected peers |
| `GET` | `/api/torrents/{id}/trackers` | Tracker list |

CORS is open (`*`) — the daemon is meant to be run on a trusted LAN and consumed by other tools. There's no built-in authentication; if you expose it publicly, put it behind a reverse proxy with auth.

## Status

Pre-1.0. The HTTP surface is small and stable in practice (it's what Reel uses), but breaking changes are still possible. Pin a commit if you build against it.

## License

See repository.
