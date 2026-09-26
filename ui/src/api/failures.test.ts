import { describe, it, expect, vi } from 'vitest'
import { mockApiClient } from '@/test/mocks/api-client'

mockApiClient()

import { apiClient } from '@/api/client'
import { fetchFailureSummary } from './failures'

const mockGet = vi.mocked(apiClient.get)

describe('fetchFailureSummary', () => {
  it('GETs the encoded failure-summary path and unwraps the data payload', async () => {
    const payload = { enabled: true, cached: false, disclaimer: 'AI hypothesis — verify.' }
    mockGet.mockResolvedValue({ data: { data: payload, metadata: { message: 'ok' } } })

    expect(await fetchFailureSummary(1, 42, 'h1/with space')).toEqual(payload)
    expect(mockGet).toHaveBeenCalledWith(
      '/projects/1/builds/42/tests/h1%2Fwith%20space/failure-summary',
    )
  })
})
