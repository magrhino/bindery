### Fixed
- **A junk indexer response no longer ends a book search early**
  ([#1891](https://github.com/vavallee/bindery/issues/1891)) — an indexer search
  runs a cascade of increasingly specific queries and stops at the first one
  that works, but only the structured `t=book` query checked that what came
  back was actually about the book. The freeform tiers stopped on any response
  at all, so an indexer answering "author surname + title" with unrelated
  releases ended the cascade there, the relevance filter then discarded every
  one of them, and the search finished with nothing — never having tried the
  queries that would have found the book. Broad parent categories (#1571) make
  that response much more likely, so an indexer opted in to them could return
  fewer results than a narrow category list. Every tier now has to return
  something plausibly on-target before it stops the cascade, and if none of
  them do, the earliest tier's results are still what comes back.
