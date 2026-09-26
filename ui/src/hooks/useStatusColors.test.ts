import { describe, it, expect, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useTheme } from 'next-themes'
import { STATUS_COLORS, STATUS_DARK_COLORS } from '@/lib/status-colors'
import { useStatusColors } from './useStatusColors'

vi.mock('next-themes', () => ({
  useTheme: vi.fn(),
}))

describe('useStatusColors', () => {
  // Only a resolved dark theme picks the dark palette; before hydration it is light.
  it.each([
    ['dark', STATUS_DARK_COLORS],
    ['light', STATUS_COLORS],
    [undefined, STATUS_COLORS],
  ])('resolvedTheme %s selects its palette', (resolvedTheme, expected) => {
    vi.mocked(useTheme).mockReturnValue({ resolvedTheme, themes: [], setTheme: vi.fn() })
    expect(renderHook(() => useStatusColors()).result.current).toBe(expected)
  })
})
