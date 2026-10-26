import { render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router'
import { vi } from 'vitest'
import { createTestQueryClient } from '@/test/render'
import { RunFailures } from '../RunFailures'
import { useUIStore } from '@/store/ui'
import type {
  PipelineRun,
  PipelineSuite,
  RunFailure,
  RunFailuresResponse,
  ApiResponse,
  ConfigData,
} from '@/types/api'

vi.mock('@/api/pipeline', () => ({ fetchRunFailures: vi.fn() }))
vi.mock('@/api/failures', () => ({ fetchFailureSummary: vi.fn() }))
vi.mock('@/api/system', () => ({ getConfig: vi.fn() }))

import { fetchRunFailures } from '@/api/pipeline'
import { getConfig } from '@/api/system'

function makeSuite(project_id: number, slug: string): PipelineSuite {
  return {
    project_id,
    slug,
    build_number: 3,
    build_id: 100 + project_id,
    pass_rate: 90,
    total: 10,
    failed: 1,
    duration_ms: 1000,
    status: 'degraded',
    builds: [{ build_id: 100 + project_id, build_number: 3 }],
  }
}

function makeRun(suites: PipelineSuite[]): PipelineRun {
  return {
    pipeline_id: '196765',
    commit_sha: 'abc1234',
    branch: 'main',
    timestamp: '2026-04-03T18:00:00Z',
    group_project_id: 10,
    group_slug: 'acme',
    suites,
    aggregate: {
      suites_passed: 0,
      suites_total: suites.length,
      tests_passed: 9 * suites.length,
      tests_total: 10 * suites.length,
      pass_rate: 90,
      total_duration_ms: 1000,
    },
  }
}

function makeFailure(project_id: number, slug: string, test_name: string): RunFailure {
  return {
    project_id,
    slug,
    build_id: 100 + project_id,
    build_number: 3,
    test_name,
    full_name: `${test_name}.spec.js`,
    status: 'failed',
    duration_ms: 100,
    history_id: `h-${test_name}`,
    flaky: false,
    retries: 0,
    new_failed: false,
    known: false,
    error_message: 'TimeoutError: locator.click',
  }
}

const config: ApiResponse<ConfigData> = {
  data: { llm_enabled: false } as ConfigData,
  metadata: { message: 'ok' },
}

describe('RunFailures while a newly failed suite loads its failures', () => {
  beforeEach(() => {
    vi.mocked(fetchRunFailures).mockReset()
    vi.mocked(getConfig).mockReset()
    vi.mocked(getConfig).mockResolvedValue(config)
    useUIStore.setState({ runsFailureGrouping: 'suite' })
  })

  // The failing-suite set is part of the list's key, so a suite that uploads and fails
  // makes the old list stand in (keepPreviousData) without any row of the new suite.
  it('says the new suite is loading rather than that it has no failing tests', async () => {
    const first: RunFailuresResponse = {
      data: [makeFailure(1, 'api-cloud', 'a1')],
      metadata: { message: 'ok', truncated: false },
    }
    vi.mocked(fetchRunFailures)
      .mockResolvedValueOnce(first)
      .mockReturnValueOnce(new Promise(() => {}))
    const client = createTestQueryClient()
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={client}>
        <MemoryRouter>{children}</MemoryRouter>
      </QueryClientProvider>
    )
    const apiCloud = makeSuite(1, 'api-cloud')

    const { rerender } = render(<RunFailures run={makeRun([apiCloud])} asPage />, { wrapper })
    expect(await screen.findByText('a1')).toBeInTheDocument()

    rerender(<RunFailures run={makeRun([apiCloud, makeSuite(2, 'ui-tests')])} asPage />)

    await waitFor(() => expect(fetchRunFailures).toHaveBeenCalledTimes(2))
    expect(await screen.findByText("Loading this suite's failures…")).toBeInTheDocument()
    expect(screen.queryByText('No failing tests listed for this suite.')).not.toBeInTheDocument()
  })

  // A suite whose report could not be read never has rows: its section was already final.
  it('keeps the final wording for a suite that cannot have rows', async () => {
    const first: RunFailuresResponse = {
      data: [makeFailure(1, 'api-cloud', 'a1')],
      metadata: { message: 'ok', truncated: false },
    }
    vi.mocked(fetchRunFailures)
      .mockResolvedValueOnce(first)
      .mockReturnValueOnce(new Promise(() => {}))
    const client = createTestQueryClient()
    const wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={client}>
        <MemoryRouter>{children}</MemoryRouter>
      </QueryClientProvider>
    )
    const apiCloud = makeSuite(1, 'api-cloud')
    const unreadable = {
      ...makeSuite(3, 'unreadable'),
      total: 0,
      failed: 0,
      status: 'failed' as const,
    }

    const { rerender } = render(<RunFailures run={makeRun([apiCloud, unreadable])} asPage />, {
      wrapper,
    })
    expect(await screen.findByText('a1')).toBeInTheDocument()

    rerender(<RunFailures run={makeRun([apiCloud, unreadable, makeSuite(2, 'ui-tests')])} asPage />)

    await waitFor(() => expect(fetchRunFailures).toHaveBeenCalledTimes(2))
    expect(await screen.findByText("Loading this suite's failures…")).toBeInTheDocument()
    expect(screen.getAllByText("Loading this suite's failures…")).toHaveLength(1)
    expect(screen.getByText('No failing tests listed for this suite.')).toBeInTheDocument()
  })
})
