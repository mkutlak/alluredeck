import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import type { MultiTimelineData } from '@/types/api'

// Mock the API module before importing the component
vi.mock('@/api/reports', () => ({
  fetchProjectTimeline: vi.fn(),
}))

// Mock BranchSelector
vi.mock('@/features/projects/BranchSelector', () => ({
  BranchSelector: () => <div data-testid="mock-branch-selector" />,
}))

// Mock DateRangePicker
vi.mock('../DateRangePicker', () => ({
  DateRangePicker: () => <div data-testid="mock-date-range-picker" />,
}))

// Mock BuildCountSelector
vi.mock('../BuildCountSelector', () => ({
  BuildCountSelector: () => <div data-testid="mock-build-count-selector" />,
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

import { fetchProjectTimeline } from '@/api/reports'
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

const mockMultiTimeline: MultiTimelineData = {
  builds: [
    {
      build_order: 1,
      created_at: '2026-03-25T12:00:00Z',
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
    },
  ],
  total_builds_in_range: 1,
  builds_returned: 1,
  global_min_start: 1700000000000,
  global_max_stop: 1700000005000,
}

describe('TimelineTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows loading skeletons while fetching', () => {
    vi.mocked(fetchProjectTimeline).mockReturnValue(new Promise(() => {}))
    renderTab()
    // Skeleton elements use animate-pulse
    const skeletons = document.querySelectorAll('[class*="animate-pulse"]')
    expect(skeletons.length).toBeGreaterThan(0)
  })

  it('renders chart when data is available', async () => {
    vi.mocked(fetchProjectTimeline).mockResolvedValue(mockMultiTimeline)
    renderTab()
    expect(await screen.findByTestId('timeline-chart')).toBeInTheDocument()
    expect(screen.getByText('2 items')).toBeInTheDocument()
  })

  it('shows empty state when no test cases', async () => {
    vi.mocked(fetchProjectTimeline).mockResolvedValue({
      builds: [],
      total_builds_in_range: 0,
      builds_returned: 0,
      global_min_start: 0,
      global_max_stop: 0,
    })
    renderTab()
    expect(await screen.findByText(/no timeline data/i)).toBeInTheDocument()
  })

  it('shows truncation warning when summary.truncated is true', async () => {
    vi.mocked(fetchProjectTimeline).mockResolvedValue({
      ...mockMultiTimeline,
      builds: [
        {
          ...mockMultiTimeline.builds[0],
          summary: { ...mockMultiTimeline.builds[0].summary, total: 10000, truncated: true },
        },
      ],
    })
    renderTab()
    expect(await screen.findByText(/truncated/i)).toBeInTheDocument()
  })

  it('displays project id and test count in header', async () => {
    vi.mocked(fetchProjectTimeline).mockResolvedValue(mockMultiTimeline)
    renderTab('my-project')
    expect(await screen.findByText('my-project')).toBeInTheDocument()
    expect(screen.getByText(/2 tests/i)).toBeInTheDocument()
  })
})
