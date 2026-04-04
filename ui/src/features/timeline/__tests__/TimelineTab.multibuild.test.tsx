import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import type { MultiTimelineData } from '@/types/api'

// Mock the API module
vi.mock('@/api/reports', () => ({
  fetchReportTimeline: vi.fn(),
  fetchProjectTimeline: vi.fn(),
}))

// Mock BranchSelector
vi.mock('@/features/projects/BranchSelector', () => ({
  BranchSelector: ({
    onBranchChange,
  }: {
    projectId: string
    selectedBranch: string | undefined
    onBranchChange: (b: string | undefined) => void
  }) => (
    <select
      data-testid="mock-branch-selector"
      onChange={(e) => onBranchChange(e.target.value || undefined)}
    >
      <option value="">All branches</option>
      <option value="main">main</option>
    </select>
  ),
}))

// Mock TimelineChart for unit testing
vi.mock('../TimelineChart', () => ({
  TimelineChart: (props: Record<string, unknown>) => (
    <div data-testid="timeline-chart" data-props={JSON.stringify(props)} />
  ),
}))

// Mock DateRangePicker
vi.mock('../DateRangePicker', () => ({
  DateRangePicker: ({
    from,
    to,
    onRangeChange,
  }: {
    from: string | undefined
    to: string | undefined
    onRangeChange: (from: string | undefined, to: string | undefined) => void
  }) => (
    <div data-testid="mock-date-range-picker" data-from={from ?? ''} data-to={to ?? ''}>
      <button onClick={() => onRangeChange('2026-01-01', '2026-03-01')}>Set range</button>
    </div>
  ),
}))

// Mock BuildCountSelector
vi.mock('../BuildCountSelector', () => ({
  BuildCountSelector: ({ value, onChange }: { value: number; onChange: (v: number) => void }) => (
    <select
      data-testid="mock-build-count-selector"
      value={value}
      onChange={(e) => onChange(Number(e.target.value))}
    >
      <option value="1">1</option>
      <option value="5">5</option>
    </select>
  ),
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
      build_order: 42,
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
      ],
      summary: {
        total: 1,
        min_start: 1700000000000,
        max_stop: 1700000005000,
        total_duration: 5000,
        truncated: false,
      },
    },
  ],
  total_builds_in_range: 1,
  builds_returned: 1,
  global_min_start: 1700000000000,
  global_max_stop: 1700000005000,
}

describe('TimelineTab (multi-build)', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls fetchProjectTimeline with projectId', async () => {
    vi.mocked(fetchProjectTimeline).mockResolvedValue(mockMultiTimeline)
    renderTab('proj1')
    await screen.findByTestId('timeline-chart')
    expect(fetchProjectTimeline).toHaveBeenCalledWith('proj1', expect.objectContaining({}))
  })

  it('renders BranchSelector', async () => {
    vi.mocked(fetchProjectTimeline).mockResolvedValue(mockMultiTimeline)
    renderTab()
    await screen.findByTestId('timeline-chart')
    expect(screen.getByTestId('mock-branch-selector')).toBeInTheDocument()
  })

  it('renders DateRangePicker', async () => {
    vi.mocked(fetchProjectTimeline).mockResolvedValue(mockMultiTimeline)
    renderTab()
    await screen.findByTestId('timeline-chart')
    expect(screen.getByTestId('mock-date-range-picker')).toBeInTheDocument()
  })

  it('shows warning banner when total_builds_in_range > builds_returned', async () => {
    vi.mocked(fetchProjectTimeline).mockResolvedValue({
      ...mockMultiTimeline,
      total_builds_in_range: 25,
      builds_returned: 5,
    })
    renderTab()
    expect(await screen.findByText(/showing 5 of 25 builds/i)).toBeInTheDocument()
  })

  it('shows truncation warning when any build summary is truncated', async () => {
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

  it('shows empty state when no builds returned', async () => {
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

  it('passes multi-build data to TimelineChart', async () => {
    vi.mocked(fetchProjectTimeline).mockResolvedValue(mockMultiTimeline)
    renderTab()
    const chart = await screen.findByTestId('timeline-chart')
    const props = JSON.parse(chart.getAttribute('data-props') ?? '{}')
    expect(props.builds).toHaveLength(1)
    expect(props.minStart).toBe(1700000000000)
    expect(props.maxStop).toBe(1700000005000)
  })
})
