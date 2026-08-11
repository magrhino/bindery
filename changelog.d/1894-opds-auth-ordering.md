### Fixed
- **The OPDS guard now checks the API key before the local-only bypass**
  ([#1894](https://github.com/vavallee/bindery/issues/1894)) — no user-visible
  change today; this closes off a repeat of
  [#1849](https://github.com/vavallee/bindery/issues/1849) before it can happen.
  In `OPDSAuth` the `auth.mode=local-only` bypass ran ahead of the `X-Api-Key`
  check, so a request arriving from an address local-only trusts was let through
  without its key ever being verified. That is byte-for-byte the ordering that
  caused #1849 in `auth.Middleware`, where the swallowed key meant a valid
  API-key mutation was rejected `403` by the CSRF header guard. It stays
  harmless in OPDS only because every `/opds` route is a read, so nothing
  downstream cares how the request was authenticated. The moment a mutating
  OPDS route exists, the same client with the same documented credential would
  have started getting the same unexplained `403`. The key check now runs first,
  matching `auth.Middleware`; a missing or wrong key still falls through to the
  bypass exactly as before, so local-only OPDS readers are unaffected.
