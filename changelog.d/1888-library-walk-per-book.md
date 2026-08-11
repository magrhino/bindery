### Fixed
- **An author sync no longer walks the entire library once per new book**
  ([#1888](https://github.com/vavallee/bindery/issues/1888),
  [#1929](https://github.com/vavallee/bindery/issues/1929)) — before queuing a
  search for each book it creates, the sync checks whether the file already
  exists on disk, and that check did a full recursive walk of every library
  root, per book. A sync that added 65 books walked the whole library 65 times.
  On local disk the OS caches the directory tree and the cost hides; on an NFS
  or SMB mount every walk is real network round trips per directory entry, and
  at a few dozen seconds per walk this alone accounts for the reported
  hour-long refresh. The sync now takes one snapshot of the library per
  refresh: each root is walked once, on first use, and every per-book check is
  answered from memory with the same matching rules as before — same root
  selection per media type, same author-folder pre-filter, same title and
  author comparison, in the same order. The walk also now honours cancellation,
  which it previously ignored, so deleting an author mid-refresh stops the
  filesystem work instead of letting it run to completion. One-off checks
  (adding a single book, series add, recommendations) keep their per-call walk
  and see the library exactly as it is at that moment; only files copied in by
  hand while a refresh is mid-flight are invisible to that refresh's snapshot,
  and the next refresh sees them.
