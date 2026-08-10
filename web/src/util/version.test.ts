import { describe, expect, it } from 'vitest'
import { isNewerRelease, isReleaseVersion, releaseHref } from './version'

describe('isNewerRelease', () => {
  it('detects newer patch, minor, and major', () => {
    expect(isNewerRelease('1.28.0', '1.28.1')).toBe(true)
    expect(isNewerRelease('1.28.2', '1.30.0')).toBe(true)
    expect(isNewerRelease('1.30.0', '2.0.0')).toBe(true)
  })

  it('is false for equal or older latest', () => {
    expect(isNewerRelease('1.30.0', '1.30.0')).toBe(false)
    expect(isNewerRelease('1.30.0', '1.29.1')).toBe(false)
  })

  it('compares numerically, not lexically', () => {
    expect(isNewerRelease('1.9.0', '1.10.0')).toBe(true)
    expect(isNewerRelease('1.10.0', '1.9.0')).toBe(false)
  })

  it('tolerates a leading v on either side', () => {
    expect(isNewerRelease('v1.28.0', '1.30.0')).toBe(true)
    expect(isNewerRelease('1.28.0', 'v1.30.0')).toBe(true)
  })

  it('never fires for dev/sha builds or unknown latest', () => {
    expect(isNewerRelease('sha-9ecd99e', '1.30.0')).toBe(false)
    expect(isNewerRelease('dev', '1.30.0')).toBe(false)
    expect(isNewerRelease('1.28.0', '')).toBe(false)
    expect(isNewerRelease('1.28.0', 'sha-9ecd99e')).toBe(false)
  })
})

describe('isReleaseVersion / releaseHref', () => {
  it('classifies versions and builds hrefs', () => {
    expect(isReleaseVersion('1.30.0')).toBe(true)
    expect(isReleaseVersion('sha-9ecd99e')).toBe(false)
    expect(releaseHref('1.30.0')).toBe('https://github.com/vavallee/bindery/releases/tag/v1.30.0')
    expect(releaseHref('sha-9ecd99e')).toBe('https://github.com/vavallee/bindery/releases')
  })
})
