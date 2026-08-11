### Fixed
- **Hardcover search results no longer file a book under its narrator**
  ([#1892](https://github.com/vavallee/bindery/issues/1892)) — #1733 added a
  contribution-role filter so an audiobook's narrator stops being treated as its
  author, but it covered only the GraphQL book queries. The Typesense search
  documents carry the same `contribution` field and it was never decoded, so
  every search-sourced credit arrived with an empty role, which the filter reads
  as "this is the author", and the first credit won. Hardcover lists the narrator
  first on plenty of audiobook-bearing works, so anything resolved through search
  rather than through a book query kept the pre-#1733 behaviour. The field name
  was confirmed against the live API rather than guessed — a wrong guess would
  have decoded to empty and silently preserved the bug while looking fixed.
