import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'

// i18n: return the key (with crude interpolation) so assertions are stable.
vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: Record<string, unknown>) => {
      if (!options) return key
      let out = key
      for (const [k, v] of Object.entries(options)) {
        out += ` ${k}=${String(v)}`
      }
      return out
    },
  }),
}))

vi.mock('../../api/client', () => ({
  api: {
    goodreadsPreview: vi.fn(),
    goodreadsCommit: vi.fn(),
  },
}))

import { api, GoodreadsPreview, GoodreadsCommitResult } from '../../api/client'
import GoodreadsImportSection from './GoodreadsImportSection'

const mockPreview = api.goodreadsPreview as ReturnType<typeof vi.fn>
const mockCommit = api.goodreadsCommit as ReturnType<typeof vi.fn>

function preview(overrides: Partial<GoodreadsPreview> = {}): GoodreadsPreview {
  return {
    token: 'tok-123',
    totalRows: 3,
    resolved: 2,
    skippedShelf: 0,
    skippedExisting: 0,
    unresolved: 1,
    shelfFilter: ['to-read'],
    rows: [
      { row: { rowNumber: 1, title: 'Book A', author: 'Author A', exclusiveShelf: 'to-read' }, outcome: 'resolved', matchedBy: 'isbn13' },
      { row: { rowNumber: 2, title: 'Book B', author: 'Author B', exclusiveShelf: 'to-read' }, outcome: 'resolved', matchedBy: 'title+author' },
      { row: { rowNumber: 3, title: 'Book C', author: 'Author C', exclusiveShelf: 'to-read' }, outcome: 'unresolved', reason: 'no metadata match' },
    ],
    ...overrides,
  }
}

function uploadFile() {
  const input = document.querySelector('input[type="file"]') as HTMLInputElement
  const file = new File(['Title,Author\nBook A,Author A\n'], 'goodreads.csv', { type: 'text/csv' })
  fireEvent.change(input, { target: { files: [file] } })
}

describe('GoodreadsImportSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders the shelf filter with to-read checked by default', () => {
    render(<GoodreadsImportSection />)
    const checkboxes = screen.getAllByRole('checkbox') as HTMLInputElement[]
    expect(checkboxes).toHaveLength(3)
    // to-read is the first shelf and the only one ticked.
    expect(checkboxes[0].checked).toBe(true)
    expect(checkboxes[1].checked).toBe(false)
    expect(checkboxes[2].checked).toBe(false)
  })

  it('uploads a CSV and shows the dry-run preview summary', async () => {
    mockPreview.mockResolvedValue(preview())
    render(<GoodreadsImportSection />)
    uploadFile()

    await waitFor(() => {
      expect(screen.getByText('settings.import.goodreadsPreviewHeading')).toBeInTheDocument()
    })
    expect(mockPreview).toHaveBeenCalledTimes(1)
    // The commit button reflects the resolved count.
    expect(screen.getByText(/goodreadsCommit count=2/)).toBeInTheDocument()
  })

  it('sends the selected shelves with the upload', async () => {
    mockPreview.mockResolvedValue(preview())
    render(<GoodreadsImportSection />)
    // Tick the "read" shelf (third checkbox).
    const checkboxes = screen.getAllByRole('checkbox')
    fireEvent.click(checkboxes[2])
    uploadFile()

    await waitFor(() => expect(mockPreview).toHaveBeenCalledTimes(1))
    const fd = mockPreview.mock.calls[0][0] as FormData
    expect(fd.get('shelves')).toBe('to-read,read')
  })

  it('commits the preview token and shows the result', async () => {
    mockPreview.mockResolvedValue(preview())
    const result: GoodreadsCommitResult = { added: 2, skipped: 0, failed: 0 }
    mockCommit.mockResolvedValue(result)
    render(<GoodreadsImportSection />)
    uploadFile()

    await waitFor(() => screen.getByText(/goodreadsCommit count=2/))
    fireEvent.click(screen.getByText(/goodreadsCommit count=2/))

    await waitFor(() => {
      expect(screen.getByText(/goodreadsCommitResult/)).toBeInTheDocument()
    })
    expect(mockCommit).toHaveBeenCalledWith('tok-123')
  })

  it('disables import when no rows resolve', async () => {
    mockPreview.mockResolvedValue(preview({ resolved: 0, unresolved: 3, rows: [] }))
    render(<GoodreadsImportSection />)
    uploadFile()

    await waitFor(() => {
      expect(screen.getByText('settings.import.goodreadsNoResolved')).toBeInTheDocument()
    })
    expect(screen.queryByText(/goodreadsCommit count=/)).not.toBeInTheDocument()
  })

  it('surfaces an upload error', async () => {
    mockPreview.mockRejectedValue(new Error('bad csv'))
    render(<GoodreadsImportSection />)
    uploadFile()

    await waitFor(() => {
      expect(screen.getByText('bad csv')).toBeInTheDocument()
    })
  })
})
