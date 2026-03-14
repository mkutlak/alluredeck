import React from 'react'
import { render, type RenderResult } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter, RouterProvider, createMemoryRouter } from 'react-router'

export function createTestQueryClient(): QueryClient {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } })
}

/**
 * Renders UI wrapped with the standard provider tree used across the app:
 * - QueryClientProvider (retry disabled, no stale cache between tests)
 * - MemoryRouter (React Router v7) or RouterProvider when a custom router is supplied
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
 *   // With a custom memory router (e.g. for createMemoryRouter):
 *   const router = createMemoryRouter([...], { initialEntries: [...] })
 *   renderWithProviders(<></>, { router })
 *
 * Each call creates a fresh QueryClient so tests are fully isolated.
 */
export function renderWithProviders(
  ui: React.ReactElement,
  opts?: { route?: string; router?: ReturnType<typeof createMemoryRouter> },
): RenderResult {
  const qc = createTestQueryClient()
  if (opts?.router) {
    return render(
      <QueryClientProvider client={qc}>
        <RouterProvider router={opts.router} />
      </QueryClientProvider>,
    )
  }
  return render(
    <QueryClientProvider client={qc}>
      <MemoryRouter initialEntries={[opts?.route ?? '/']}>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )
}
