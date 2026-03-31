import { env } from '@/lib/env'

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

async function request<T>(
  method: string,
  url: string,
  body?: unknown,
  config?: RequestConfig,
): Promise<FetchResponse<T>> {
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

  // CSRF injection on mutating methods
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

  // Execute fetch
  const response = await fetch(fullUrl, {
    method,
    headers,
    body: serializedBody,
    credentials: 'include',
  })

  // Handle non-ok responses
  if (!response.ok) {
    if (response.status === 401) {
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

  // Handle empty responses (204 No Content)
  const contentLength = response.headers.get('content-length')
  if (response.status === 204 || contentLength === '0') {
    return { data: undefined as T }
  }

  // Parse JSON response
  const data = (await response.json()) as T
  return { data }
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

  delete<T>(url: string, config?: RequestConfig): Promise<FetchResponse<T>> {
    return request<T>('DELETE', url, undefined, config)
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
