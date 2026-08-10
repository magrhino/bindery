import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import AuthorsPage from './AuthorsPage'
import { api } from '../api/client'
import type { SetupState } from '../api/client'

vi.mock('../api/client', async importOriginal => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listAuthors: vi.fn(),
      setupState: vi.fn(),
    },
  }
})

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => {
      const labels: Record<string, string> = {
        'authors.empty': 'No authors yet',
        'authors.emptyHint': 'Click Add Author to start',
        'setupChecklist.title': 'Setup progress',
        'setupChecklist.indexer': 'Add an indexer',
        'setupChecklist.client': 'Add a download client',
        'setupChecklist.author': 'Add an author',
        'setupChecklist.grab': 'Grab a book',
        'setupChecklist.import': 'First book imported',
        'gettingStarted.indexers': 'Set up Indexers',
        'gettingStarted.downloadClients': 'Set up Download Clients',
        'common.dismiss': 'Dismiss',
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
  useServerPagination: (total: number) => ({
    page: 1,
    pageSize: 50,
    setPage: vi.fn(),
    reset: vi.fn(),
    paginationProps: { page: 1, totalPages: 1, pageSize: 50, totalItems: total, onPageChange: vi.fn(), onPageSizeChange: vi.fn() },
  }),
}))

vi.mock('../components/Pagination', () => ({ default: () => null }))

function state(overrides: Partial<SetupState> = {}): SetupState {
  return {
    hasIndexer: false,
    hasClient: false,
    hasAuthor: false,
    hasGrab: false,
    hasImport: false,
    complete: false,
    ...overrides,
  }
}

function renderPage() {
  return render(
    <MemoryRouter>
      <AuthorsPage />
    </MemoryRouter>,
  )
}

describe('AuthorsPage setup checklist', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    vi.mocked(api.listAuthors).mockResolvedValue({ items: [], total: 0, limit: 100, offset: 0 })
  })

  it('lists every outstanding step with links to the settings tabs', async () => {
    vi.mocked(api.setupState).mockResolvedValue(state())

    renderPage()

    expect(await screen.findByText('Setup progress')).toBeInTheDocument()
    expect(screen.getByText('0/5')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Set up Indexers' })).toHaveAttribute('href', '/settings?tab=indexers')
    expect(screen.getByRole('link', { name: 'Set up Download Clients' })).toHaveAttribute('href', '/settings?tab=clients')
  })

  it('counts completed steps and drops their action links', async () => {
    vi.mocked(api.setupState).mockResolvedValue(state({ hasIndexer: true, hasClient: true, hasAuthor: true }))

    renderPage()

    expect(await screen.findByText('3/5')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Set up Indexers' })).not.toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'Set up Download Clients' })).not.toBeInTheDocument()
    // Outstanding steps still listed.
    expect(screen.getByText('Grab a book')).toBeInTheDocument()
  })

  // The checklist is a first-run aid, not a permanent fixture: once the
  // pipeline has produced an import it must disappear for good.
  it('hides itself once setup is complete', async () => {
    vi.mocked(api.setupState).mockResolvedValue(
      state({ hasIndexer: true, hasClient: true, hasAuthor: true, hasGrab: true, hasImport: true, complete: true }),
    )

    renderPage()

    expect(await screen.findByText('No authors yet')).toBeInTheDocument()
    await waitFor(() => expect(api.setupState).toHaveBeenCalled())
    expect(screen.queryByText('Setup progress')).not.toBeInTheDocument()
  })

  it('stays hidden when the setup-state request fails', async () => {
    vi.mocked(api.setupState).mockRejectedValue(new Error('boom'))

    renderPage()

    expect(await screen.findByText('No authors yet')).toBeInTheDocument()
    await waitFor(() => expect(api.setupState).toHaveBeenCalled())
    expect(screen.queryByText('Setup progress')).not.toBeInTheDocument()
  })

  it('respects a stored dismissal', async () => {
    localStorage.setItem('bindery.setupChecklistDismissed', '1')
    vi.mocked(api.setupState).mockResolvedValue(state())

    renderPage()

    expect(await screen.findByText('No authors yet')).toBeInTheDocument()
    expect(screen.queryByText('Setup progress')).not.toBeInTheDocument()
  })
})
