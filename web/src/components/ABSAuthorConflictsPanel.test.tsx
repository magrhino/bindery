import { describe, it, expect, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import ABSConflictPanel from './ABSAuthorConflictsPanel'
import type { ABSMetadataConflict } from '../api/client'

function makeConflict(overrides: Partial<ABSMetadataConflict> = {}): ABSMetadataConflict {
  return {
    id: 1,
    sourceId: 'default',
    libraryId: 'lib-books',
    itemId: 'li-hobbit',
    entityType: 'author',
    localId: 42,
    entityName: 'J. R. R. Tolkien',
    fieldName: 'description',
    fieldLabel: 'Description',
    absValue: '',
    upstreamValue: 'Author of The Hobbit.',
    appliedSource: 'upstream',
    appliedValue: 'Author of The Hobbit.',
    preferredSource: '',
    authorRelinkEligible: true,
    resolutionStatus: 'unresolved',
    updatedAt: '2026-04-23T00:00:00Z',
    ...overrides,
  }
}

describe('ABSConflictPanel', () => {
  it('renders one relink action per eligible author group and forwards clicks', () => {
    const onRelinkAuthor = vi.fn()
    const onResolveConflict = vi.fn()
    const conflicts = [
      makeConflict(),
      makeConflict({ id: 2, fieldName: 'imageUrl', fieldLabel: 'Image', localId: 42 }),
      makeConflict({ id: 3, localId: 99, entityName: 'Frank Herbert', authorRelinkEligible: false }),
    ]

    render(
      <ABSConflictPanel
        title="Author conflicts"
        description="Review placeholder ABS authors first."
        entityType="author"
        conflicts={conflicts}
        show
        emptyMessage="No author conflicts recorded yet."
        resolvedHeading="Resolved author choices"
        conflictError={null}
        resolvingConflictId={null}
        onToggle={() => {}}
        onRefresh={() => {}}
        onResolveConflict={onResolveConflict}
        relinkAction={{ loadingId: null, onRelink: onRelinkAuthor }}
      />,
    )

    const relinkButtons = screen.getAllByRole('button', { name: 'Attempt upstream relink' })
    expect(relinkButtons).toHaveLength(1)

    fireEvent.click(relinkButtons[0])
    expect(onRelinkAuthor).toHaveBeenCalledWith(42)

    fireEvent.click(screen.getAllByRole('button', { name: 'Use ABS' })[0])
    expect(onResolveConflict).toHaveBeenCalledWith(1, 'abs')
  })

  it('shows relink loading state and backend errors', () => {
    render(
      <ABSConflictPanel
        title="Author conflicts"
        description="Review placeholder ABS authors first."
        entityType="author"
        conflicts={[
          makeConflict({ resolutionStatus: 'resolved', preferredSource: 'upstream' }),
        ]}
        show
        emptyMessage="No author conflicts recorded yet."
        resolvedHeading="Resolved author choices"
        conflictError="upstream author already exists locally"
        resolvingConflictId={null}
        onToggle={() => {}}
        onRefresh={() => {}}
        onResolveConflict={() => {}}
        relinkAction={{ loadingId: 42, onRelink: () => {} }}
      />,
    )

    expect(screen.getByRole('button', { name: 'Relinking…' })).toBeDisabled()
    expect(screen.getByText('Resolved author choices')).toBeInTheDocument()
    expect(screen.getByText('upstream author already exists locally')).toBeInTheDocument()
  })
})
