import { describe, it, expect, vi, beforeEach, onTestFinished } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithProviders } from '@/test/render'
import { PipelineRunsTab } from '../PipelineRunsTab'
import type { PaginatedResponse, PipelineRun } from '@/types/api'
import { useUIStore } from '@/store/ui'

vi.mock('@/api/pipeline', () => ({ fetchPipelineRuns: vi.fn() }))
vi.mock('@/api/branches', () => ({ fetchBranches: vi.fn() }))

import { fetchPipelineRuns } from '@/api/pipeline'
import { fetchBranches } from '@/api/branches'

function makeRun(pipeline_id: string): PipelineRun {
  return {
    pipeline_id,
    commit_sha: 'abc1234def5678',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
    suites: [],
    aggregate: {
      suites_passed: 1,
      suites_total: 1,
      tests_passed: 42,
      tests_total: 42,
      pass_rate: 100,
      total_duration_ms: 15000,
    },
  }
}

describe('PipelineRunsTab run keys', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(fetchBranches).mockResolvedValue([])
    useUIStore.setState({ selectedBranch: undefined })
  })

  // A re-run of the same commit is a second pipeline: the API groups by pipeline
  // ID, so the commit SHA alone is not a unique key.
  it('renders both runs of one commit without a duplicate-key warning', async () => {
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    onTestFinished(() => consoleError.mockRestore())
    const response: PaginatedResponse<PipelineRun[]> = {
      data: [makeRun('196765'), makeRun('196766')],
      metadata: { message: 'ok' },
      pagination: { page: 1, per_page: 10, total: 2, total_pages: 1 },
    }
    vi.mocked(fetchPipelineRuns).mockResolvedValue(response)
    renderWithProviders(<PipelineRunsTab projectId="parent" childIds={['api-cloud']} />)

    expect(await screen.findByText('Pipeline 196765')).toBeInTheDocument()
    expect(screen.getByText('Pipeline 196766')).toBeInTheDocument()
    const logged = consoleError.mock.calls.map((c) => String(c[0])).join('\n')
    expect(logged).not.toMatch(/same key/)
  })
})
