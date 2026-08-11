### Fixed
- **Import and upgrade webhooks reach Apprise again**
  ([#1886](https://github.com/vavallee/bindery/issues/1886), thanks @nathang21)
  — `bookImported` and `upgrade` payloads carried the media format under a
  `format` key, but Apprise's REST API reserves `format` for the *body markup*
  and accepts only `text`, `html`, or `markdown`. It rejected `ebook` and
  `audiobook` with HTTP 400 before dispatching anything, so an Apprise relay
  delivered every grab, failure, and health notification — none of which carry
  a `format` — and silently dropped every successful import. The reserved key
  is now omitted for Apprise targets only, identified by a `/notify` path
  segment in the webhook URL. **No other consumer is affected**: ntfy, Home
  Assistant and Discord-proxy relays still receive `format` exactly as before,
  so existing templates keep working. Every payload also carries the same value
  as `mediaFormat`, which is never stripped, so an Apprise template has a key to
  read and anyone else can migrate at their own pace. The report diagnosed this
  as an empty `body`; the body was in fact populated, and the reserved key was
  the real reason for the 400.
