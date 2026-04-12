import { useEffect, useState, useMemo } from 'react'
import { api, Book, SearchResult } from '../api/client'
import Pagination from '../components/Pagination'
import { usePagination } from '../components/usePagination'

export default function WantedPage() {
  const [books, setBooks] = useState<Book[]>([])
  const [loading, setLoading] = useState(true)
  const [searchingId, setSearchingId] = useState<number | null>(null)
  const [results, setResults] = useState<SearchResult[]>([])
  const [showResults, setShowResults] = useState<number | null>(null)
  const [search, setSearch] = useState('')

  useEffect(() => {
    api.listWanted().then(setBooks).catch(console.error).finally(() => setLoading(false))
  }, [])

  const filtered = useMemo(() => {
    if (!search.trim()) return books
    const q = search.trim().toLowerCase()
    return books.filter(b =>
      b.title.toLowerCase().includes(q) ||
      (b.author?.authorName && b.author.authorName.toLowerCase().includes(q))
    )
  }, [books, search])

  const searchBook = async (book: Book) => {
    setSearchingId(book.id)
    try {
      const res = await api.searchBook(book.id)
      setResults(res)
      setShowResults(book.id)
    } catch (err) {
      console.error(err)
    } finally {
      setSearchingId(null)
    }
  }

  const grab = async (result: SearchResult, bookId: number) => {
    try {
      await api.grab({
        guid: result.guid,
        title: result.title,
        nzbUrl: result.nzbUrl,
        size: result.size,
        bookId,
      })
      setShowResults(null)
      // Refresh wanted list
      const updated = await api.listWanted()
      setBooks(updated)
    } catch (err: unknown) {
      alert(err instanceof Error ? err.message : 'Grab failed')
    }
  }

  const { pageItems, paginationProps, reset } = usePagination(filtered, 50)

  useEffect(() => { reset() }, [search, reset])

  const formatSize = (bytes: number) => {
    if (bytes > 1073741824) return (bytes / 1073741824).toFixed(1) + ' GB'
    if (bytes > 1048576) return (bytes / 1048576).toFixed(1) + ' MB'
    return (bytes / 1024).toFixed(0) + ' KB'
  }

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-2xl font-bold">Wanted</h2>
        <span className="text-sm text-zinc-500">{filtered.length} of {books.length}</span>
      </div>

      <input
        type="search"
        value={search}
        onChange={e => setSearch(e.target.value)}
        placeholder="Search by title or author..."
        className="w-full mb-4 bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm focus:outline-none focus:border-zinc-600 placeholder-zinc-600"
      />

      {loading ? (
        <div className="text-zinc-500">Loading...</div>
      ) : books.length === 0 ? (
        <div className="text-center py-16 text-zinc-500">
          <p>No wanted books. Add an author to start tracking.</p>
        </div>
      ) : filtered.length === 0 ? (
        <div className="text-center py-16 text-zinc-500">
          <p>No books match your search.</p>
        </div>
      ) : (
        <div className="space-y-2">
          {pageItems.map(book => (
            <div key={book.id}>
              <div className="flex items-center justify-between p-3 border border-zinc-800 rounded-lg bg-zinc-900">
                <div className="flex items-center gap-3 min-w-0">
                  {book.imageUrl && (
                    <img src={book.imageUrl} alt="" className="w-10 h-14 object-cover rounded flex-shrink-0" />
                  )}
                  <div className="min-w-0">
                    <h3 className="font-medium text-sm truncate">{book.title}</h3>
                    {book.releaseDate && (
                      <p className="text-xs text-zinc-500">{new Date(book.releaseDate).getFullYear()}</p>
                    )}
                  </div>
                </div>
                <button
                  onClick={() => searchBook(book)}
                  disabled={searchingId === book.id}
                  className="px-3 py-1.5 bg-zinc-800 hover:bg-zinc-700 rounded text-xs font-medium flex-shrink-0 disabled:opacity-50"
                >
                  {searchingId === book.id ? 'Searching...' : 'Search'}
                </button>
              </div>

              {showResults === book.id && results.length === 0 && (
                <div className="mt-1 mb-3 px-3 py-2 bg-zinc-800/50 rounded text-xs text-zinc-500">
                  No results found on any indexer.
                </div>
              )}

              {showResults === book.id && results.length > 0 && (
                <div className="mt-1 mb-3 space-y-1">
                  {results.slice(0, 10).map(r => (
                    <div key={r.guid} className="flex items-center justify-between p-2 bg-zinc-800/50 rounded text-xs">
                      <div className="min-w-0 mr-3">
                        <span className="truncate block">{r.title}</span>
                        <span className="text-zinc-500 truncate block">{r.indexerName} &middot; {formatSize(r.size)} &middot; {r.grabs} grabs</span>
                      </div>
                      <button
                        onClick={() => grab(r, book.id)}
                        className="px-2 py-2 bg-emerald-600 hover:bg-emerald-500 rounded text-[10px] font-medium flex-shrink-0 touch-manipulation"
                      >
                        Grab
                      </button>
                    </div>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}
      <Pagination {...paginationProps} />
    </div>
  )
}
