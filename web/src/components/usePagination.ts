import { useMemo, useState } from 'react'

/**
 * usePagination: client-side slicing helper.
 * Pass the full filtered list; get back the visible page + props for Pagination.
 *
 * storageKey: when provided, page size is persisted to localStorage under that
 * key so the user's preference survives navigation and page reloads.
 */
export function usePagination<T>(items: T[], defaultPageSize = 50, storageKey?: string) {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(() => {
    if (storageKey) {
      const stored = localStorage.getItem(`pageSize:${storageKey}`)
      if (stored) {
        const n = parseInt(stored, 10)
        if (!isNaN(n) && n > 0) return n
      }
    }
    return defaultPageSize
  })

  const totalPages = Math.max(1, Math.ceil(items.length / pageSize))
  const safePage = Math.min(page, totalPages)
  const paged = useMemo(() => items.slice((safePage - 1) * pageSize, safePage * pageSize), [items, safePage, pageSize])

  const reset = () => setPage(1)

  const handlePageSizeChange = (size: number) => {
    setPageSize(size)
    setPage(1)
    if (storageKey) {
      localStorage.setItem(`pageSize:${storageKey}`, String(size))
    }
  }

  return {
    pageItems: paged,
    paginationProps: {
      page: safePage,
      totalPages,
      pageSize,
      totalItems: items.length,
      onPageChange: setPage,
      onPageSizeChange: handlePageSizeChange,
    },
    reset,
  }
}
