<p align="center">
  <img src="https://raw.githubusercontent.com/vavallee/bindery/main/.github/assets/logo.png" alt="Bindery" width="120" />
</p>

<h1 align="center">Bindery</h1>

<p align="center">
  <strong>Automated book download manager for Usenet & Torrents</strong><br>
  Monitor authors. Search indexers. Download. Organize. Done.
</p>

<p align="center">
  <a href="https://github.com/vavallee/bindery/actions/workflows/ci.yml"><img src="https://github.com/vavallee/bindery/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="https://codecov.io/gh/vavallee/bindery"><img src="https://img.shields.io/codecov/c/github/vavallee/bindery?logo=codecov&logoColor=white" alt="codecov" /></a>
  <a href="https://github.com/vavallee/bindery/releases"><img src="https://img.shields.io/github/v/release/vavallee/bindery" alt="Release" /></a>
  <a href="https://github.com/vavallee/bindery/pkgs/container/bindery"><img src="https://img.shields.io/badge/ghcr.io-vavallee%2Fbindery-blue" alt="Docker" /></a>
  <a href="https://goreportcard.com/report/github.com/vavallee/bindery"><img src="https://goreportcard.com/badge/github.com/vavallee/bindery" alt="Go Report Card" /></a>
  <a href="https://github.com/vavallee/bindery/blob/main/LICENSE"><img src="https://img.shields.io/github/license/vavallee/bindery" alt="License" /></a>
</p>

---

<p align="center">
  <img src="https://raw.githubusercontent.com/vavallee/bindery/main/.github/assets/screenshot.png" alt="Bindery Authors page" width="800" />
</p>

---

## Why Bindery?

**Readarr is dead.** The official project was archived in June 2025 and its metadata backend (`api.bookinfo.club`) is permanently offline. Community forks rely on fragile Goodreads scrapers that break regularly. There was no reliable, open-source tool for automated book management on Usenet.

**Bindery is the clean-room replacement.** Built from scratch in Go with a modern React UI, Bindery uses only stable, documented public APIs for book metadata. No scraping. No dead backends. No fragile dependencies.

## Features

### Library management
- **Author monitoring** — Add authors and Bindery tracks all their works automatically via OpenLibrary's author works endpoint
- **Book tracking** — Per-book monitor toggle, status workflow (wanted → downloading → downloaded → imported)
- **Ebooks and audiobooks** — Mark any book as `ebook` or `audiobook`; the search pipeline picks the right Newznab categories (7020 vs 3030), ranker prefers the matching format, and the importer moves whole audiobook folders (multi-part `.m4b` / `.mp3`) as one unit into a separate audiobook library root.
- **Series support** — Books grouped by series with position tracking and dedicated Series page
- **Edition tracking** — Multiple editions per work, with format, ISBN, publisher, page count
- **Library scan** — Walk `/books/` and reconcile existing files with wanted books in the database

### Search & downloads
- **Newznab + Torznab** — Query multiple Usenet and torrent indexers in parallel, deduplicated and ranked
- **SABnzbd + qBittorrent** — Full support for both Usenet and torrent download clients
- **Auto-grab** — Scheduler searches for wanted books every 12h and automatically grabs the best result
- **Interactive search** — Manual per-book search from the Wanted page with full result details
- **Smart matching** — Four-tier query fallback (`t=book` → `surname+title` → `author+title` → title); word-boundary keyword matching; contiguous-phrase requirement for multi-word titles; dual-author-anchor for ambiguous short titles; subtitle-aware (`Title: Subtitle`)
- **Composite ranking** — Results scored by format quality, edition tags (RETAIL / UNABRIDGED / ABRIDGED), year match to the book's release year, grab count, size, and ISBN exact-match bonus
- **Quality profiles** — Preference order for EPUB / MOBI / AZW3 / PDF, with cutoff rules
- **Language filter** — Preferred language setting (English by default); filters releases with foreign-language tags at word boundaries
- **Custom formats** — Regex-based release scoring for freeleech, retail tags, etc.
- **Delay profiles** — Wait N hours before grabbing to let higher-quality releases appear
- **Blocklist** — Consulted on every search and auto-grab; prevents re-grabbing releases you've rejected. Add entries directly from History with one click
- **Failure visibility** — Download errors surfaced in Queue (active) and History (permanent)

