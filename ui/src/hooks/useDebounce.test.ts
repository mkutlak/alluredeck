import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useDebounce } from './useDebounce'

describe('useDebounce', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns the latest value only once it has been stable for the delay', () => {
    vi.useFakeTimers()
    const { result, rerender } = renderHook(({ value }) => useDebounce(value, 300), {
      initialProps: { value: 'a' },
    })
    expect(result.current).toBe('a')

    // Each change restarts the timer.
    rerender({ value: 'b' })
    act(() => {
      vi.advanceTimersByTime(200)
    })
    rerender({ value: 'c' })
    act(() => {
      vi.advanceTimersByTime(200)
    })
    expect(result.current).toBe('a')

    act(() => {
      vi.advanceTimersByTime(100)
    })
    expect(result.current).toBe('c')
  })
})
