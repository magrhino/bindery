import { describe, expect, it } from 'vitest'
import { render, screen } from '@testing-library/react'
import MediaBadge from './MediaBadge'

describe('MediaBadge', () => {
  it('renders the ebook label', () => {
    render(<MediaBadge type="ebook" />)
    expect(screen.getByText('Ebook')).toBeInTheDocument()
  })

  it('renders the audiobook label', () => {
    render(<MediaBadge type="audiobook" />)
    expect(screen.getByText('Audiobook')).toBeInTheDocument()
  })

  // The badge was a binary `type === 'audiobook'` check, so a dual-format
  // book rendered as "📖 Ebook" — the badge actively denied half the book.
  it('renders both formats for a dual-format book', () => {
    render(<MediaBadge type="both" />)
    expect(screen.getByText('Ebook + Audiobook')).toBeInTheDocument()
  })

  it('falls back to ebook when the type is missing', () => {
    render(<MediaBadge />)
    expect(screen.getByText('Ebook')).toBeInTheDocument()
  })
})
