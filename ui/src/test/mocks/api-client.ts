import { vi } from 'vitest'

/**
 * Shared mock for @/api/client.
 *
 * Call this at module scope in test files that import modules depending on
 * apiClient or extractErrorMessage:
 *
 *   import { mockApiClient } from '@/test/mocks/api-client'
 *   mockApiClient()
 *
 * The mock registers a vi.mock() factory that replaces apiClient with
 * jest-style spy functions covering all HTTP verbs used in the codebase.
 * extractErrorMessage mirrors the real implementation so error-path tests
 * remain meaningful without hitting the network.
 */
export function mockApiClient(): void {
  vi.mock('@/api/client', () => ({
    apiClient: {
      get: vi.fn(),
      post: vi.fn(),
      put: vi.fn(),
      patch: vi.fn(),
      delete: vi.fn(),
    },
    extractErrorMessage: (e: unknown) => (e instanceof Error ? e.message : String(e)),
  }))
}
