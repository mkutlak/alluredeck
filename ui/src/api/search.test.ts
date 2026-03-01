import { describe, it, expect, vi, beforeEach } from 'vitest'
import type { ApiResponse, SearchData } from '@/types/api'

vi.mock('@/api/client', () => ({
  setAccessToken: vi.fn(),
  getAccessToken: vi.fn(),
  apiClient: { get: vi.fn(), post: vi.fn(), delete: vi.fn() },
  extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
}))

import { apiClient } from '@/api/client'
import { search } from './search'

const mockGet = vi.mocked(apiClient.get)

describe('search', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls GET /search with q param', async () => {
    const response: ApiResponse<SearchData> = {
      data: { projects: [], tests: [] },
      metadata: { message: 'Search results' },
    }
    mockGet.mockResolvedValue({ data: response })

    const result = await search({ q: 'login' })

    expect(mockGet).toHaveBeenCalledWith('/search', {
      params: { q: 'login' },
    })
    expect(result).toEqual(response)
  })

  it('passes limit param when provided', async () => {
    const response: ApiResponse<SearchData> = {
      data: { projects: [], tests: [] },
      metadata: { message: 'Search results' },
    }
    mockGet.mockResolvedValue({ data: response })

    await search({ q: 'test', limit: 5 })

    expect(mockGet).toHaveBeenCalledWith('/search', {
      params: { q: 'test', limit: 5 },
    })
  })

  it('does not include limit when not provided', async () => {
    const response: ApiResponse<SearchData> = {
      data: { projects: [], tests: [] },
      metadata: { message: 'Search results' },
    }
    mockGet.mockResolvedValue({ data: response })

    await search({ q: 'hello' })

    expect(mockGet).toHaveBeenCalledWith('/search', {
      params: { q: 'hello' },
    })
  })
})
