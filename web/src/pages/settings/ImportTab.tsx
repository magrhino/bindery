import { useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { api, HardcoverList, ImportList } from '../../api/client'
import { inputCls } from './formStyles'
import GoodreadsImportSection from './GoodreadsImportSection'

interface MigrateResult {
  requested?: number
  added?: number
  skipped?: number
  errors?: number
  addedNames?: string[]
  failures?: Record<string, string>
}

interface ReadarrResult {
  authors?: MigrateResult
  indexers?: MigrateResult
  downloadClients?: MigrateResult
  blocklist?: MigrateResult
}

export default function ImportTab() {
  const { t } = useTranslation()
  const [csvResult, setCsvResult] = useState<MigrateResult | null>(null)
  const [readarrResult, setReadarrResult] = useState<ReadarrResult | null>(null)
  const [uploading, setUploading] = useState<'csv' | 'readarr' | null>(null)
  const [err, setErr] = useState<string | null>(null)

  const upload = async (endpoint: 'csv' | 'readarr', file: File) => {
    setUploading(endpoint)
    setErr(null)
    setCsvResult(null)
    setReadarrResult(null)
    try {
      const fd = new FormData()
      fd.append('file', file)
      const data = await api.uploadMigrate<MigrateResult | ReadarrResult>(endpoint, fd)
      if (endpoint === 'csv') setCsvResult(data as MigrateResult)
      else setReadarrResult(data as ReadarrResult)
    } catch (e) {
      setErr(e instanceof Error ? e.message : 'Upload failed')
    } finally {
      setUploading(null)
    }
  }

  const renderResult = (r: MigrateResult | undefined, label: string) => {
    if (!r) return null
    return (
      <div className="p-3 border border-slate-200 dark:border-zinc-800 rounded bg-slate-100 dark:bg-zinc-900 space-y-1">
        <div className="text-sm font-medium">{label}</div>
        <div className="text-xs text-slate-600 dark:text-zinc-500">
          {r.requested ?? 0} requested · {r.added ?? 0} added · {r.skipped ?? 0} skipped (already exist) · {r.errors ?? 0} failed
        </div>
        {r.failures && Object.keys(r.failures).length > 0 && (
          <details className="text-xs">
            <summary className="cursor-pointer text-red-600 dark:text-red-400">Show {Object.keys(r.failures).length} failures</summary>
            <ul className="mt-2 space-y-0.5 font-mono">
              {Object.entries(r.failures).map(([name, reason]) => (
                <li key={name}><span className="text-slate-800 dark:text-zinc-200">{name}</span>: <span className="text-slate-500 dark:text-zinc-500">{reason}</span></li>
              ))}
            </ul>
          </details>
        )}
      </div>
    )
  }

  return (
    <div className="space-y-8 max-w-2xl">
      <section>
        <h3 className="text-base font-semibold mb-2 text-slate-800 dark:text-zinc-200">{t('settings.import.csvHeading')}</h3>
        <p className="text-xs text-slate-600 dark:text-zinc-500 mb-3">
          {t('settings.import.csvDescription')}
        </p>
        <label className="inline-flex items-center gap-2 px-3 py-2 bg-emerald-600 hover:bg-emerald-500 rounded text-sm font-medium cursor-pointer">
          {uploading === 'csv' ? t('settings.import.importingCsv') : t('settings.import.uploadCsv')}
          <input
            type="file"
            accept=".csv,.txt,text/csv,text/plain"
            className="hidden"
            disabled={uploading !== null}
            onChange={e => { const f = e.target.files?.[0]; if (f) upload('csv', f); e.currentTarget.value = '' }}
          />
        </label>
        {csvResult && <div className="mt-4">{renderResult(csvResult, 'Authors')}</div>}
      </section>

      <section>
        <h3 className="text-base font-semibold mb-2 text-slate-800 dark:text-zinc-200">{t('settings.import.readarrHeading')}</h3>
        <p className="text-xs text-slate-600 dark:text-zinc-500 mb-3">
          {t('settings.import.readarrDescription')}
        </p>
        <label className="inline-flex items-center gap-2 px-3 py-2 bg-emerald-600 hover:bg-emerald-500 rounded text-sm font-medium cursor-pointer">
          {uploading === 'readarr' ? t('settings.import.importingReadarr') : t('settings.import.uploadReadarr')}
          <input
            type="file"
            accept=".db,.sqlite,application/x-sqlite3,application/octet-stream"
            className="hidden"
            disabled={uploading !== null}
            onChange={e => { const f = e.target.files?.[0]; if (f) upload('readarr', f); e.currentTarget.value = '' }}
          />
        </label>
        {readarrResult && (
          <div className="mt-4 space-y-2">
            {renderResult(readarrResult.authors, 'Authors')}
            {renderResult(readarrResult.indexers, 'Indexers')}
            {renderResult(readarrResult.downloadClients, 'Download clients')}
            {renderResult(readarrResult.blocklist, 'Blocklist')}
          </div>
        )}
      </section>
      <GoodreadsImportSection />
      <HardcoverListsSection />

      {err && (
        <div className="px-3 py-2 bg-red-100 dark:bg-red-950/30 border border-red-300 dark:border-red-900 rounded text-sm text-red-800 dark:text-red-300">
          {err}
        </div>
      )}
    </div>
  )
}

function HardcoverListsSection() {
  const { t } = useTranslation()
  const [lists, setLists] = useState<ImportList[]>([])
  const [showAdd, setShowAdd] = useState(false)
  const [syncingId, setSyncingId] = useState<number | null>(null)
  const [syncError, setSyncError] = useState<{ id: number; message: string } | null>(null)

  useEffect(() => {
    api.listImportLists().then(all => setLists(all.filter(l => l.type === 'hardcover'))).catch(console.error)
  }, [])

  const handleDelete = async (id: number) => {
    await api.deleteImportList(id)
    setLists(prev => prev.filter(l => l.id !== id))
  }

  const handleToggle = async (il: ImportList) => {
    const updated = await api.updateImportList(il.id, { ...il, enabled: !il.enabled })
    setLists(prev => prev.map(l => l.id === il.id ? updated : l))
  }

  const handleSync = async (id: number) => {
    setSyncingId(id)
    setSyncError(null)
    try {
      await api.syncImportList(id)
      const all = await api.listImportLists()
      setLists(all.filter(l => l.type === 'hardcover'))
    } catch (e: unknown) {
      setSyncError({ id, message: e instanceof Error ? e.message : 'Sync failed' })
    } finally {
      setSyncingId(null)
    }
  }

  return (
    <section>
      <div className="flex justify-between items-center mb-2">
        <h3 className="text-base font-semibold text-slate-800 dark:text-zinc-200">{t('settings.import.hardcoverHeading')}</h3>
        <button onClick={() => setShowAdd(true)} className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 rounded text-xs font-medium">
          {t('settings.import.hardcoverAddButton')}
        </button>
      </div>
      <p className="text-xs text-slate-600 dark:text-zinc-500 mb-3">
        {t('settings.import.hardcoverDescription')}
      </p>

      {lists.length === 0 && !showAdd && (
        <p className="text-sm text-slate-500 dark:text-zinc-600">{t('settings.import.hardcoverEmpty')}</p>
      )}

      {lists.map(il => (
        <div key={il.id} className="flex items-center justify-between p-3 mb-2 border border-slate-200 dark:border-zinc-800 rounded-lg bg-slate-100 dark:bg-zinc-900">
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <span className="text-sm font-medium">{il.name}</span>
              <span className="text-[10px] px-1.5 py-0.5 bg-slate-200 dark:bg-zinc-800 text-slate-600 dark:text-zinc-400 rounded">{il.url}</span>
            </div>
            <div className="text-xs text-slate-500 dark:text-zinc-600 mt-0.5">
              {il.lastSyncAt
                ? t('settings.import.hardcoverLastSync', { date: new Date(il.lastSyncAt).toLocaleString() })
                : t('settings.import.hardcoverNeverSynced')}
            </div>
            {syncError?.id === il.id && (
              <div className="text-xs text-red-600 dark:text-red-400 mt-1 break-words">{syncError.message}</div>
            )}
          </div>
          <div className="flex items-center gap-2 ml-3">
            <button
              onClick={() => handleSync(il.id)}
              disabled={syncingId === il.id}
              className="text-xs px-2 py-1 rounded bg-slate-200 dark:bg-zinc-800 text-slate-700 dark:text-zinc-300 hover:bg-slate-300 dark:hover:bg-zinc-700 disabled:opacity-50"
            >
              {syncingId === il.id ? 'Syncing…' : 'Sync now'}
            </button>
            <button
              onClick={() => handleToggle(il)}
              className={`text-xs px-2 py-1 rounded ${il.enabled ? 'bg-emerald-100 dark:bg-emerald-950 text-emerald-700 dark:text-emerald-400' : 'bg-slate-200 dark:bg-zinc-800 text-slate-500 dark:text-zinc-500'}`}
            >
              {il.enabled ? t('common.enable') : t('common.disable')}
            </button>
            <button onClick={() => handleDelete(il.id)} className="text-xs text-red-600 dark:text-red-400 hover:underline">{t('common.delete')}</button>
          </div>
        </div>
      ))}

      {showAdd && (
        <AddHardcoverListForm
          onSaved={il => { setLists(prev => [...prev, il]); setShowAdd(false) }}
          onCancel={() => setShowAdd(false)}
        />
      )}
    </section>
  )
}

function AddHardcoverListForm({ onSaved, onCancel }: { onSaved: (il: ImportList) => void; onCancel: () => void }) {
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [token, setToken] = useState('')
  const [hcLists, setHcLists] = useState<HardcoverList[]>([])
  const [selectedSlug, setSelectedSlug] = useState('')
  const [loadingLists, setLoadingLists] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  const fetchLists = (tok: string) => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    if (!tok.trim()) { setHcLists([]); return }
    debounceRef.current = setTimeout(async () => {
      setLoadingLists(true)
      try {
        const lists = await api.hardcoverLists(tok)
        setHcLists(lists)
        if (lists.length > 0 && !selectedSlug) setSelectedSlug(lists[0].slug)
      } catch (e) {
        setHcLists([])
        setError(e instanceof Error ? e.message : 'Failed to load lists')
      } finally {
        setLoadingLists(false)
      }
    }, 500)
  }

  const handleTokenChange = (tok: string) => {
    setToken(tok)
    setError(null)
    fetchLists(tok)
  }

  const handleSave = async () => {
    if (!name.trim() || !token.trim() || !selectedSlug) return
    setSaving(true)
    setError(null)
    try {
      const il = await api.addImportList({
        name: name.trim(),
        type: 'hardcover',
        apiKey: token.trim(),
        url: selectedSlug,
        enabled: true,
        monitorNew: true,
        autoAdd: true,
      })
      onSaved(il)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="p-4 border border-slate-200 dark:border-zinc-800 rounded-lg bg-slate-100 dark:bg-zinc-900 space-y-3">
      <div>
        <label className="block text-xs text-slate-600 dark:text-zinc-400 mb-1">{t('settings.import.hardcoverName')}</label>
        <input className={inputCls} placeholder={t('settings.import.hardcoverNamePlaceholder')} value={name} onChange={e => setName(e.target.value)} />
      </div>
      <div>
        <label className="block text-xs text-slate-600 dark:text-zinc-400 mb-1">{t('settings.import.hardcoverToken')}</label>
        <input className={inputCls} type="password" placeholder={t('settings.import.hardcoverTokenPlaceholder')} value={token} onChange={e => handleTokenChange(e.target.value)} />
      </div>
      <div>
        <label className="block text-xs text-slate-600 dark:text-zinc-400 mb-1">{t('settings.import.hardcoverList')}</label>
        {loadingLists ? (
          <p className="text-xs text-slate-500">{t('settings.import.hardcoverListLoading')}</p>
        ) : hcLists.length > 0 ? (
          <select className={inputCls} value={selectedSlug} onChange={e => setSelectedSlug(e.target.value)}>
            {hcLists.map(l => (
              <option key={l.slug} value={l.slug}>{l.name} ({l.booksCount} books)</option>
            ))}
          </select>
        ) : (
          <p className="text-xs text-slate-500">{token ? t('settings.import.hardcoverNoLists') : t('settings.import.hardcoverListPlaceholder')}</p>
        )}
      </div>
      {error && <p className="text-xs text-red-600 dark:text-red-400">{error}</p>}
      <div className="flex gap-2">
        <button
          onClick={handleSave}
          disabled={saving || !name.trim() || !token.trim() || !selectedSlug}
          className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 disabled:opacity-50 rounded text-xs font-medium"
        >
          {saving ? t('common.saving') : t('common.save')}
        </button>
        <button onClick={onCancel} className="px-3 py-1.5 bg-slate-300 dark:bg-zinc-700 hover:bg-slate-400 dark:hover:bg-zinc-600 rounded text-xs font-medium">
          {t('common.cancel')}
        </button>
      </div>
    </div>
  )
}
