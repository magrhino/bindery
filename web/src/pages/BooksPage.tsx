import { useEffect, useState, useMemo } from 'react'
import { api, Book } from '../api/client'
import Pagination from '../components/Pagination'
import { usePagination } from '../components/usePagination'

type SortMode = 'title-az' | 'title-za' | 'date-new' | 'date-old'

const statusColors: Record<string, string> = {
  wanted: 'bg-amber-500/20 text-amber-400',
  downloading: 'bg-blue-500/20 text-blue-400',
  downloaded: 'bg-cyan-500/20 text-cyan-400',
  imported: 'bg-emerald-500/20 text-emerald-400',
  skipped: 'bg-zinc-700 text-zinc-400',
}

const statusLabel: Record<string, string> = {
  wanted: 'Wanted',
  downloading: 'Downloading',
  downloaded: 'Downloaded',
  imported: 'In Library',
  skipped: 'Skipped',
}

export default function BooksPage() {
  const [books, setBooks] = useState<Book[]>([])
  const [loading, setLoading] = useState(true)
  const [statusFilter, setStatusFilter] = useState('')
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<SortMode>('title-az')

  useEffect(() => {
    api.listBooks().then(setBooks).catch(console.error).finally(() => setLoading(false))
  }, [])

  const filtered = useMemo(() => {
    let list = books
    if (statusFilter) list = list.filter(b => b.status === statusFilter)
    if (search.trim()) {
      const q = search.trim().toLowerCase()
      list = list.filter(b =>
        b.title.toLowerCase().includes(q) ||
        (b.author?.authorName && b.author.authorName.toLowerCase().includes(q))
      )
    }
    if (sort === 'title-az') list = [...list].sort((a, b) => a.title.localeCompare(b.title))
    else if (sort === 'title-za') list = [...list].sort((a, b) => b.title.localeCompare(a.title))
    else if (sort === 'date-new') list = [...list].sort((a, b) => {
      const da = a.releaseDate ? new Date(a.releaseDate).getTime() : 0
      const db = b.releaseDate ? new Date(b.releaseDate).getTime() : 0
      return db - da
    })
    else if (sort === 'date-old') list = [...list].sort((a, b) => {
      const da = a.releaseDate ? new Date(a.releaseDate).getTime() : 0
      const db = b.releaseDate ? new Date(b.releaseDate).getTime() : 0
      return da - db
    })
    return list
  }, [books, statusFilter, search, sort])

  const { pageItems, paginationProps, reset } = usePagination(filtered, 50)

  useEffect(() => { reset() }, [statusFilter, search, sort, reset])

  const statusBtnCls = (active: boolean) =>
    `px-3 py-1 rounded-md text-xs font-medium transition-colors ${active ? 'bg-zinc-700 text-white' : 'text-zinc-400 hover:text-white'}`

  const sortBtnCls = (active: boolean) =>
    `px-3 py-1 rounded-md text-xs font-medium transition-colors ${active ? 'bg-zinc-700 text-white' : 'text-zinc-400 hover:text-white hover:bg-zinc-800/50'}`

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-2xl font-bold">Books</h2>
        <span className="text-sm text-zinc-500">{filtered.length} of {books.length}</span>
      </div>

      {/* Controls */}
      <div className="flex flex-col sm:flex-row gap-3 mb-6">
        <input
          type="search"
          value={search}
          onChange={e => setSearch(e.target.value)}
          placeholder="Search books or authors..."
          className="flex-1 bg-zinc-800 border border-zinc-700 rounded px-3 py-2 text-sm focus:outline-none focus:border-zinc-600 placeholder-zinc-600"
        />
        <div className="flex gap-1 flex-wrap">
          {(['', 'wanted', 'downloading', 'imported', 'skipped'] as const).map(s => (
            <button
              key={s}
              onClick={() => setStatusFilter(s)}
              className={statusBtnCls(statusFilter === s)}
            >
              {s ? (statusLabel[s] ?? s) : 'All'}
            </button>
          ))}
        </div>
      </div>

      <div className="flex gap-1 mb-4">
        <span className="text-xs text-zinc-500 mr-1 self-center">Sort:</span>
        <button onClick={() => setSort('title-az')} className={sortBtnCls(sort === 'title-az')}>A–Z</button>
        <button onClick={() => setSort('title-za')} className={sortBtnCls(sort === 'title-za')}>Z–A</button>
        <button onClick={() => setSort('date-new')} className={sortBtnCls(sort === 'date-new')}>Newest</button>
        <button onClick={() => setSort('date-old')} className={sortBtnCls(sort === 'date-old')}>Oldest</button>
      </div>

      {loading ? (
        <div className="text-zinc-500">Loading...</div>
      ) : filtered.length === 0 ? (
        <div className="text-center py-16 text-zinc-500">
          <p>{books.length === 0 ? 'No books found' : 'No books match your filters'}</p>
        </div>
      ) : (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
          {pageItems.map(book => (
            <div key={book.id} className="border border-zinc-800 rounded-lg bg-zinc-900 overflow-hidden group">
              <div className="aspect-[2/3] bg-zinc-800 relative">
                {book.imageUrl ? (
                  <img src={book.imageUrl} alt={book.title} className="w-full h-full object-cover" />
                ) : (
                  <div className="w-full h-full flex items-center justify-center p-3 text-center">
                    <span className="text-sm text-zinc-600">{book.title}</span>
                  </div>
                )}
                <div className={`absolute top-2 right-2 px-2 py-0.5 rounded text-[10px] font-medium ${statusColors[book.status] || 'bg-zinc-700 text-zinc-400'}`}>
                  {statusLabel[book.status] ?? book.status}
                </div>
              </div>
              <div className="p-2">
                <h3 className="text-xs font-medium truncate" title={book.title}>{book.title}</h3>
                <div className="flex items-center justify-between mt-0.5">
                  {book.releaseDate && (
                    <p className="text-[10px] text-zinc-500">{new Date(book.releaseDate).getFullYear()}</p>
                  )}
                  {book.filePath && (
                    <a
                      href={`/api/v1/book/${book.id}/file`}
                      className="text-[10px] text-emerald-400 hover:text-emerald-300"
                      title="Download file"
                    >
                      Download
                    </a>
                  )}
                </div>
              </div>
            </div>
          ))}
        </div>
      )}
      <Pagination {...paginationProps} />
    </div>
  )
}
