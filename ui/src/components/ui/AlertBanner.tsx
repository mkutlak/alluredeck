import { type ReactNode } from 'react'
import { cn } from '@/lib/utils'

export function AlertBanner({
  variant,
  children,
  className,
}: {
  variant: 'info' | 'warning'
  children: ReactNode
  className?: string
}) {
  return (
    <div
      role={variant === 'info' ? 'status' : 'alert'}
      className={cn(
        'rounded-md border px-4 py-3 text-sm',
        variant === 'info' && 'border-info/30 bg-info/10 text-info',
        variant === 'warning' && 'border-warning/30 bg-warning/10 text-warning',
        className,
      )}
    >
      {children}
    </div>
  )
}
