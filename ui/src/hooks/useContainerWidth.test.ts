import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useContainerWidth } from './useContainerWidth'

let resizeCallback: ResizeObserverCallback
const mockObserve = vi.fn()
const mockDisconnect = vi.fn()

beforeEach(() => {
  vi.stubGlobal(
    'ResizeObserver',
    vi.fn(function (cb: ResizeObserverCallback) {
      resizeCallback = cb
      return { observe: mockObserve, disconnect: mockDisconnect, unobserve: vi.fn() }
    }),
  )
})

afterEach(() => {
  vi.unstubAllGlobals()
  mockObserve.mockReset()
  mockDisconnect.mockReset()
})

describe('useContainerWidth', () => {
  it('returns 0 when ref.current is null', () => {
    const ref = { current: null }
    const { result } = renderHook(() => useContainerWidth(ref))
    expect(result.current).toBe(0)
  })

  it('returns observed width when ResizeObserver fires', () => {
    const el = document.createElement('div')
    Object.defineProperty(el, 'clientWidth', { value: 400, configurable: true })
    const ref = { current: el }

    const { result } = renderHook(() => useContainerWidth(ref))

    act(() => {
      resizeCallback(
        [{ contentRect: { width: 400 } } as ResizeObserverEntry],
        {} as ResizeObserver,
      )
    })

    expect(result.current).toBe(400)
  })

  it('updates width when container resizes', () => {
    const el = document.createElement('div')
    Object.defineProperty(el, 'clientWidth', { value: 200, configurable: true })
    const ref = { current: el }

    const { result } = renderHook(() => useContainerWidth(ref))

    act(() => {
      resizeCallback(
        [{ contentRect: { width: 200 } } as ResizeObserverEntry],
        {} as ResizeObserver,
      )
    })
    expect(result.current).toBe(200)

    act(() => {
      resizeCallback(
        [{ contentRect: { width: 800 } } as ResizeObserverEntry],
        {} as ResizeObserver,
      )
    })
    expect(result.current).toBe(800)
  })

  it('cleans up ResizeObserver on unmount', () => {
    const el = document.createElement('div')
    const ref = { current: el }

    const { unmount } = renderHook(() => useContainerWidth(ref))
    unmount()

    expect(mockDisconnect).toHaveBeenCalled()
  })
})
