# Improvements Over aria2c

Tango is a lightweight orchestrator built on top of aria2c, extending its capabilities with a focus on usability, reliability, and automation.

## Key Features

### 1. Embedded Web UI
- **Single binary deployment** – Alpine.js + Bootstrap 5 are bundled via `go:embed`, so no external static files or separate server are needed.
- **REST endpoint** – UI is reachable at `http://localhost:7246/ui`.
- **Responsive design** – Works on desktop browsers and headless terminal emulators.

### 2. Process Supervision & Session Persistence
- **Automatic restart** – If the `aria2c` subprocess crashes, Tango restarts it and restores the session file from disk.
- **Graceful shutdown** – Listens for SIGINT/SIGTERM, drains active downloads, and finishes in‑flight torrents before exiting.

### 3. Global Tracker Enrichment
- Periodically fetches a public tracker list (`https://raw.githubusercontent.com/ngosang/trackerslist/master/trackers_all.txt`) and injects it into new torrents.
- Controlled by `trackers.auto_add` and `trackers.update_interval`.

### 4. Transparent Magnet Metadata Handling
- When a magnet URI is added, the initial GID is the *metadata* GID; the content GID is stored in SQLite so it survives after metadata is fetched.
- Callers can continue using the original GID without tracking the redirect manually.

### 5. Smart Seeding Control
- A background goroutine monitors the **upload/download ratio** of all active torrents.
- When the ratio exceeds `seeding.target_ratio`, the engine can **pause** the torrent or **auto‑remove** it (configurable via `auto_remove` and `auto_delete_data`).
- Configurable `max_seed_time_minutes` to limit total seeding duration.

### 6. Automatic Completion Handling & File Placement
- Upon reaching *complete* status, Tango moves the downloaded files to a user‑specified `target_path` (hardlink or copy‑then‑delete fallback).
- The original `data_dir` remains available for ongoing downloads, allowing a clear separation of “active” vs. “archived” data.

### 7. REST‑API Friendly Status Reporting
- Human‑readable status strings (`Downloading`, `Seeding`, `Waiting`, `Stopped`, `Error`, `Complete`, `Removed`).
- Progress fields: speed, peer count, seeders, seed ratio, eta, etc.
- Error responses include HTTP status codes and underlying aria2c messages.

### 8. Configurability via YAML
- All runtime options are stored in `config.yaml` (host, port, data directory, aria2c RPC URL, seeding limits, tracker list, etc.).
- Configuration can also be overridden via command‑line flags or environment variables.

### 9. Extensible Modular Architecture
- Core components (process, client, engine, watchdog) are defined as separate interfaces.
- New features can be added without touching the binary; they are wired through the existing configuration system.

### 10. Integrated Logging & Version Reporting
- Version string printed on startup.
- Detailed logs for process lifecycle, tracker enrichment, seeding decisions, and error conditions.

These enhancements transform aria2c from a raw downloader into a **self‑contained, production‑ready download orchestrator** that can run on minimal hardware (e.g., Raspberry Pi) and be driven by scripts, a web UI, or external applications.