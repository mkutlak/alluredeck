import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mockApiClient } from '@/test/mocks/api-client'

mockApiClient()

import { apiClient } from '@/api/client'
import { fetchPipelineRuns, fetchRunsFeed } from './pipeline'

const mockGet = vi.mocked(apiClient.get)

describe('fetchPipelineRuns', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGet.mockResolvedValue({
      data: { data: [], metadata: { message: 'ok' }, pagination: {} },
    })
  })

  it('calls the project-scoped pipeline-runs endpoint', async () => {
    await fetchPipelineRuns('proj')
    expect(mockGet).toHaveBeenCalledWith('/projects/proj/pipeline-runs', { params: {} })
  })

  it('passes page and branch params', async () => {
    await fetchPipelineRuns('proj', 2, undefined, 'main')
    expect(mockGet).toHaveBeenCalledWith('/projects/proj/pipeline-runs', {
      params: { page: 2, branch: 'main' },
    })
  })
})

describe('fetchRunsFeed', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockGet.mockResolvedValue({
      data: { data: [], metadata: { message: 'ok' }, pagination: {} },
    })
  })

  it('calls GET /pipeline-runs with no query string when no args given', async () => {
    await fetchRunsFeed()
    expect(mockGet).toHaveBeenCalledWith('/pipeline-runs')
  })

  it('includes page and per_page in the query string', async () => {
    await fetchRunsFeed(2, 10)
    expect(mockGet).toHaveBeenCalledWith('/pipeline-runs?page=2&per_page=10')
  })

  it('includes branch in the query string', async () => {
    await fetchRunsFeed(undefined, undefined, 'main')
    expect(mockGet).toHaveBeenCalledWith('/pipeline-runs?branch=main')
  })

  it('serializes repeated group_id params for each selected group', async () => {
    await fetchRunsFeed(undefined, undefined, undefined, [3, 7])
    expect(mockGet).toHaveBeenCalledWith('/pipeline-runs?group_id=3&group_id=7')
  })

  it('omits group_id entirely when groupIds is an empty array', async () => {
    await fetchRunsFeed(undefined, undefined, undefined, [])
    expect(mockGet).toHaveBeenCalledWith('/pipeline-runs')
  })

  it('combines page, branch, and group_id together', async () => {
    await fetchRunsFeed(1, undefined, 'main', [3])
    expect(mockGet).toHaveBeenCalledWith('/pipeline-runs?page=1&branch=main&group_id=3')
  })

  it('returns the response data', async () => {
    const payload = { data: [{ commit_sha: 'abc' }], metadata: { message: 'ok' }, pagination: {} }
    mockGet.mockResolvedValue({ data: payload })
    const result = await fetchRunsFeed()
    expect(result).toEqual(payload)
  })
})
