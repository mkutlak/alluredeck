import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mockApiClient } from '@/test/mocks/api-client'

mockApiClient()

import { apiClient } from '@/api/client'
import { cancelJob, cleanAdminResults, deleteJob } from './admin'

const mockPost = vi.mocked(apiClient.post)
const mockDelete = vi.mocked(apiClient.delete)

describe('admin API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockPost.mockResolvedValue({ data: {} })
    mockDelete.mockResolvedValue({ data: {} })
  })

  describe('cancelJob', () => {
    it('calls POST /admin/jobs/:jobId/cancel', async () => {
      await cancelJob('job-123')
      expect(mockPost).toHaveBeenCalledWith('/admin/jobs/job-123/cancel')
    })

    it('encodes special characters in jobId', async () => {
      await cancelJob('job/with/slashes')
      expect(mockPost).toHaveBeenCalledWith(
        `/admin/jobs/${encodeURIComponent('job/with/slashes')}/cancel`,
      )
    })

    it('encodes path traversal attempt in jobId', async () => {
      await cancelJob('../../etc/passwd')
      expect(mockPost).toHaveBeenCalledWith(
        `/admin/jobs/${encodeURIComponent('../../etc/passwd')}/cancel`,
      )
    })
  })

  describe('cleanAdminResults', () => {
    it('calls DELETE /admin/results/:projectId', async () => {
      await cleanAdminResults('my-project')
      expect(mockDelete).toHaveBeenCalledWith('/admin/results/my-project')
    })

    it('encodes special characters in projectId', async () => {
      await cleanAdminResults('project/with/slashes')
      expect(mockDelete).toHaveBeenCalledWith(
        `/admin/results/${encodeURIComponent('project/with/slashes')}`,
      )
    })
  })

  describe('deleteJob', () => {
    it('calls DELETE /admin/jobs/:jobId', async () => {
      await deleteJob('job-456')
      expect(mockDelete).toHaveBeenCalledWith('/admin/jobs/job-456')
    })

    it('encodes special characters in jobId', async () => {
      await deleteJob('job with spaces')
      expect(mockDelete).toHaveBeenCalledWith(
        `/admin/jobs/${encodeURIComponent('job with spaces')}`,
      )
    })

    it('encodes percent-encoded jobId', async () => {
      await deleteJob('job%2Fencoded')
      expect(mockDelete).toHaveBeenCalledWith(`/admin/jobs/${encodeURIComponent('job%2Fencoded')}`)
    })
  })
})
