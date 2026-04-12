import { useEffect, useState } from 'react'
import { api, QueueItem } from '../api/client'

export default function QueuePage() {
  const [queue, setQueue] = useState<QueueItem[]>([])
  const [loading, setLoading] = useState(true)

  const load = () => {
    api.listQueue().then(setQueue).catch(console.error).finally(() => setLoading(false))
  }

  useEffect(() => {
    load()
    const interval = setInterval(load, 5000)
    return () => clearInterval(interval)
  }, [])

  const handleDelete = async (id: number) => {
    await api.deleteFromQueue(id)
    load()
  }

  const statusColors: Record<string, string> = {
    queued: 'text-slate-600 dark:text-zinc-400',
    downloading: 'text-blue-400',
    completed: 'text-emerald-400',
    failed: 'text-red-400',
    imported: 'text-emerald-400',
  }

  const formatSize = (bytes: number) => {
    if (bytes > 1073741824) return (bytes / 1073741824).toFixed(1) + ' GB'
    if (bytes > 1048576) return (bytes / 1048576).toFixed(1) + ' MB'
    return (bytes / 1024).toFixed(0) + ' KB'
  }

  return (
    <div>
      <h2 className="text-2xl font-bold mb-6">Queue</h2>

      {loading ? (
        <div className="text-slate-600 dark:text-zinc-500">Loading...</div>
      ) : queue.length === 0 ? (
        <div className="text-center py-16 text-slate-600 dark:text-zinc-500">
          <p>Queue is empty</p>
        </div>
      ) : (
        <div className="space-y-2">
          {queue.map(item => (
            <div key={item.id} className="flex items-center justify-between p-4 border border-slate-200 dark:border-zinc-800 rounded-lg bg-slate-100 dark:bg-zinc-900">
              <div className="min-w-0 flex-1">
                <h3 className="font-medium text-sm truncate">{item.title}</h3>
                <div className="flex flex-wrap items-center gap-x-3 gap-y-1 mt-1 text-xs">
                  <span className={statusColors[item.status] || 'text-slate-600 dark:text-zinc-400'}>
                    {item.status}
                  </span>
                  <span className="text-slate-600 dark:text-zinc-500">{formatSize(item.size)}</span>
                  {item.percentage && (
                    <span className="text-blue-400">{item.percentage}%</span>
                  )}
                  {item.timeLeft && (
                    <span className="text-slate-600 dark:text-zinc-500">{item.timeLeft} remaining</span>
                  )}
                  {item.protocol && (
                    <span className="text-slate-500 dark:text-zinc-600">{item.protocol}</span>
                  )}
                </div>
                {item.errorMessage && (
                  <div className="mt-1 text-xs text-red-400 bg-red-400/10 rounded px-2 py-1">
                    {item.errorMessage}
                  </div>
                )}
                {item.percentage && (
                  <div className="mt-2 h-1 bg-slate-200 dark:bg-zinc-800 rounded-full overflow-hidden">
                    <div
                      className="h-full bg-blue-500 transition-all"
                      style={{ width: `${item.percentage}%` }}
                    />
                  </div>
                )}
              </div>
              <button
                onClick={() => handleDelete(item.id)}
                className="ml-4 px-3 py-2 text-xs text-red-400 hover:text-red-300 flex-shrink-0 touch-manipulation"
              >
                Remove
              </button>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
