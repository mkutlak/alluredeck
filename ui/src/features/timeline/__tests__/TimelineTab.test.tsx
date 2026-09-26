import { describe, it, expect, vi, beforeEach } from 'vitest'
import { act, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router'
import { QueryClientProvider } from '@tanstack/react-query'
import { createTestQueryClient } from '@/test/render'
import type { MultiTimelineData, TimelineBuildEntry, TimelineTestCase } from '@/types/api'

vi.mock('@/api/reports', () => ({ fetchProjectTimeline: vi.fn() }))
vi.mock('@/api/branches', () => ({ fetchBranches: vi.fn() }))
vi.mock('../TimelineChart', () => ({
  TimelineChart: (props: Record<string, unknown>) => (
    <div data-testid="timeline-chart" data-props={JSON.stringify(props)} />
  ),
}))

import { fetchProjectTimeline } from '@/api/reports'
import { fetchBranches } from '@/api/branches'
import { useUIStore } from '@/store/ui'
import { TimelineTab } from '../TimelineTab'

function renderTab() {
  return render(
    <QueryClientProvider client={createTestQueryClient()}>
      <MemoryRouter initialEntries={['/projects/proj1/timeline']}>
        <Routes>
          <Route path="projects/:id/timeline" element={<TimelineTab />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  )
}

function tc(name: string, start: number, duration: number): TimelineTestCase {
  return {
    name,
    full_name: `com.example.${name}`,
    status: 'passed',
    start,
    stop: start + duration,
    duration,
    thread: 'worker-1',
    host: 'node-1',
  }
}

function build(order: number, cases: TimelineTestCase[], truncated = false): TimelineBuildEntry {
  return {
    build_order: order,
    created_at: '2026-03-25T12:00:00Z',
    test_cases: cases,
    summary: {
      total: cases.length,
      min_start: cases[0]?.start ?? 0,
      max_stop: cases[cases.length - 1]?.stop ?? 0,
      total_duration: cases.reduce((sum, c) => sum + c.duration, 0),
      truncated,
    },
  }
}

function timeline(builds: TimelineBuildEntry[], overrides: Partial<MultiTimelineData> = {}) {
  return {
    builds,
    total_builds_in_range: builds.length,
    builds_returned: builds.length,
    global_min_start: 1_700_000_000_000,
    global_max_stop: 1_700_000_009_000,
    ...overrides,
  }
}

const oneBuild = [build(1, [tc('Login', 1_700_000_000_000, 5000)])]

describe('TimelineTab', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(fetchBranches).mockResolvedValue([])
    useUIStore.setState({ selectedBranch: undefined })
  })

  it('flattens test cases across builds and sums the summary line', async () => {
    vi.mocked(fetchProjectTimeline).mockResolvedValue(
      timeline([
        build(2, [tc('Login', 1_700_000_000_000, 5000), tc('Logout', 1_700_000_001_000, 2000)]),
        build(1, [tc('Signup', 1_700_000_006_000, 3000)]),
      ]),
    )
    renderTab()

    const chart = await screen.findByTestId('timeline-chart')
    const props = JSON.parse(chart.getAttribute('data-props') ?? '{}') as {
      testCases: unknown[]
      builds: unknown[]
      minStart: number
      maxStop: number
    }
    expect(props.testCases).toHaveLength(3)
    expect(props.builds).toHaveLength(2)
    expect(props.minStart).toBe(1_700_000_000_000)
    expect(props.maxStop).toBe(1_700_000_009_000)
    expect(screen.getByText('3 tests · 10.0s total')).toBeInTheDocument()
  })

  it('shows empty state when no builds returned', async () => {
    vi.mocked(fetchProjectTimeline).mockResolvedValue(timeline([]))
    renderTab()
    expect(await screen.findByText(/no timeline data/i)).toBeInTheDocument()
  })

  it.each([
    [
      'shows warning banner when total_builds_in_range > builds_returned',
      timeline(oneBuild, { total_builds_in_range: 25, builds_returned: 5 }),
      /showing 5 of 25 builds/i,
    ],
    [
      'shows truncation warning when any build summary is truncated',
      timeline([build(1, [tc('Login', 1_700_000_000_000, 5000)], true)]),
      /truncated/i,
    ],
  ])('%s', async (_name, data, banner) => {
    vi.mocked(fetchProjectTimeline).mockResolvedValue(data)
    renderTab()
    expect(await screen.findByText(banner)).toBeInTheDocument()
  })

  // Regression (d66811e): the stored branch is shared across projects, so one
  // this project lacks must not filter its timeline.
  it('filters by the stored branch only when the project has it', async () => {
    vi.mocked(fetchBranches).mockResolvedValue([
      { id: 1, project_id: 1, name: 'main', is_default: true, created_at: '2024-01-01T00:00:00Z' },
    ])
    vi.mocked(fetchProjectTimeline).mockResolvedValue(timeline(oneBuild))
    useUIStore.setState({ selectedBranch: 'gone' })
    renderTab()
    await screen.findByTestId('timeline-chart')

    act(() => {
      useUIStore.setState({ selectedBranch: 'main' })
    })

    await waitFor(() =>
      expect(fetchProjectTimeline).toHaveBeenLastCalledWith(
        'proj1',
        expect.objectContaining({ branch: 'main' }),
      ),
    )
    expect(fetchProjectTimeline).not.toHaveBeenCalledWith(
      'proj1',
      expect.objectContaining({ branch: 'gone' }),
    )
  })
})
