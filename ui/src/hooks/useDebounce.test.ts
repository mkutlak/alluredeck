import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useDebounce } from './useDebounce'

describe('useDebounce', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('returns initial value immediately', () => {
    const { result } = renderHook(() => useDebounce('hello', 300))
    expect(result.current).toBe('hello')
  })

  it('does not update value before delay', () => {
    const { result, rerender } = renderHook(({ value, delay }) => useDebounce(value, delay), {
      initialProps: { value: 'hello', delay: 300 },
    })

    rerender({ value: 'world', delay: 300 })
    act(() => {
      vi.advanceTimersByTime(200)
    })

    expect(result.current).toBe('hello')
  })

  it('updates value after delay', () => {
    const { result, rerender } = renderHook(({ value, delay }) => useDebounce(value, delay), {
      initialProps: { value: 'hello', delay: 300 },
    })

    rerender({ value: 'world', delay: 300 })
    act(() => {
      vi.advanceTimersByTime(300)
    })

    expect(result.current).toBe('world')
  })

  it('resets timer on rapid changes', () => {
    const { result, rerender } = renderHook(({ value, delay }) => useDebounce(value, delay), {
      initialProps: { value: 'a', delay: 300 },
    })

    rerender({ value: 'b', delay: 300 })
    act(() => {
      vi.advanceTimersByTime(100)
    })
    rerender({ value: 'c', delay: 300 })
    act(() => {
      vi.advanceTimersByTime(100)
    })

    // Only 200ms since last change — should still be 'a'
    expect(result.current).toBe('a')

    act(() => {
      vi.advanceTimersByTime(200)
    })

    // 300ms since last change — should now be 'c'
    expect(result.current).toBe('c')
  })
})
