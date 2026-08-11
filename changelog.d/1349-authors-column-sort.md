### Added
- **Sortable column headers on the Authors page**
  ([#1349](https://github.com/vavallee/bindery/issues/1349)) — Name, Books,
  Rating and Monitored are now clickable, ascending on the first click and
  descending on a second, matching the Books page. This completes #1349, whose
  Books half shipped in v1.28.0 while the Authors half never did. Sort keys are
  whitelisted server-side and every new sort carries a name tiebreaker, so ties
  cannot shuffle rows between pages of a paginated list.

### Fixed
- **The Books column on the Authors page shows a real count** — it rendered `—`
  for every author, in every version that has had the column. The list query
  selected no count and nothing on that path ever set the author's statistics,
  so the field was omitted from the API response entirely and the UI fell back
  to its placeholder. The count is now computed per author and scoped to the
  books the requesting user can see, so it cannot report rows that belong to
  another user.
