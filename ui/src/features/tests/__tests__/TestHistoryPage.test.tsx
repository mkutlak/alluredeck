import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { TestHistoryPage } from '../TestHistoryPage'
import * as testHistoryApi from '@/api/test-history'
import type { TestHistoryData } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/test-history')
mockApiClient()

function makeHistoryData(overrides: Partial<TestHistoryData> = {}): TestHistoryData {
  return {
    history_id: 'abc123fullhashvalue',
    branch_name: 'main',
    history: [
      {
        build_order: 5,
        build_id: 105,
        status: 'passed',
        duration_ms: 3500,
        created_at: '2026-03-01T10:00:00Z',
        ci_commit_sha: 'deadbeef1234567',
      },
      {
        build_order: 4,
        build_id: 104,
        status: 'failed',
        duration_ms: 1200,
        created_at: '2026-02-28T09:00:00Z',
        ci_commit_sha: 'cafebabe9876543',
      },
      {
        build_order: 3,
        build_id: 103,
        status: 'broken',
        duration_ms: 900,
        created_at: '2026-02-27T08:00:00Z',
        ci_commit_sha: undefined,
      },
    ],
    ...overrides,
  }
}

function renderPage(search = '?history_id=abc123fullhashvalue') {
  const router = createMemoryRouter(
    [{ path: '/projects/:id/tests', element: <TestHistoryPage /> }],
    { initialEntries: [`/projects/my-project/tests${search}`] },
  )
  return renderWithProviders(<></>, { router })
}

describe('TestHistoryPage', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows error message when history_id param is missing', () => {
    renderPage('') // no query params
    expect(screen.getByText(/missing test history id/i)).toBeInTheDocument()
  })

  it('shows loading state while fetching', () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockReturnValue(new Promise(() => {}))
    renderPage()
    const skeletons = document.querySelectorAll('[data-testid="history-skeleton"]')
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it('renders page header with history id', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockResolvedValue(makeHistoryData())
    renderPage()

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: /test history/i })).toBeInTheDocument()
    })
    expect(screen.getByRole('heading', { name: /abc123fullhashvalue/i })).toBeInTheDocument()
  })

  it('renders build order in table rows', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockResolvedValue(makeHistoryData())
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('#5')).toBeInTheDocument()
    })
    expect(screen.getByText('#4')).toBeInTheDocument()
    expect(screen.getByText('#3')).toBeInTheDocument()
  })

  it('renders status badges for each entry', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockResolvedValue(makeHistoryData())
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('passed')).toBeInTheDocument()
    })
    expect(screen.getByText('failed')).toBeInTheDocument()
    expect(screen.getByText('broken')).toBeInTheDocument()
  })

  it('renders duration formatted as seconds', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockResolvedValue(makeHistoryData())
    renderPage()

    await waitFor(() => {
      // 3500ms -> 3s
      expect(screen.getByText('3s')).toBeInTheDocument()
    })
    // 1200ms -> 1s
    expect(screen.getByText('1s')).toBeInTheDocument()
    // 900ms -> 0s
    expect(screen.getByText('0s')).toBeInTheDocument()
  })

  it('renders commit SHA truncated to 7 chars', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockResolvedValue(makeHistoryData())
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('deadbee')).toBeInTheDocument()
    })
    expect(screen.getByText('cafebab')).toBeInTheDocument()
  })

  it('renders branch badge when branch param is in URL', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockResolvedValue(makeHistoryData())
    renderPage('?history_id=abc123fullhashvalue&branch=feature-x')

    await waitFor(() => {
      expect(screen.getByText('feature-x')).toBeInTheDocument()
    })
  })

  it('does not render branch badge when branch param is absent', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockResolvedValue(makeHistoryData())
    renderPage()

    await waitFor(() => {
      expect(screen.getByText('#5')).toBeInTheDocument()
    })
    // branch_name from response data should NOT appear as a badge
    expect(screen.queryByText('main')).not.toBeInTheDocument()
  })

  it('shows trend summary with build count', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockResolvedValue(makeHistoryData())
    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/3 builds/i)).toBeInTheDocument()
    })
  })

  it('shows empty state when history is empty', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockResolvedValue(makeHistoryData({ history: [] }))
    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/no history/i)).toBeInTheDocument()
    })
  })

  it('shows error state when fetch fails', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockRejectedValue(new Error('Network error'))
    renderPage()

    await waitFor(() => {
      expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
    })
  })
})
