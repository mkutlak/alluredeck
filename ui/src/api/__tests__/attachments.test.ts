import { describe, it, expect, vi, beforeEach } from 'vitest'
import { fetchAttachments, attachmentFileUrl } from '../attachments'

vi.mock('../client', () => ({
  apiClient: {
    get: vi.fn(),
    defaults: { baseURL: '/api/v1' },
  },
}))

import { apiClient } from '../client'

const mockGet = vi.mocked(apiClient.get)

describe('fetchAttachments', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls correct endpoint and returns grouped data with urls', async () => {
    const mockData = {
      groups: [
        {
          test_name: 'shouldRegister',
          test_status: 'failed',
          attachments: [
            { id: 1, name: 'screenshot.png', source: 'abc123.png', mime_type: 'image/png', size_bytes: 1024 },
          ],
        },
      ],
      total: 1,
      limit: 100,
      offset: 0,
    }
    mockGet.mockResolvedValueOnce({ data: { data: mockData } })

    const result = await fetchAttachments('p1', 'latest')

    expect(mockGet).toHaveBeenCalledWith(
      '/projects/p1/reports/latest/attachments',
      { params: {} },
    )
    expect(result.groups).toHaveLength(1)
    expect(result.groups[0].test_name).toBe('shouldRegister')
    expect(result.groups[0].attachments[0].url).toBe(
      '/api/v1/projects/p1/reports/latest/attachments/abc123.png',
    )
  })

  it('passes mime_type filter param', async () => {
    mockGet.mockResolvedValueOnce({ data: { data: { groups: [], total: 0, limit: 100, offset: 0 } } })

    await fetchAttachments('p1', 'latest', { mimeType: 'image/' })

    expect(mockGet).toHaveBeenCalledWith(
      '/projects/p1/reports/latest/attachments',
      { params: { mime_type: 'image/' } },
    )
  })

  it('passes limit and offset params', async () => {
    mockGet.mockResolvedValueOnce({ data: { data: { groups: [], total: 0, limit: 50, offset: 10 } } })

    await fetchAttachments('p1', '5', { limit: 50, offset: 10 })

    expect(mockGet).toHaveBeenCalledWith(
      '/projects/p1/reports/5/attachments',
      { params: { limit: 50, offset: 10 } },
    )
  })
})

describe('attachmentFileUrl', () => {
  it('constructs correct URL', () => {
    const url = attachmentFileUrl('my-project', 'latest', 'screenshot.png')
    expect(url).toBe('/api/v1/projects/my-project/reports/latest/attachments/screenshot.png')
  })

  it('encodes special characters', () => {
    const url = attachmentFileUrl('my project', '5', 'file name.png')
    expect(url).toBe('/api/v1/projects/my%20project/reports/5/attachments/file%20name.png')
  })
})
