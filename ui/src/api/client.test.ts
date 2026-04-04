import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// ---------------------------------------------------------------------------
// extractErrorMessage (static import — no fetch dependency)
// ---------------------------------------------------------------------------
describe('extractErrorMessage', () => {
  // These tests import statically because extractErrorMessage has no
  // runtime dependency on fetch.
  it('extracts metadata.message from ApiError', async () => {
    const { extractErrorMessage, ApiError } = await import('./client')
    const error = new ApiError('Request failed', {
      status: 400,
      data: { metadata: { message: 'Invalid credentials' } },
    })
    expect(extractErrorMessage(error)).toBe('Invalid credentials')
  })

  it('falls back to error.message when metadata absent', async () => {
    const { extractErrorMessage, ApiError } = await import('./client')
    const error = new ApiError('Network Error', {
      status: 500,
      data: {},
    })
    expect(extractErrorMessage(error)).toBe('Network Error')
  })

  it('extracts message from standard Error', async () => {
    const { extractErrorMessage } = await import('./client')
    expect(extractErrorMessage(new Error('Something went wrong'))).toBe('Something went wrong')
  })

  it('returns generic message for unknown error', async () => {
    const { extractErrorMessage } = await import('./client')
    expect(extractErrorMessage('oops')).toBe('An unexpected error occurred')
  })
})

