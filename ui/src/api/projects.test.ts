import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mockApiClient } from '@/test/mocks/api-client'

mockApiClient()

import { apiClient } from '@/api/client'
import { getProjects, updateProjectTags } from './projects'

const mockGet = vi.mocked(apiClient.get)
const mockPut = vi.mocked(apiClient.put)

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

describe('updateProjectTags', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPut.mockResolvedValue({ data: { data: {}, metadata: {} } })
  })

  it('calls PUT /projects/:projectId/tags with encoded project ID', async () => {
    await updateProjectTags('my-project', ['tag1', 'tag2'])
    expect(mockPut).toHaveBeenCalledWith('/projects/my-project/tags', { tags: ['tag1', 'tag2'] })
  })

  it('encodes special characters in projectId', async () => {
    await updateProjectTags('project/with/slashes', ['tag1'])
    expect(mockPut).toHaveBeenCalledWith(
      `/projects/${encodeURIComponent('project/with/slashes')}/tags`,
      { tags: ['tag1'] },
    )
  })
})
