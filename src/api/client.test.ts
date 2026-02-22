import { describe, it, expect, vi, beforeEach } from 'vitest'
import {
  setAccessToken,
  getAccessToken,
  extractErrorMessage,
} from './client'
import axios from 'axios'

describe('token management', () => {
  beforeEach(() => {
    setAccessToken(null)
  })

  it('stores and retrieves access token', () => {
    setAccessToken('abc123')
    expect(getAccessToken()).toBe('abc123')
  })

  it('clears access token when set to null', () => {
    setAccessToken('abc123')
    setAccessToken(null)
    expect(getAccessToken()).toBeNull()
  })
})

describe('extractErrorMessage', () => {
  it('extracts meta_data.message from Axios error', () => {
    const error = {
      isAxiosError: true,
      response: {
        data: { meta_data: { message: 'Invalid credentials' } },
      },
      message: 'Request failed',
    }
    vi.spyOn(axios, 'isAxiosError').mockReturnValue(true)
    const msg = extractErrorMessage(error)
    expect(msg).toBe('Invalid credentials')
    vi.restoreAllMocks()
  })

  it('falls back to error.message when meta_data absent', () => {
    const error = {
      isAxiosError: true,
      response: { data: {} },
      message: 'Network Error',
    }
    vi.spyOn(axios, 'isAxiosError').mockReturnValue(true)
    const msg = extractErrorMessage(error)
    expect(msg).toBe('Network Error')
    vi.restoreAllMocks()
  })

  it('extracts message from standard Error', () => {
    vi.spyOn(axios, 'isAxiosError').mockReturnValue(false)
    const msg = extractErrorMessage(new Error('Something went wrong'))
    expect(msg).toBe('Something went wrong')
    vi.restoreAllMocks()
  })

  it('returns generic message for unknown error', () => {
    vi.spyOn(axios, 'isAxiosError').mockReturnValue(false)
    const msg = extractErrorMessage('oops')
    expect(msg).toBe('An unexpected error occurred')
    vi.restoreAllMocks()
  })
})
