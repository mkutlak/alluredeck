import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mockApiClient } from '@/test/mocks/api-client'

mockApiClient()

import { apiClient } from '@/api/client'
import { fetchBuildFailedTests } from './builds'

const mockGet = vi.mocked(apiClient.get)

describe('fetchBuildFailedTests', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGet.mockResolvedValue({ data: { data: [], metadata: { message: 'ok' } } })
  })

  it('calls the project/build tests endpoint with status=failed and default limit', async () => {
    await fetchBuildFailedTests(1, 42)
    expect(mockGet).toHaveBeenCalledWith('/projects/1/builds/42/tests', {
      params: { status: 'failed', limit: 50 },
    })
  })

  it('passes a custom limit when provided', async () => {
    await fetchBuildFailedTests(1, 42, 10)
    expect(mockGet).toHaveBeenCalledWith('/projects/1/builds/42/tests', {
      params: { status: 'failed', limit: 10 },
    })
  })

  it('returns the unwrapped data array (plain envelope, not paginated)', async () => {
    const tests = [
      {
        test_name: 'should login',
        full_name: 'suite.should login',
        status: 'failed',
        duration_ms: 120,
        history_id: 'h1',
        flaky: false,
        retries: 0,
        new_failed: true,
        known: false,
        error_message: 'boom',
      },
    ]
    mockGet.mockResolvedValue({ data: { data: tests, metadata: { message: 'ok' } } })
    const result = await fetchBuildFailedTests(1, 42)
    expect(result).toEqual(tests)
  })
})
