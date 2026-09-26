import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mockApiClient } from '@/test/mocks/api-client'

mockApiClient()

import { apiClient } from '@/api/client'
import { fetchPipelineRuns, fetchRunsFeed } from './pipeline'

const mockGet = vi.mocked(apiClient.get)
const payload = { data: [{ commit_sha: 'abc' }], metadata: { message: 'ok' }, pagination: {} }

beforeEach(() => {
  vi.clearAllMocks()
  mockGet.mockResolvedValue({ data: payload })
})

describe('fetchPipelineRuns', () => {
  it.each<[Parameters<typeof fetchPipelineRuns>, object]>([
    [['proj'], {}],
    [['proj', 2, undefined, 'main'], { page: 2, branch: 'main' }],
  ])('fetchPipelineRuns(%j) GETs the project-scoped runs with params %j', async (args, params) => {
    await fetchPipelineRuns(...args)
    expect(mockGet).toHaveBeenCalledWith('/projects/proj/pipeline-runs', { params })
  })
})

describe('fetchRunsFeed', () => {
  // group_id repeats per selected group, which apiClient params cannot express.
  it.each<[Parameters<typeof fetchRunsFeed>, string]>([
    [[], '/pipeline-runs'],
    [[2, 10], '/pipeline-runs?page=2&per_page=10'],
    [[undefined, undefined, undefined, [3, 7]], '/pipeline-runs?group_id=3&group_id=7'],
    [[undefined, undefined, undefined, []], '/pipeline-runs'],
    [[1, undefined, 'main', [3]], '/pipeline-runs?page=1&branch=main&group_id=3'],
  ])('fetchRunsFeed(%j) GETs %s and returns the response', async (args, url) => {
    expect(await fetchRunsFeed(...args)).toEqual(payload)
    expect(mockGet).toHaveBeenCalledWith(url)
  })
})
