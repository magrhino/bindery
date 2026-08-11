### Added
- **Optional broad indexer categories** (#1571) — an indexer can now opt in to
  searching the Newznab Books (`7000`) or Audio (`3000`) parent category
  alongside its configured subcategories, which recovers releases from trackers
  that file things loosely instead of under a specific child. The parent is only
  ever added for a media type the indexer already carries subcategories under,
  so a books-only indexer never gets an audio query, and indexers with
  non-standard taxonomies (MyAnonaMouse-style `100xxx` IDs) are left alone. Off
  by default — broad categories also return comics, magazines and music. Set it
  when adding or editing an indexer; Prowlarr syncs preserve the choice.
