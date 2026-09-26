import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mockApiClient } from '@/test/mocks/api-client'

mockApiClient()

import { apiClient } from '@/api/client'
import { getProjects } from './projects'

const mockGet = vi.mocked(apiClient.get)

describe('getProjects', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGet.mockResolvedValue({ data: { data: { projects: [] }, metadata: {} } })
  })

  // Zero values must survive (no falsy checks).
  it.each([
    [[], {}],
    [[0], { page: 0 }],
    [[undefined, 0], { per_page: 0 }],
    [[2, 25], { page: 2, per_page: 25 }],
  ] as const)('getProjects(%j) sends params %j', async (args, params) => {
    await getProjects(...args)
    expect(mockGet).toHaveBeenCalledWith('/projects', { params })
  })
})
