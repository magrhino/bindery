### Fixed
- **The frontend package version no longer poses as a release version**
  ([#1897](https://github.com/vavallee/bindery/issues/1897)) — `web/package.json`
  had been frozen at `1.22.3` since that release while the app shipped 1.30.x,
  because nothing in the release pipeline bumps it. This is metadata only, with
  no user-visible effect: the version shown in the UI comes from the API
  (`/system/status`), which reports the Go build's stamped version, so the app
  never displayed the stale number. The risk was in tooling that reads the file
  — npm banners, the vitest header, an SBOM generated from the frontend tree —
  where `1.22.3` looked like a real, current answer. Rather than adding release
  machinery to bump a number nothing consumes, the field is now pinned to the
  `0.0.0` sentinel, which reads unmistakably as "not a version" and cannot drift
  again. A test keeps it pinned.
