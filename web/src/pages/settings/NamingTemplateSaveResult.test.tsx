import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useState } from 'react'
import { describe, expect, it, vi } from 'vitest'
import NamingTemplateField from './NamingTemplateField'
import { useSaveResult } from './useSaveResult'

// Reproduces the reported case (#1668): saving the Book Naming Template gave no
// feedback whatsoever, while nearby fields showed "Saved ✓". The field renders
// a plain button until it is handed a saveResult.
function Harness({ save }: { save: () => Promise<unknown> }) {
  const [value, setValue] = useState('{Author}/{Title}.{ext}')
  const [result, run] = useSaveResult()
  return (
    <NamingTemplateField
      label="Book naming template"
      kind="book"
      placeholder="{Author}/{Title}.{ext}"
      value={value}
      onChange={setValue}
      onSave={() => run(save)}
      saving={false}
      saveResult={result}
    />
  )
}

describe('NamingTemplateField save confirmation', () => {
  it('confirms the save on the template field', async () => {
    const save = vi.fn().mockResolvedValue(undefined)
    render(<Harness save={save} />)

    const btn = screen.getByTestId('naming-save-book')
    expect(btn).toHaveTextContent(/^common\.save$/)

    fireEvent.click(btn)
    await waitFor(() => expect(save).toHaveBeenCalled())
    await waitFor(() => expect(screen.getByTestId('naming-save-book')).toHaveTextContent('Saved ✓'))
  })

  it('shows an error rather than a false confirmation when the save fails', async () => {
    render(<Harness save={() => Promise.reject(new Error('server said no'))} />)

    fireEvent.click(screen.getByTestId('naming-save-book'))
    await waitFor(() => expect(screen.getByTestId('naming-save-book')).toHaveTextContent('Error'))
  })

  // Save stays blocked on an invalid template, so an unsaved change can never
  // be mistaken for a saved one.
  it('does not save an invalid template', () => {
    const save = vi.fn()
    render(<Harness save={save} />)

    fireEvent.change(screen.getByPlaceholderText('{Author}/{Title}.{ext}'), {
      target: { value: '../{Title}.{ext}' },
    })
    const btn = screen.getByTestId('naming-save-book')
    expect(btn).toBeDisabled()
    fireEvent.click(btn)
    expect(save).not.toHaveBeenCalled()
  })
})
