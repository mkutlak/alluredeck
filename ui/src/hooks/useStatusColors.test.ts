import { describe, it, expect, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { STATUS_COLORS, STATUS_DARK_COLORS } from '@/lib/status-colors'
import { useStatusColors } from './useStatusColors'

vi.mock('next-themes', () => ({
  useTheme: vi.fn(),
}))

import { useTheme } from 'next-themes'

const mockUseTheme = vi.mocked(useTheme)

describe('useStatusColors', () => {
  it('returns STATUS_COLORS when theme is light', () => {
    mockUseTheme.mockReturnValue({ resolvedTheme: 'light', theme: 'light', setTheme: vi.fn(), themes: [], systemTheme: undefined })
    const { result } = renderHook(() => useStatusColors())
    expect(result.current).toBe(STATUS_COLORS)
  })

  it('returns STATUS_DARK_COLORS when theme is dark', () => {
    mockUseTheme.mockReturnValue({ resolvedTheme: 'dark', theme: 'dark', setTheme: vi.fn(), themes: [], systemTheme: undefined })
    const { result } = renderHook(() => useStatusColors())
    expect(result.current).toBe(STATUS_DARK_COLORS)
  })

  it('returns STATUS_COLORS when theme is system (fallback)', () => {
    mockUseTheme.mockReturnValue({ resolvedTheme: 'system', theme: 'system', setTheme: vi.fn(), themes: [], systemTheme: undefined })
    const { result } = renderHook(() => useStatusColors())
    expect(result.current).toBe(STATUS_COLORS)
  })

  it('returns STATUS_COLORS when theme is undefined (before hydration)', () => {
    mockUseTheme.mockReturnValue({ resolvedTheme: undefined, theme: undefined, setTheme: vi.fn(), themes: [], systemTheme: undefined })
    const { result } = renderHook(() => useStatusColors())
    expect(result.current).toBe(STATUS_COLORS)
  })
})
