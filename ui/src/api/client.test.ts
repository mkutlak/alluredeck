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

// ---------------------------------------------------------------------------
// refresh-on-401 retry logic
// ---------------------------------------------------------------------------
describe('apiClient refresh-on-401', () => {
  let fetchSpy: ReturnType<typeof vi.fn>

  function jsonResponse(body: unknown, status = 200): Response {
    return new Response(JSON.stringify(body), {
      status,
      headers: { 'Content-Type': 'application/json' },
    })
  }

  function refreshBody(overrides: Record<string, unknown> = {}) {
    return {
      data: {
        csrf_token: 'new-csrf',
        expires_in: 3600,
        roles: ['admin'],
        username: 'alice',
        provider: 'local',
        ...overrides,
      },
      metadata: { message: 'Session refreshed' },
    }
  }

  beforeEach(() => {
    fetchSpy = vi.fn()
    vi.stubGlobal('fetch', fetchSpy)
    Object.defineProperty(document, 'cookie', { value: '', writable: true, configurable: true })
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    vi.restoreAllMocks()
  })

  async function getModule() {
    vi.resetModules()
    return import('./client')
  }

  async function getAuthStore() {
    return import('@/store/auth')
  }

  it('successfully refreshes and retries on 401', async () => {
    fetchSpy
      .mockResolvedValueOnce(jsonResponse({ error: 'unauthorized' }, 401))
      .mockResolvedValueOnce(jsonResponse(refreshBody()))
      .mockResolvedValueOnce(jsonResponse({ ok: true, value: 42 }))

    const { apiClient } = await getModule()
    const { useAuthStore } = await getAuthStore()
    useAuthStore.getState().clearAuth()

    const res = await apiClient.get<{ ok: boolean; value: number }>('/widgets')

    expect(res.data).toEqual({ ok: true, value: 42 })
    expect(fetchSpy).toHaveBeenCalledTimes(3)

    const [firstUrl] = fetchSpy.mock.calls[0] as [string]
    const [refreshUrl, refreshInit] = fetchSpy.mock.calls[1] as [string, RequestInit]
    const [retryUrl] = fetchSpy.mock.calls[2] as [string]
    expect(firstUrl).toContain('/widgets')
    expect(refreshUrl).toContain('/auth/refresh')
    expect(refreshInit.method).toBe('POST')
    expect(retryUrl).toContain('/widgets')

    const state = useAuthStore.getState()
    expect(state.isAuthenticated).toBe(true)
    expect(state.username).toBe('alice')
    expect(state.roles).toEqual(['admin'])
    expect(state.provider).toBe('local')
  })

  it('dispatches allure:unauthorized when refresh also fails', async () => {
    fetchSpy
      .mockResolvedValueOnce(jsonResponse({ error: 'unauthorized' }, 401))
      .mockResolvedValueOnce(jsonResponse({ error: 'refresh denied' }, 401))

    const { apiClient, ApiError } = await getModule()
    const handler = vi.fn()
    window.addEventListener('allure:unauthorized', handler)

    await expect(apiClient.get('/widgets')).rejects.toThrow(ApiError)

    expect(handler).toHaveBeenCalledTimes(1)
    expect(fetchSpy).toHaveBeenCalledTimes(2) // original + refresh, no retry
    const [, refreshUrl] = fetchSpy.mock.calls.map((c) => c[0] as string)
    expect(refreshUrl).toContain('/auth/refresh')

    window.removeEventListener('allure:unauthorized', handler)
  })

  it('does not attempt refresh on /auth/refresh 401', async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ error: 'refresh denied' }, 401))

    const { apiClient, ApiError } = await getModule()
    const handler = vi.fn()
    window.addEventListener('allure:unauthorized', handler)

    await expect(apiClient.post('/auth/refresh')).rejects.toThrow(ApiError)

    // Exactly one fetch: the original call. No follow-up /auth/refresh loop.
    expect(fetchSpy).toHaveBeenCalledTimes(1)
    const [url] = fetchSpy.mock.calls[0] as [string]
    expect(url).toContain('/auth/refresh')
    expect(handler).toHaveBeenCalledTimes(1)

    window.removeEventListener('allure:unauthorized', handler)
  })

  it('does not attempt refresh on /login 401', async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ error: 'bad credentials' }, 401))

    const { apiClient, ApiError } = await getModule()
    const handler = vi.fn()
    window.addEventListener('allure:unauthorized', handler)

    await expect(apiClient.post('/login', { username: 'x', password: 'y' })).rejects.toThrow(
      ApiError,
    )

    expect(fetchSpy).toHaveBeenCalledTimes(1)
    const [url] = fetchSpy.mock.calls[0] as [string]
    expect(url).toContain('/login')
    // /login must not trigger a refresh POST
    for (const call of fetchSpy.mock.calls) {
      expect((call[0] as string)).not.toContain('/auth/refresh')
    }
    expect(handler).toHaveBeenCalledTimes(1)

    window.removeEventListener('allure:unauthorized', handler)
  })

  it('concurrent 401s share single refresh promise', async () => {
    // Gate the refresh so all 5 original calls 401 before the refresh resolves,
    // guaranteeing they all contend for the same in-flight refresh promise.
    let resolveRefresh: (value: Response) => void = () => undefined
    const refreshPending = new Promise<Response>((resolve) => {
      resolveRefresh = resolve
    })

    fetchSpy.mockImplementation((input: string | URL) => {
      const url = typeof input === 'string' ? input : input.toString()
      if (url.includes('/auth/refresh')) {
        return refreshPending
      }
      // First time each /x is hit, return 401; after the refresh we switch
      // to returning 200. Track by occurrence count.
      const prior = fetchSpy.mock.calls.filter(
        (c) => typeof c[0] === 'string' && (c[0] as string).includes('/x'),
      ).length
      // prior counts the CURRENT call, so the first hit has prior === 1
      // and the first five calls all return 401, subsequent retries return 200.
      if (prior <= 5) {
        return Promise.resolve(jsonResponse({ error: 'unauthorized' }, 401))
      }
      return Promise.resolve(jsonResponse({ ok: true, n: prior }))
    })

    const { apiClient } = await getModule()
    const { useAuthStore } = await getAuthStore()
    useAuthStore.getState().clearAuth()

    const promises = [
      apiClient.get<{ ok: boolean; n: number }>('/x'),
      apiClient.get<{ ok: boolean; n: number }>('/x'),
      apiClient.get<{ ok: boolean; n: number }>('/x'),
      apiClient.get<{ ok: boolean; n: number }>('/x'),
      apiClient.get<{ ok: boolean; n: number }>('/x'),
    ]

    // Give the 5 original 401s a tick to land and register as waiters on the
    // single-flight refresh promise, then resolve /auth/refresh.
    await new Promise((r) => setTimeout(r, 0))
    resolveRefresh(jsonResponse(refreshBody()))

    const results = await Promise.all(promises)

    // All 5 callers received a successful retry response.
    expect(results).toHaveLength(5)
    for (const res of results) {
      expect(res.data.ok).toBe(true)
    }

    // Exactly one call to /auth/refresh across the entire test.
    const refreshCalls = fetchSpy.mock.calls.filter(
      (c) => typeof c[0] === 'string' && (c[0] as string).includes('/auth/refresh'),
    )
    expect(refreshCalls).toHaveLength(1)

    // Sanity: auth store was updated once with the refreshed session.
    const state = useAuthStore.getState()
    expect(state.isAuthenticated).toBe(true)
    expect(state.username).toBe('alice')
  })
})
