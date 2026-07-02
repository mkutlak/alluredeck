import { Badge } from '@/components/ui/badge'

export type TestStatus = 'passed' | 'failed' | 'broken' | 'skipped' | 'unknown'

const KNOWN_STATUSES: ReadonlySet<string> = new Set(['passed', 'failed', 'broken', 'skipped'])

export function StatusBadge({
  status,
  className,
}: {
  status: TestStatus | string
  className?: string
}) {
  const variant = KNOWN_STATUSES.has(status)
    ? (status as 'passed' | 'failed' | 'broken' | 'skipped')
    : 'secondary'
  return (
    <Badge variant={variant} className={className}>
      {status}
    </Badge>
  )
}
