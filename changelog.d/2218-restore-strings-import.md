### Fixed
- **Build break on main from two credential PRs landing together** — #2211 dropped the `strings` import from the download client handlers when host parsing moved into `clienthost`, while #2216 added a `strings.TrimSpace` call in the same file. Each passed CI against its own base and the merged result did not compile.
