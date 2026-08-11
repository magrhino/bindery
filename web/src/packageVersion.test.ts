import { describe, expect, it } from 'vitest'
import pkg from '../package.json'

// `web/package.json` is a private, never-published package, and the version the
// UI shows comes from the API (`/system/status` → `main.version`, which the Go
// build stamps from `git describe`), not from this file. A real-looking number
// here is therefore a second, unmaintained copy of the release version: it sat
// at 1.22.3 while the app shipped 1.30.x, because nothing in the release
// pipeline bumps it (#1897).
//
// So it is pinned to the 0.0.0 sentinel — obviously a placeholder, so an SBOM
// or an npm banner generated from this tree cannot quietly report a stale
// version as if it were real. package.json has no comment syntax to say that
// in place, which is what this test is for: if someone "helpfully" sets it to
// the current release, this fails and points them here.
describe('web package version', () => {
  it('stays pinned to the 0.0.0 sentinel', () => {
    expect(pkg.version).toBe('0.0.0')
  })

  it('is a private package, so npm never publishes the sentinel', () => {
    expect(pkg.private).toBe(true)
  })
})