// ---------------------------------------------------------------------------
// apiClient (fetch wrapper)
// ---------------------------------------------------------------------------
describe('apiClient', () => {
  let fetchSpy: ReturnType<typeof vi.fn>

  function jsonResponse(body: unknown, status = 200): Response {
    return new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    })
  }

  beforeEach(() => {
    fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)
    // Clear any CSRF cookie
    Object.defineProperty(document, 'cookie', { value: '', writable: true, configurable: true })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  // Re-import after each vi.resetModules() so the module picks up the
  // stubbed fetch AND we get the same ApiError class the module uses.
  async function getModule() {
    vi.resetModules()
    return import('./client')
  }

  it('GET constructs correct URL with params', async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ ok: true }))
    const { apiClient } = await getModule()

    const res = await apiClient.get<{ ok: boolean }>('/test', {
      params: { page: 1, q: 'hello' },
    })

    expect(res.data).toEqual({ ok: true })
    const [url, init] = fetchSpy.mock.calls[0] as [string, RequestInit]
    expect(url).toContain('/test?')
    expect(url).toContain('page=1')
    expect(url).toContain('q=hello')
    expect(init.method).toBe('GET')
    expect(init.credentials).toBe('include')
  })

  it('GET skips undefined params but preserves falsy values', async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}))
    const { apiClient } = await getModule()

    await apiClient.get('/test', {
      params: { page: 0, q: '', missing: undefined },
    })

    const [url] = fetchSpy.mock.calls[0] as [string]
    expect(url).toContain('page=0')
    expect(url).toContain('q=')
    expect(url).not.toContain('missing')
  })

  it('POST sends JSON body with correct headers', async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ id: 1 }))
    const { apiClient } = await getModule()

    const res = await apiClient.post<{ id: number }>('/items', { name: 'test' })

    expect(res.data).toEqual({ id: 1 })
    const [, init] = fetchSpy.mock.calls[0] as [string, RequestInit]
    expect(init.method).toBe('POST')
    expect(init.body).toBe(JSON.stringify({ name: 'test' }))
    const headers = init.headers as Record<string, string>
    expect(headers['Content-Type']).toBe('application/json')
  })

  it('injects CSRF token on mutating methods', async () => {
    document.cookie = 'csrf_token=abc123'
    // Each call needs its own Response (body can only be read once)
    fetchSpy
      .mockResolvedValueOnce(jsonResponse({}))
      .mockResolvedValueOnce(jsonResponse({}))
      .mockResolvedValueOnce(jsonResponse({}))
      .mockResolvedValueOnce(jsonResponse({}))
    const { apiClient } = await getModule()

    await apiClient.post('/a', null)
    await apiClient.put('/b', null)
    await apiClient.delete('/c')
    await apiClient.patch('/d', null)

    for (const call of fetchSpy.mock.calls) {
      const [, init] = call as [string, RequestInit]
      const headers = init.headers as Record<string, string>
      expect(headers['X-CSRF-Token']).toBe('abc123')
    }
  })

  it('does not inject CSRF token on GET', async () => {
    document.cookie = 'csrf_token=abc123'
    fetchSpy.mockResolvedValueOnce(jsonResponse({}))
    const { apiClient } = await getModule()

    await apiClient.get('/test')

    const [, init] = fetchSpy.mock.calls[0] as [string, RequestInit]
    const headers = init.headers as Record<string, string>
    expect(headers['X-CSRF-Token']).toBeUndefined()
  })

  it('dispatches allure:unauthorized event on 401', async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ error: 'unauthorized' }, 401))
    const { apiClient, ApiError } = await getModule()
    const handler = vi.fn()
    window.addEventListener('allure:unauthorized', handler)

    await expect(apiClient.get('/secret')).rejects.toThrow(ApiError)
    expect(handler).toHaveBeenCalledTimes(1)

    window.removeEventListener('allure:unauthorized', handler)
  })

  it('throws ApiError with parsed body on non-ok response', async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ metadata: { message: 'Not found' } }, 404))
    const { apiClient, ApiError } = await getModule()

    try {
      await apiClient.get('/missing')
      expect.unreachable('should have thrown')
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError)
      const apiErr = err as InstanceType<typeof ApiError>
      expect(apiErr.response?.status).toBe(404)
      expect(apiErr.response?.data).toEqual({ metadata: { message: 'Not found' } })
    }
  })

  it('handles 204 No Content response', async () => {
    fetchSpy.mockResolvedValueOnce(new Response(null, { status: 204 }))
    const { apiClient } = await getModule()

    const res = await apiClient.delete('/items/1')

    expect(res.data).toBeUndefined()
  })

  it('strips Content-Type for FormData body', async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}))
    const { apiClient } = await getModule()
    const formData = new FormData()
    formData.append('file', new Blob(['data']), 'test.txt')

    await apiClient.post('/upload', formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })

    const [, init] = fetchSpy.mock.calls[0] as [string, RequestInit]
    const headers = init.headers as Record<string, string>
    // Content-Type must NOT be set so the browser auto-generates the boundary
    expect(headers['Content-Type']).toBeUndefined()
    expect(init.body).toBe(formData)
  })

  it('preserves Content-Type for raw File body', async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}))
    const { apiClient } = await getModule()
    const file = new File(['data'], 'archive.tar.gz')

    await apiClient.post('/upload', file, {
      headers: { 'Content-Type': 'application/gzip' },
    })

    const [, init] = fetchSpy.mock.calls[0] as [string, RequestInit]
    const headers = init.headers as Record<string, string>
    expect(headers['Content-Type']).toBe('application/gzip')
    expect(init.body).toBe(file)
  })

  it('handles non-JSON error body gracefully', async () => {
    fetchSpy.mockResolvedValueOnce(
      new Response('<html>Bad Gateway</html>', {
        status: 502,
        statusText: 'Bad Gateway',
      }),
    )
    const { apiClient, ApiError } = await getModule()

    try {
      await apiClient.get('/broken')
      expect.unreachable('should have thrown')
    } catch (err) {
      expect(err).toBeInstanceOf(ApiError)
      const apiErr = err as InstanceType<typeof ApiError>
      expect(apiErr.response?.status).toBe(502)
    }
  })

  it('exposes defaults.baseURL', async () => {
    const { apiClient } = await getModule()
    expect(apiClient.defaults.baseURL).toBeDefined()
    expect(typeof apiClient.defaults.baseURL).toBe('string')
  })
})
