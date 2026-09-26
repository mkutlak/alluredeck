import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { ApiResponse, SearchData } from '@/types/api'
import { mockApiClient } from '@/test/mocks/api-client'

mockApiClient()

import { apiClient } from '@/api/client'
import { search } from './search'

const mockGet = vi.mocked(apiClient.get)

const mockResponse: ApiResponse<SearchData> = {
  data: { projects: [], tests: [] },
  metadata: { message: 'Search results' },
}

describe('search', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGet.mockResolvedValue({ data: mockResponse })
  })

  // limit=0 must survive (no falsy check).
  it.each([[{ q: 'login' }], [{ q: 'test', limit: 0 }]])(
    'sends %o as GET /search params and returns the envelope',
    async (params) => {
      expect(await search(params)).toEqual(mockResponse)
      expect(mockGet).toHaveBeenCalledWith('/search', { params })
    },
  )
})
