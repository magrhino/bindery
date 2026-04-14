import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import ViewToggle from '../components/ViewToggle'
import { useView } from '../components/useView'
import { api, Book } from '../api/client'
import Pagination from '../components/Pagination'
import { usePagination } from '../components/usePagination'

type SortMode = 'title-az' | 'title-za' | 'date-new' | 'date-old'

const statusColors: Record<string, string> = {
  wanted: 'bg-amber-500/20 text-amber-400',
  downloading: 'bg-blue-500/20 text-blue-400',
  downloaded: 'bg-cyan-500/20 text-cyan-400',
  imported: 'bg-emerald-500/20 text-emerald-400',
  skipped: 'bg-slate-300 dark:bg-zinc-700 text-slate-600 dark:text-zinc-400',
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
  const [mediaFilter, setMediaFilter] = useState<'' | 'ebook' | 'audiobook'>('')
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<SortMode>('title-az')
  const [view, setView] = useView('books', 'grid')

  useEffect(() => {
    api.listBooks().then(setBooks).catch(console.error).finally(() => setLoading(false))
  }, [])

  const filtered = useMemo(() => {
    let list = books
    if (statusFilter) list = list.filter(b => b.status === statusFilter)
    if (mediaFilter) list = list.filter(b => (b.mediaType || 'ebook') === mediaFilter)
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
  }, [books, statusFilter, mediaFilter, search, sort])

  const { pageItems, paginationProps, reset } = usePagination(filtered, 50, 'books')

  useEffect(() => { reset() }, [statusFilter, mediaFilter, search, sort, reset])

  const statusBtnCls = (active: boolean) =>
    `px-3 py-1 rounded-md text-xs font-medium transition-colors ${active ? 'bg-slate-300 dark:bg-zinc-700 text-slate-900 dark:text-white' : 'text-slate-600 dark:text-zinc-400 hover:text-slate-900 dark:hover:text-white'}`

  const sortBtnCls = (active: boolean) =>
    `px-3 py-1 rounded-md text-xs font-medium transition-colors ${active ? 'bg-slate-300 dark:bg-zinc-700 text-slate-900 dark:text-white' : 'text-slate-600 dark:text-zinc-400 hover:text-slate-900 dark:hover:text-white hover:bg-slate-200/50 dark:hover:bg-zinc-800/50'}`

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-2xl font-bold">Books</h2>
        <div className="flex items-center gap-3">
          <span className="text-sm text-slate-600 dark:text-zinc-500">{filtered.length} of {books.length}</span>
          <ViewToggle view={view} onChange={setView} />
        </div>
      </div>

      {/* Controls */}
      <div className="flex flex-col sm:flex-row gap-3 mb-6">
        <input
          type="search"
          value={search}
          onChange={e => setSearch(e.target.value)}
          placeholder="Search books or authors..."
          className="flex-1 bg-slate-200 dark:bg-zinc-800 border border-slate-300 dark:border-zinc-700 rounded px-3 py-2 text-sm focus:outline-none focus:border-slate-400 dark:focus:border-zinc-600 placeholder-slate-400 dark:placeholder-zinc-600"
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

      <div className="flex gap-1 mb-4 flex-wrap">
        <span className="text-xs text-slate-600 dark:text-zinc-500 mr-1 self-center">Sort:</span>
        <button onClick={() => setSort('title-az')} className={sortBtnCls(sort === 'title-az')}>A–Z</button>
        <button onClick={() => setSort('title-za')} className={sortBtnCls(sort === 'title-za')}>Z–A</button>
        <button onClick={() => setSort('date-new')} className={sortBtnCls(sort === 'date-new')}>Newest</button>
        <button onClick={() => setSort('date-old')} className={sortBtnCls(sort === 'date-old')}>Oldest</button>

        <span className="text-xs text-slate-600 dark:text-zinc-500 mx-2 self-center">Type:</span>
        <button onClick={() => setMediaFilter('')} className={sortBtnCls(mediaFilter === '')}>All</button>
        <button onClick={() => setMediaFilter('ebook')} className={sortBtnCls(mediaFilter === 'ebook')}>📖 Ebook</button>
        <button onClick={() => setMediaFilter('audiobook')} className={sortBtnCls(mediaFilter === 'audiobook')}>🎧 Audiobook</button>
      </div>

      {loading ? (
        <div className="text-slate-600 dark:text-zinc-500">Loading...</div>
      ) : filtered.length === 0 ? (
        <div className="text-center py-16 text-slate-600 dark:text-zinc-500">
          <p>{books.length === 0 ? 'No books found' : 'No books match your filters'}</p>
        </div>
      ) : (
        view === 'table' ? (
        <div className="border border-slate-200 dark:border-zinc-800 rounded-lg overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="bg-slate-100 dark:bg-zinc-900 border-b border-slate-200 dark:border-zinc-800">
                  <th className="text-left px-3 py-2 text-xs font-medium text-slate-600 dark:text-zinc-400 uppercase">Title</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-slate-600 dark:text-zinc-400 uppercase hidden md:table-cell">Author</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-slate-600 dark:text-zinc-400 uppercase hidden sm:table-cell">Year</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-slate-600 dark:text-zinc-400 uppercase">Type</th>
                  <th className="text-left px-3 py-2 text-xs font-medium text-slate-600 dark:text-zinc-400 uppercase">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-200 dark:divide-zinc-800">
                {pageItems.map(book => (
                  <tr
                    key={book.id}
                    className="bg-slate-100/50 dark:bg-zinc-900/50 hover:bg-slate-200/50 dark:hover:bg-zinc-800/50 cursor-pointer"
                    onClick={() => (window.location.href = `/book/${book.id}`)}
                  >
                    <td className="px-3 py-2">
                      <Link to={`/book/${book.id}`} className="flex items-center gap-2" onClick={e => e.stopPropagation()}>
                        {book.imageUrl ? (
                          <img src={book.imageUrl} alt="" className="w-6 h-9 object-cover rounded flex-shrink-0" />
                        ) : (
                          <div className="w-6 h-9 bg-slate-200 dark:bg-zinc-800 rounded flex-shrink-0" />
                        )}
                        <span className="text-slate-800 dark:text-zinc-200 truncate">{book.title}</span>
                      </Link>
                    </td>
                    <td className="px-3 py-2 text-slate-600 dark:text-zinc-400 whitespace-nowrap hidden md:table-cell">{book.author?.authorName || '—'}</td>
                    <td className="px-3 py-2 text-slate-600 dark:text-zinc-400 whitespace-nowrap hidden sm:table-cell">{book.releaseDate ? new Date(book.releaseDate).getFullYear() : '—'}</td>
                    <td className="px-3 py-2 text-xs whitespace-nowrap">
                      {book.mediaType === 'audiobook' ? '🎧 Audiobook' : '📖 Ebook'}
                    </td>
                    <td className="px-3 py-2 whitespace-nowrap">
                      <span className={`inline-block px-2 py-0.5 rounded text-[10px] font-medium ${statusColors[book.status] || 'bg-slate-300 dark:bg-zinc-700 text-slate-600 dark:text-zinc-400'}`}>
                        {statusLabel[book.status] ?? book.status}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
        ) : (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 xl:grid-cols-6 gap-4">
          {pageItems.map(book => (
            <Link
              key={book.id}
              to={`/book/${book.id}`}
              className="border border-slate-200 dark:border-zinc-800 rounded-lg bg-slate-100 dark:bg-zinc-900 overflow-hidden group text-left hover:border-emerald-500 transition-colors block"
            >
              <div className="aspect-[2/3] bg-slate-200 dark:bg-zinc-800 relative">
                {book.imageUrl ? (
                  <img src={book.imageUrl} alt={book.title} className="w-full h-full object-cover" />
                ) : (
                  <div className="w-full h-full flex items-center justify-center p-3 text-center">
                    <span className="text-sm text-slate-500 dark:text-zinc-600">{book.title}</span>
                  </div>
                )}
                <div className={`absolute top-2 right-2 px-2 py-0.5 rounded text-[10px] font-medium ${statusColors[book.status] || 'bg-slate-300 dark:bg-zinc-700 text-slate-600 dark:text-zinc-400'}`}>
                  {statusLabel[book.status] ?? book.status}
                </div>
                {book.mediaType === 'audiobook' && (
                  <div className="absolute top-2 left-2 px-1.5 py-0.5 rounded text-[10px] font-medium bg-indigo-600/90 text-white">🎧</div>
                )}
              </div>
              <div className="p-2">
                <h3 className="text-xs font-medium truncate" title={book.title}>{book.title}</h3>
                <div className="flex items-center justify-between mt-0.5">
                  {book.releaseDate && (
                    <p className="text-[10px] text-slate-600 dark:text-zinc-500">{new Date(book.releaseDate).getFullYear()}</p>
                  )}
                  {book.filePath && (
                    <a
                      href={`/api/v1/book/${book.id}/file`}
                      onClick={e => e.stopPropagation()}
                      className="text-[10px] text-emerald-400 hover:text-emerald-300"
                      title="Download file"
                    >
                      Download
                    </a>
                  )}
                </div>
              </div>
            </Link>
          ))}
        </div>
        )
      )}
      <Pagination {...paginationProps} />
    </div>
  )
}
