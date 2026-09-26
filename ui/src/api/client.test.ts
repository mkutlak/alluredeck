import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { ApiError, NetworkError, extractErrorMessage } from './client'

let fetchSpy: ReturnType<typeof vi.fn<typeof fetch>>

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

function refreshBody() {
  return {
    data: {
      csrf_token: 'new-csrf',
      expires_in: 3600,
      roles: ['admin'],
      username: 'alice',
      provider: 'local',
    },
    metadata: { message: 'Session refreshed' },
  }
}

const call = (i: number) => fetchSpy.mock.calls[i] as [string, RequestInit]
const headersOf = (i: number) => call(i)[1].headers as Record<string, string>

// Re-import after vi.resetModules() so the module picks up the stubbed fetch,
// a fresh single-flight refresh promise, and the ApiError class it throws.
async function getModule() {
  vi.resetModules()
  return import('./client')
}

function onUnauthorized() {
  const handler = vi.fn()
  window.addEventListener('allure:unauthorized', handler)
  return handler
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

describe('extractErrorMessage', () => {
  it.each([
    [
      'ApiError metadata.message',
      new ApiError('Request failed', {
        status: 400,
        data: { metadata: { message: 'Invalid credentials' } },
      }),
      'Invalid credentials',
    ],
    [
      'ApiError without metadata',
      new ApiError('Network Error', { status: 500, data: {} }),
      'Network Error',
    ],
    [
      'NetworkError',
      new NetworkError('Request timed out after 30s'),
      'Cannot reach AllureDeck API — check your connection or the server.',
    ],
    ['plain Error', new Error('Something went wrong'), 'Something went wrong'],
    ['non-Error value', 'oops', 'An unexpected error occurred'],
  ])('maps %s', (_, error, expected) => {
    expect(extractErrorMessage(error)).toBe(expected)
  })
})

describe('apiClient', () => {
  it('GET builds the query string, skipping undefined params but keeping falsy ones', async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ ok: true }))
    const { apiClient } = await getModule()

    const res = await apiClient.get<{ ok: boolean }>('/test', {
      params: { page: 0, q: '', missing: undefined },
    })

    expect(res.data).toEqual({ ok: true })
    const [url, init] = call(0)
    expect(url).toMatch(/\/test\?page=0&q=$/)
    expect(init.method).toBe('GET')
    expect(init.credentials).toBe('include')
  })

  it('POST sends a JSON body', async () => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ id: 1 }))
    const { apiClient } = await getModule()

    const res = await apiClient.post<{ id: number }>('/items', { name: 'test' })

    expect(res.data).toEqual({ id: 1 })
    expect(call(0)[1].method).toBe('POST')
    expect(call(0)[1].body).toBe(JSON.stringify({ name: 'test' }))
    expect(headersOf(0)['Content-Type']).toBe('application/json')
  })

  it.each([
    ['get', undefined],
    ['post', 'abc123'],
    ['put', 'abc123'],
    ['delete', 'abc123'],
    ['patch', 'abc123'],
  ] as const)(
    '%s sends X-CSRF-Token=%s (cookie only on mutating methods)',
    async (method, expected) => {
      document.cookie = 'csrf_token=abc123'
      fetchSpy.mockResolvedValueOnce(jsonResponse({}))
      const { apiClient } = await getModule()

      await apiClient[method]('/x')

      expect(headersOf(0)['X-CSRF-Token']).toBe(expected)
    },
  )

  it.each([
    [
      'JSON',
      jsonResponse({ metadata: { message: 'Not found' } }, 404),
      404,
      { metadata: { message: 'Not found' } },
    ],
    [
      'non-JSON',
      new Response('<html>Bad Gateway</html>', { status: 502, statusText: 'Bad Gateway' }),
      502,
      { message: 'Bad Gateway' },
    ],
  ])('throws ApiError carrying status and a %s error body', async (_, response, status, data) => {
    fetchSpy.mockResolvedValueOnce(response)
    const { apiClient, ApiError } = await getModule()

    const err: unknown = await apiClient.get('/missing').catch((e: unknown) => e)

    expect(err).toBeInstanceOf(ApiError)
    expect((err as InstanceType<typeof ApiError>).response).toEqual({ status, data })
  })

  it('handles 204 No Content response', async () => {
    fetchSpy.mockResolvedValueOnce(new Response(null, { status: 204 }))
    const { apiClient } = await getModule()

    const res = await apiClient.delete('/items/1')

    expect(res.data).toBeUndefined()
  })

  it.each([
    // The browser must generate the multipart boundary itself.
    ['FormData drops', new FormData(), undefined],
    ['a raw File keeps', new File(['data'], 'archive.tar.gz'), 'application/gzip'],
  ])('%s the caller Content-Type and sends the body as-is', async (_, body, expected) => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({}))
    const { apiClient } = await getModule()

    await apiClient.post('/upload', body, { headers: { 'Content-Type': 'application/gzip' } })

    expect(headersOf(0)['Content-Type']).toBe(expected)
    expect(call(0)[1].body).toBe(body)
  })
})

