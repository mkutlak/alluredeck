import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import * as searchApi from '@/api/search'
import { GlobalSearch } from '../GlobalSearch'

vi.mock('@/api/search')
vi.mock('@/api/client', () => ({
  setAccessToken: vi.fn(),
  getAccessToken: vi.fn(),
  apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

function renderSearch() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <MemoryRouter>
      <QueryClientProvider client={qc}>
        <GlobalSearch />
      </QueryClientProvider>
    </MemoryRouter>,
  )
}

describe('GlobalSearch', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders search button', () => {
    renderSearch()
    expect(screen.getByRole('button', { name: /search/i })).toBeInTheDocument()
  })

  it('opens dialog on button click', async () => {
    const user = userEvent.setup()
    renderSearch()

    await user.click(screen.getByRole('button', { name: /search/i }))
    expect(screen.getByPlaceholderText(/search projects/i)).toBeInTheDocument()
  })

  it('opens dialog on Cmd+K', async () => {
    const user = userEvent.setup()
    renderSearch()

    await user.keyboard('{Meta>}k{/Meta}')
    expect(screen.getByPlaceholderText(/search projects/i)).toBeInTheDocument()
  })

  it('shows empty state when no results', async () => {
    const user = userEvent.setup()
    vi.mocked(searchApi.search).mockResolvedValue({
      data: { projects: [], tests: [] },
      metadata: { message: 'Search results' },
    })
    renderSearch()

    await user.click(screen.getByRole('button', { name: /search/i }))
    await user.type(screen.getByPlaceholderText(/search projects/i), 'nonexistent')

    await waitFor(() => {
      expect(screen.getByText(/no results/i)).toBeInTheDocument()
    })
  })

  it('displays grouped results', async () => {
    const user = userEvent.setup()
    vi.mocked(searchApi.search).mockResolvedValue({
      data: {
        projects: [{ project_id: 'my-project', created_at: '2026-01-01T00:00:00Z' }],
        tests: [
          {
            project_id: 'my-project',
            test_name: 'LoginTest',
            full_name: 'com.auth.LoginTest',
            status: 'passed',
          },
        ],
      },
      metadata: { message: 'Search results' },
    })
    renderSearch()

    await user.click(screen.getByRole('button', { name: /search/i }))
    await user.type(screen.getByPlaceholderText(/search projects/i), 'login')

    await waitFor(() => {
      // "my-project" appears in both the project result and the test subtitle
      expect(screen.getAllByText('my-project')).toHaveLength(2)
      expect(screen.getByText('LoginTest')).toBeInTheDocument()
    })
  })

  it('shows loading state while fetching', async () => {
    const user = userEvent.setup()
    vi.mocked(searchApi.search).mockReturnValue(new Promise(() => {}))
    renderSearch()

    await user.click(screen.getByRole('button', { name: /search/i }))
    await user.type(screen.getByPlaceholderText(/search projects/i), 'login')

    await waitFor(() => {
      expect(screen.getByText(/searching/i)).toBeInTheDocument()
    })
  })
})
