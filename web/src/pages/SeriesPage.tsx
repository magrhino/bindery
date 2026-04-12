import { useEffect, useState } from 'react'
import { api, Series } from '../api/client'

export default function SeriesPage() {
  const [seriesList, setSeriesList] = useState<Series[]>([])
  const [loading, setLoading] = useState(true)
  const [expanded, setExpanded] = useState<number | null>(null)

  useEffect(() => {
    api.listSeries().then(setSeriesList).catch(console.error).finally(() => setLoading(false))
  }, [])

  const toggleExpanded = (id: number) => {
    setExpanded(prev => (prev === id ? null : id))
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-6">
        <h2 className="text-2xl font-bold">Series</h2>
        <span className="text-sm text-zinc-500">{seriesList.length} series</span>
      </div>

      {loading ? (
        <div className="text-zinc-500">Loading...</div>
      ) : seriesList.length === 0 ? (
        <div className="text-center py-16 text-zinc-500">
          <p className="text-lg mb-2">No series found</p>
          <p className="text-sm">Series are populated automatically from your monitored authors' books</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-4">
          {seriesList.map(series => {
            const bookCount = series.books?.length ?? 0
            const isOpen = expanded === series.id
            const sortedBooks = series.books
              ? [...series.books].sort((a, b) => {
                  const posA = parseFloat(a.positionInSeries) || 0
                  const posB = parseFloat(b.positionInSeries) || 0
                  return posA - posB
                })
              : []

            return (
              <div key={series.id} className="border border-zinc-800 rounded-lg bg-zinc-900 overflow-hidden">
                <div
                  className="p-4 cursor-pointer hover:bg-zinc-800/50 transition-colors"
                  onClick={() => toggleExpanded(series.id)}
                >
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <h3 className="font-semibold truncate">{series.title}</h3>
                      {series.description && (
                        <p className="text-xs text-zinc-500 mt-1 line-clamp-2">{series.description}</p>
                      )}
                    </div>
                    <div className="flex-shrink-0 flex items-center gap-2">
                      <span className="text-xs text-zinc-500 bg-zinc-800 px-2 py-0.5 rounded-full">
                        {bookCount} {bookCount === 1 ? 'book' : 'books'}
                      </span>
                      <span className="text-zinc-600 text-xs">{isOpen ? '▲' : '▼'}</span>
                    </div>
                  </div>
                </div>

                {isOpen && bookCount > 0 && (
                  <div className="border-t border-zinc-800 divide-y divide-zinc-800/50">
                    {sortedBooks.map(entry => (
                      <div key={entry.bookId} className="flex items-center gap-3 px-4 py-3 bg-zinc-900/80">
                        <span className="text-xs text-zinc-500 w-10 flex-shrink-0 font-mono">
                          #{entry.positionInSeries || '?'}
                        </span>
                        {entry.book?.imageUrl ? (
                          <img
                            src={entry.book.imageUrl}
                            alt={entry.book.title}
                            className="w-8 h-10 object-cover rounded flex-shrink-0"
                          />
                        ) : (
                          <div className="w-8 h-10 bg-zinc-800 rounded flex-shrink-0" />
                        )}
                        <div className="min-w-0">
                          <p className="text-sm font-medium truncate">
                            {entry.book?.title ?? `Book ${entry.bookId}`}
                          </p>
                          {entry.book?.releaseDate && (
                            <p className="text-xs text-zinc-500">
                              {new Date(entry.book.releaseDate).getFullYear()}
                            </p>
                          )}
                        </div>
                        {entry.book?.status && (
                          <span className={`ml-auto text-xs px-2 py-0.5 rounded flex-shrink-0 ${
                            entry.book.status === 'imported'
                              ? 'bg-emerald-500/20 text-emerald-400'
                              : entry.book.status === 'wanted'
                              ? 'bg-amber-500/20 text-amber-400'
                              : 'bg-zinc-700 text-zinc-400'
                          }`}>
                            {entry.book.status}
                          </span>
                        )}
                      </div>
                    ))}
                  </div>
                )}

                {isOpen && bookCount === 0 && (
                  <div className="border-t border-zinc-800 px-4 py-3 text-sm text-zinc-500">
                    No books in this series yet
                  </div>
                )}
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}
