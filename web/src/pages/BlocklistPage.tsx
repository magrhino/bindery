import { useEffect, useState } from 'react'
import { api, BlocklistEntry } from '../api/client'

function formatDate(s: string) {
  return new Date(s).toLocaleString(undefined, {
    year: 'numeric', month: 'short', day: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}

export default function BlocklistPage() {
  const [entries, setEntries] = useState<BlocklistEntry[]>([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [deleting, setDeleting] = useState(false)

  const load = () => {
    setLoading(true)
    api.listBlocklist().then(setEntries).catch(console.error).finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const handleDelete = async (id: number) => {
    await api.deleteBlocklistEntry(id).catch(console.error)
    setEntries(prev => prev.filter(e => e.id !== id))
    setSelected(prev => { const s = new Set(prev); s.delete(id); return s })
  }

  const handleBulkDelete = async () => {
    if (selected.size === 0) return
    if (!confirm(`Delete ${selected.size} blocklist entries?`)) return
    setDeleting(true)
    try {
      await api.bulkDeleteBlocklist(Array.from(selected))
      setEntries(prev => prev.filter(e => !selected.has(e.id)))
      setSelected(new Set())
    } catch (err) {
      console.error(err)
    } finally {
      setDeleting(false)
    }
  }

  const toggleSelect = (id: number) => {
    setSelected(prev => {
      const s = new Set(prev)
      if (s.has(id)) s.delete(id)
      else s.add(id)
      return s
    })
  }

  const toggleAll = () => {
    if (selected.size === entries.length) {
      setSelected(new Set())
    } else {
      setSelected(new Set(entries.map(e => e.id)))
    }
  }

  const allSelected = entries.length > 0 && selected.size === entries.length

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold">Blocklist</h2>
        <div className="flex items-center gap-3">
          {selected.size > 0 && (
            <button
              onClick={handleBulkDelete}
              disabled={deleting}
              className="px-3 py-1.5 bg-red-600 hover:bg-red-500 rounded text-xs font-medium transition-colors disabled:opacity-50"
            >
              {deleting ? 'Deleting...' : `Delete selected (${selected.size})`}
            </button>
          )}
          <span className="text-sm text-zinc-500">{entries.length} entries</span>
        </div>
      </div>

      {loading ? (
        <div className="text-zinc-500">Loading...</div>
      ) : entries.length === 0 ? (
        <div className="text-center py-16 text-zinc-500">
          <p className="text-lg mb-2">Blocklist is empty</p>
          <p className="text-sm">Failed downloads are automatically added here to prevent re-grabbing</p>
        </div>
      ) : (
        <div className="border border-zinc-800 rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-zinc-900 border-b border-zinc-800">
                <th className="px-4 py-3 w-10">
                  <input
                    type="checkbox"
                    checked={allSelected}
                    onChange={toggleAll}
                    className="accent-emerald-500"
                  />
                </th>
                <th className="text-left px-4 py-3 text-xs font-medium text-zinc-400 uppercase tracking-wider">Title</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-zinc-400 uppercase tracking-wider">Reason</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-zinc-400 uppercase tracking-wider">Date</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800">
              {entries.map(entry => (
                <tr key={entry.id} className={`transition-colors hover:bg-zinc-800/50 ${selected.has(entry.id) ? 'bg-zinc-800/30' : 'bg-zinc-900/50'}`}>
                  <td className="px-4 py-3">
                    <input
                      type="checkbox"
                      checked={selected.has(entry.id)}
                      onChange={() => toggleSelect(entry.id)}
                      className="accent-emerald-500"
                    />
                  </td>
                  <td className="px-4 py-3">
                    <p className="text-zinc-200 truncate max-w-xs" title={entry.title}>{entry.title}</p>
                    {entry.guid && (
                      <p className="text-[10px] text-zinc-600 mt-0.5 font-mono truncate">{entry.guid}</p>
                    )}
                  </td>
                  <td className="px-4 py-3">
                    <span className="text-xs px-2 py-0.5 rounded bg-red-500/20 text-red-400">
                      {entry.reason || 'Unknown'}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-zinc-400 whitespace-nowrap text-xs">
                    {formatDate(entry.createdAt)}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button
                      onClick={() => handleDelete(entry.id)}
                      className="text-xs text-red-400 hover:text-red-300 transition-colors"
                    >
                      Delete
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
