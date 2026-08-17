### Fixed

- **Bulk "Set monitor mode" can now set whether authors accept newly
  discovered books** (#2065) — the dialog wrote `monitor_mode` and nothing
  else, so setting a whole library to *None* left every author still pulling in
  its back-catalogue on the next refresh. The two settings are independent
  (monitor mode decides what gets monitored, monitor new items decides whether a
  refresh may create the rows at all), and the field was only reachable from the
  single-author edit form. The bulk dialog now carries a **Monitor newly
  discovered books** control alongside monitor mode, defaulting to *Leave
  unchanged* so an existing bulk action behaves as before. Mode *None* still
  does not imply it: that pairing is the supported "list the whole catalogue,
  monitor none of it" setup.
