import { useEffect, useState } from 'react'
import { api, Indexer, DownloadClient, NotificationConfig, QualityProfile, MetadataProfile } from '../api/client'

type Tab = 'indexers' | 'clients' | 'notifications' | 'quality' | 'metadata' | 'general'

const inputCls = 'w-full bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm focus:outline-none focus:border-zinc-600'
const tabCls = (active: boolean) =>
  `px-4 py-2 rounded-md text-sm font-medium transition-colors ${active ? 'bg-zinc-800 text-white' : 'text-zinc-400 hover:text-white hover:bg-zinc-800/50'}`

export default function SettingsPage() {
  const [tab, setTab] = useState<Tab>('indexers')
  const [indexers, setIndexers] = useState<Indexer[]>([])
  const [clients, setClients] = useState<DownloadClient[]>([])
  const [notifications, setNotifications] = useState<NotificationConfig[]>([])
  const [qualityProfiles, setQualityProfiles] = useState<QualityProfile[]>([])
  const [metadataProfiles, setMetadataProfiles] = useState<MetadataProfile[]>([])
  const [showAddIndexer, setShowAddIndexer] = useState(false)
  const [showAddClient, setShowAddClient] = useState(false)
  const [showAddNotification, setShowAddNotification] = useState(false)

  useEffect(() => {
    api.listIndexers().then(setIndexers).catch(console.error)
    api.listDownloadClients().then(setClients).catch(console.error)
  }, [])

  useEffect(() => {
    if (tab === 'notifications') api.listNotifications().then(setNotifications).catch(console.error)
    if (tab === 'quality') api.listQualityProfiles().then(setQualityProfiles).catch(console.error)
    if (tab === 'metadata') api.listMetadataProfiles().then(setMetadataProfiles).catch(console.error)
  }, [tab])

  return (
    <div>
      <h2 className="text-2xl font-bold mb-6">Settings</h2>

      <div className="flex flex-wrap gap-2 mb-6">
        {(['indexers', 'clients', 'notifications', 'quality', 'metadata', 'general'] as Tab[]).map(t => (
          <button key={t} onClick={() => setTab(t)} className={tabCls(tab === t)}>
            {t === 'indexers' ? 'Indexers'
              : t === 'clients' ? 'Download Clients'
              : t === 'notifications' ? 'Notifications'
              : t === 'quality' ? 'Quality Profiles'
              : t === 'metadata' ? 'Metadata Profiles'
              : 'General'}
          </button>
        ))}
      </div>

      {/* Indexers */}
      {tab === 'indexers' && (
        <div>
          <div className="flex justify-between items-center mb-4">
            <h3 className="text-lg font-semibold">Indexers</h3>
            <button onClick={() => setShowAddIndexer(true)} className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 rounded text-xs font-medium">
              + Add Indexer
            </button>
          </div>
          {indexers.length === 0 ? (
            <p className="text-zinc-500 text-sm">No indexers configured. Add a Newznab indexer to search for books.</p>
          ) : (
            <div className="space-y-2">
              {indexers.map(idx => (
                <div key={idx.id} className="flex items-center justify-between p-4 border border-zinc-800 rounded-lg bg-zinc-900">
                  <div>
                    <h4 className="font-medium text-sm">{idx.name}</h4>
                    <p className="text-xs text-zinc-500">{idx.url}</p>
                  </div>
                  <div className="flex items-center gap-3">
                    <span className={`text-xs ${idx.enabled ? 'text-emerald-400' : 'text-zinc-500'}`}>
                      {idx.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                    <button
                      onClick={async () => {
                        try {
                          await api.testIndexer(idx.id)
                          alert('Connection successful!')
                        } catch (err: unknown) {
                          alert('Test failed: ' + (err instanceof Error ? err.message : 'Unknown error'))
                        }
                      }}
                      className="text-xs text-zinc-400 hover:text-white"
                    >
                      Test
                    </button>
                    <button
                      onClick={async () => {
                        await api.deleteIndexer(idx.id)
                        setIndexers(indexers.filter(i => i.id !== idx.id))
                      }}
                      className="text-xs text-red-400 hover:text-red-300"
                    >
                      Delete
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
          {showAddIndexer && (
            <AddIndexerForm
              onClose={() => setShowAddIndexer(false)}
              onAdded={(idx) => { setIndexers([...indexers, idx]); setShowAddIndexer(false) }}
            />
          )}
        </div>
      )}

      {/* Download Clients */}
      {tab === 'clients' && (
        <div>
          <div className="flex justify-between items-center mb-4">
            <h3 className="text-lg font-semibold">Download Clients</h3>
            <button onClick={() => setShowAddClient(true)} className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 rounded text-xs font-medium">
              + Add Client
            </button>
          </div>
          {clients.length === 0 ? (
            <p className="text-zinc-500 text-sm">No download clients configured. Add SABnzbd to enable downloads.</p>
          ) : (
            <div className="space-y-2">
              {clients.map(c => (
                <div key={c.id} className="flex items-center justify-between p-4 border border-zinc-800 rounded-lg bg-zinc-900">
                  <div>
                    <h4 className="font-medium text-sm">{c.name}</h4>
                    <p className="text-xs text-zinc-500">{c.host}:{c.port} ({c.category})</p>
                  </div>
                  <div className="flex items-center gap-3">
                    <span className={`text-xs ${c.enabled ? 'text-emerald-400' : 'text-zinc-500'}`}>
                      {c.enabled ? 'Enabled' : 'Disabled'}
                    </span>
                    <button
                      onClick={async () => {
                        try {
                          await api.testDownloadClient(c.id)
                          alert('Connection successful!')
                        } catch (err: unknown) {
                          alert('Test failed: ' + (err instanceof Error ? err.message : 'Unknown error'))
                        }
                      }}
                      className="text-xs text-zinc-400 hover:text-white"
                    >
                      Test
                    </button>
                    <button
                      onClick={async () => {
                        await api.deleteDownloadClient(c.id)
                        setClients(clients.filter(x => x.id !== c.id))
                      }}
                      className="text-xs text-red-400 hover:text-red-300"
                    >
                      Delete
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
          {showAddClient && (
            <AddClientForm
              onClose={() => setShowAddClient(false)}
              onAdded={(c) => { setClients([...clients, c]); setShowAddClient(false) }}
            />
          )}
        </div>
      )}

      {/* Notifications */}
      {tab === 'notifications' && (
        <div>
          <div className="flex justify-between items-center mb-4">
            <h3 className="text-lg font-semibold">Webhook Notifications</h3>
            <button onClick={() => setShowAddNotification(true)} className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 rounded text-xs font-medium">
              + Add Notification
            </button>
          </div>
          {notifications.length === 0 ? (
            <p className="text-zinc-500 text-sm">No notifications configured. Add a webhook to receive event alerts.</p>
          ) : (
            <div className="space-y-2">
              {notifications.map(n => (
                <div key={n.id} className="p-4 border border-zinc-800 rounded-lg bg-zinc-900">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <h4 className="font-medium text-sm">{n.name}</h4>
                      <p className="text-xs text-zinc-500 truncate mt-0.5">{n.url}</p>
                      <div className="flex flex-wrap gap-1 mt-2">
                        {n.onGrab && <span className="text-[10px] px-1.5 py-0.5 bg-zinc-800 text-zinc-400 rounded">On Grab</span>}
                        {n.onImport && <span className="text-[10px] px-1.5 py-0.5 bg-zinc-800 text-zinc-400 rounded">On Import</span>}
                        {n.onUpgrade && <span className="text-[10px] px-1.5 py-0.5 bg-zinc-800 text-zinc-400 rounded">On Upgrade</span>}
                        {n.onFailure && <span className="text-[10px] px-1.5 py-0.5 bg-zinc-800 text-zinc-400 rounded">On Failure</span>}
                        {n.onHealth && <span className="text-[10px] px-1.5 py-0.5 bg-zinc-800 text-zinc-400 rounded">On Health</span>}
                      </div>
                    </div>
                    <div className="flex items-center gap-3 flex-shrink-0">
                      <span className={`text-xs ${n.enabled ? 'text-emerald-400' : 'text-zinc-500'}`}>
                        {n.enabled ? 'Enabled' : 'Disabled'}
                      </span>
                      <button
                        onClick={async () => {
                          try {
                            await api.testNotification(n.id)
                            alert('Test notification sent!')
                          } catch (err: unknown) {
                            alert('Test failed: ' + (err instanceof Error ? err.message : 'Unknown error'))
                          }
                        }}
                        className="text-xs text-zinc-400 hover:text-white"
                      >
                        Test
                      </button>
                      <button
                        onClick={async () => {
                          await api.deleteNotification(n.id)
                          setNotifications(notifications.filter(x => x.id !== n.id))
                        }}
                        className="text-xs text-red-400 hover:text-red-300"
                      >
                        Delete
                      </button>
                    </div>
                  </div>
                </div>
              ))}
            </div>
          )}
          {showAddNotification && (
            <AddNotificationForm
              onClose={() => setShowAddNotification(false)}
              onAdded={(n) => { setNotifications([...notifications, n]); setShowAddNotification(false) }}
            />
          )}
        </div>
      )}

      {/* Quality Profiles */}
      {tab === 'quality' && (
        <div>
          <h3 className="text-lg font-semibold mb-4">Quality Profiles</h3>
          {qualityProfiles.length === 0 ? (
            <p className="text-zinc-500 text-sm">No quality profiles configured.</p>
          ) : (
            <div className="space-y-3">
              {qualityProfiles.map(p => (
                <div key={p.id} className="p-4 border border-zinc-800 rounded-lg bg-zinc-900">
                  <div className="flex items-center justify-between mb-2">
                    <h4 className="font-medium text-sm">{p.name}</h4>
                    <div className="flex items-center gap-3 text-xs text-zinc-500">
                      <span>Cutoff: <span className="text-zinc-300">{p.cutoff}</span></span>
                      {p.upgradeAllowed && <span className="text-emerald-400">Upgrades allowed</span>}
                    </div>
                  </div>
                  {p.items && p.items.length > 0 && (
                    <div className="flex flex-wrap gap-1.5 mt-2">
                      {p.items.map((item, i) => (
                        <span key={i} className={`text-[10px] px-2 py-0.5 rounded ${item.allowed ? 'bg-emerald-500/20 text-emerald-400' : 'bg-zinc-800 text-zinc-600'}`}>
                          {item.quality}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* Metadata Profiles */}
      {tab === 'metadata' && (
        <div>
          <h3 className="text-lg font-semibold mb-4">Metadata Profiles</h3>
          {metadataProfiles.length === 0 ? (
            <p className="text-zinc-500 text-sm">No metadata profiles configured.</p>
          ) : (
            <div className="space-y-3">
              {metadataProfiles.map(p => (
                <div key={p.id} className="p-4 border border-zinc-800 rounded-lg bg-zinc-900">
                  <div className="flex items-start justify-between">
                    <div>
                      <h4 className="font-medium text-sm">{p.name}</h4>
                      <div className="flex flex-wrap gap-3 mt-2 text-xs text-zinc-400">
                        <span>Min popularity: <span className="text-zinc-200">{p.minPopularity}</span></span>
                        <span>Min pages: <span className="text-zinc-200">{p.minPages}</span></span>
                        {p.allowedLanguages && <span>Languages: <span className="text-zinc-200">{p.allowedLanguages}</span></span>}
                      </div>
                      <div className="flex flex-wrap gap-1.5 mt-2">
                        {p.skipMissingDate && <span className="text-[10px] px-2 py-0.5 bg-zinc-800 text-zinc-400 rounded">Skip missing date</span>}
                        {p.skipMissingIsbn && <span className="text-[10px] px-2 py-0.5 bg-zinc-800 text-zinc-400 rounded">Skip missing ISBN</span>}
                        {p.skipPartBooks && <span className="text-[10px] px-2 py-0.5 bg-zinc-800 text-zinc-400 rounded">Skip part books</span>}
                      </div>
                    </div>
                    <button
                      onClick={async () => {
                        if (!confirm('Delete this metadata profile?')) return
                        await api.deleteMetadataProfile(p.id)
                        setMetadataProfiles(metadataProfiles.filter(x => x.id !== p.id))
                      }}
                      className="text-xs text-red-400 hover:text-red-300 flex-shrink-0"
                    >
                      Delete
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}

      {/* General */}
      {tab === 'general' && (
        <GeneralTab />
      )}
    </div>
  )
}

function GeneralTab() {
  const [settings, setSettings] = useState<Record<string, string>>({})
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState<string | null>(null)
  const [backups, setBackups] = useState<string[]>([])
  const [creatingBackup, setCreatingBackup] = useState(false)

  useEffect(() => {
    api.listSettings()
      .then(list => {
        const map: Record<string, string> = {}
        list.forEach(s => { map[s.key] = s.value })
        setSettings(map)
      })
      .catch(console.error)
      .finally(() => setLoading(false))
    api.listBackups().then(setBackups).catch(console.error)
  }, [])

  const saveSetting = async (key: string) => {
    setSaving(key)
    try {
      await api.setSetting(key, settings[key] ?? '')
    } catch (err) {
      console.error(err)
    } finally {
      setSaving(null)
    }
  }

  const handleBackup = async () => {
    setCreatingBackup(true)
    try {
      const result = await api.createBackup()
      setBackups(prev => [result.filename, ...prev])
      alert(`Backup created: ${result.filename}`)
    } catch (err) {
      alert('Backup failed: ' + (err instanceof Error ? err.message : 'Unknown error'))
    } finally {
      setCreatingBackup(false)
    }
  }

  if (loading) return <div className="text-zinc-500">Loading...</div>

  return (
    <div className="space-y-8">
      {/* Downloads */}
      <section>
        <h3 className="text-base font-semibold mb-3 text-zinc-200">Downloads</h3>
        <div className="p-4 border border-zinc-800 rounded-lg bg-zinc-900 space-y-3">
          <div>
            <label className="block text-xs text-zinc-400 mb-1">Preferred Language</label>
            <p className="text-xs text-zinc-500 mb-2">Filter search results to the selected language. Releases with detected foreign-language tags in the title will be excluded.</p>
            <div className="flex gap-2">
              <select
                value={settings['search.preferredLanguage'] ?? 'en'}
                onChange={e => setSettings(s => ({ ...s, 'search.preferredLanguage': e.target.value }))}
                className="flex-1 bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm focus:outline-none focus:border-zinc-600"
              >
                <option value="any">Any (no filter)</option>
                <option value="en">English</option>
              </select>
              <button
                onClick={() => saveSetting('search.preferredLanguage')}
                disabled={saving === 'search.preferredLanguage'}
                className="px-3 py-2 bg-emerald-600 hover:bg-emerald-500 rounded text-xs font-medium disabled:opacity-50"
              >
                {saving === 'search.preferredLanguage' ? 'Saving...' : 'Save'}
              </button>
            </div>
          </div>
        </div>
      </section>

      {/* Naming */}
      <section>
        <h3 className="text-base font-semibold mb-3 text-zinc-200">File Naming</h3>
        <div className="p-4 border border-zinc-800 rounded-lg bg-zinc-900 space-y-3">
          <div>
            <label className="block text-xs text-zinc-400 mb-1">Book Naming Template</label>
            <div className="flex gap-2">
              <input
                value={settings['naming.bookTemplate'] ?? ''}
                onChange={e => setSettings(s => ({ ...s, 'naming.bookTemplate': e.target.value }))}
                placeholder="{Author Name}/{Book Title}"
                className="flex-1 bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm focus:outline-none focus:border-zinc-600"
              />
              <button
                onClick={() => saveSetting('naming.bookTemplate')}
                disabled={saving === 'naming.bookTemplate'}
                className="px-3 py-2 bg-emerald-600 hover:bg-emerald-500 rounded text-xs font-medium disabled:opacity-50"
              >
                {saving === 'naming.bookTemplate' ? 'Saving...' : 'Save'}
              </button>
            </div>
          </div>
        </div>
      </section>

      {/* API Keys */}
      <section>
        <h3 className="text-base font-semibold mb-3 text-zinc-200">API Keys</h3>
        <div className="p-4 border border-zinc-800 rounded-lg bg-zinc-900 space-y-4">
          <div>
            <label className="block text-xs text-zinc-400 mb-1">Bindery API Key</label>
            <div className="flex items-center gap-2">
              <code className="flex-1 bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm font-mono text-zinc-300 truncate">
                {settings['api.key'] || '(not set)'}
              </code>
            </div>
          </div>
          <div>
            <label className="block text-xs text-zinc-400 mb-1">Google Books API Key</label>
            <div className="flex gap-2">
              <input
                value={settings['googlebooks.apiKey'] ?? ''}
                onChange={e => setSettings(s => ({ ...s, 'googlebooks.apiKey': e.target.value }))}
                placeholder="AIza..."
                type="password"
                className="flex-1 bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm focus:outline-none focus:border-zinc-600"
              />
              <button
                onClick={() => saveSetting('googlebooks.apiKey')}
                disabled={saving === 'googlebooks.apiKey'}
                className="px-3 py-2 bg-emerald-600 hover:bg-emerald-500 rounded text-xs font-medium disabled:opacity-50"
              >
                {saving === 'googlebooks.apiKey' ? 'Saving...' : 'Save'}
              </button>
            </div>
          </div>
        </div>
      </section>

      {/* Backup */}
      <section>
        <h3 className="text-base font-semibold mb-3 text-zinc-200">Backup & Restore</h3>
        <div className="p-4 border border-zinc-800 rounded-lg bg-zinc-900 space-y-3">
          <div className="flex items-center justify-between">
            <div>
              <p className="text-sm text-zinc-300">Create a backup of all Bindery configuration</p>
              <p className="text-xs text-zinc-500 mt-0.5">Includes authors, books, indexers, and settings</p>
            </div>
            <button
              onClick={handleBackup}
              disabled={creatingBackup}
              className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 rounded text-sm font-medium disabled:opacity-50 flex-shrink-0"
            >
              {creatingBackup ? 'Creating...' : 'Create Backup'}
            </button>
          </div>
          {backups.length > 0 && (
            <div className="mt-3 border-t border-zinc-800 pt-3">
              <p className="text-xs text-zinc-500 mb-2">Existing backups:</p>
              <ul className="space-y-1">
                {backups.map(b => (
                  <li key={b} className="text-xs text-zinc-400 font-mono">{b}</li>
                ))}
              </ul>
            </div>
          )}
        </div>
      </section>
    </div>
  )
}

function AddIndexerForm({ onClose, onAdded }: { onClose: () => void; onAdded: (idx: Indexer) => void }) {
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [apiKey, setApiKey] = useState('')

  const submit = async () => {
    const idx = await api.addIndexer({ name, url, apiKey, type: 'newznab', categories: [7000, 7020], enabled: true })
    onAdded(idx)
  }

  return (
    <div className="mt-4 p-4 border border-zinc-700 rounded-lg bg-zinc-800/50 space-y-3">
      <input value={name} onChange={e => setName(e.target.value)} placeholder="Name (e.g. NZBGeek)" className={inputCls} />
      <input value={url} onChange={e => setUrl(e.target.value)} placeholder="URL (e.g. https://api.nzbgeek.info)" className={inputCls} />
      <input value={apiKey} onChange={e => setApiKey(e.target.value)} placeholder="API Key" type="password" className={inputCls} />
      <div className="flex gap-2 justify-end">
        <button onClick={onClose} className="px-3 py-1.5 text-sm text-zinc-400">Cancel</button>
        <button onClick={submit} className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 rounded text-sm font-medium">Save</button>
      </div>
    </div>
  )
}

function AddClientForm({ onClose, onAdded }: { onClose: () => void; onAdded: (c: DownloadClient) => void }) {
  const [name, setName] = useState('SABnzbd')
  const [host, setHost] = useState('')
  const [port, setPort] = useState('8080')
  const [apiKey, setApiKey] = useState('')
  const [category, setCategory] = useState('books')

  const submit = async () => {
    const c = await api.addDownloadClient({
      name, host, port: parseInt(port), apiKey, category, type: 'sabnzbd', enabled: true,
    })
    onAdded(c)
  }

  return (
    <div className="mt-4 p-4 border border-zinc-700 rounded-lg bg-zinc-800/50 space-y-3">
      <input value={name} onChange={e => setName(e.target.value)} placeholder="Name" className={inputCls} />
      <div className="flex gap-2">
        <input value={host} onChange={e => setHost(e.target.value)} placeholder="Host" className="flex-1 bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm focus:outline-none focus:border-zinc-600" />
        <input value={port} onChange={e => setPort(e.target.value)} placeholder="Port" className="w-24 bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm focus:outline-none focus:border-zinc-600" />
      </div>
      <input value={apiKey} onChange={e => setApiKey(e.target.value)} placeholder="API Key" type="password" className={inputCls} />
      <input value={category} onChange={e => setCategory(e.target.value)} placeholder="Category" className={inputCls} />
      <div className="flex gap-2 justify-end">
        <button onClick={onClose} className="px-3 py-1.5 text-sm text-zinc-400">Cancel</button>
        <button onClick={submit} className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 rounded text-sm font-medium">Save</button>
      </div>
    </div>
  )
}

function AddNotificationForm({ onClose, onAdded }: { onClose: () => void; onAdded: (n: NotificationConfig) => void }) {
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [method, setMethod] = useState('POST')
  const [onGrab, setOnGrab] = useState(true)
  const [onImport, setOnImport] = useState(true)
  const [onFailure, setOnFailure] = useState(true)
  const [onUpgrade, setOnUpgrade] = useState(false)
  const [onHealth, setOnHealth] = useState(false)

  const submit = async () => {
    const n = await api.addNotification({
      name, url, method, type: 'webhook',
      headers: '{}',
      onGrab, onImport, onFailure, onUpgrade, onHealth,
      enabled: true,
    })
    onAdded(n)
  }

  const toggleCls = (active: boolean) =>
    `px-3 py-1.5 rounded text-xs font-medium border transition-colors cursor-pointer select-none ${
      active
        ? 'bg-emerald-500/20 border-emerald-500/40 text-emerald-400'
        : 'bg-zinc-800 border-zinc-700 text-zinc-400'
    }`

  return (
    <div className="mt-4 p-4 border border-zinc-700 rounded-lg bg-zinc-800/50 space-y-4">
      <div className="grid grid-cols-2 gap-3">
        <input value={name} onChange={e => setName(e.target.value)} placeholder="Name" className={inputCls} />
        <select value={method} onChange={e => setMethod(e.target.value)} className={inputCls}>
          <option value="POST">POST</option>
          <option value="PUT">PUT</option>
          <option value="GET">GET</option>
        </select>
      </div>
      <input value={url} onChange={e => setUrl(e.target.value)} placeholder="Webhook URL" className={inputCls} />
      <div>
        <p className="text-xs text-zinc-400 mb-2">Trigger on:</p>
        <div className="flex flex-wrap gap-2">
          <button type="button" onClick={() => setOnGrab(!onGrab)} className={toggleCls(onGrab)}>Grab</button>
          <button type="button" onClick={() => setOnImport(!onImport)} className={toggleCls(onImport)}>Import</button>
          <button type="button" onClick={() => setOnFailure(!onFailure)} className={toggleCls(onFailure)}>Failure</button>
          <button type="button" onClick={() => setOnUpgrade(!onUpgrade)} className={toggleCls(onUpgrade)}>Upgrade</button>
          <button type="button" onClick={() => setOnHealth(!onHealth)} className={toggleCls(onHealth)}>Health</button>
        </div>
      </div>
      <div className="flex gap-2 justify-end">
        <button onClick={onClose} className="px-3 py-1.5 text-sm text-zinc-400">Cancel</button>
        <button onClick={submit} className="px-3 py-1.5 bg-emerald-600 hover:bg-emerald-500 rounded text-sm font-medium">Save</button>
      </div>
    </div>
  )
}
