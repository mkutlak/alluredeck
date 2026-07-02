import * as React from 'react'
import { type LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

export interface SegmentedOption<T extends string> {
  value: T
  label: React.ReactNode
  /** Renders as "Label (3)" — a muted count suffix. */
  count?: number
  /** Optional leading icon. */
  icon?: LucideIcon
  /** Per-option testid passthrough. */
  'data-testid'?: string
}

export interface SegmentedProps<T extends string> {
  value: T
  onValueChange: (value: T) => void
  options: SegmentedOption<T>[]
  size?: 'sm' | 'xs'
  'aria-label': string
  className?: string
}

const sizeClasses = {
  sm: 'h-8 px-3 text-xs',
  xs: 'h-7 px-2 text-xs',
}

export function Segmented<T extends string>({
  value,
  onValueChange,
  options,
  size = 'sm',
  'aria-label': ariaLabel,
  className,
}: SegmentedProps<T>) {
  return (
    <div
      role="group"
      aria-label={ariaLabel}
      className={cn('inline-flex items-center gap-0.5 rounded-md border p-0.5', className)}
    >
      {options.map((option) => {
        const isActive = option.value === value
        const Icon = option.icon
        return (
          <button
            key={option.value}
            type="button"
            aria-pressed={isActive}
            data-testid={option['data-testid']}
            onClick={() => onValueChange(option.value)}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-sm font-medium whitespace-nowrap transition-colors',
              'hover:bg-accent hover:text-accent-foreground',
              sizeClasses[size],
              isActive && 'bg-muted font-semibold',
            )}
          >
            {Icon && <Icon className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />}
            {option.label}
            {option.count !== undefined && (
              <span className="text-muted-foreground">{` (${option.count})`}</span>
            )}
          </button>
        )
      })}
    </div>
  )
}
