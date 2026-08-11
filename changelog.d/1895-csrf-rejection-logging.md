### Fixed
- **A 403 from the CSRF guards now says why in the log**
  ([#1895](https://github.com/vavallee/bindery/issues/1895)) — `RequireXRequestedWith`
  and `RequireCSRFToken` rejected a mutating request with a bare
  `{"error":"forbidden"}` and wrote nothing at any level, not even debug. That
  is what made [#1849](https://github.com/vavallee/bindery/issues/1849) so hard
  to report: a client with a valid API key got a 403 and the container log had
  nothing tied to the request, so the cause had to be read out of the source
  rather than out of a log. Both guards now emit a debug line naming the guard
  that fired, the reason, the method, the path and the peer address, plus
  whether an `X-Api-Key` header, an `?apikey=` parameter (ignored on mutating
  methods, and a common cause of exactly this 403), a session cookie, an
  `X-CSRF-Token` and an `X-Requested-With` header were present. Only presence is
  logged, never a value — no API key, session cookie, CSRF token or
  `Authorization` header content reaches the log. Debug rather than warn because
  this guard also fires on genuine cross-site forgery attempts, and an
  unauthenticated caller controls how often it fires; set the log level to debug
  in Settings → Logs when chasing an unexplained 403.
