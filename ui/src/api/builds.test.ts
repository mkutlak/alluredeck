import { describe, it, expect, vi } from 'vitest'
import { mockApiClient } from '@/test/mocks/api-client'

mockApiClient()

import { apiClient } from '@/api/client'
import { fetchBuildFailedTests } from './builds'

const mockGet = vi.mocked(apiClient.get)

describe('fetchBuildFailedTests', () => {
  it.each([
    [undefined, 50],
    [10, 10],
  ])(
    'GETs failed tests with limit %s -> %s and unwraps the plain (non-paginated) envelope',
    async (limit, expectedLimit) => {
      const tests = [{ test_name: 'should login', status: 'failed', history_id: 'h1' }]
      mockGet.mockResolvedValue({ data: { data: tests, metadata: { message: 'ok' } } })

      expect(await fetchBuildFailedTests(1, 42, limit)).toEqual(tests)
      expect(mockGet).toHaveBeenCalledWith('/projects/1/builds/42/tests', {
        params: { status: 'failed', limit: expectedLimit },
      })
    },
  )
})
