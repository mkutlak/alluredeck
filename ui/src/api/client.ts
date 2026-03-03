import axios, { type AxiosError } from 'axios'
import { env } from '@/lib/env'

// ---------------------------------------------------------------------------
// Axios instance
// ---------------------------------------------------------------------------
export const apiClient = axios.create({
  baseURL: env.apiUrl,
  headers: { 'Content-Type': 'application/json' },
  withCredentials: true, // send httpOnly cookies automatically
})

// Read csrf_token cookie for double-submit CSRF pattern (REVIEW #11).
function getCSRFToken(): string | null {
  const match = document.cookie.match(/(?:^|;\s*)csrf_token=([^;]+)/)
  return match ? decodeURIComponent(match[1]) : null
}

// Attach CSRF header for state-changing requests
apiClient.interceptors.request.use((config) => {
  const method = config.method?.toUpperCase()
  if (method && method !== 'GET' && method !== 'HEAD' && method !== 'OPTIONS') {
    const csrfToken = getCSRFToken()
    if (csrfToken) {
      config.headers['X-CSRF-Token'] = csrfToken
    }
  }

  return config
})

// On 401, fire a custom DOM event so the AuthProvider can react
apiClient.interceptors.response.use(
  (response) => response,
  (error: AxiosError) => {
    if (error.response?.status === 401) {
      window.dispatchEvent(new CustomEvent('allure:unauthorized'))
    }
    return Promise.reject(error)
  },
)

// ---------------------------------------------------------------------------
// Error helper
// ---------------------------------------------------------------------------
export function extractErrorMessage(error: unknown): string {
  if (axios.isAxiosError(error)) {
    const body = error.response?.data as { metadata?: { message?: string } } | undefined
    if (body?.metadata?.message) return body.metadata.message
    return error.message
  }
  if (error instanceof Error) return error.message
  return 'An unexpected error occurred'
}
