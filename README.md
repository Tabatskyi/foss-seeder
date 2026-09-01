# FOSS Seeder 🌱

Automated FOSS Linux distro and open-source software seeder helper with a modern Web Interface and qBittorrent integration.

Written in **Go** with embedded **HTML templates + HTMX** — compiles into a single ~12MB lightweight static binary with zero external runtime dependencies.

---

## 🌟 Features

* **Multi-Feed RSS Aggregation & Selector**:
  * Support for multiple concurrent RSS feeds (e.g. `fosstorrents.com`, `distrowatch.com`, `academictorrents.com`, etc.).
  * Instant search and architecture filtering (e.g. `arch`, `debian`, `x86_64`, `aarch64`).
  * Filter releases by all feeds or by a specific source feed.
  * Automatic cross-feed deduplication with top-to-bottom priority ordering (feeds listed higher in settings take priority when the same torrent exists across multiple feeds).
  * **Automatic Size Detection & Preview**: Real-time extraction from RSS metadata, qBittorrent active sync, or async `.torrent` metainfo bencode parsing with background caching.
  * 1-click **Track Distro** to automatically create smart regex rules and queue downloads.
  * Direct `.torrent` file download buttons.
* **Flexible View Modes (Unified vs. Dedicated Tabs)**:
  * **Unified Distro Selector**: All releases aggregated in one view with a feed filter dropdown and source badges.
  * **Separate Feed Tabs Mode**: Dedicated tabs for each configured feed (`📦 FOSSTorrents`, `📦 DistroWatch`, etc.) with independent counters.
  * Quick toggle in settings or UI to switch between view modes.
* **Feed-Tied & Global Rules**:
  * Automatically tie rules to their source feed or configure rules to match across all feeds.
  * Dedicated "Source Feed" indicators in the Tracked Rules management table.
* **Auto-Purge Obsolete Versions**:
  * Automatically detects when a newer ISO version of a tracked family is released, adds the new version to qBittorrent, and purges older releases to save disk space.
* **qBittorrent Integration**:
  * Native qBittorrent Web API v2 client.
  * View active torrents, seed ratio, progress, and upload speeds.
  * Tag management and sequential download (first/last piece priority) support.
* **Real-time Event Stream**:
  * Live activity log stream powered by Server-Sent Events (SSE) directly in your browser.
* **Zero Runtime Dependencies**:
  * Entire web UI and static assets are embedded inside the Go executable (`//go:embed`).
  * Tiny Docker image (<25MB) using minimal RAM (~15MB).

---

## 🚀 Quick Start with Docker

```bash
docker compose up -d --build
```

Open your browser at:
👉 **`http://localhost:7474`**

---

## ⚙️ Configuration & Environment Variables

| Variable | Default | Description |
| :--- | :--- | :--- |
| `PORT` | `7474` | Web UI HTTP listening port |
| `FEED_URL` | `https://fosstorrents.com/feed/torrents.xml` | RSS feed endpoint |
| `FEED_URL` | `https://fosstorrents.com/feed/torrents.xml` | RSS feed endpoint(s) — supports multiple comma or newline-separated URLs |
| `QBIT_HOST` | `http://localhost:8800` | qBittorrent Web UI host URL |
| `QBIT_USER` | `admin` | qBittorrent username |
| `QBIT_PASS` | `adminadmin` | qBittorrent password |
| `QBIT_CATEGORY` | `foss-torrents` | Category assigned in qBittorrent |
| `SAVE_PATH` | `/downloads/foss` | Default download directory on disk |
| `CHECK_INTERVAL_SECONDS` | `43200` | Check interval in seconds (default: 12 hours) |
| `SEQUENTIAL_DOWNLOAD` | `true` | Enable sequential download and first/last piece priority |
| `CONFIG_PATH` | `data/config.json` | Persistent configuration & rules storage path |

---

## 🛠️ Building & Running Locally

### Prerequisites

* Go 1.22+

```bash
# Navigate to source directory
cd src

# Build the binary
go build -ldflags="-s -w" -o ../bin/foss-seeder ./cmd/foss-seeder

# Run tests
go test ./...

# Start the application
PORT=7474 ../bin/foss-seeder
```

---

## To-Do

- [x] Add multi-feed RSS support for additional FOSS distros and software sources.
- [x] Add size preview for ISOs.
- [ ] Add available space on disk to the UI.
- [ ] Add support for additional torrent clients (e.g. Transmission, Deluge, etc.).
- [ ] Replace Emojis with SVG icons for better cross-platform compatibility.

--- 

## Known Bugs

- UI frequently freezes, refreshing the page usually resolves the issue. This is likely due to a bug in HTMX or the SSE implementation.
