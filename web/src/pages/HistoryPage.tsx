import { useEffect, useState } from 'react'
import { api, HistoryEvent } from '../api/client'

const EVENT_TYPE_COLORS: Record<string, string> = {
  grabbed: 'bg-blue-500/20 text-blue-400',
  imported: 'bg-emerald-500/20 text-emerald-400',
  downloadFailed: 'bg-red-500/20 text-red-400',
  deleted: 'bg-red-500/20 text-red-400',
  renamed: 'bg-purple-500/20 text-purple-400',
  ignored: 'bg-zinc-700 text-zinc-400',
  bookFileRenamed: 'bg-purple-500/20 text-purple-400',
}

function formatDate(s: string) {
  return new Date(s).toLocaleString(undefined, {
    year: 'numeric', month: 'short', day: 'numeric',
    hour: '2-digit', minute: '2-digit',
  })
}

export default function HistoryPage() {
  const [events, setEvents] = useState<HistoryEvent[]>([])
  const [loading, setLoading] = useState(true)
  const [typeFilter, setTypeFilter] = useState('')

  const load = (filter?: string) => {
    setLoading(true)
    api.listHistory(filter ? { eventType: filter } : undefined)
      .then(setEvents)
      .catch(console.error)
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const handleFilterChange = (val: string) => {
    setTypeFilter(val)
    load(val || undefined)
  }

  const handleDelete = async (id: number) => {
    await api.deleteHistory(id).catch(console.error)
    setEvents(prev => prev.filter(e => e.id !== id))
  }

  const eventTypes = Array.from(new Set(events.map(e => e.eventType))).sort()

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold">History</h2>
        <select
          value={typeFilter}
          onChange={e => handleFilterChange(e.target.value)}
          className="bg-zinc-800 border border-zinc-700 rounded px-3 py-1.5 text-sm text-zinc-200 focus:outline-none focus:border-zinc-600"
        >
          <option value="">All event types</option>
          {eventTypes.map(t => (
            <option key={t} value={t}>{t}</option>
          ))}
        </select>
      </div>

      {loading ? (
        <div className="text-zinc-500">Loading...</div>
      ) : events.length === 0 ? (
        <div className="text-center py-16 text-zinc-500">
          <p>No history events found</p>
        </div>
      ) : (
        <div className="border border-zinc-800 rounded-lg overflow-hidden">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-zinc-900 border-b border-zinc-800">
                <th className="text-left px-4 py-3 text-xs font-medium text-zinc-400 uppercase tracking-wider">Event Type</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-zinc-400 uppercase tracking-wider">Source Title</th>
                <th className="text-left px-4 py-3 text-xs font-medium text-zinc-400 uppercase tracking-wider">Date</th>
                <th className="px-4 py-3" />
              </tr>
            </thead>
            <tbody className="divide-y divide-zinc-800">
              {events.map(event => (
                <tr key={event.id} className="bg-zinc-900/50 hover:bg-zinc-800/50 transition-colors">
                  <td className="px-4 py-3">
                    <span className={`inline-block px-2 py-0.5 rounded text-xs font-medium ${EVENT_TYPE_COLORS[event.eventType] ?? 'bg-zinc-700 text-zinc-400'}`}>
                      {event.eventType}
                    </span>
                  </td>
                  <td className="px-4 py-3 text-zinc-200 max-w-xs truncate" title={event.sourceTitle}>
                    {event.sourceTitle || <span className="text-zinc-600">—</span>}
                  </td>
                  <td className="px-4 py-3 text-zinc-400 whitespace-nowrap">
                    {formatDate(event.createdAt)}
                  </td>
                  <td className="px-4 py-3 text-right">
                    <button
                      onClick={() => handleDelete(event.id)}
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
