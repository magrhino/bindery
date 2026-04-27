import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import AuthorsPage from './AuthorsPage'
import { api } from '../api/client'

vi.mock('../api/client', async importOriginal => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listAuthors: vi.fn(),
      createSeries: vi.fn(),
    },
  }
})

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => {
      const labels: Record<string, string> = {
        'authors.title': 'Authors',
        'authors.merge': 'Merge',
        'authors.addAuthor': 'Add Author',
        'authors.searchPlaceholder': 'Search authors...',
        'authors.sortAZ': 'A-Z',
        'authors.sortZA': 'Z-A',
        'authors.sortRecent': 'Recent',
        'authors.filterMonitored': 'Monitored:',
        'authors.filterAll': 'All',
        'authors.filterMonitoredOnly': 'Monitored',
        'authors.filterUnmonitored': 'Unmonitored',
        'authors.empty': 'No authors found',
        'authors.emptyHint': 'Add an author to get started',
      }
      return labels[key] ?? fallback ?? key
    },
  }),
}))

vi.mock('../components/usePagination', () => ({
  usePagination: <T,>(items: T[]) => ({
    pageItems: items,
    paginationProps: { page: 1, totalPages: 1, pageSize: 50, totalItems: items.length, onPageChange: vi.fn(), onPageSizeChange: vi.fn() },
    reset: vi.fn(),
  }),
}))

vi.mock('../components/Pagination', () => ({ default: () => null }))

describe('AuthorsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.listAuthors).mockResolvedValue([])
  })

  it('opens the add series flow from the authors toolbar', async () => {
    render(
      <MemoryRouter>
        <AuthorsPage />
      </MemoryRouter>,
    )

    fireEvent.click(await screen.findByRole('button', { name: 'Add Series' }))

    expect(await screen.findByRole('dialog', { name: 'Add Series' })).toBeInTheDocument()
  })
})
