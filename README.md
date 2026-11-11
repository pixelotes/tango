# Tango

Tango is a web UI and REST API around [aria2c](https://aria2.github.io/) that adds tracker injection, seeding control and per-download target paths on top of aria2's JSON-RPC.

Part of [MangoStack](https://github.com/pixelotes/MangoStack), a self-hosted media stack for the Raspberry Pi.

Tango is not a fork of aria2. It starts `aria2c` as a child process, talks to it over JSON-RPC on `127.0.0.1:6800`, and restarts it if it crashes.

## Footprint

The container runs two processes: the Go server (static binary, `CGO_ENABLED=0`) and the `aria2c` child.

- MangoStack measured about 26 MB RSS idle for the whole container (Tango plus aria2c), with a 192 MB memory limit and the shipped settings tuned for a Raspberry Pi 3B (`disk_cache: 16777216`, `max_concurrent: 5`).
- Most of the memory under load is aria2c's: its disk cache (`aria2.disk_cache`, 16 MiB by default), the number of active downloads (`aria2.max_concurrent`) and their peers. The Go server stays small; it holds the download list and polls aria2c.
- Tracker enrichment (`trackers.auto_add`) adds a periodic HTTP fetch and keeps the tracker list in memory.

CPU is negligible at idle; under load it is aria2c hashing and moving data.

Disk writes:

- SQLite database at `database_path` (default `./session/tango.db`, WAL mode): download metadata (target path, label, ratio, moved flag).
- In `data_dir`: the downloads themselves, `aria2.session` (saved every 30 s, reloaded at start so downloads survive a restart), `dht.dat` and `dht6.dat` (when DHT is enabled), and saved torrent metadata (`--bt-save-metadata`).
- At `target_path`: finished files, hardlinked when possible, otherwise copied and the original deleted.

## Features

- Starts and supervises `aria2c`; restarts it if it exits.
- Per-download `target_path`: finished downloads are moved there (hardlink, falling back to copy + delete across filesystems). The check runs every 10 seconds.
- Seeding policy: a monitor checks seeding torrents every `check_interval_seconds` and pauses them at `target_ratio` or after `max_seed_time_minutes`; optionally removes them (`auto_remove`) and their data (`auto_delete_data`).
- Tracker injection: a built-in baseline tracker list is always passed to aria2c. With `trackers.auto_add`, Tango also fetches a public list (default [ngosang/trackerslist](https://github.com/ngosang/trackerslist)) and sets it as aria2c's global `bt-tracker`, so it applies to every new torrent.
- Magnet links: the GID returned to the client stays stable when aria2c switches from the metadata download to the real content; target path and label carry over.
- REST API (see [docs/api.md](docs/api.md)), compatible with the Storm API, so clients with a Storm client can drive Tango.
- Embedded web UI (Alpine.js + Bootstrap 5, served from the binary via `go:embed`).

## Quick start

```yaml
services:
  tango:
    image: pixelotes/tango:latest
    container_name: tango
    restart: unless-stopped
    ports:
      - "7246:7246"
      - "6881:6881"
      - "6881:6881/udp"
    volumes:
      - ./config.yaml:/tango/config.yaml:ro
      - ./session:/tango/session
      - ./downloads:/tango/data
```

Use [config.docker.yaml](config.docker.yaml) as `config.yaml`; it sets `data_dir: /tango/data`, `database_path: /tango/session/tango.db` and `bin_path: /usr/bin/aria2c`. The config file is required: Tango exits if it cannot read it.

Images are published as `pixelotes/tango:latest` and `pixelotes/tango:<last 8 chars of the commit SHA>`, for linux/amd64 and linux/arm64.

Then open `http://localhost:7246`, or check `http://localhost:7246/api/health`. Ports: `7246/tcp` for the API and web UI, `6881/tcp+udp` for BitTorrent peers and DHT.

Inside MangoStack it is already wired in `docker-compose.yaml`, with the downloads directory under `/storage`.

## Configuration

A YAML file passed with `--config` (default `config.yaml`). `--port` overrides `port`. There are no environment variables and no `${VAR}` substitution. Defaults come from `internal/config/config.go`:

```yaml
host: "0.0.0.0"
port: 7246
data_dir: "./data"                  # aria2c --dir: downloads, aria2.session, dht.dat
database_path: "./session/tango.db"
webui_enabled: true                 # false = API only

aria2:
  rpc_url: "http://127.0.0.1:6800/jsonrpc"
  rpc_secret: ""                    # set it if rpc_listen_all is true
  rpc_listen_all: false             # bind aria2's JSON-RPC on all interfaces
  bin_path: ""                      # empty = search $PATH
  listen_port: 6881                 # BitTorrent and DHT port
  max_concurrent: 5
  max_upload_speed: 0               # bytes/s, 0 = unlimited
  max_download_speed: 0             # bytes/s, 0 = unlimited
  disk_cache: 16777216              # bytes (16 MiB)
  dht_enabled: true                 # DHT and DHT6 (also controls dht.dat)
  pex_enabled: true                 # peer exchange
  max_connection_per_server: 4      # HTTP/FTP downloads, 1-16
  min_split_size: "10M"             # HTTP/FTP downloads

trackers:
  auto_add: false
  list_url: "https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_all.txt"
  update_interval: "168h"           # Go duration

seeding:
  target_ratio: 1.5                 # also passed to aria2c as --seed-ratio
  max_seed_time_minutes: 0          # 0 = no time limit
  check_interval_seconds: 30
  auto_remove: false
  auto_delete_data: false
```

Local peer discovery (LPD) is always on.

## Usage

### Web UI

`http://<host>:7246`: list, add (magnet/URL or `.torrent` file), pause, resume and remove downloads.

### REST API

Full reference: [docs/api.md](docs/api.md).

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/health` | Liveness, version, torrent count |
| `GET` | `/api/stats` | Global speeds and counters |
| `GET` | `/api/torrents` | List downloads |
| `POST` | `/api/torrents` | Add a magnet/HTTP/FTP URI |
| `POST` | `/api/torrents/file` | Add a `.torrent` file (multipart) |
| `GET` | `/api/torrents/{id}` | One download with files, peers, trackers |
| `DELETE` | `/api/torrents/{id}?delete_data=true` | Remove |
| `POST` | `/api/torrents/{id}/start` | Resume |
| `POST` | `/api/torrents/{id}/stop` | Pause |
| `GET` | `/api/torrents/{id}/files` | Files with per-file progress |
| `GET` | `/api/torrents/{id}/peers` | Connected peers |
| `GET` | `/api/torrents/{id}/trackers` | Trackers |
| `POST` | `/api/torrents/{id}/trackers` | Accepted as a no-op (Tango injects trackers itself) |

CORS is open (`*`).

### Reel

[Reel](https://github.com/pixelotes/reel) drives Tango with its `storm` torrent client type, because Tango implements the same REST API as Storm. Downloads added that way get Tango's tracker injection and per-download paths:

```yaml
torrent_client:
  type: "storm"
  host: "http://tango:7246"
```

## Building from source

```bash
go build -trimpath -o tango .
./tango --config config.yaml
```

Requires Go 1.27 (see `go.mod`) and `aria2c` on `$PATH` (or `aria2.bin_path`).

## Limits

- Do not expose Tango to the internet; it assumes a trusted LAN. The API has no authentication and CORS allows any origin. Put it behind an authenticating reverse proxy if it must be reachable from elsewhere.
- If you set `rpc_listen_all: true`, aria2's JSON-RPC is reachable from the network; always set `rpc_secret`.
- Pre-1.0: the API is what Reel uses and is stable in practice, but breaking changes are possible.

## Part of MangoStack

| App | Role |
|---|---|
| [Scarf](https://github.com/pixelotes/scarf) | Indexer proxy (Torznab) |
| [Parasol](https://github.com/pixelotes/parasol) | Optional Cloudflare solver for Scarf |
| [Reel](https://github.com/pixelotes/reel) | Movie & TV automation |
| [Tango](https://github.com/pixelotes/tango) | Web UI and REST API for aria2c |
| [RMS](https://github.com/pixelotes/rms) | Media server (Jellyfin API) |
| [Suika](https://github.com/pixelotes/suika) | Manga reader |
| [Kaze](https://github.com/pixelotes/kaze) | Music server (Subsonic API) |
| [Sumi](https://github.com/pixelotes/sumi) | EPUB reader |
| [Dashboarr](https://github.com/pixelotes/dashboarr) | Homepage |

## License

MIT
