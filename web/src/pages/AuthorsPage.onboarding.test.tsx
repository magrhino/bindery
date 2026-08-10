import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import AuthorsPage from './AuthorsPage'
import { api } from '../api/client'
import type { Indexer, DownloadClient } from '../api/client'

vi.mock('../api/client', async importOriginal => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listAuthors: vi.fn(),
      listIndexers: vi.fn(),
      listDownloadClients: vi.fn(),
    },
  }
})

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: string) => {
      const labels: Record<string, string> = {
        'authors.empty': 'No authors yet',
        'authors.emptyHint': 'Click Add Author to start',
        'gettingStarted.title': 'Getting started',
        'gettingStarted.reasonAuthors': 'Configure an indexer and a download client before adding authors.',
        'gettingStarted.indexers': 'Set up Indexers',
        'gettingStarted.downloadClients': 'Set up Download Clients',
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

const fakeIndexer = { id: 1, name: 'NZBgeek' } as unknown as Indexer
const fakeClient = { id: 1, name: 'qBittorrent' } as unknown as DownloadClient

function renderPage() {
  return render(
    <MemoryRouter>
      <AuthorsPage />
    </MemoryRouter>,
  )
}

describe('AuthorsPage first-run onboarding guidance', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.listAuthors).mockResolvedValue({ items: [], total: 0, limit: 100, offset: 0 })
  })

  it('shows the getting-started guidance with links to settings when there are no authors and no indexers/clients', async () => {
    vi.mocked(api.listIndexers).mockResolvedValue([])
    vi.mocked(api.listDownloadClients).mockResolvedValue([])

    renderPage()

    const heading = await screen.findByText('Getting started')
    expect(heading).toBeInTheDocument()

    const indexersLink = screen.getByRole('link', { name: 'Set up Indexers' })
    expect(indexersLink).toHaveAttribute('href', '/settings?tab=indexers')
    const clientsLink = screen.getByRole('link', { name: 'Set up Download Clients' })
    expect(clientsLink).toHaveAttribute('href', '/settings?tab=clients')
  })

  // Half-configured states used to show NOTHING (the hook required both
  // lists empty), which is exactly the state where grabs fail silently.
  // Now the guidance names the one missing step and links only to it.
  it('shows only the download-client step when an indexer exists but no client', async () => {
    vi.mocked(api.listIndexers).mockResolvedValue([fakeIndexer])
    vi.mocked(api.listDownloadClients).mockResolvedValue([])

    renderPage()

    expect(await screen.findByText('Getting started')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Set up Download Clients' })).toHaveAttribute('href', '/settings?tab=clients')
    expect(screen.queryByRole('link', { name: 'Set up Indexers' })).not.toBeInTheDocument()
  })

  it('shows only the indexer step when a client exists but no indexer', async () => {
    vi.mocked(api.listIndexers).mockResolvedValue([])
    vi.mocked(api.listDownloadClients).mockResolvedValue([fakeClient])

    renderPage()

    expect(await screen.findByText('Getting started')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Set up Indexers' })).toHaveAttribute('href', '/settings?tab=indexers')
    expect(screen.queryByRole('link', { name: 'Set up Download Clients' })).not.toBeInTheDocument()
  })

  it('does NOT show the guidance when both an indexer and a client exist', async () => {
    vi.mocked(api.listIndexers).mockResolvedValue([fakeIndexer])
    vi.mocked(api.listDownloadClients).mockResolvedValue([fakeClient])

    renderPage()

    expect(await screen.findByText('No authors yet')).toBeInTheDocument()
    await waitFor(() => expect(api.listIndexers).toHaveBeenCalled())
    expect(screen.queryByText('Getting started')).not.toBeInTheDocument()
  })
})
