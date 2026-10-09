import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunRow } from '../RunRow'
import type { PipelineRun } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchRunFailures: vi.fn().mockResolvedValue({
    data: [],
    metadata: { message: 'ok', truncated: false },
  }),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({
    data: [{ project_id: 10, slug: 'acme', display_name: 'Acme', parent_id: null, children: [2] }],
    metadata: { message: 'ok' },
  }),
  getProjects: vi.fn(),
}))

const run: PipelineRun = {
  pipeline_id: '196765',
  commit_sha: 'abc1234def5678',
  branch: 'main',
  timestamp: '2026-04-03T18:00:00Z',
  group_project_id: 10,
  group_slug: 'acme',
  suites: [
    {
      project_id: 2,
      slug: 'ui-tests',
      build_number: 3,
      build_id: 103,
      pass_rate: 85,
      total: 100,
      failed: 15,
      duration_ms: 30000,
      status: 'degraded',
      builds: [{ build_id: 103, build_number: 3 }],
    },
  ],
  aggregate: {
    suites_passed: 0,
    suites_total: 1,
    tests_passed: 85,
    tests_total: 100,
    pass_rate: 85,
    total_duration_ms: 30000,
  },
}

describe('RunRow for keyboard and screen-reader users', () => {
  it('points the toggle at the failures panel while it is open', async () => {
    const user = userEvent.setup()
    renderWithProviders(<RunRow run={run} />)
    const toggle = screen.getByRole('button', { name: 'Toggle failures for 196765' })
    expect(toggle).not.toHaveAttribute('aria-controls')

    await user.click(toggle)
    const panel = document.getElementById(toggle.getAttribute('aria-controls') ?? '')
    expect(panel).toContainElement(await screen.findByTestId('run-failures'))
  })

  // The group cell truncates on a narrow feed; the full name stays one hover away.
  it('gives the group link its full name as a tooltip', async () => {
    renderWithProviders(<RunRow run={run} />)
    expect(await screen.findByRole('link', { name: 'Acme' })).toHaveAttribute('title', 'Acme')
  })
})
