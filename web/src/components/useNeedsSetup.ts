import { useEffect, useState } from 'react'
import { api } from '../api/client'

export interface NeedsSetup {
  /** No indexers configured — searches will find nothing. */
  needsIndexer: boolean
  /** No download clients configured — grabs will fail. */
  needsClient: boolean
  /** Either piece missing. */
  needsAny: boolean
}

const NONE: NeedsSetup = { needsIndexer: false, needsClient: false, needsAny: false }

// Reports which half of the search→download pipeline is unconfigured, so
// guidance can name the missing step. Per-item on purpose: the previous
// all-or-nothing boolean (indexers AND clients both empty) meant a user who
// configured only one of the two — the most common half-finished state, and
// one where grabs fail silently — got no guidance at all. Defaults to "all
// configured" (and stays there on any error) so the guidance never blocks
// or replaces the normal empty state if the checks fail.
export function useNeedsSetup(): NeedsSetup {
  const [needs, setNeeds] = useState<NeedsSetup>(NONE)

  useEffect(() => {
    let cancelled = false
    Promise.all([api.listIndexers(), api.listDownloadClients()])
      .then(([indexers, clients]) => {
        if (cancelled) return
        const needsIndexer = indexers.length === 0
        const needsClient = clients.length === 0
        setNeeds({ needsIndexer, needsClient, needsAny: needsIndexer || needsClient })
      })
      .catch(() => {
        // Fall back to the normal empty state on failure.
        if (!cancelled) setNeeds(NONE)
      })
    return () => { cancelled = true }
  }, [])

  return needs
}
