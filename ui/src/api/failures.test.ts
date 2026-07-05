import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mockApiClient } from '@/test/mocks/api-client'

mockApiClient()

import { apiClient } from '@/api/client'
import { fetchFailureSummary } from './failures'

const mockGet = vi.mocked(apiClient.get)

describe('fetchFailureSummary', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGet.mockResolvedValue({ data: { data: { enabled: false }, metadata: { message: 'ok' } } })
  })

  it('calls the failure-summary endpoint with project/build/history ids', async () => {
    await fetchFailureSummary(1, 42, 'h1')
    expect(mockGet).toHaveBeenCalledWith('/projects/1/builds/42/tests/h1/failure-summary')
  })

  it('encodes a history id containing special characters', async () => {
    await fetchFailureSummary(1, 42, 'h1/with space')
    expect(mockGet).toHaveBeenCalledWith(
      '/projects/1/builds/42/tests/h1%2Fwith%20space/failure-summary',
    )
  })

  it('returns the unwrapped data payload', async () => {
    const payload = {
      enabled: true,
      cached: false,
      summary: {
        hypothesis: 'Looks like a null pointer',
        category: 'product_bug',
        confidence: 'medium',
        evidence: ['NPE at line 12'],
      },
      disclaimer: 'AI hypothesis — verify before acting.',
    }
    mockGet.mockResolvedValue({ data: { data: payload, metadata: { message: 'ok' } } })

    const result = await fetchFailureSummary(1, 42, 'h1')
    expect(result).toEqual(payload)
  })
})
