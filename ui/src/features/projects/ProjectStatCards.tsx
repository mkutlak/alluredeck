import { CheckCircle2, BarChart3, Clock, XCircle } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { formatDate, formatDuration } from '@/lib/utils'
import { getPassRateColorClass, STATUS_TEXT_CLASSES } from '@/lib/status-colors'
import type { ReportHistoryEntry, PaginationMeta, AllureStatistic } from '@/types/api'

interface ProjectStatCardsProps {
  isLoading: boolean
  stat: AllureStatistic | null | undefined
  passRate: number | null
  adjustedPassRate: number | null
  knownCount: number
  latest: ReportHistoryEntry | undefined
  pagination: PaginationMeta | undefined
}

export function ProjectStatCards({
  isLoading,
  stat,
  passRate,
  adjustedPassRate,
  knownCount,
  latest,
  pagination,
}: ProjectStatCardsProps) {
  if (isLoading) {
    return (
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <Skeleton key={i} className="h-28 w-full rounded-lg" />
        ))}
      </div>
    )
  }

  return (
    <div className="grid grid-cols-2 gap-4 sm:grid-cols-2 lg:grid-cols-4">
      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-muted-foreground text-sm font-medium">Pass Rate</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <CheckCircle2 size={20} className={STATUS_TEXT_CLASSES.passed} />
            <span
              className={`text-2xl font-bold ${passRate !== null ? getPassRateColorClass(passRate) : ''}`}
            >
              {passRate !== null ? `${passRate}%` : '—'}
            </span>
          </div>
          {adjustedPassRate !== null && adjustedPassRate !== passRate && (
            <p className="text-muted-foreground mt-1 text-xs">
              {adjustedPassRate}% adjusted
              <span className="ml-1 text-xs opacity-70">(excl. {knownCount} known)</span>
            </p>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-muted-foreground text-sm font-medium">Total Tests</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <BarChart3 size={20} className="text-blue-600" />
            <span className="text-2xl font-bold">{stat?.total ?? '—'}</span>
          </div>
          {stat && (
            <div className="mt-1 flex flex-wrap gap-1">
              <Badge variant="passed" className="text-xs">
                {stat.passed} passed
              </Badge>
              <Badge variant="failed" className="text-xs">
                {stat.failed} failed
              </Badge>
              <Badge variant="broken" className="text-xs">
                {stat.broken} broken
              </Badge>
              <Badge variant="secondary" className="text-xs">
                {stat.skipped} skipped
              </Badge>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-muted-foreground text-sm font-medium">Last Duration</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <Clock size={20} className="text-purple-600" />
            <span className="text-2xl font-bold">
              {latest?.duration_ms ? formatDuration(latest.duration_ms) : '—'}
            </span>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-2">
          <CardTitle className="text-muted-foreground text-sm font-medium">Last Run</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex items-center gap-2">
            <XCircle size={20} className="text-orange-500" />
            <span className="text-sm font-medium">
              {latest?.generated_at ? formatDate(latest.generated_at) : '—'}
            </span>
          </div>
          {pagination && pagination.total > 0 && (
            <p className="text-muted-foreground mt-1 text-xs">
              {pagination.total} report{pagination.total !== 1 ? 's' : ''} total
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