describe('apiClient refresh-on-401', () => {
  it('successfully refreshes and retries on 401', async () => {
    fetchSpy
      .mockResolvedValueOnce(jsonResponse({ error: 'unauthorized' }, 401))
      .mockResolvedValueOnce(jsonResponse(refreshBody()))
      .mockResolvedValueOnce(jsonResponse({ ok: true, value: 42 }))

    const { apiClient } = await getModule()
    const { useAuthStore } = await import('@/store/auth') // same instance the client uses
    useAuthStore.getState().clearAuth()

    const res = await apiClient.get<{ ok: boolean; value: number }>('/widgets')

    expect(res.data).toEqual({ ok: true, value: 42 })
    expect(fetchSpy.mock.calls.map((_, i) => new URL(call(i)[0]).pathname)).toEqual([
      '/widgets',
      '/auth/refresh',
      '/widgets',
    ])
    expect(call(1)[1].method).toBe('POST')
    expect(useAuthStore.getState()).toMatchObject({
      isAuthenticated: true,
      username: 'alice',
      roles: ['admin'],
      provider: 'local',
    })
  })

  it('dispatches allure:unauthorized when refresh also fails', async () => {
    fetchSpy
      .mockResolvedValueOnce(jsonResponse({ error: 'unauthorized' }, 401))
      .mockResolvedValueOnce(jsonResponse({ error: 'refresh denied' }, 401))

    const { apiClient, ApiError } = await getModule()
    const handler = onUnauthorized()

    await expect(apiClient.get('/widgets')).rejects.toThrow(ApiError)

    expect(handler).toHaveBeenCalledTimes(1)
    expect(fetchSpy).toHaveBeenCalledTimes(2) // original + refresh, no retry
    expect(call(1)[0]).toContain('/auth/refresh')
    window.removeEventListener('allure:unauthorized', handler)
  })

  it.each(['/auth/refresh', '/login'])('does not attempt refresh on %s 401', async (path) => {
    fetchSpy.mockResolvedValueOnce(jsonResponse({ error: 'denied' }, 401))

    const { apiClient, ApiError } = await getModule()
    const handler = onUnauthorized()

    await expect(apiClient.post(path)).rejects.toThrow(ApiError)

    // Exactly one fetch: the original call. No follow-up /auth/refresh loop.
    expect(fetchSpy).toHaveBeenCalledTimes(1)
    expect(call(0)[0]).toContain(path)
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

    fetchSpy.mockImplementation((input: RequestInfo | URL) => {
      const url =
        typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url
      if (url.includes('/auth/refresh')) {
        return refreshPending
      }
      // prior counts the CURRENT call, so the first five hits of /x return 401
      // and every retry after the refresh returns 200.
      const prior = fetchSpy.mock.calls.filter(
        (c) => typeof c[0] === 'string' && c[0].includes('/x'),
      ).length
      if (prior <= 5) {
        return Promise.resolve(jsonResponse({ error: 'unauthorized' }, 401))
      }
      return Promise.resolve(jsonResponse({ ok: true, n: prior }))
    })

    const { apiClient } = await getModule()
    const { useAuthStore } = await import('@/store/auth') // same instance the client uses
    useAuthStore.getState().clearAuth()

    const promises = Array.from({ length: 5 }, () =>
      apiClient.get<{ ok: boolean; n: number }>('/x'),
    )

    // Give the 5 original 401s a tick to land and register as waiters on the
    // single-flight refresh promise, then resolve /auth/refresh.
    await new Promise((r) => setTimeout(r, 0))
    resolveRefresh(jsonResponse(refreshBody()))

    const results = await Promise.all(promises)

    expect(results.map((res) => res.data.ok)).toEqual([true, true, true, true, true])
    const refreshCalls = fetchSpy.mock.calls.filter(
      (c) => typeof c[0] === 'string' && c[0].includes('/auth/refresh'),
    )
    expect(refreshCalls).toHaveLength(1)
    expect(useAuthStore.getState()).toMatchObject({ isAuthenticated: true, username: 'alice' })
  })
})
