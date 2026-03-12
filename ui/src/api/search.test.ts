import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { ApiResponse, SearchData } from '@/types/api'

vi.mock('@/api/client', () => ({
  apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

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

  it('calls GET /search with q param and returns response', async () => {
    const result = await search({ q: 'login' })

    expect(mockGet).toHaveBeenCalledWith('/search', { params: { q: 'login' } })
    expect(result).toEqual(mockResponse)
  })

  it('passes limit param when provided', async () => {
    await search({ q: 'test', limit: 5 })

    expect(mockGet).toHaveBeenCalledWith('/search', { params: { q: 'test', limit: 5 } })
  })

  it('passes limit=0 correctly (not dropped by falsy check)', async () => {
    await search({ q: 'test', limit: 0 })
    expect(mockGet).toHaveBeenCalledWith('/search', { params: { q: 'test', limit: 0 } })
  })

  it('omits limit param when not provided', async () => {
    await search({ q: 'test' })
    expect(mockGet).toHaveBeenCalledWith('/search', { params: { q: 'test' } })
  })
})
