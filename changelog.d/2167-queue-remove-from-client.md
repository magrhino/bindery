### Added
- **Clear a queue item without touching the download client** (#2167) — `DELETE /api/v1/queue/{id}?removeFromClient=false`, and `"removeFromClient": false` on `POST /api/v1/queue/bulk-delete`. Removing a queue item has always told the client to drop the job, which for a torrent ends the seed, so a stale row left behind by an out-of-band import could not be cleared without losing the release. The default is unchanged, and `deleteFiles=true` alongside `removeFromClient=false` is rejected with a 400 rather than silently ignoring one of them.

### Fixed
- **Corrected the queue-removal doc comment** (#2167) — it claimed removing an item "preserves the seed" for torrent clients. The data survives on disk, but the torrent itself was deleted from the client, so nothing was seeding it.
