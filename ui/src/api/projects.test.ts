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

  it('calls GET /projects with no params when none provided', async () => {
    await getProjects()
    expect(mockGet).toHaveBeenCalledWith('/projects', { params: {} })
  })

  it('passes page param when provided', async () => {
    await getProjects(1)
    expect(mockGet).toHaveBeenCalledWith('/projects', { params: { page: 1 } })
  })

  it('passes page=0 correctly (not dropped by falsy check)', async () => {
    await getProjects(0)
    expect(mockGet).toHaveBeenCalledWith('/projects', { params: { page: 0 } })
  })

  it('passes perPage param when provided', async () => {
    await getProjects(undefined, 10)
    expect(mockGet).toHaveBeenCalledWith('/projects', { params: { per_page: 10 } })
  })

  it('passes perPage=0 correctly (not dropped by falsy check)', async () => {
    await getProjects(undefined, 0)
    expect(mockGet).toHaveBeenCalledWith('/projects', { params: { per_page: 0 } })
  })

  it('passes both page and perPage params', async () => {
    await getProjects(2, 25)
    expect(mockGet).toHaveBeenCalledWith('/projects', { params: { page: 2, per_page: 25 } })
  })
})

