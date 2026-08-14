**OPDS reading apps now fetch covers from Bindery, not from the metadata
provider.** The catalogue feed emitted the provider's own image URL, so every
KOReader or Moon+ Reader client hot-linked Hardcover's CDN directly, once per
client, with none of the local caching the web UI has had since v1.5. Covers
now come from `/opds/images`, the same handler and the same
`<dataDir>/image-cache/` the browser uses, which is also what Hardcover's API
rules require of any deployment that isn't a personal one. See
`docs/third-party-data.md`.
