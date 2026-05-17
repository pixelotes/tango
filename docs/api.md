# Tango REST API

Base URL: `http://<host>:<port>/api` (default `http://localhost:7246/api`).

All requests and responses use JSON unless otherwise noted. Responses are not wrapped — endpoints return the resource directly or, on error, an `{ "error": "..." }` object.

CORS is open (`Access-Control-Allow-Origin: *`, methods `GET, POST, DELETE, OPTIONS`). There is no built-in authentication; if you expose Tango publicly, put it behind a reverse proxy that handles auth.

## Conventions

- **IDs.** The `id` field on a download is the aria2c **GID** (a short hex string). When you add a magnet link, the GID you get back is the metadata GID; once metadata is fetched, aria2c spawns a separate content GID. Tango records the redirect, so calls to `GET /api/torrents/{old-id}` keep working against the new content GID — you don't have to track the swap yourself.
- **Numbers.** Speeds are bytes/second. Sizes are bytes. `eta` is seconds (omitted/`null` if unknown).
- **Timestamps.** RFC 3339 (`added_at`).
- **Status strings.** One of `Downloading`, `Seeding`, `Waiting`, `Stopped`, `Error`, `Complete`, `Removed`. (Tango maps aria2c's lowercase states to these.)
- **Errors.** HTTP status code carries the result; the body is `{ "error": "message" }` on 4xx/5xx.

---

## `GET /api/health`

Liveness probe with version and current torrent count.

**Response 200**

```json
{
  "status": "ok",
  "version": "0.1.0",
  "torrents": 5
}
```

---

## `GET /api/stats`

Global session statistics, summed across all downloads.

**Response 200**

```json
{
  "torrent_count": 5,
  "speed_download": 1843204,
  "speed_upload": 521340,
  "total_download": 0,
  "total_upload": 0,
  "active_count": 2,
  "waiting_count": 0,
  "stopped_count": 3
}
```

| Field | Type | Notes |
|---|---|---|
| `torrent_count` | int | `active + waiting + stopped` |
| `speed_download` | int | bytes/sec, global |
| `speed_upload` | int | bytes/sec, global |
| `total_download` | int | reserved; currently `0` |
| `total_upload` | int | reserved; currently `0` |
| `active_count` | int | downloads currently transferring |
| `waiting_count` | int | queued |
| `stopped_count` | int | paused / completed / errored / removed (still in aria2c's result list) |

---

## `GET /api/torrents`

List every known download (active + waiting + stopped). Metadata-only magnet phases are filtered out — only the user-facing content download is returned for each magnet.

**Response 200** — array of downloads (without `files`, `peer_list`, `tracker_list`):

```json
[
  {
    "id": "2089b05ecca3424f",
    "name": "ubuntu-24.04.1-desktop-amd64.iso",
    "info_hash": "9f9165d9a281a9b8e782cd5176bbcc8256fd1871",
    "status": "Downloading",
    "progress": 0.4231,
    "size": 6203355136,
    "downloaded": 2624577536,
    "uploaded": 0,
    "download_speed": 1843204,
    "upload_speed": 0,
    "eta": 1942,
    "peers": 27,
    "seeders": 15,
    "seed_ratio": 0,
    "label": "iso",
    "target_path": "/srv/media/iso",
    "save_dir": "/tango/data",
    "moved": false,
    "added_at": "2026-05-17T11:42:03Z"
  }
]
```

See the field reference under [Download object](#download-object).

---

## `POST /api/torrents`

Add a download from a URI: magnet link, HTTP(S) URL, FTP URL, or Metalink URL.

**Request body**

```json
{
  "uri": "magnet:?xt=urn:btih:9F9165D9A281A9B8E782CD5176BBCC8256FD1871&dn=ubuntu...",
  "target_path": "/srv/media/iso",
  "label": "iso"
}
```

| Field | Required | Notes |
|---|---|---|
| `uri` | yes | Magnet, HTTP, HTTPS, FTP, or Metalink URI |
| `target_path` | no | Final destination. If set and different from `data_dir`, Tango moves files here after completion. If empty, files stay in `data_dir`. |
| `label` | no | Arbitrary label, stored and echoed back for filtering on the client side |

**Response 200** — the newly created [download object](#download-object) (without `files`/`peer_list`/`tracker_list`).

**Errors**
- `400 invalid JSON body`
- `400 uri is required`
- `500` with aria2c error message (e.g. bad magnet)

---

## `POST /api/torrents/file`

Add a download from a `.torrent` file. Multipart form upload.

**Request** — `multipart/form-data`:

| Field | Type | Required |
|---|---|---|
| `file` | file | yes — the `.torrent` payload |
| `target_path` | string | no |
| `label` | string | no |

**Response 200** — the newly created [download object](#download-object).

**Errors**
- `400 invalid multipart form`
- `400 file field is required`
- `500` with aria2c error message

Example with `curl`:

```bash
curl -F file=@ubuntu.torrent \
     -F target_path=/srv/media/iso \
     -F label=iso \
     http://localhost:7246/api/torrents/file
```

---

## `GET /api/torrents/{id}`

Single download with full detail — includes `files`, `peer_list`, and `tracker_list`.

**Response 200**

```json
{
  "id": "2089b05ecca3424f",
  "name": "ubuntu-24.04.1-desktop-amd64.iso",
  "info_hash": "9f9165d9a281a9b8e782cd5176bbcc8256fd1871",
  "status": "Downloading",
  "progress": 0.4231,
  "size": 6203355136,
  "downloaded": 2624577536,
  "uploaded": 0,
  "download_speed": 1843204,
  "upload_speed": 0,
  "eta": 1942,
  "peers": 27,
  "seeders": 15,
  "seed_ratio": 0,
  "label": "iso",
  "target_path": "/srv/media/iso",
  "save_dir": "/tango/data",
  "moved": false,
  "added_at": "2026-05-17T11:42:03Z",
  "files": [
    {
      "path": "ubuntu-24.04.1-desktop-amd64.iso",
      "size": 6203355136,
      "completed": 2624577536,
      "progress": 0.4231
    }
  ],
  "peer_list": [
    {
      "addr": "192.0.2.42:51413",
      "download_speed": 86420,
      "upload_speed": 0,
      "seeder": true
    }
  ],
  "tracker_list": [
    { "url": "udp://tracker.opentrackr.org:1337/announce" }
  ]
}
```

**Errors**
- `404 download not found: {id}`

---

## `DELETE /api/torrents/{id}`

Remove a download. The torrent is removed from aria2c's result list; files on disk are kept unless `delete_data=true`.

**Query parameters**

| Param | Type | Default | Notes |
|---|---|---|---|
| `delete_data` | bool | `false` | When `true`, also deletes the files on disk |

**Response** — `204 No Content` on success.

**Errors**
- `404 download not found: {id}` (only for outright unknown IDs; an already-stopped download is removed silently)

```bash
curl -X DELETE 'http://localhost:7246/api/torrents/2089b05ecca3424f?delete_data=true'
```

---

## `POST /api/torrents/{id}/start`

Resume a paused download.

**Response** — `204 No Content`.

**Errors** — `404 download not found: {id}`

---

## `POST /api/torrents/{id}/stop`

Pause a running download.

**Response** — `204 No Content`.

**Errors** — `404 download not found: {id}`

---

## `GET /api/torrents/{id}/files`

File list with per-file progress.

**Response 200**

```json
[
  {
    "path": "Show.Name.S01/Show.Name.S01E01.mkv",
    "size": 1547283456,
    "completed": 1547283456,
    "progress": 1
  },
  {
    "path": "Show.Name.S01/Show.Name.S01E02.mkv",
    "size": 1593847234,
    "completed": 234567890,
    "progress": 0.1471
  }
]
```

Paths are relative to `save_dir`. (aria2c returns absolute paths; Tango strips the `save_dir` prefix so consumers don't double it.)

**Errors** — `404 download not found: {id}`

---

## `GET /api/torrents/{id}/peers`

Connected peers, BitTorrent only.

**Response 200**

```json
[
  {
    "addr": "192.0.2.42:51413",
    "download_speed": 86420,
    "upload_speed": 0,
    "seeder": true
  }
]
```

For non-BitTorrent downloads, returns `[]`.

**Errors** — `404 download not found: {id}`

---

## `GET /api/torrents/{id}/trackers`

Tracker URLs from the torrent's announce list (flattened across tiers).

**Response 200**

```json
[
  { "url": "udp://tracker.opentrackr.org:1337/announce" },
  { "url": "udp://exodus.desync.com:6969/announce" }
]
```

For non-BitTorrent downloads, returns `null`.

**Errors** — `404 download not found: {id}`

---

## `POST /api/torrents/{id}/trackers`

Accepted for compatibility but no-op: Tango handles tracker enrichment globally via the `trackers.auto_add` config. Returns `204 No Content`.

---

## Download object

Fields returned by the download endpoints.

| Field | Type | Notes |
|---|---|---|
| `id` | string | aria2c GID. Stable across magnet metadata → content redirects. |
| `name` | string | Torrent name (BitTorrent) or first file path (HTTP/FTP). |
| `info_hash` | string | 40-char hex info hash (BitTorrent only; empty otherwise). |
| `status` | string | `Downloading` \| `Seeding` \| `Waiting` \| `Stopped` \| `Error` \| `Complete` \| `Removed` |
| `progress` | float | `0.0`–`1.0`. `0` when total size is unknown. |
| `size` | int | Total size in bytes (`0` until known). |
| `downloaded` | int | Bytes downloaded. |
| `uploaded` | int | Bytes uploaded. |
| `download_speed` | int | bytes/sec. |
| `upload_speed` | int | bytes/sec. |
| `eta` | int \| null | Seconds. `null` when not computable (paused, no progress, complete). |
| `peers` | int | aria2c "connections" count. |
| `seeders` | int | Number of remote seeders. |
| `seed_ratio` | float | `uploaded / downloaded`. `0` when nothing downloaded yet. |
| `label` | string | Whatever you passed at add time. |
| `target_path` | string | Final destination set at add time. Empty = stays in `save_dir`. |
| `save_dir` | string | aria2c's working directory for this download (`data_dir`). |
| `moved` | bool | `true` once the completion handler has moved files to `target_path`. |
| `added_at` | RFC3339 string | When Tango first saw the download. |
| `files` | `FileInfo[]` | Only on the single-download endpoint. |
| `peer_list` | `PeerInfo[]` | Only on the single-download endpoint. |
| `tracker_list` | `TrackerInfo[]` | Only on the single-download endpoint. |

### FileInfo

| Field | Type | Notes |
|---|---|---|
| `path` | string | Path relative to `save_dir`. |
| `size` | int | bytes |
| `completed` | int | bytes |
| `progress` | float | `0.0`–`1.0` |

### PeerInfo

| Field | Type | Notes |
|---|---|---|
| `addr` | string | `ip:port` |
| `download_speed` | int | bytes/sec |
| `upload_speed` | int | bytes/sec |
| `seeder` | bool |  |

### TrackerInfo

| Field | Type |
|---|---|
| `url` | string |

---

## Notes on magnet links

When you POST a magnet URI, aria2c first downloads the metadata (`.torrent`) and then spawns a separate "content" download. Both phases have their own GID, and aria2c eventually purges the metadata GID from memory.

Tango papers over this so callers don't have to:

- On `GET /api/torrents`, only the content download is listed (metadata-only entries are filtered).
- On `GET /api/torrents/{metadata-gid}`, Tango transparently redirects to the content download. The metadata→content mapping is persisted in SQLite, so the redirect keeps working after aria2c forgets the metadata GID.
- `target_path` and `label` set on the magnet are inherited by the content download automatically.

In other words: the GID you got back from `POST /api/torrents` is the one to keep using.

---

## Errors

| Status | Meaning |
|---|---|
| `400` | Malformed request body / missing required field |
| `404` | Unknown download ID |
| `500` | Internal error — body contains the underlying message (often from aria2c) |

Error body shape:

```json
{ "error": "uri is required" }
```

---

## Quick `curl` cheatsheet

```bash
# Add a magnet
curl -X POST http://localhost:7246/api/torrents \
  -H 'Content-Type: application/json' \
  -d '{"uri":"magnet:?xt=urn:btih:...", "target_path":"/srv/media/movies", "label":"movies"}'

# Add a .torrent file
curl -F file=@/path/to/file.torrent \
     -F target_path=/srv/media/movies \
     http://localhost:7246/api/torrents/file

# List
curl http://localhost:7246/api/torrents

# Single
curl http://localhost:7246/api/torrents/2089b05ecca3424f

# Pause / resume
curl -X POST http://localhost:7246/api/torrents/2089b05ecca3424f/stop
curl -X POST http://localhost:7246/api/torrents/2089b05ecca3424f/start

# Delete and keep data
curl -X DELETE http://localhost:7246/api/torrents/2089b05ecca3424f

# Delete and remove files
curl -X DELETE 'http://localhost:7246/api/torrents/2089b05ecca3424f?delete_data=true'

# Stats
curl http://localhost:7246/api/stats
```
