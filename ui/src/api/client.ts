import { env } from '@/lib/env'
import { useAuthStore, type Role } from '@/store/auth'

// ---------------------------------------------------------------------------
// ApiError — replaces AxiosError
// ---------------------------------------------------------------------------
export class ApiError extends Error {
  response?: { status: number; data: unknown }

  constructor(message: string, response?: { status: number; data: unknown }) {
    super(message)
    this.name = 'ApiError'
    this.response = response
  }
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
interface RequestConfig {
  params?: Record<string, unknown>
  headers?: Record<string, string>
}

interface FetchResponse<T> {
  data: T
}

interface RefreshResponseBody {
  data: {
    csrf_token: string
    expires_in: number
    roles: Role[]
    username: string
    provider?: 'local' | 'oidc'
  }
  metadata?: { message?: string }
}

// ---------------------------------------------------------------------------
// CSRF helper (unchanged)
// ---------------------------------------------------------------------------
function getCSRFToken(): string | null {
  const match = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]+)/)
  return match ? decodeURIComponent(match[1]) : null
}

// ---------------------------------------------------------------------------
// Core request function
// ---------------------------------------------------------------------------
const BASE_URL = env.apiUrl

// Paths that must never trigger a refresh attempt (no infinite loop).
const NO_REFRESH_PATHS = new Set(['/auth/refresh', '/login', '/logout'])

// Module-level single-flight refresh promise. Ensures concurrent 401s within
// the same tab share one /auth/refresh request.
let refreshPromise: Promise<boolean> | null = null

// ---------------------------------------------------------------------------
// Web Locks type guards (graceful jsdom fallback)
// ---------------------------------------------------------------------------
interface WebLocksLike {
  request: (
    name: string,
    callback: () => Promise<boolean>,
  ) => Promise<boolean>
}

function getWebLocks(): WebLocksLike | null {
  const nav = globalThis.navigator as unknown as { locks?: unknown } | undefined
  if (!nav || typeof nav !== 'object') return null
  const locks = nav.locks
  if (!locks || typeof locks !== 'object') return null
  const request = (locks as { request?: unknown }).request
  if (typeof request !== 'function') return null
  return locks as WebLocksLike
}

// ---------------------------------------------------------------------------
// Refresh flow
// ---------------------------------------------------------------------------
function isRefreshResponseBody(value: unknown): value is RefreshResponseBody {
  if (!value || typeof value !== 'object') return false
  const data = (value as { data?: unknown }).data
  if (!data || typeof data !== 'object') return false
  const d = data as Record<string, unknown>
  return (
    typeof d.csrf_token === 'string' &&
    typeof d.expires_in === 'number' &&
    Array.isArray(d.roles) &&
    typeof d.username === 'string'
  )
}

async function performRefresh(): Promise<boolean> {
  try {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    }
    const csrfToken = getCSRFToken()
    if (csrfToken) {
      headers['X-CSRF-Token'] = csrfToken
    }
    const response = await fetch(`${BASE_URL}/auth/refresh`, {
      method: 'POST',
      headers,
      credentials: 'include',
    })
    if (!response.ok) return false

    let body: unknown
    try {
      body = await response.json()
    } catch {
      return false
    }
    if (!isRefreshResponseBody(body)) return false

    const { roles, username, expires_in, provider } = body.data
    useAuthStore.getState().setAuth(roles, username, expires_in, provider ?? 'local')
    return true
  } catch {
    return false
  }
}

export async function attemptRefresh(): Promise<boolean> {
  // Single-flight inside this tab — any concurrent callers await the same
  // promise so only one /auth/refresh request is in flight at a time.
  if (refreshPromise) return refreshPromise

  const locks = getWebLocks()
  const task = async (): Promise<boolean> => {
    if (locks) {
      return locks.request('allure-auth-refresh', () => performRefresh())
    }
    return performRefresh()
  }

  refreshPromise = task().finally(() => {
    refreshPromise = null
  })
  return refreshPromise
}

// ---------------------------------------------------------------------------
// Response parsing (extracted so retry path can reuse it)
// ---------------------------------------------------------------------------
async function parseSuccess<T>(response: Response): Promise<FetchResponse<T>> {
  const contentLength = response.headers.get('content-length')
  if (response.status === 204 || contentLength === '0') {
    return { data: undefined as T }
  }
  const data = (await response.json()) as T
  return { data }
}

// ---------------------------------------------------------------------------
// Request execution
// ---------------------------------------------------------------------------
interface BuiltRequest {
  fullUrl: string
  init: RequestInit
}

