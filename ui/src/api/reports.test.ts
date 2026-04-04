import { describe, it, expect, vi, beforeEach } from 'vitest'
import { sendResultsMultipart, fetchReportSummary } from './reports'
import { apiClient } from './client'

vi.mock('@/lib/env', () => ({
  env: { apiUrl: 'http://localhost:5050/api/v1' },
}))

vi.mock('./client', () => ({
  apiClient: {
    post: vi.fn().mockResolvedValue({ data: {} }),
    get: vi.fn(),
  },
}))

const mockedPost = vi.mocked(apiClient.post)

describe('sendResultsMultipart', () => {
  beforeEach(() => {
    mockedPost.mockClear()
  })

  it('sends regular files as multipart/form-data', async () => {
    const files = [
      new File(['{}'], 'result1.json', { type: 'application/json' }),
      new File(['{}'], 'result2.json', { type: 'application/json' }),
    ]
    await sendResultsMultipart('my-project', files)

    expect(mockedPost).toHaveBeenCalledOnce()
    const [url, body, config] = mockedPost.mock.calls[0]
    expect(url).toBe('/projects/my-project/results')
    expect(body).toBeInstanceOf(FormData)
    expect(config?.headers?.['Content-Type']).toBe('multipart/form-data')
  })

  it.each([
    ['results.tar.gz', 'application/gzip'],
    ['results.tgz', 'application/x-compressed-tar'],
  ])('sends a single %s file as application/gzip body', async (filename, mimeType) => {
    const blob = new File([new Uint8Array([0x1f, 0x8b])], filename, { type: mimeType })
    await sendResultsMultipart('my-project', [blob])

    expect(mockedPost).toHaveBeenCalledOnce()
    const [url, body, config] = mockedPost.mock.calls[0]
    expect(url).toBe('/projects/my-project/results')
    expect(body).toBeInstanceOf(File)
    expect(config?.headers?.['Content-Type']).toBe('application/gzip')
  })

  it('sends multiple files including a .tar.gz as multipart/form-data', async () => {
    const files = [
      new File([new Uint8Array([0x1f, 0x8b])], 'results.tar.gz', {
        type: 'application/gzip',
      }),
      new File(['{}'], 'extra.json', { type: 'application/json' }),
    ]
    await sendResultsMultipart('my-project', files)

    expect(mockedPost).toHaveBeenCalledOnce()
    const [, body, config] = mockedPost.mock.calls[0]
    expect(body).toBeInstanceOf(FormData)
    expect(config?.headers?.['Content-Type']).toBe('multipart/form-data')
  })

  it('sends a single .zip file as multipart/form-data (not gzip)', async () => {
    const file = new File([new Uint8Array(10)], 'results.zip', {
      type: 'application/zip',
    })
    await sendResultsMultipart('my-project', [file])

    expect(mockedPost).toHaveBeenCalledOnce()
    const [, body, config] = mockedPost.mock.calls[0]
    expect(body).toBeInstanceOf(FormData)
    expect(config?.headers?.['Content-Type']).toBe('multipart/form-data')
  })
})

describe('fetchReportSummary', () => {
  const mockGet = vi.mocked(apiClient.get)

  beforeEach(() => {
    mockGet.mockClear()
  })

  it('calls apiClient.get with encoded path and returns summary data', async () => {
    const summary = {
      statistic: { passed: 5, failed: 0, broken: 0, skipped: 0, unknown: 0, total: 5 },
    }
    mockGet.mockResolvedValueOnce({ data: summary })

    const result = await fetchReportSummary('my-project', 'report-42')

    expect(mockGet).toHaveBeenCalledWith(
      '/projects/my-project/reports/report-42/widgets/summary.json',
    )
    expect(result).toEqual(summary)
  })

  it('encodes special characters in projectId and reportId', async () => {
    const summary = {
      statistic: { passed: 1, failed: 0, broken: 0, skipped: 0, unknown: 0, total: 1 },
    }
    mockGet.mockResolvedValueOnce({ data: summary })

    await fetchReportSummary('project/x', 'report#1')

    expect(mockGet).toHaveBeenCalledWith(
      `/projects/${encodeURIComponent('project/x')}/reports/${encodeURIComponent('report#1')}/widgets/summary.json`,
    )
  })

  it('returns null when apiClient throws', async () => {
    mockGet.mockRejectedValueOnce(new Error('Network error'))

    const result = await fetchReportSummary('my-project', 'missing-report')

    expect(result).toBeNull()
  })
})
