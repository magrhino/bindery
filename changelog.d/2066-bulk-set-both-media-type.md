### Added

- **Bulk media-type editing gained a "Set Both" action, and works from the
  author page** (#2066) — the Books view's bulk bar could set Ebook or
  Audiobook but never the pair, because `POST /book/bulk` rejected
  `mediaType: "both"` even though the author-level bulk action accepted it and
  the underlying write is media-type agnostic. A batch of books owned in both
  formats had to be corrected one at a time from each book's edit page. `both`
  is now accepted, **📖🎧 Set Both** sits alongside the existing two buttons,
  and all three are also available on an author page's book list — where a
  media-type correction after a bad sync usually starts, and where the previous
  workaround (filter the global Books view by author) cost the author-scoped
  context the cleanup needs.
