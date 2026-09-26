import { describe, it, expect, vi, beforeEach } from 'vitest'
import { sendResultsMultipart } from './reports'
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
const gz = (name: string, type = 'application/gzip') =>
  new File([new Uint8Array([0x1f, 0x8b])], name, { type })
const json = (name: string) => new File(['{}'], name, { type: 'application/json' })
const zip = new File([new Uint8Array(10)], 'results.zip', { type: 'application/zip' })

describe('sendResultsMultipart', () => {
  beforeEach(() => {
    mockedPost.mockClear()
  })

  // Only a single tar.gz/tgz goes as a raw gzip body for server-side extraction;
  // everything else (several files, or a .zip) is sent as multipart form data.
  it.each([
    ['two result files', [json('result1.json'), json('result2.json')], 'multipart'],
    ['a single .tar.gz', [gz('results.tar.gz')], 'gzip'],
    ['a single .tgz', [gz('results.tgz', 'application/x-compressed-tar')], 'gzip'],
    ['a .tar.gz among other files', [gz('results.tar.gz'), json('extra.json')], 'multipart'],
    ['a single .zip', [zip], 'multipart'],
  ])('sends %s as a %s body', async (_, files, kind) => {
    await sendResultsMultipart('my-project', files)

    expect(mockedPost).toHaveBeenCalledOnce()
    const [url, body, config] = mockedPost.mock.calls[0]!
    expect(url).toBe('/projects/my-project/results')
    expect(body).toBeInstanceOf(kind === 'gzip' ? File : FormData)
    expect(config?.headers?.['Content-Type']).toBe(
      kind === 'gzip' ? 'application/gzip' : 'multipart/form-data',
    )
  })
})
