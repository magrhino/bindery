import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import SettingsPage from './SettingsPage'
import { api } from '../api/client'
import type { ABSReviewItem, Author, Book } from '../api/client'

vi.mock('../settings/AuthSettings', () => ({ default: () => <div data-testid="auth-settings" /> }))
vi.mock('../components/ThemeToggle', () => ({ default: () => <button type="button">Theme</button> }))
vi.mock('../components/LanguageSwitcher', () => ({ default: () => <select aria-label="Language" /> }))
vi.mock('../auth/AuthContext', () => ({
  useAuth: () => ({ isAdmin: true }),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
    i18n: { changeLanguage: vi.fn() },
  }),
}))
vi.mock('../api/client', async importOriginal => {
  const actual = await importOriginal<typeof import('../api/client')>()
  return {
    ...actual,
    api: {
      ...actual.api,
      listIndexers: vi.fn(),
      listDownloadClients: vi.fn(),
      listProwlarr: vi.fn(),
      absConfig: vi.fn(),
      absSetConfig: vi.fn(),
      absLibraries: vi.fn(),
      absImportStart: vi.fn(),
      absImportStatus: vi.fn(),
      absImportRuns: vi.fn(),
      absReviewItems: vi.fn(),
      absConflicts: vi.fn(),
      resolveAbsReviewAuthor: vi.fn(),
      resolveAbsReviewBook: vi.fn(),
      searchAuthors: vi.fn(),
      searchBooks: vi.fn(),
      listSettings: vi.fn(),
      listBackups: vi.fn(),
      libraryScanStatus: vi.fn(),
      getStorage: vi.fn(),
      listRootFolders: vi.fn(),
      status: vi.fn(),
      setSetting: vi.fn(),
      testHardcover: vi.fn(),
      authConfig: vi.fn(),
    },
  }
})

const makeAuthor = (index: number, overrides: Partial<Author> = {}): Author => ({
  id: index,
  foreignAuthorId: `author-${index}`,
  authorName: `Author ${index}`,
  sortName: `Author ${index}`,
  description: '',
  imageUrl: '',
  disambiguation: '',
  ratingsCount: 0,
  averageRating: 0,
  monitored: true,
  ...overrides,
})

const makeBook = (index: number, overrides: Partial<Book> = {}): Book => ({
  id: index,
  foreignBookId: `book-${index}`,
  authorId: 100 + index,
  title: `Book ${index}`,
  description: '',
  imageUrl: '',
  genres: [],
  monitored: true,
  status: 'wanted',
  filePath: '',
  mediaType: 'audiobook',
  ebookFilePath: '',
  audiobookFilePath: '',
  excluded: false,
  author: makeAuthor(100 + index, { authorName: `Book Author ${index}` }),
  ...overrides,
})

const makeReviewItem = (overrides: Partial<ABSReviewItem> = {}): ABSReviewItem => ({
  id: 1,
  sourceId: 'default',
  libraryId: 'lib-books',
  itemId: 'item-1',
  title: 'All Systems Red',
  primaryAuthor: 'Martha Wells',
  asin: '',
  mediaType: 'audiobook',
  reviewReason: 'unmatched_author',
  payloadJson: '{}',
  fileMappingFound: false,
  latestRunId: null,
  status: 'pending',
  createdAt: '2026-01-01T00:00:00Z',
  updatedAt: '2026-01-01T00:00:00Z',
  ...overrides,
})

