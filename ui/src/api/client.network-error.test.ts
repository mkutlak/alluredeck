import { describe, it, expect } from 'vitest'

// Tests for NetworkError class and extractErrorMessage mapping.
// Uses dynamic import + vi.resetModules() to match the existing client.test.ts pattern.

describe('NetworkError + extractErrorMessage', () => {
  async function getModule() {
    return import('./client')
  }

  it('NetworkError carries name "NetworkError"', async () => {
    const { NetworkError } = await getModule()
    const err = new NetworkError('down')
    expect(err.name).toBe('NetworkError')
    expect(err.message).toBe('down')
    expect(err instanceof Error).toBe(true)
  })

  it('NetworkError stores optional cause', async () => {
    const { NetworkError } = await getModule()
    const cause = new TypeError('Failed to fetch')
    const err = new NetworkError('connection refused', cause)
    expect(err.cause).toBe(cause)
  })

  it('extractErrorMessage maps NetworkError to the canonical connection message', async () => {
    const { NetworkError, extractErrorMessage } = await getModule()
    const err = new NetworkError('Request timed out after 30s — the server may be overloaded.')
    expect(extractErrorMessage(err)).toBe(
      'Cannot reach AllureDeck API — check your connection or the server.',
    )
  })

  it('extractErrorMessage still handles ApiError correctly', async () => {
    const { ApiError, extractErrorMessage } = await getModule()
    const err = new ApiError('Bad request', {
      status: 400,
      data: { metadata: { message: 'Field required' } },
    })
    expect(extractErrorMessage(err)).toBe('Field required')
  })

  it('extractErrorMessage handles plain Error after NetworkError branch', async () => {
    const { extractErrorMessage } = await getModule()
    expect(extractErrorMessage(new Error('plain error'))).toBe('plain error')
  })

  it('extractErrorMessage returns generic message for non-Error values', async () => {
    const { extractErrorMessage } = await getModule()
    expect(extractErrorMessage(null)).toBe('An unexpected error occurred')
    expect(extractErrorMessage(42)).toBe('An unexpected error occurred')
  })
})
