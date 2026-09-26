import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { createElement, type ReactNode } from 'react'
import { createTestQueryClient } from '@/test/render'
import type { JobData, JobStatus } from '@/types/api'
import * as reportsApi from '@/api/reports'
import { useJobPolling } from './useJobPolling'

import { mockApiClient } from '@/test/mocks/api-client'

vi.mock('@/api/reports')
mockApiClient()

function renderPolling(jobId: string | null) {
  const qc = createTestQueryClient()
  return renderHook(() => useJobPolling('my-project', jobId), {
    wrapper: ({ children }: { children: ReactNode }) =>
      createElement(QueryClientProvider, { client: qc }, children),
  })
}

describe('useJobPolling', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('is disabled and not polling when jobId is null', () => {
    const { result } = renderPolling(null)

    expect(result.current.isPolling).toBe(false)
    expect(result.current.status).toBeUndefined()
    expect(reportsApi.getJobStatus).not.toHaveBeenCalled()
  })

  // Terminal statuses stop polling and surface the job's output or error.
  it.each<[JobStatus, Partial<JobData>, object]>([
    ['pending', {}, { isPolling: true, isCompleted: false, isFailed: false }],
    ['running', {}, { isPolling: true, isCompleted: false, isFailed: false }],
    [
      'completed',
      { output: 'report-id-xyz' },
      { isPolling: false, isCompleted: true, isFailed: false, output: 'report-id-xyz' },
    ],
    [
      'failed',
      { error: 'something went wrong' },
      { isPolling: false, isCompleted: false, isFailed: true, error: 'something went wrong' },
    ],
  ])('status %s -> %o', async (status, job, expected) => {
    vi.mocked(reportsApi.getJobStatus).mockResolvedValue({
      data: {
        job_id: 'job-123',
        project_id: 1,
        slug: 'my-project',
        status,
        created_at: '',
        started_at: null,
        completed_at: null,
        output: '',
        error: '',
        ...job,
      },
      metadata: { message: 'ok' },
    })

    const { result } = renderPolling('job-123')

    await waitFor(() => expect(result.current.status).toBe(status))
    expect(result.current).toMatchObject(expected)
  })
})
