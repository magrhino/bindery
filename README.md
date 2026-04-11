<p align="center">
  <img src="https://raw.githubusercontent.com/vavallee/bindery/main/.github/assets/logo.png" alt="Bindery" width="120" />
</p>

<h1 align="center">Bindery</h1>

<p align="center">
  <strong>Automated book download manager for Usenet</strong><br>
  Monitor authors. Search indexers. Download. Organize. Done.
</p>

<p align="center">
  <a href="https://github.com/vavallee/bindery/actions/workflows/ci.yml"><img src="https://github.com/vavallee/bindery/actions/workflows/ci.yml/badge.svg" alt="CI" /></a>
  <a href="https://codecov.io/gh/vavallee/bindery"><img src="https://codecov.io/gh/vavallee/bindery/branch/main/graph/badge.svg" alt="Coverage" /></a>
  <a href="https://github.com/vavallee/bindery/releases"><img src="https://img.shields.io/github/v/release/vavallee/bindery" alt="Release" /></a>
  <a href="https://github.com/vavallee/bindery/pkgs/container/bindery"><img src="https://img.shields.io/badge/ghcr.io-vavallee%2Fbindery-blue" alt="Docker" /></a>
  <a href="https://goreportcard.com/report/github.com/vavallee/bindery"><img src="https://goreportcard.com/badge/github.com/vavallee/bindery" alt="Go Report Card" /></a>
  <a href="https://github.com/vavallee/bindery/blob/main/LICENSE"><img src="https://img.shields.io/github/license/vavallee/bindery" alt="License" /></a>
</p>

---

## Why Bindery?

**Readarr is dead.** The official project was archived in June 2025 and its metadata backend (`api.bookinfo.club`) is permanently offline. Community forks rely on fragile Goodreads scrapers that break regularly. There was no reliable, open-source tool for automated book management on Usenet.

**Bindery is the clean-room replacement.** Built from scratch in Go with a modern React UI, Bindery uses only stable, documented public APIs for book metadata. No scraping. No dead backends. No fragile dependencies.

## Features

- **Monitor authors** — Add your favorite authors and Bindery automatically tracks all their books
- **Search Usenet indexers** — Query multiple Newznab-compatible indexers (NZBGeek, NZBFinder, NZBPlanet, and more) simultaneously
- **Automated downloads** — Send NZBs to SABnzbd with one click, or let Bindery grab them automatically
- **Smart import** — Completed downloads are matched, renamed, and organized into your library
- **Multiple metadata sources** — OpenLibrary (primary), Google Books, Hardcover.app, and BookBrainz provide rich, reliable book data
- **Modern web UI** — Clean, responsive interface built with React and shadcn/ui
- **Single binary** — One executable with the frontend embedded. No nginx, no sidecars, no complexity
- **Multi-arch Docker images** — `linux/amd64` and `linux/arm64` images published to GHCR on every release
- **Kubernetes-ready** — Helm chart included for easy deployment with ArgoCD or Flux
- **Quality profiles** — Define format preferences (ebook, audiobook, PDF) and let Bindery find the best match
- **Series tracking** — Automatically detects and displays series information with reading order
- **Calendar view** — See upcoming book releases from your monitored authors
- **REST API** — Full API for integration with other tools and automation

## Quick Start

### Docker (recommended)

```bash
docker run -d \
  --name bindery \
  -p 8787:8787 \
  -v /path/to/config:/config \
  -v /path/to/books:/books \
  -v /path/to/downloads:/downloads \
  ghcr.io/vavallee/bindery:latest
```

### Docker Compose

```yaml
services:
  bindery:
    image: ghcr.io/vavallee/bindery:latest
    container_name: bindery
    ports:
      - 8787:8787
    volumes:
      - ./config:/config
      - /media/books:/books
      - /media/downloads:/downloads
    environment:
      - BINDERY_LOG_LEVEL=info
    restart: unless-stopped
```

### Kubernetes (Helm)

```bash
helm install bindery charts/bindery \
  --set image.tag=latest \
  --set persistence.config.storageClass=longhorn \
  --set ingress.host=bindery.example.com
```

See [`charts/bindery/values.yaml`](charts/bindery/values.yaml) for all configuration options.

### Binary

