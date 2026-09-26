import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { renderWithProviders } from '@/test/render'
import { TestHistoryPage } from '../TestHistoryPage'
import * as testHistoryApi from '@/api/test-history'
import type { TestHistoryData, TestHistoryEntry } from '@/types/api'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/test-history')
mockApiClient()

function entry(
  build_order: number,
  status: string,
  ci_commit_sha: string | undefined,
  retries = 0,
): TestHistoryEntry {
  return {
    build_order,
    build_id: 100 + build_order,
    status,
    duration_ms: 1000,
    created_at: '2026-03-01T10:00:00Z',
    ci_commit_sha,
    flaky: retries > 0,
    retries,
  }
}

function historyData(history: TestHistoryEntry[]): TestHistoryData {
  return { history_id: 'abc123fullhashvalue', branch_name: 'main', history }
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
    renderPage('')
    expect(screen.getByText(/missing test history id/i)).toBeInTheDocument()
  })

  it('lists every build with its status, flaky retries and short commit', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockResolvedValue(
      historyData([
        entry(5, 'passed', 'deadbeef1234567'),
        entry(4, 'failed', 'cafebabe9876543', 2),
        entry(3, 'broken', undefined),
      ]),
    )
    renderPage()

    expect(await screen.findByText(/3 builds/i)).toBeInTheDocument()
    for (const text of ['#5', '#4', '#3', 'passed', 'failed', 'broken', 'deadbee', 'cafebab']) {
      expect(screen.getByText(text)).toBeInTheDocument()
    }
    // Only the flaky entry carries a flaky badge.
    expect(screen.getAllByTestId('flaky-badge')).toHaveLength(1)
    expect(screen.getByText('flaky · 2x')).toBeInTheDocument()
  })

  // The badge shows the URL branch the history is filtered by, never the response branch_name.
  it.each<[string, string, string[]]>([
    ['renders branch badge when branch param is in URL', '&branch=feature-x', ['feature-x']],
    ['does not render branch badge when branch param is absent', '', []],
  ])('%s', async (_name, search, present) => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockResolvedValue(
      historyData([entry(5, 'passed', 'deadbeef1234567')]),
    )
    renderPage(`?history_id=abc123fullhashvalue${search}`)
    await screen.findByText('#5')

    for (const text of present) expect(screen.getByText(text)).toBeInTheDocument()
    expect(screen.queryByText('main')).not.toBeInTheDocument()
  })

  it('shows empty state when history is empty', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockResolvedValue(historyData([]))
    renderPage()
    expect(await screen.findByText(/no history/i)).toBeInTheDocument()
  })

  it('shows error state when fetch fails', async () => {
    vi.mocked(testHistoryApi.fetchTestHistory).mockRejectedValue(new Error('Network error'))
    renderPage()
    expect(await screen.findByText(/failed to load/i)).toBeInTheDocument()
  })
})
