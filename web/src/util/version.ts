// Version helpers shared by the header badge (App.tsx) and Settings → About.
//
// A tagged build reports a semver-ish version (e.g. "1.22.1"); an untagged
// build reports a sha ("sha-9ecd99e"). Only tagged builds participate in
// update comparison — a dev build is never "outdated".

export function isReleaseVersion(version: string): boolean {
  return /^\d+\.\d+/.test(version)
}

export function releaseHref(version: string): string {
  return isReleaseVersion(version)
    ? `https://github.com/vavallee/bindery/releases/tag/v${version}`
    : 'https://github.com/vavallee/bindery/releases'
}

/** True when `latest` is a strictly newer release than `current`. False
 *  whenever either side is missing or not a plain x.y.z release string, so
 *  dev/sha builds and unknown-latest states never produce a badge. */
export function isNewerRelease(current: string, latest: string): boolean {
  const parse = (s: string): number[] | null => {
    const m = /^v?(\d+)\.(\d+)\.(\d+)$/.exec(s)
    return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : null
  }
  const c = parse(current)
  const l = parse(latest)
  if (!c || !l) return false
  for (let i = 0; i < 3; i++) {
    if (l[i] !== c[i]) return l[i] > c[i]
  }
  return false
}
