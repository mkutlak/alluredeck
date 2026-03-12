import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import type { JobData } from '@/types/api'
import * as reportsApi from '@/api/reports'
import { useJobPolling } from './useJobPolling'

vi.mock('@/api/reports')
vi.mock('@/api/client', () => ({
  apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

function makeJobData(overrides: Partial<JobData> = {}): JobData {
  return {
    job_id: 'job-123',
    project_id: 'my-project',
    status: 'pending',
    created_at: '2026-01-01T00:00:00Z',
    started_at: null,
    completed_at: null,
    output: '',
    error: '',
    ...overrides,
  }
}

function makeWrapper() {
  const qc = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
    },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: qc }, children)
}

describe('useJobPolling', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('is disabled and not polling when jobId is null', () => {
    const { result } = renderHook(() => useJobPolling('my-project', null), {
      wrapper: makeWrapper(),
    })

    expect(result.current.isPolling).toBe(false)
    expect(result.current.status).toBeUndefined()
    expect(reportsApi.getJobStatus).not.toHaveBeenCalled()
  })

  it.each(['pending', 'running'] as const)(
    'isPolling is true when status is %s',
    async (status) => {
      vi.mocked(reportsApi.getJobStatus).mockResolvedValue({
        data: makeJobData({ status }),
        metadata: { message: 'ok' },
      })

      const { result } = renderHook(() => useJobPolling('my-project', 'job-123'), {
        wrapper: makeWrapper(),
      })

      await waitFor(() => {
        expect(result.current.status).toBe(status)
      })

      expect(result.current.isPolling).toBe(true)
      expect(result.current.isCompleted).toBe(false)
      expect(result.current.isFailed).toBe(false)
    },
  )

  it('isPolling is false and isCompleted is true when status is completed', async () => {
    vi.mocked(reportsApi.getJobStatus).mockResolvedValue({
      data: makeJobData({
        status: 'completed',
        output: 'report-id-xyz',
        completed_at: '2026-01-01T00:01:00Z',
      }),
      metadata: { message: 'ok' },
    })

    const { result } = renderHook(() => useJobPolling('my-project', 'job-123'), {
      wrapper: makeWrapper(),
    })

    await waitFor(() => {
      expect(result.current.status).toBe('completed')
    })

    expect(result.current.isPolling).toBe(false)
    expect(result.current.isCompleted).toBe(true)
    expect(result.current.isFailed).toBe(false)
    expect(result.current.output).toBe('report-id-xyz')
  })

  it('isPolling is false and isFailed is true when status is failed', async () => {
    vi.mocked(reportsApi.getJobStatus).mockResolvedValue({
      data: makeJobData({ status: 'failed', error: 'something went wrong' }),
      metadata: { message: 'ok' },
    })

    const { result } = renderHook(() => useJobPolling('my-project', 'job-123'), {
      wrapper: makeWrapper(),
    })

    await waitFor(() => {
      expect(result.current.status).toBe('failed')
    })

    expect(result.current.isPolling).toBe(false)
    expect(result.current.isCompleted).toBe(false)
    expect(result.current.isFailed).toBe(true)
    expect(result.current.error).toBe('something went wrong')
  })
})
