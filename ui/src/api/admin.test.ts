import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mockApiClient } from '@/test/mocks/api-client'

mockApiClient()

import { apiClient } from '@/api/client'
import { cancelJob, cleanAdminResults, deleteJob } from './admin'

describe('admin API', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  // Ids are path-encoded, so a traversal attempt or an already-encoded id
  // cannot escape its path segment.
  it.each([
    [
      'cancelJob',
      'post',
      '/admin/jobs/..%2F..%2Fetc%2Fpasswd/cancel',
      () => cancelJob('../../etc/passwd'),
    ],
    [
      'cleanAdminResults',
      'delete',
      '/admin/results/project%2Fwith%2Fslashes',
      () => cleanAdminResults('project/with/slashes'),
    ],
    ['deleteJob', 'delete', '/admin/jobs/job%20with%20spaces', () => deleteJob('job with spaces')],
    ['deleteJob', 'delete', '/admin/jobs/job%252Fencoded', () => deleteJob('job%2Fencoded')],
  ] as const)('%s calls %s %s', async (_, method, url, run) => {
    await run()
    expect(vi.mocked(apiClient[method])).toHaveBeenCalledWith(url)
  })
})
