import { useEffect, useState, useMemo } from 'react'
import { api, Author } from '../api/client'
import AddAuthorModal from '../components/AddAuthorModal'
import Pagination from '../components/Pagination'
import { usePagination } from '../components/usePagination'

type SortMode = 'az' | 'za' | 'recent'

export default function AuthorsPage() {
  const [authors, setAuthors] = useState<Author[]>([])
  const [loading, setLoading] = useState(true)
  const [showAdd, setShowAdd] = useState(false)
  const [search, setSearch] = useState('')
  const [sort, setSort] = useState<SortMode>('az')

  const load = () => {
    setLoading(true)
    api.listAuthors().then(setAuthors).catch(console.error).finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [])

  const handleDelete = async (id: number) => {
    if (!confirm('Delete this author and all their books?')) return
    await api.deleteAuthor(id)
    load()
  }

  const handleToggleMonitored = async (author: Author) => {
    await api.updateAuthor(author.id, { monitored: !author.monitored } as Partial<Author>)
    load()
  }

  const filtered = useMemo(() => {
    let list = authors
    if (search.trim()) {
      const q = search.trim().toLowerCase()
      list = list.filter(a =>
        a.authorName.toLowerCase().includes(q) ||
        (a.description && a.description.toLowerCase().includes(q))
      )
    }
    if (sort === 'az') list = [...list].sort((a, b) => a.authorName.localeCompare(b.authorName))
    else if (sort === 'za') list = [...list].sort((a, b) => b.authorName.localeCompare(a.authorName))
    // 'recent' keeps server order (typically by id desc)
    return list
  }, [authors, search, sort])

  const { pageItems, paginationProps, reset } = usePagination(filtered, 50)

  useEffect(() => { reset() }, [search, sort, reset])

  const sortBtnCls = (active: boolean) =>
    `px-3 py-1 rounded-md text-xs font-medium transition-colors ${active ? 'bg-slate-300 dark:bg-zinc-700 text-slate-900 dark:text-white' : 'text-slate-600 dark:text-zinc-400 hover:text-slate-900 dark:hover:text-white hover:bg-slate-200/50 dark:hover:bg-zinc-800/50'}`

  return (
    <div>
      <div className="flex items-center justify-between mb-4">
        <h2 className="text-2xl font-bold">Authors</h2>
        <button
          onClick={() => setShowAdd(true)}
          className="px-4 py-2 bg-emerald-600 hover:bg-emerald-500 rounded-md text-sm font-medium transition-colors"
        >
          + Add Author
        </button>
      </div>

      {/* Search & Sort controls */}
      <div className="flex flex-col sm:flex-row gap-3 mb-6">
        <input
          type="search"
          value={search}
          onChange={e => setSearch(e.target.value)}
          placeholder="Search authors..."
          className="flex-1 bg-slate-200 dark:bg-zinc-800 border border-slate-300 dark:border-zinc-700 rounded px-3 py-2 text-sm focus:outline-none focus:border-slate-400 dark:focus:border-zinc-600 placeholder-slate-400 dark:placeholder-zinc-600"
        />
        <div className="flex gap-1">
          <button onClick={() => setSort('az')} className={sortBtnCls(sort === 'az')}>A–Z</button>
          <button onClick={() => setSort('za')} className={sortBtnCls(sort === 'za')}>Z–A</button>
          <button onClick={() => setSort('recent')} className={sortBtnCls(sort === 'recent')}>Recent</button>
        </div>
      </div>

      {loading ? (
        <div className="text-slate-600 dark:text-zinc-500">Loading...</div>
      ) : filtered.length === 0 && authors.length === 0 ? (
        <div className="text-center py-16 text-slate-600 dark:text-zinc-500">
          <p className="text-lg mb-2">No authors yet</p>
          <p className="text-sm">Click "Add Author" to start tracking your favorite authors</p>
        </div>
      ) : filtered.length === 0 ? (
        <div className="text-center py-16 text-slate-600 dark:text-zinc-500">
          <p>No authors match "{search}"</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
          {pageItems.map(author => (
            <div key={author.id} className="border border-slate-200 dark:border-zinc-800 rounded-lg bg-slate-100 dark:bg-zinc-900 overflow-hidden">
              <div className="flex gap-3 p-4">
                {author.imageUrl ? (
                  <img src={author.imageUrl} alt={author.authorName} className="w-16 h-16 rounded-full object-cover flex-shrink-0" />
                ) : (
                  <div className="w-16 h-16 rounded-full bg-slate-200 dark:bg-zinc-800 flex items-center justify-center flex-shrink-0 text-xl font-bold text-slate-500 dark:text-zinc-600">
                    {author.authorName.charAt(0)}
                  </div>
                )}
                <div className="min-w-0">
                  <h3 className="font-semibold truncate">{author.authorName}</h3>
                  <p className="text-xs text-slate-600 dark:text-zinc-500 mt-1 line-clamp-2">
                    {author.description || 'No description available'}
                  </p>
                </div>
              </div>
              <div className="flex items-center justify-between px-4 py-2 bg-slate-200/50 dark:bg-zinc-800/50 border-t border-slate-200 dark:border-zinc-800">
                <button
                  onClick={() => handleToggleMonitored(author)}
                  className={`text-xs px-2 py-1 rounded ${author.monitored ? 'bg-emerald-500/20 text-emerald-400' : 'bg-slate-300 dark:bg-zinc-700 text-slate-600 dark:text-zinc-400'}`}
                >
                  {author.monitored ? 'Monitored' : 'Unmonitored'}
                </button>
                <div className="flex gap-2">
                  <button
                    onClick={() => api.refreshAuthor(author.id).then(load)}
                    className="text-xs text-slate-600 dark:text-zinc-400 hover:text-slate-900 dark:hover:text-white"
                    title="Refresh metadata"
                  >
                    Refresh
                  </button>
                  <button
                    onClick={() => handleDelete(author.id)}
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
      <Pagination {...paginationProps} />

      {showAdd && <AddAuthorModal onClose={() => setShowAdd(false)} onAdded={load} />}
    </div>
  )
}
