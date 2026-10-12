import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithProviders } from '@/test/render'
import * as searchApi from '@/api/search'
import * as projectsApi from '@/api/projects'
import { SearchCommand } from '../SearchCommand'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/search')
vi.mock('@/api/projects')
mockApiClient()

describe('SearchCommand secondary lines', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(projectsApi.getProjectIndex).mockResolvedValue({
      data: [],
      metadata: { message: 'ok' },
    })
  })

  // muted-foreground is 1.76:1 on the highlighted item by day; text-fact is the readable ink,
  // and the highlighted item is drawn on the canvas so the line clears 4.5:1 there too.
  it("sets a run's branch and group line in the readable secondary ink", async () => {
    const user = userEvent.setup()
    vi.mocked(searchApi.search).mockResolvedValue({
      data: {
        projects: [],
        tests: [],
        runs: [
          {
            group_project_id: 10,
            group_slug: 'checkout',
            pipeline_id: '48213',
            branch: 'feature/login-retry',
            timestamp: '2026-05-01T10:00:00Z',
          },
        ],
      },
      metadata: { message: 'Search results' },
    })
    renderWithProviders(<SearchCommand />)
    await user.keyboard('{Meta>}k{/Meta}')
    await user.type(screen.getByPlaceholderText(/search projects/i), 'query')

    const option = await screen.findByRole('option', { name: /48213/ })
    const line = screen.getByText(/feature\/login-retry/)
    expect(line).toHaveClass('text-fact')
    expect(line).not.toHaveClass('text-muted-foreground')
    expect(option).toHaveClass('data-[selected=true]:bg-background')
    expect(option).not.toHaveClass('data-[selected=true]:bg-accent')
  })
})
