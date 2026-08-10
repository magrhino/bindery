import { useEffect, useState } from 'react'
import { Link } from 'react-router'
import { useTranslation } from 'react-i18next'
import { api, type SetupState } from '../api/client'

const STORAGE_KEY = 'bindery.setupChecklistDismissed'

interface Step {
  key: keyof Omit<SetupState, 'complete'>
  labelKey: string
  fallback: string
  to?: string
  actionKey?: string
  actionFallback?: string
}

// Ordered as the pipeline runs. Account creation is deliberately absent:
// the user cannot be looking at this without having done it, so listing it
// would spend the first line of the checklist on a step that is always
// already ticked.
const STEPS: Step[] = [
  {
    key: 'hasIndexer',
    labelKey: 'setupChecklist.indexer',
    fallback: 'Add an indexer',
    to: '/settings?tab=indexers',
    actionKey: 'gettingStarted.indexers',
    actionFallback: 'Set up Indexers',
  },
  {
    key: 'hasClient',
    labelKey: 'setupChecklist.client',
    fallback: 'Add a download client',
    to: '/settings?tab=clients',
    actionKey: 'gettingStarted.downloadClients',
    actionFallback: 'Set up Download Clients',
  },
  { key: 'hasAuthor', labelKey: 'setupChecklist.author', fallback: 'Add an author' },
  { key: 'hasGrab', labelKey: 'setupChecklist.grab', fallback: 'Grab a book' },
  { key: 'hasImport', labelKey: 'setupChecklist.import', fallback: 'First book imported' },
]

// First-run progress checklist, shown on the Authors page until the
// pipeline has produced an import. This is the "your setup works"
// confirmation the app never had: previously the only signal that setup
// succeeded was a download appearing hours later, so a user who had
// mis-wired something had no way to tell the difference between "working,
// still searching" and "silently broken".
//
// Auto-hides for good once complete; also dismissible early for users who
// deliberately run a partial setup (e.g. catalogue-only, no downloads).
export default function SetupChecklist() {
  const { t } = useTranslation()
  const [state, setState] = useState<SetupState | null>(null)
  const [dismissed, setDismissed] = useState(() => {
    try {
      return localStorage.getItem(STORAGE_KEY) === '1'
    } catch {
      return false
    }
  })

  useEffect(() => {
    let cancelled = false
    api.setupState()
      .then(s => { if (!cancelled) setState(s) })
      .catch(() => { /* leave hidden — never block the page on this */ })
    return () => { cancelled = true }
  }, [])

  if (!state || state.complete || dismissed) return null

  const done = STEPS.filter(s => state[s.key]).length

  return (
    <div className="max-w-xl mx-auto mb-8 px-5 py-4 bg-slate-100 dark:bg-zinc-900 border border-slate-200 dark:border-zinc-800 rounded-lg text-left">
      <div className="flex items-center justify-between gap-3 mb-3">
        <h3 className="text-sm font-semibold text-slate-900 dark:text-white">
          {t('setupChecklist.title', 'Setup progress')}{' '}
          <span className="font-normal text-fg-muted">{done}/{STEPS.length}</span>
        </h3>
        <button
          onClick={() => {
            try {
              localStorage.setItem(STORAGE_KEY, '1')
            } catch {
              // localStorage unavailable — dismiss for this render only.
            }
            setDismissed(true)
          }}
          className="text-xs text-fg-muted hover:text-slate-900 dark:hover:text-white transition-colors"
        >
          {t('common.dismiss', 'Dismiss')}
        </button>
      </div>
      <ul className="space-y-1.5">
        {STEPS.map(step => {
          const complete = state[step.key]
          return (
            <li key={step.key} className="flex items-center gap-2 text-sm">
              <span
                aria-hidden="true"
                className={`inline-flex items-center justify-center w-4 h-4 rounded-full text-[10px] flex-shrink-0 ${
                  complete
                    ? 'bg-emerald-600 text-white'
                    : 'border border-slate-300 dark:border-zinc-700 text-transparent'
                }`}
              >
                ✓
              </span>
              <span className={complete ? 'text-fg-muted line-through' : 'text-slate-800 dark:text-zinc-200'}>
                {t(step.labelKey, step.fallback)}
              </span>
              {!complete && step.to && (
                <Link to={step.to} className="text-xs text-emerald-700 dark:text-emerald-400 hover:underline">
                  {t(step.actionKey!, step.actionFallback!)}
                </Link>
              )}
            </li>
          )
        })}
      </ul>
    </div>
  )
}
