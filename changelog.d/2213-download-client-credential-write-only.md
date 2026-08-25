### Security
- **Download client passwords and API keys are no longer sent to the browser** (#2213). The download client API used to return the stored qBittorrent, Deluge, Transmission, rTorrent and NZBGet passwords, and the SABnzbd and NZBGet API keys, in full on every list and fetch, so they sat in the settings page in plain text behind a password mask that only hid them visually. Responses now blank them and report `apiKeyConfigured` and `passwordConfigured` instead. The edit form starts blank: leave the credential field alone to keep the saved one, or type a new one to replace it. Changing a client's type still drops the credential it no longer uses.

### Fixed
- **Editing a download client no longer wipes fields the request left out** (#2213). An update now applies on top of the saved row, so a client that omits `enabled`, `useSsl`, `category` or `priority` leaves them as they were. Sending `false` explicitly still turns a setting off.
