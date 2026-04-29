import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import AuthorDetailPage from './AuthorDetailPage'
import { api, Author, Book } from '../api/client'

vi.mock('../api/client', async importOriginal => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      getAuthor: vi.fn(),
      listBooks: vi.fn(),
      searchAuthorWanted: vi.fn(),
    },
  }
})

vi.mock('../components/ViewToggle', () => ({ default: () => null }))

vi.mock('../components/useView', () => ({
  useView: () => ['grid', vi.fn()],
}))

const author: Author = {
  id: 42,
  foreignAuthorId: 'OL1A',
  authorName: 'Test Author',
  sortName: 'Author, Test',
  description: '',
  imageUrl: '',
  disambiguation: '',
  ratingsCount: 0,
  averageRating: 0,
  monitored: true,
}

function book(overrides: Partial<Book>): Book {
  return {
    id: 1,
    foreignBookId: 'OL1W',
    authorId: 42,
    title: 'Wanted Book',
    description: '',
    imageUrl: '',
    genres: [],
    monitored: true,
    status: 'wanted',
    filePath: '',
    mediaType: 'ebook',
    ebookFilePath: '',
    audiobookFilePath: '',
    excluded: false,
    ...overrides,
  }
}

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/author/42']}>
      <Routes>
        <Route path="/author/:id" element={<AuthorDetailPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('AuthorDetailPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.getAuthor).mockResolvedValue(author)
    vi.mocked(api.searchAuthorWanted).mockResolvedValue({
      results: { '42': { ok: true } },
    })
  })

  it('searches all wanted books for the current author', async () => {
    vi.mocked(api.listBooks).mockResolvedValue([
      book({ id: 10, title: 'Wanted Book' }),
      book({ id: 11, title: 'Imported Book', status: 'imported' }),
    ])

    renderPage()

    const button = await screen.findByRole('button', { name: 'Search all wanted' })
    expect(button).toBeEnabled()

    fireEvent.click(button)

    await waitFor(() => expect(api.searchAuthorWanted).toHaveBeenCalledWith(42))
  })

  it('disables author search when there are no monitored wanted books', async () => {
    vi.mocked(api.listBooks).mockResolvedValue([
      book({ id: 10, title: 'Unmonitored Wanted Book', monitored: false }),
      book({ id: 11, title: 'Imported Book', status: 'imported' }),
    ])

    renderPage()

    const button = await screen.findByRole('button', { name: 'Search all wanted' })
    expect(button).toBeDisabled()

    fireEvent.click(button)

    expect(api.searchAuthorWanted).not.toHaveBeenCalled()
  })
})
