import React from 'react'
import { render, type RenderResult } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'

/**
 * Renders UI wrapped with the standard provider tree used across the app:
 * - QueryClientProvider (retry disabled, no stale cache between tests)
 * - MemoryRouter (React Router v7)
 *
 * Usage:
 *
 *   import { renderWithProviders } from '@/test/render'
 *
 *   it('shows the list', () => {
 *     renderWithProviders(<MyPage />, { route: '/projects/foo' })
 *     expect(screen.getByRole('heading')).toHaveTextContent('Foo')
 *   })
 *
 * Each call creates a fresh QueryClient so tests are fully isolated.
 */
export function renderWithProviders(
  ui: React.ReactElement,
  opts?: { route?: string },
): RenderResult {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[opts?.route ?? '/']}>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )
}