### Import & organize
- **Automatic import** — Completed downloads matched by NZO ID, moved to library with configurable naming template
- **Naming tokens** — `{Author}`, `{SortAuthor}`, `{Title}`, `{Year}`, `{ext}` with sanitized path components
- **Cross-filesystem moves** — Atomic rename when possible, copy+verify+delete for NFS/separate volumes
- **History** — Every grab, import, and failure recorded with full detail (shown inline on History page)

### Metadata
- **OpenLibrary** (primary) — Authors, books, editions, covers, ISBN lookup
- **Google Books** (enricher) — Richer descriptions and ratings
- **Hardcover.app** (enricher) — Community ratings and series data via GraphQL
- **Audnex** — Audiobook narrator, duration, cover, and description by Audible ASIN via the free [api.audnex.us](https://api.audnex.us) wrapper. Trigger with `POST /api/v1/book/{id}/enrich-audiobook`.
- No Goodreads scraping. All sources use documented, stable public APIs.

### Migration
- **CSV import** — Upload a newline-separated list of author names (or a `name,monitored,searchOnAdd` CSV); each name is resolved against OpenLibrary.
- **Readarr import** — Upload `readarr.db` directly. Authors are re-resolved via OpenLibrary (Goodreads IDs aren't portable since `bookinfo.club` is dead); Indexers, download clients, and blocklist entries port structurally. Run a library scan afterward to match existing files.
- **CLI** — `bindery migrate csv <path>` and `bindery migrate readarr <path>` for first-time bulk imports without opening the UI.
- **UI** — Settings → Import tab with file upload + per-section result summary.

### Operations
- **Webhook notifications** — Configurable HTTP callbacks for grab / import / failure events (pipe to Apprise, ntfy, Home Assistant, etc.)
- **Metadata profiles** — Filter books by language, popularity, page count, ISBN presence
- **Import lists** — Auto-add authors/books from external sources; exclusion list to skip unwanted entries
- **Tag system** — Scope indexers/profiles/notifications to specific authors
- **Backup/restore** — Snapshot the SQLite database on demand
- **Authentication** — First-run setup creates an admin account (argon2id password hashing, signed session cookies). Three modes: **Enabled** (always require login), **Local only** (bypass auth for private IPs — home network convenience), **Disabled** (no auth, for trusted reverse-proxy deployments). Per-account API key for external integrations. Per-IP rate limiting on the login endpoint.

### UI
- **Light and dark themes** — iOS-style slider toggle in Settings → General → Appearance. First-load default respects the browser's `prefers-color-scheme`; preference persists to localStorage.
- **Modern React SPA** — React 19 + TypeScript + Tailwind CSS 3, built with Vite.
- **Detail pages** — Routed `/book/:id` and `/author/:id` pages replace the previous modal flow. Deep-linkable, back-button friendly, hold per-book history inline.
- **Grid / Table view toggle** — Switch between poster-grid and dense-table views on the Books and Authors pages; choice persists per page.
- **Mobile-friendly** — Responsive layout with hamburger nav, card views for History/Blocklist, agenda view for Calendar. Table views hide less-critical columns on narrow viewports.
- **Pagination everywhere** — First/Prev/Next/Last + page numbers + configurable page size on all list pages
- **Search, filter, sort** — On Authors, Books, Wanted, and History pages; Books filter chips include `Type: Ebook / Audiobook`.
- **Calendar view** — Upcoming book releases from monitored authors, with compact dot-indicator grid on mobile
- **Full REST API** — Every feature accessible via HTTP for scripting and integration

### Packaging
- **Single binary** — Frontend embedded via `go:embed`. No nginx, no sidecars, no complexity
- **Distroless Docker image** — Minimal attack surface, published to GHCR
- **Kubernetes-ready** — Helm chart included for ArgoCD / Flux deployments
- **SQLite + WAL** — Pure Go driver (`modernc.org/sqlite`), no CGO, no external database to manage

## Quick Start

### Docker

```bash
docker run -d \
  --name bindery \
  -p 8787:8787 \
  -v /path/to/config:/config \
  -v /path/to/books:/books \
  -v /path/to/downloads:/downloads \
  ghcr.io/vavallee/bindery:latest
```

Open <http://localhost:8787>, follow the first-run setup to create the admin account, and you're in.

### Binary (Linux / macOS / Windows)

Download the archive for your platform from the [latest release](https://github.com/vavallee/bindery/releases/latest), extract, and run:

```bash
# Linux / macOS
tar -xzf bindery_<version>_<os>_<arch>.tar.gz
./bindery
```

```cmd
:: Windows
:: Unzip bindery_<version>_windows_amd64.zip, then:
bindery.exe
```

On first run the database lands in the platform's standard location:

| Platform | Default `BINDERY_DB_PATH` | Default `BINDERY_DATA_DIR` |
|----------|---------------------------|----------------------------|
| Linux / Docker / Helm | `/config/bindery.db` | `/config` |
| Windows | `%APPDATA%\Bindery\bindery.db` | `%APPDATA%\Bindery` |
| macOS | `~/Library/Application Support/Bindery/bindery.db` | `~/Library/Application Support/Bindery` |

Set `BINDERY_DB_PATH` / `BINDERY_DATA_DIR` if you want them elsewhere. The resolved paths are logged at startup (`"starting bindery"` log line).

For Docker Compose, Kubernetes (Helm), running as a specific UID/GID, and upgrade notes, see **[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md)**.

## Configuration

Bindery is configured through the web UI under **Settings**. Core env vars:

| Variable | Default | Description |
|----------|---------|-------------|
| `BINDERY_PORT` | `8787` | HTTP server port |
| `BINDERY_DB_PATH` | platform-dependent (see [Binary install](#binary-linux--macos--windows)) | SQLite database path |
| `BINDERY_DATA_DIR` | platform-dependent (see [Binary install](#binary-linux--macos--windows)) | Config directory (backups live here) |
| `BINDERY_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error` |
| `BINDERY_DOWNLOAD_DIR` | `/downloads` | Where the download client places completed downloads |
| `BINDERY_LIBRARY_DIR` | `/books` | Destination for imported ebook files |
| `BINDERY_AUDIOBOOK_DIR` | falls back to `BINDERY_LIBRARY_DIR` | Destination for imported audiobook folders |

The full variable reference (path remapping, API key seeding, `BINDERY_PUID` / `BINDERY_PGID` sanity checks) is in **[docs/DEPLOYMENT.md](docs/DEPLOYMENT.md#environment-variables)**.

## Metadata Sources

Bindery aggregates book metadata from multiple open sources:

| Source | Auth Required | Used For |
|--------|---------------|----------|
| [OpenLibrary](https://openlibrary.org) | None | Primary: authors, books, editions, covers, ISBN lookup |
| [Google Books](https://developers.google.com/books) | API key (free) | Enrichment: descriptions, ratings |
| [Hardcover.app](https://hardcover.app) | None (public GraphQL) | Enrichment: community ratings, series |

No Goodreads scraping. All sources use documented, stable public APIs.

## Supported Integrations

### Download clients
- **SABnzbd** — full support (NZB submission, queue/history polling, pause/resume/delete)
- **qBittorrent** — WebUI API v2 with cookie-based auth (add magnet/URL, list/delete torrents)

### Indexers
- **Newznab** (Usenet) — NZBGeek, NZBFinder, NZBPlanet, DrunkenSlug, etc.
- **Torznab** (Torrents) — Prowlarr, Jackett, or direct Torznab endpoints
- **Configurable categories per indexer** — set custom Newznab category IDs in Settings → Indexers (e.g. `7120` for German books, `3130` for German audio on SceneNZBs). Bindery routes 7xxx IDs to ebook searches and 3xxx IDs to audiobook searches automatically.

### Notifications
- **Generic webhooks** — Any HTTP endpoint. Pipe to Apprise, ntfy, Home Assistant, Slack, Discord via proxies.

## Architecture

Bindery is a single Go binary with the React frontend embedded via `go:embed`:

```
   Newznab / Torznab
      indexers
         │
         ▼
┌────────────────────────────┐
│         Bindery            │──► SABnzbd / qBittorrent
│  Go backend + React SPA    │──► /books/ library
│  SQLite (WAL mode)         │──► Webhook notifications
└────────────────────────────┘
    ▲                    ▲
    │                    │
OpenLibrary          Google Books, Hardcover.app
 (primary)                (enrichers)
```

- **Backend:** Go 1.25 with [chi](https://github.com/go-chi/chi) router
- **Database:** SQLite via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go, no CGO)
- **Frontend:** React 19 + TypeScript + Tailwind CSS + [Vite](https://vite.dev)
- **Container:** Multi-stage build on [distroless](https://github.com/GoogleContainerTools/distroless) (minimal attack surface)

## API

Bindery exposes a full REST API under `/api/v1`. A few highlights:

```
GET    /api/v1/health                    - server health
GET    /api/v1/author                    - list authors
POST   /api/v1/author                    - add author (triggers async book fetch)
GET    /api/v1/book?status=wanted        - filter books by status
POST   /api/v1/book/{id}/search          - manual indexer search for a book
GET    /api/v1/queue                     - active downloads with live SABnzbd overlay
POST   /api/v1/queue/grab                - submit a search result to download client
GET    /api/v1/history                   - grab/import/failure events
POST   /api/v1/history/{id}/blocklist    - add a history event's release to the blocklist
GET    /api/v1/blocklist                 - blocked releases
POST   /api/v1/notification/{id}/test    - fire a test webhook
POST   /api/v1/backup                    - snapshot the database
```

### Authentication

Every request to `/api/v1/*` (except `/health`, `/auth/status`, `/auth/login`, `/auth/logout`, `/auth/setup`) is authenticated. A request is allowed if **any** of:

- Auth mode is **Disabled**.
- Auth mode is **Local only** and the request originates from a private-range IP (`10/8`, `172.16/12`, `192.168/16`, `127/8`, IPv6 ULA, link-local, loopback).
- A valid `X-Api-Key` header (or `?apikey=` query param) matches the stored key.
- A valid `bindery_session` cookie is present.

Otherwise the server responds with `401`. The API key lives in **Settings → General → Security** — copy it from there for scripts and integrations. Regenerating the key invalidates any existing consumers.

## Documentation

| Topic | Where |
|-------|-------|
| **Deployment** — Docker, Compose, k8s/Helm, binary, UID/GID, upgrades | [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md) |
| **Roadmap** — planned work, scope notes, and explicitly-out-of-scope items (Z-Library, OpenBooks, etc.) | [docs/ROADMAP.md](docs/ROADMAP.md) |
| **Contributing & CI checks** — dev setup, full quality/security matrix, local check suite | [CONTRIBUTING.md](CONTRIBUTING.md) |
| **Changelog** — release notes | [CHANGELOG.md](CHANGELOG.md) |
| **Reverse-proxy & SSO setups** — Traefik / Caddy / Nginx / Authelia / Authentik recipes | [Wiki](https://github.com/vavallee/bindery/wiki/Reverse-proxy-and-SSO) |
| **Troubleshooting** — permission-denied, path-remap, import failures | [Wiki](https://github.com/vavallee/bindery/wiki/Troubleshooting) |
| **Indexer & download-client recipes** — NZBGeek / DrunkenSlug / Prowlarr / Jackett / SAB / qBit tips | [Wiki](https://github.com/vavallee/bindery/wiki/Indexer-and-downloader-recipes) |
| **Migrating from Readarr** — step-by-step with known failure modes | [Wiki](https://github.com/vavallee/bindery/wiki/Migrating-from-Readarr) |

## Contributing

PRs, issues, and feedback welcome. See **[CONTRIBUTING.md](CONTRIBUTING.md)** for the dev setup, the full local check suite, and the PR flow. Tracked feature work lives in **[docs/ROADMAP.md](docs/ROADMAP.md)** — open an issue before starting anything substantial.

## License

MIT. See [LICENSE](LICENSE) for details.

## Acknowledgments

- The [*arr community](https://wiki.servarr.com/) for pioneering the monitor-search-download-import pattern
- [OpenLibrary](https://openlibrary.org) for free, open book metadata
- The Readarr project for the original vision, even though the implementation couldn't be sustained