function buildRequest(
  method: string,
  url: string,
  body: unknown,
  config: RequestConfig | undefined,
): BuiltRequest {
  // Build URL with query params
  let fullUrl = `${BASE_URL}${url}`
  if (config?.params) {
    const searchParams = new URLSearchParams()
    for (const [key, value] of Object.entries(config.params)) {
      if (value !== undefined) {
        searchParams.append(key, String(value))
      }
    }
    const qs = searchParams.toString()
    if (qs) fullUrl += `?${qs}`
  }

  // Build headers
  const headers: Record<string, string> = {}
  const isFormData = body instanceof FormData

  // Set Content-Type for non-FormData bodies (FormData needs browser-set boundary)
  if (!isFormData) {
    headers['Content-Type'] = 'application/json'
  }

  // Merge caller-provided headers, stripping Content-Type for FormData
  if (config?.headers) {
    for (const [key, value] of Object.entries(config.headers)) {
      if (isFormData && key.toLowerCase() === 'content-type') continue
      headers[key] = value
    }
  }

  // CSRF injection on mutating methods — read cookie at build time so a
  // refresh that rotates csrf_token is picked up by the retry path.
  const mutating = method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS'
  if (mutating) {
    const csrfToken = getCSRFToken()
    if (csrfToken) {
      headers['X-CSRF-Token'] = csrfToken
    }
  }

  // Serialize body
  let serializedBody: BodyInit | undefined
  if (body instanceof FormData || body instanceof File || body instanceof Blob) {
    serializedBody = body
  } else if (body !== null && body !== undefined) {
    serializedBody = JSON.stringify(body)
  }

  return {
    fullUrl,
    init: {
      method,
      headers,
      body: serializedBody,
      credentials: 'include',
    },
  }
}

async function request<T>(
  method: string,
  url: string,
  body?: unknown,
  config?: RequestConfig,
): Promise<FetchResponse<T>> {
  const built = buildRequest(method, url, body, config)

  // Execute fetch
  const response = await fetch(built.fullUrl, built.init)

  // Handle non-ok responses
  if (!response.ok) {
    if (response.status === 401) {
      // Skip refresh for paths that cannot or must not be refreshed.
      if (!NO_REFRESH_PATHS.has(url)) {
        const refreshed = await attemptRefresh()
        if (refreshed) {
          // Rebuild the request so the rotated csrf_token cookie is picked up.
          const retryBuilt = buildRequest(method, url, body, config)
          const retryResponse = await fetch(retryBuilt.fullUrl, retryBuilt.init)
          if (retryResponse.ok) {
            return parseSuccess<T>(retryResponse)
          }
          // Retry failed — fall through and treat as normal error. If the retry
          // itself is another 401 we still surface the event so higher layers
          // can react (the refresh did not actually buy us access).
          if (retryResponse.status === 401) {
            window.dispatchEvent(new CustomEvent('allure:unauthorized'))
          }
          let retryErrorData: unknown
          try {
            retryErrorData = await retryResponse.json()
          } catch {
            retryErrorData = { message: retryResponse.statusText }
          }
          throw new ApiError(
            retryResponse.statusText || `HTTP ${retryResponse.status}`,
            { status: retryResponse.status, data: retryErrorData },
          )
        }
      }

      // Either refresh was skipped for this path, or refresh failed.
      window.dispatchEvent(new CustomEvent('allure:unauthorized'))
    }

    let errorData: unknown
    try {
      errorData = await response.json()
    } catch {
      errorData = { message: response.statusText }
    }

    throw new ApiError(response.statusText || `HTTP ${response.status}`, {
      status: response.status,
      data: errorData,
    })
  }

  return parseSuccess<T>(response)
}

// ---------------------------------------------------------------------------
// API client
// ---------------------------------------------------------------------------
export const apiClient = {
  defaults: { baseURL: BASE_URL },

  get<T>(url: string, config?: RequestConfig): Promise<FetchResponse<T>> {
    return request<T>('GET', url, undefined, config)
  },

  post<T>(url: string, data?: unknown, config?: RequestConfig): Promise<FetchResponse<T>> {
    return request<T>('POST', url, data, config)
  },

  put<T>(url: string, data?: unknown, config?: RequestConfig): Promise<FetchResponse<T>> {
    return request<T>('PUT', url, data, config)
  },

  patch<T>(url: string, data?: unknown, config?: RequestConfig): Promise<FetchResponse<T>> {
    return request<T>('PATCH', url, data, config)
  },

  delete<T>(url: string, data?: unknown, config?: RequestConfig): Promise<FetchResponse<T>> {
    return request<T>('DELETE', url, data, config)
  },
}

// ---------------------------------------------------------------------------
// Error helper
// ---------------------------------------------------------------------------
export function extractErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    const body = error.response?.data as { metadata?: { message?: string } } | undefined
    if (body?.metadata?.message) return body.metadata.message
    return error.message
  }
  if (error instanceof Error) return error.message
  return 'An unexpected error occurred'
}
