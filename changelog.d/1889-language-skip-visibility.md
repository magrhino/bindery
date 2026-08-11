### Fixed
- **An author refresh now says which books it skipped, and why**
  ([#1889](https://github.com/vavallee/bindery/issues/1889)) — the catalogue
  sync already counted the works it dropped, but the counts only ever reached a
  Debug log line per book plus one Info summary, so an author whose catalogue
  had been filtered down to a handful looked exactly like an author who only
  wrote a handful. One reporter lost 65 books from a single author to the
  allowed-languages filter and found out only by going looking in the logs,
  which a rootless container does not hand them. The author detail response now
  carries a `lastSync` summary — works returned, books added, and how many each
  filter dropped — and the author page shows a note above the book list naming
  the language set that was applied, whether the profile also rejects works with
  no reported language, and a few of the dropped titles. The run's summary log
  line moves from `INFO` to `WARN` when anything was skipped, so it also shows
  up in Settings → Logs at the default level. Nothing about the filtering
  changed: a metadata profile set to reject unknown languages still rejects
  them, it just no longer does it silently. The summary is kept in memory, so it
  reports syncs this process has run rather than surviving a restart.
