### Fixed
- **Adding a book by ISBN no longer loses the author** (#2187) — an OpenLibrary edition that isn't linked to a work now carries its author through the lookup instead of coming back with just a title and cover. The Add button also no longer refuses a result that has no author name: the backend resolves the author from the book itself, so the request goes through.
