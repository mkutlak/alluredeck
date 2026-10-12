import { screen } from '@testing-library/react'
import { createMemoryRouter } from 'react-router'
import { vi } from 'vitest'
import { renderWithProviders } from '@/test/render'
import { RunPage } from '../RunPage'
import type { PipelineRun } from '@/types/api'

vi.mock('@/api/pipeline', () => ({
  fetchPipelineRun: vi.fn(),
  fetchRunFailures: vi.fn(),
  fetchPipelineRuns: vi.fn(),
  fetchRunsFeed: vi.fn(),
}))

vi.mock('@/api/projects', () => ({
  getProjectIndex: vi.fn().mockResolvedValue({ data: [], metadata: { message: 'ok' } }),
  getProjects: vi.fn(),
}))

vi.mock('@/api/system', () => ({
  getConfig: vi
    .fn()
    .mockResolvedValue({ data: { llm_enabled: false }, metadata: { message: 'ok' } }),
}))

import { fetchPipelineRun, fetchRunFailures } from '@/api/pipeline'

const run: PipelineRun = {
  pipeline_id: '48213',
  pipeline_url: 'https://ci.example/48213',
  commit_sha: 'a1b2c3d4e5f6',
  branch: 'feature/sepa',
  timestamp: '2026-04-03T18:00:00Z',
  group_project_id: 10,
  group_slug: 'acme',
  suites: [
    {
      project_id: 1,
      slug: 'api-cloud',
      build_number: 11,
      build_id: 101,
      pass_rate: 100,
      total: 10,
      failed: 0,
      duration_ms: 1000,
      status: 'passed',
    },
  ],
  aggregate: {
    suites_passed: 1,
    suites_total: 1,
    tests_passed: 10,
    tests_total: 10,
    pass_rate: 100,
    total_duration_ms: 1000,
  },
}

describe('RunPage CI link', () => {
  it('says it opens in a new tab', async () => {
    vi.mocked(fetchPipelineRun).mockResolvedValue({ data: run, metadata: { message: 'ok' } })
    vi.mocked(fetchRunFailures).mockResolvedValue({
      data: [],
      metadata: { message: 'ok', truncated: false },
    })
    const router = createMemoryRouter([{ path: '/runs/:groupId/:runKey', element: <RunPage /> }], {
      initialEntries: ['/runs/10/48213'],
    })
    renderWithProviders(<></>, { router })

    expect(
      await screen.findByRole('link', { name: 'CI pipeline (opens in new tab)' }),
    ).toHaveAttribute('href', 'https://ci.example/48213')
  })
})
