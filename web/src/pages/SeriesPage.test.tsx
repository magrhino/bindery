import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import SeriesPage from './SeriesPage'
import { api } from '../api/client'
import type { Series, SystemStatus } from '../api/client'

vi.mock('../api/client', async importOriginal => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      status: vi.fn(),
      listSeries: vi.fn(),
      monitorSeries: vi.fn(),
      fillSeries: vi.fn(),
      autoLinkSeriesHardcover: vi.fn(),
      getSeriesHardcoverLink: vi.fn(),
      searchHardcoverSeries: vi.fn(),
      linkSeriesHardcover: vi.fn(),
      unlinkSeriesHardcover: vi.fn(),
      getSeriesHardcoverDiff: vi.fn(),
    },
  }
})

function renderSeriesPage(series: Series[], status: SystemStatus = { version: 'dev', commit: 'unknown', buildDate: '', enhancedHardcoverApi: true, hardcoverTokenConfigured: true }) {
  vi.mocked(api.listSeries).mockResolvedValue(series)
  vi.mocked(api.status).mockResolvedValue(status)
  return render(
    <MemoryRouter>
      <SeriesPage />
    </MemoryRouter>,
  )
}

describe('SeriesPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.status).mockResolvedValue({ version: 'dev', commit: 'unknown', buildDate: '', enhancedHardcoverApi: true, hardcoverTokenConfigured: true })
    vi.mocked(api.getSeriesHardcoverLink).mockRejectedValue(new Error('not linked'))
    vi.mocked(api.searchHardcoverSeries).mockResolvedValue([])
  })

  it('hides Hardcover controls when enhanced Hardcover API is disabled', async () => {
    renderSeriesPage([
      {
        id: 11,
        foreignSeriesId: 'series-11',
        title: 'The Stormlight Archive',
        description: '',
        monitored: true,
        books: [],
        hardcoverLink: {
          id: 1,
          seriesId: 11,
          hardcoverSeriesId: 'hc-series:42',
          hardcoverProviderId: '42',
          hardcoverTitle: 'The Stormlight Archive',
          hardcoverAuthorName: 'Brandon Sanderson',
          hardcoverBookCount: 10,
          confidence: 1,
          linkedBy: 'manual',
          linkedAt: '2026-01-01T00:00:00Z',
          createdAt: '2026-01-01T00:00:00Z',
          updatedAt: '2026-01-01T00:00:00Z',
        },
      },
    ], { version: 'dev', commit: 'unknown', buildDate: '', enhancedHardcoverApi: false, hardcoverTokenConfigured: true, enhancedHardcoverDisabledReason: 'env_disabled' })

    expect(await screen.findByRole('heading', { name: 'The Stormlight Archive' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /link/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Auto' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('heading', { name: 'The Stormlight Archive' }))
    expect(screen.queryByText(/Hardcover:/)).not.toBeInTheDocument()
    expect(api.getSeriesHardcoverDiff).not.toHaveBeenCalled()
  })

  it('links expanded series book rows to their book pages', async () => {
    renderSeriesPage([
      {
        id: 7,
        foreignSeriesId: 'series-7',
        title: 'Defiance of the Fall',
        description: '',
        monitored: true,
        books: [
          {
            seriesId: 7,
            bookId: 102,
            positionInSeries: '2',
            book: {
              id: 102,
              foreignBookId: 'book-102',
              authorId: 12,
              title: 'Defiance of the Fall 2',
              description: '',
              imageUrl: '',
              releaseDate: '2020-01-01',
              genres: [],
              monitored: true,
              status: 'imported',
              filePath: '',
              mediaType: 'ebook',
              ebookFilePath: '',
              audiobookFilePath: '',
              excluded: false,
            },
          },
        ],
      },
    ])

    fireEvent.click(await screen.findByRole('heading', { name: 'Defiance of the Fall' }))

    const bookLink = screen.getByRole('link', { name: /Defiance of the Fall 2/ })
    expect(bookLink).toHaveAttribute('href', '/book/102')
  })

  it('opens the Hardcover series link modal from the Auto control', async () => {
    vi.mocked(api.autoLinkSeriesHardcover).mockResolvedValue({
      linked: false,
      reason: 'low confidence',
      candidates: [
        {
          foreignId: 'hc-series:42',
          providerId: '42',
          title: 'The Stormlight Archive',
          authorName: 'Brandon Sanderson',
          bookCount: 10,
          readersCount: 19323,
          books: null as unknown as string[],
          confidence: 0.7,
        },
      ],
    })

    renderSeriesPage([
      {
        id: 9,
        foreignSeriesId: 'series-9',
        title: 'Rhythm of War',
        description: '',
        monitored: true,
        books: [],
      },
    ])

    fireEvent.click(await screen.findByRole('button', { name: 'Auto' }))

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('The Stormlight Archive')).toBeInTheDocument()
    expect(screen.getByText('70% match')).toBeInTheDocument()
  })

  it('opens linked Hardcover series without auto-linking again', async () => {
    renderSeriesPage([
      {
        id: 10,
        foreignSeriesId: 'series-10',
        title: 'The Stormlight Archive',
        description: '',
        monitored: true,
        books: [],
        hardcoverLink: {
          id: 1,
          seriesId: 10,
          hardcoverSeriesId: 'hc-series:42',
          hardcoverProviderId: '42',
          hardcoverTitle: 'The Stormlight Archive',
          hardcoverAuthorName: 'Brandon Sanderson',
          hardcoverBookCount: 10,
          confidence: 1,
          linkedBy: 'manual',
          linkedAt: '2026-01-01T00:00:00Z',
          createdAt: '2026-01-01T00:00:00Z',
          updatedAt: '2026-01-01T00:00:00Z',
        },
      },
    ])

    fireEvent.click(await screen.findByRole('button', { name: 'Manual link' }))

    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    expect(screen.getByText('Currently linked')).toBeInTheDocument()
    expect(screen.getByText('Brandon Sanderson')).toBeInTheDocument()
    expect(api.autoLinkSeriesHardcover).not.toHaveBeenCalled()
  })
})
