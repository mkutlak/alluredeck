import { useMemo } from 'react'
import { useTheme } from 'next-themes'
import { STATUS_COLORS, STATUS_DARK_COLORS } from '@/lib/status-colors'

export type StatusColorMap = Record<keyof typeof STATUS_COLORS, string>

export function useStatusColors(): StatusColorMap {
  const { resolvedTheme } = useTheme()
  return useMemo(
    () => (resolvedTheme === 'dark' ? STATUS_DARK_COLORS : STATUS_COLORS),
    [resolvedTheme],
  )
}
