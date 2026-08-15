### Fixed

- **rTorrent downloads no longer burn their retry budget while the files are
  absent** (#1884) — every other download client stopped counting a retry
  attempt against a download whose files are not on this host, but rTorrent's
  retry branch was added without that guard, so the one client that missed it
  spent all five attempts on polls that could not have imported anything and
  then blocked the download terminally. It now waits like the others and
  imports the moment the files appear.
