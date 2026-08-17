### Fixed

- **Deleting one format no longer destroys the other format's file when both
  live in the same folder** (#2052) — audiobook imports register the destination
  *folder* in `book_files`, so a `?format=audiobook` delete lands on a directory.
  That branch was an unconditional `os.RemoveAll`, which discarded the format
  filter every other part of the delete path honours, and took the ebook sitting
  beside the audio files with it. The ebook's `book_files` row survived, so the
  book kept advertising a file that was no longer on disk and downloading it
  returned an error. The directory branch now walks the folder and removes only
  the files belonging to the format being deleted; cover art and other sidecars
  are removed once no book file of any format is left, and the folder itself
  only when nothing remains in it.
- **A folder delete can no longer unlink a file another book still tracks**
  (#1368, surfaced by #2052) — the `book_files` ownership guard was applied only
  to the tracked path handed to the delete, so any file nested under a deleted
  folder was unguarded. The same check now runs for every file the sweep
  reaches, including same-stem siblings.
