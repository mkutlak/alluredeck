import { renderHook } from '@testing-library/react'
import { useDocumentTitle } from '@/lib/useDocumentTitle'

beforeEach(() => {
  document.title = 'AllureDeck'
})

describe('useDocumentTitle', () => {
  it('sets the title while mounted and restores the previous one on unmount', () => {
    const { unmount } = renderHook(() => useDocumentTitle('Runs — AllureDeck'))
    expect(document.title).toBe('Runs — AllureDeck')
    unmount()
    expect(document.title).toBe('AllureDeck')
  })

  // A page whose data is still loading has no title to give yet.
  it('leaves the title alone while undefined, then follows each change', () => {
    const { rerender, unmount } = renderHook(({ title }) => useDocumentTitle(title), {
      initialProps: { title: undefined as string | undefined },
    })
    expect(document.title).toBe('AllureDeck')

    rerender({ title: 'Overview · acme — AllureDeck' })
    expect(document.title).toBe('Overview · acme — AllureDeck')
    rerender({ title: 'Analytics · acme — AllureDeck' })
    expect(document.title).toBe('Analytics · acme — AllureDeck')

    unmount()
    expect(document.title).toBe('AllureDeck')
  })
})
