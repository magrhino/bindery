- **Directory-move containment check now folds case**
  ([#1809](https://github.com/vavallee/bindery/issues/1809)) — on a
  case-insensitive filesystem (APFS, a Windows bind mount) a destination
  differing from the source only in case is the same directory, so the move was
  still able to recurse into its own output. Symlink resolution does not
  normalise case, so the comparison now runs folded as well as exact.
