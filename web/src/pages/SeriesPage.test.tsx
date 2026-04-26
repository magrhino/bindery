import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import SeriesPage from './SeriesPage'
import { api } from '../api/client'
import type { Series } from '../api/client'

vi.mock('../api/client', async importOriginal => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listSeries: vi.fn(),
      monitorSeries: vi.fn(),
      fillSeries: vi.fn(),
    },
  }
})

function renderSeriesPage(series: Series[]) {
  vi.mocked(api.listSeries).mockResolvedValue(series)
  return render(
    <MemoryRouter>
      <SeriesPage />
    </MemoryRouter>,
  )
}

describe('SeriesPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
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
})
