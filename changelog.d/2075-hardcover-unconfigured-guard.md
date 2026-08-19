### Fixed
- **Hardcover no longer hammered when no API token is set** (#2075) — a bulk CSV
  author import on a fresh config fired hundreds of Hardcover requests that came
  back `401 Unable to verify token` and then `429 Throttled`, because only one
  of the client's queries checked for a token first. Every Hardcover query now
  short-circuits as "not configured" before any network call, so an install
  without a token makes no Hardcover requests at all. Adding a token still takes
  effect immediately, with no restart.
- **Honest Hardcover line in the startup log** (#2075) — startup always logged
  `hardcover enrichment enabled`, even with no token configured. It now logs
  `hardcover enrichment idle: no api token configured` in that case.
