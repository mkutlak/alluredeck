import { describe, it, expect, vi, beforeEach } from 'vitest'
import { sendResultsMultipart } from './reports'
import { apiClient } from './client'

vi.mock('@/lib/env', () => ({
  env: { apiUrl: 'http://localhost:5050/api/v1' },
}))

vi.mock('./client', () => ({
  apiClient: {
    post: vi.fn().mockResolvedValue({ data: {} }),
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
