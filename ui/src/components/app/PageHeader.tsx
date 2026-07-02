import { type ReactNode } from 'react'
import { cn } from '@/lib/utils'

export interface PageHeaderProps {
  title: ReactNode
  /** default 'mono' (project pages); 'sans' for global pages */
  titleVariant?: 'mono' | 'sans'
  /** muted text-sm line under title */
  subtitle?: ReactNode
  /** optional row under subtitle (e.g. stat chips) */
  meta?: ReactNode
  /** right-aligned, centered with title */
  actions?: ReactNode
  /** full-width row below */
  toolbar?: ReactNode
}

export function PageHeader({
  title,
  titleVariant = 'mono',
  subtitle,
  meta,
  actions,
  toolbar,
}: PageHeaderProps) {
  return (
    <header className="space-y-2">
      <div className="flex items-start justify-between gap-4">
        <div className="min-w-0">
          <h1 className={cn('text-2xl font-semibold', titleVariant !== 'sans' && 'font-mono')}>
            {title}
          </h1>
          {subtitle && <p className="text-muted-foreground text-sm">{subtitle}</p>}
        </div>
        {actions && <div className="flex shrink-0 items-center gap-2">{actions}</div>}
      </div>
      {meta}
      {toolbar}
    </header>
  )
}