describe('SettingsPage ABS review search', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(api.listIndexers).mockResolvedValue([])
    vi.mocked(api.listDownloadClients).mockResolvedValue([])
    vi.mocked(api.listProwlarr).mockResolvedValue([])
    vi.mocked(api.absConfig).mockResolvedValue({
      featureEnabled: true,
      baseUrl: 'https://abs.example.com',
      label: 'Shelf',
      enabled: true,
      libraryId: 'lib-books',
      pathRemap: '',
      apiKeyConfigured: true,
    })
    vi.mocked(api.absSetConfig).mockResolvedValue({
      featureEnabled: true,
      baseUrl: 'https://abs.example.com',
      label: 'Shelf',
      enabled: true,
      libraryId: 'lib-books',
      pathRemap: '',
      apiKeyConfigured: true,
    })
    vi.mocked(api.absLibraries).mockResolvedValue([])
    vi.mocked(api.absImportStart).mockResolvedValue({ running: true, dryRun: true, processed: 0 })
    vi.mocked(api.absImportStatus).mockResolvedValue({ running: false, processed: 0 })
    vi.mocked(api.absImportRuns).mockResolvedValue([])
    vi.mocked(api.absConflicts).mockResolvedValue({ items: [], total: 0, limit: 50, offset: 0 })
    vi.mocked(api.resolveAbsReviewAuthor).mockResolvedValue({ updated: 1 })
    vi.mocked(api.searchAuthors).mockResolvedValue([])
    vi.mocked(api.searchBooks).mockResolvedValue([])
    vi.mocked(api.listSettings).mockResolvedValue([{ key: 'hardcover.enhanced_series_enabled', value: 'false' }])
    vi.mocked(api.listBackups).mockResolvedValue([])
    vi.mocked(api.libraryScanStatus).mockRejectedValue(new Error('no scan'))
    vi.mocked(api.getStorage).mockResolvedValue({ downloadDir: '/downloads', audiobookDownloadDir: '', libraryDir: '/books', audiobookDir: '' })
    vi.mocked(api.listRootFolders).mockResolvedValue([])
    vi.mocked(api.status).mockResolvedValue({
      version: 'dev',
      commit: 'unknown',
      buildDate: '',
      enhancedHardcoverApi: false,
      hardcoverTokenConfigured: false,
    })
    vi.mocked(api.setSetting).mockResolvedValue(undefined)
    vi.mocked(api.testHardcover).mockResolvedValue({
      ok: true,
      tokenConfigured: true,
      searchResults: 0,
      catalogOk: true,
      message: 'ok',
    })
    vi.mocked(api.authConfig).mockResolvedValue({ mode: 'disabled', apiKey: 'key', username: 'admin' })
  })

  const renderABSReview = async (items: ABSReviewItem[]) => {
    vi.mocked(api.absReviewItems).mockResolvedValue({
      items,
      total: items.length,
      limit: 50,
      offset: 0,
    })

    render(<SettingsPage />)

    fireEvent.click(await screen.findByRole('button', { name: 'settings.tabs.abs' }))
    await screen.findByText('No-match books')
  }

  it('shows capped scrollable author and book results', async () => {
    const item = makeReviewItem()
    vi.mocked(api.searchAuthors).mockResolvedValue(Array.from({ length: 12 }, (_, index) => makeAuthor(index + 1)))
    vi.mocked(api.searchBooks).mockResolvedValue(Array.from({ length: 12 }, (_, index) => makeBook(index + 1)))

    await renderABSReview([item])

    fireEvent.click(screen.getByRole('button', { name: 'Author' }))

    const firstAuthor = await screen.findByRole('button', { name: 'Author 1' })
    expect(screen.getByRole('button', { name: 'Author 4' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Author 10' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Author 11' })).not.toBeInTheDocument()
    expect(firstAuthor.parentElement).toHaveClass('max-h-48', 'overflow-y-auto')

    fireEvent.click(screen.getByRole('button', { name: 'Book' }))

    const firstBook = (await screen.findByText('Book 1')).closest('button')
    expect(screen.getByText('Book 4')).toBeInTheDocument()
    expect(screen.getByText('Book 10')).toBeInTheDocument()
    expect(screen.queryByText('Book 11')).not.toBeInTheDocument()
    expect(firstBook?.parentElement).toHaveClass('max-h-48', 'overflow-y-auto')
  })

  it('auto-links the selected book author before resolving the book', async () => {
    const item = makeReviewItem()
    const author = makeAuthor(42, {
      foreignAuthorId: 'OL42A',
      authorName: 'Martha Wells',
    })
    const book = makeBook(1, {
      foreignBookId: 'OL1W',
      title: 'All Systems Red',
      author,
    })
    vi.mocked(api.searchBooks).mockResolvedValue([book])
    vi.mocked(api.resolveAbsReviewBook).mockResolvedValue({
      ...item,
      resolvedAuthorForeignId: 'OL42A',
      resolvedAuthorName: 'Martha Wells',
      resolvedBookForeignId: 'OL1W',
      resolvedBookTitle: 'All Systems Red',
    })

    await renderABSReview([item])

    fireEvent.click(screen.getByRole('button', { name: 'Book' }))
    fireEvent.click(await screen.findByRole('button', { name: /All Systems Red/ }))

    await waitFor(() => {
      expect(api.resolveAbsReviewAuthor).toHaveBeenCalledWith(1, {
        foreignAuthorId: 'OL42A',
        authorName: 'Martha Wells',
        applyTo: 'same_author',
      })
      expect(api.resolveAbsReviewBook).toHaveBeenCalledWith(1, {
        foreignBookId: 'OL1W',
        title: 'All Systems Red',
        editedTitle: 'All Systems Red',
      })
    })
    expect(vi.mocked(api.resolveAbsReviewAuthor).mock.invocationCallOrder[0]).toBeLessThan(
      vi.mocked(api.resolveAbsReviewBook).mock.invocationCallOrder[0],
    )
  })

  it('does not auto-link the selected book author when one is already resolved', async () => {
    const item = makeReviewItem({
      resolvedAuthorForeignId: 'OL-existing',
      resolvedAuthorName: 'Existing Author',
    })
    const book = makeBook(1, {
      foreignBookId: 'OL1W',
      title: 'All Systems Red',
      author: makeAuthor(42, {
        foreignAuthorId: 'OL42A',
        authorName: 'Martha Wells',
      }),
    })
    vi.mocked(api.searchBooks).mockResolvedValue([book])
    vi.mocked(api.resolveAbsReviewBook).mockResolvedValue({
      ...item,
      resolvedBookForeignId: 'OL1W',
      resolvedBookTitle: 'All Systems Red',
    })

    await renderABSReview([item])

    fireEvent.click(screen.getByRole('button', { name: 'Book' }))
    fireEvent.click(await screen.findByRole('button', { name: /All Systems Red/ }))

    await waitFor(() => {
      expect(api.resolveAbsReviewBook).toHaveBeenCalled()
    })
    expect(api.resolveAbsReviewAuthor).not.toHaveBeenCalled()
  })
})
