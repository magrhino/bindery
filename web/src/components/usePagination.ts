import { useMemo, useState } from 'react'

/**
 * usePagination: client-side slicing helper.
 * Pass the full filtered list; get back the visible page + props for Pagination.
 */
export function usePagination<T>(items: T[], defaultPageSize = 50) {
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(defaultPageSize)

  const totalPages = Math.max(1, Math.ceil(items.length / pageSize))
  const safePage = Math.min(page, totalPages)
  const paged = useMemo(() => items.slice((safePage - 1) * pageSize, safePage * pageSize), [items, safePage, pageSize])

  const reset = () => setPage(1)

  return {
    pageItems: paged,
    paginationProps: {
      page: safePage,
      totalPages,
      pageSize,
      totalItems: items.length,
      onPageChange: setPage,
      onPageSizeChange: (size: number) => { setPageSize(size); setPage(1) },
    },
    reset,
  }
}
