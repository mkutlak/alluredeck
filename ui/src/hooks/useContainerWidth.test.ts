import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useContainerWidth } from './useContainerWidth'

let resizeCallback: ResizeObserverCallback

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('useContainerWidth', () => {
  it('returns 0 when ref.current is null', () => {
    const { result } = renderHook(() => useContainerWidth({ current: null }))
    expect(result.current).toBe(0)
  })

  it('updates width when container resizes', () => {
    vi.stubGlobal(
      'ResizeObserver',
      vi.fn(function (cb: ResizeObserverCallback) {
        resizeCallback = cb
        return { observe: vi.fn(), disconnect: vi.fn(), unobserve: vi.fn() }
      }),
    )
    const ref = { current: document.createElement('div') }
    const { result } = renderHook(() => useContainerWidth(ref))
    const resize = (width: number) =>
      act(() =>
        resizeCallback([{ contentRect: { width } } as ResizeObserverEntry], {} as ResizeObserver),
      )

    resize(200)
    expect(result.current).toBe(200)
    resize(800)
    expect(result.current).toBe(800)
  })
})
