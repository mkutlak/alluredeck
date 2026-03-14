import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import type { TimelineData } from '@/types/api'

// Mock the API module before importing the component
vi.mock('@/api/reports', () => ({
  fetchReportTimeline: vi.fn(),
}))

// Mock TimelineChart for unit testing
vi.mock('../TimelineChart', () => ({
  TimelineChart: ({
    testCases,
  }: {
    testCases: { name: string }[]
    minStart: number
    maxStop: number
  }) => <div data-testid="timeline-chart">{testCases.length} items</div>,
}))

import { fetchReportTimeline } from '@/api/reports'
import { TimelineTab } from '../TimelineTab'

function makeClient() {
  return createTestQueryClient()
}

function renderTab(projectId = 'proj1') {
  return render(
    <QueryClientProvider client={makeClient()}>
      <MemoryRouter initialEntries={[`/projects/${projectId}/timeline`]}>
        <Routes>
          <Route path="projects/:id/timeline" element={<TimelineTab />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

const mockTimeline: TimelineData = {
  test_cases: [
    {
      name: 'Login test',
      full_name: 'com.example.LoginTest#test',
      status: 'passed',
      start: 1700000000000,
      stop: 1700000005000,
      duration: 5000,
      thread: 'worker-1',
      host: 'node-1',
    },
    {
      name: 'Logout test',
      full_name: 'com.example.LogoutTest#test',
      status: 'failed',
      start: 1700000001000,
      stop: 1700000003000,
      duration: 2000,
      thread: 'worker-2',
      host: 'node-1',
    },
  ],
  summary: {
    total: 2,
    min_start: 1700000000000,
    max_stop: 1700000005000,
    total_duration: 7000,
    truncated: false,
  },
}

describe('TimelineTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading skeletons while fetching', () => {
    vi.mocked(fetchReportTimeline).mockReturnValue(new Promise(() => {}))
    renderTab()
    // Skeleton elements use animate-pulse
    const skeletons = document.querySelectorAll('[class*="animate-pulse"]')
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it('renders chart when data is available', async () => {
    vi.mocked(fetchReportTimeline).mockResolvedValue(mockTimeline)
    renderTab()
    expect(await screen.findByTestId('timeline-chart')).toBeInTheDocument()
    expect(screen.getByText('2 items')).toBeInTheDocument()
  })

  it('shows empty state when no test cases', async () => {
    vi.mocked(fetchReportTimeline).mockResolvedValue({
      test_cases: [],
      summary: { total: 0, min_start: 0, max_stop: 0, total_duration: 0, truncated: false },
    })
    renderTab()
    expect(await screen.findByText(/no timeline data/i)).toBeInTheDocument()
  })

  it('shows truncation warning when summary.truncated is true', async () => {
    vi.mocked(fetchReportTimeline).mockResolvedValue({
      ...mockTimeline,
      summary: { ...mockTimeline.summary, total: 10000, truncated: true },
    })
    renderTab()
    expect(await screen.findByText(/truncated/i)).toBeInTheDocument()
  })

  it('displays project id and test count in header', async () => {
    vi.mocked(fetchReportTimeline).mockResolvedValue(mockTimeline)
    renderTab('my-project')
    expect(await screen.findByText('my-project')).toBeInTheDocument()
    expect(screen.getByText(/2 tests/i)).toBeInTheDocument()
  })
})
