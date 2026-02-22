import axios, { type AxiosError } from 'axios'
import { env } from '@/lib/env'

// ---------------------------------------------------------------------------
// In-memory token storage — never written to localStorage
// ---------------------------------------------------------------------------
let _accessToken: string | null = null

export function setAccessToken(token: string | null): void {
  _accessToken = token
}

export function getAccessToken(): string | null {
  return _accessToken
}

// ---------------------------------------------------------------------------
// Axios instance
// ---------------------------------------------------------------------------
export const apiClient = axios.create({
  baseURL: env.apiUrl,
  headers: { 'Content-Type': 'application/json' },
  withCredentials: true, // send httpOnly cookies automatically
})

// Attach Bearer token when available
apiClient.interceptors.request.use((config) => {
  if (_accessToken) {
    config.headers.Authorization = `Bearer ${_accessToken}`
  }
  return config
})

// On 401, fire a custom DOM event so the AuthProvider can react
apiClient.interceptors.response.use(
  (response) => response,
  (error: AxiosError) => {
    if (error.response?.status === 401) {
      setAccessToken(null)
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
    const body = error.response?.data as { meta_data?: { message?: string } } | undefined
    if (body?.meta_data?.message) return body.meta_data.message
    return error.message
  }
  if (error instanceof Error) return error.message
  return 'An unexpected error occurred'
}
