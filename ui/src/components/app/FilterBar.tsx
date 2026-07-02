import { type ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface FilterBarProps {
  search?: ReactNode
  filters?: ReactNode
  end?: ReactNode
  className?: string
}

export function FilterBar({ search, filters, end, className }: FilterBarProps) {
  return (
    <div className={cn('flex flex-wrap items-center gap-3', className)}>
      {search}
      {filters}
      {end && <div className="ml-auto flex items-center gap-2">{end}</div>}
    </div>
  )
}
