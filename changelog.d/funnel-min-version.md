### Fixed
- **Setup-funnel cohort gate corrected to v1.30.1** — it was pinned to 1.31.0 on the assumption the funnel fields would ship in a minor release. With the fields landing in 1.30.1, the old gate would have excluded every install that actually reports them, leaving the /stats setup-funnel section permanently empty.
