import React from 'react'

interface AnalyticsSectionProps {
  title: string
  isEmpty?: boolean
  children: React.ReactNode
}

export function AnalyticsSection({ title, isEmpty, children }: AnalyticsSectionProps) {
  if (isEmpty) return null

  return (
    <section className="space-y-4">
      <div className="flex items-center gap-3">
        <h2 className="text-muted-foreground text-sm font-medium tracking-wide uppercase">
          {title}
        </h2>
        <div className="bg-border h-px flex-1" />
      </div>
      {children}
    </section>
  )
}