Download the latest binary from [Releases](https://github.com/vavallee/bindery/releases) and run:

```bash
./bindery --config /path/to/config
```

## Configuration

Bindery is configured through the web UI at `http://localhost:8787` after first launch. Key settings:

| Setting | Description |
|---------|-------------|
| **Indexers** | Add your Newznab indexer URLs and API keys |
| **Download Client** | Configure SABnzbd connection (host, port, API key, category) |
| **Root Folders** | Set where your book library lives |
| **Quality Profiles** | Define format preferences (EPUB > MOBI > PDF, etc.) |
| **Metadata** | Optionally add a Google Books API key or Hardcover token for richer data |

Environment variables are available for container deployments:

| Variable | Default | Description |
|----------|---------|-------------|
| `BINDERY_PORT` | `8787` | HTTP server port |
| `BINDERY_DB_PATH` | `/config/bindery.db` | SQLite database path |
| `BINDERY_LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |
| `BINDERY_API_KEY` | _(empty)_ | API key for external access (optional) |

## Metadata Sources

Bindery aggregates book metadata from multiple open sources:

| Source | Auth Required | Used For |
|--------|---------------|----------|
| [OpenLibrary](https://openlibrary.org) | None | Primary: authors, books, editions, covers, ISBN lookup |
| [Google Books](https://developers.google.com/books) | API key (free) | Enrichment: descriptions, ratings |
| [Hardcover.app](https://hardcover.app) | User token | Optional: community ratings, series data |
| [BookBrainz](https://bookbrainz.org) | None | Fallback: edition and publisher data |

No Goodreads scraping. All sources use documented, stable public APIs.

## Supported Integrations

### Download Clients
- **SABnzbd** (full support)
- More clients planned (NZBGet, etc.)

### Indexers
- Any **Newznab-compatible** indexer (NZBGeek, NZBFinder, NZBPlanet, DrunkenSlug, etc.)

## Screenshots

> Screenshots will be added once the UI is complete.

## Architecture

Bindery is a single Go binary with the React frontend embedded via `go:embed`:

```
                                    Newznab Indexers
                                    (NZBGeek, etc.)
                                         |
                                         v
OpenLibrary ──> ┌───────────────────────���─────┐
Google Books ──>│         Bindery             │──> SABnzbd
Hardcover    ──>│  Go backend + React SPA     │
BookBrainz   ──>│  SQLite (WAL mode)          │──> /books/ library
                └─────────────────────────────┘
                         |
                    :8787 (HTTP)
```

- **Backend:** Go 1.26 with [chi](https://github.com/go-chi/chi) router
- **Database:** SQLite via [modernc.org/sqlite](https://pkg.go.dev/modernc.org/sqlite) (pure Go, no CGO)
- **Frontend:** React 19 + TypeScript + [Vite](https://vite.dev) + [shadcn/ui](https://ui.shadcn.com)
- **Container:** Multi-stage build on [distroless](https://github.com/GoogleContainerTools/distroless) (minimal attack surface)

## Development

### Prerequisites

- Go 1.26+
- Node.js 22+
- Make

### Build

```bash
# Build everything
make build

# Run in development mode (hot reload)
make dev

# Run tests
make test

# Run linters
make lint

# Build Docker image
make docker-build
```

### Project Structure

```
bindery/
├── cmd/bindery/          # Application entry point
├── internal/
│   ├── api/              # HTTP handlers (chi router)
│   ├── db/               # SQLite repository layer + migrations
│   ├── models/           # Domain types
│   ├── metadata/         # Book metadata providers
│   ├── indexer/          # Newznab indexer client
│   ├── downloader/       # SABnzbd client
│   ├── importer/         # Download import pipeline
│   ├── scheduler/        # Background job runner
│   └── config/           # Application configuration
├── web/                  # React frontend (Vite)
├── charts/bindery/       # Helm chart
└── .github/workflows/    # CI/CD
```

## Contributing

Contributions are welcome! Please:

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

Please ensure tests pass (`make test`) and linters are clean (`make lint`) before submitting.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.

## Acknowledgments

- The [*arr community](https://wiki.servarr.com/) for pioneering the monitor-search-download-import pattern
- [OpenLibrary](https://openlibrary.org) for providing free, open book metadata
- The Readarr project for the original vision, even though the implementation couldn't be sustained
