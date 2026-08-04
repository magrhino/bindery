### Removed
- **Dead `POST /book/{id}/map` endpoint** — an undocumented metadata-map handler with no caller (the Fix Match UI uses `rebind`, and ABS review uses its own resolve endpoint). Removing it drops maintained authenticated surface that duplicated rebind's logic; the shared helpers it used remain in place for the audiobook ASIN-map path.
