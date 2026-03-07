import { useMemo, useState } from 'react'
import { Link, useParams, useSearchParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { fetchBuildComparison } from '@/api/reports'
import { queryKeys } from '@/lib/query-keys'
import { formatDuration, getStatusVariant } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import type { DiffCategory } from '@/types/api'

type FilterValue = DiffCategory | 'all'

const CATEGORY_LABELS: Record<DiffCategory, string> = {
  regressed: 'Regressed',
  fixed: 'Fixed',
  added: 'Added',
  removed: 'Removed',
}

const CATEGORY_VARIANTS: Record<DiffCategory, string> = {
  regressed: 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400',
  fixed: 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400',
  added: 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400',
  removed: 'bg-zinc-100 text-zinc-800 dark:bg-zinc-800 dark:text-zinc-400',
}

function DiffCategoryBadge({ category }: { category: DiffCategory }) {
  return (
    <span
      className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium ${CATEGORY_VARIANTS[category]}`}
    >
      {CATEGORY_LABELS[category]}
    </span>
  )
}

function DurationDelta({ delta }: { delta: number }) {
  if (delta === 0) return <span className="text-muted-foreground">—</span>
  const sign = delta > 0 ? '+' : ''
  const cls = delta > 0 ? 'text-red-600 dark:text-red-400' : 'text-green-600 dark:text-green-400'
  return <span className={cls}>{sign}{formatDuration(Math.abs(delta))}</span>
}

export function ComparePage() {
  const { id: projectId } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()
  const [activeFilter, setActiveFilter] = useState<FilterValue>('all')

  const buildA = parseInt(searchParams.get('a') ?? '', 10)
  const buildB = parseInt(searchParams.get('b') ?? '', 10)
  const paramsValid = !isNaN(buildA) && !isNaN(buildB) && buildA > 0 && buildB > 0

  const { data, isLoading } = useQuery({
    queryKey: queryKeys.buildComparison(projectId ?? '', buildA, buildB),
    queryFn: () => fetchBuildComparison(projectId!, buildA, buildB),
    enabled: paramsValid && !!projectId,
  })

  const filteredTests = useMemo(() => {
    if (!data) return []
    if (activeFilter === 'all') return data.tests
    return data.tests.filter((t) => t.category === activeFilter)
  }, [data, activeFilter])

  if (!paramsValid) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
        <p className="text-lg font-medium text-destructive">Invalid comparison parameters</p>
        <p className="text-sm text-muted-foreground">
          Both <code>a</code> and <code>b</code> query parameters must be positive integers.
        </p>
        <Button asChild variant="outline" size="sm">
          <Link to={`/projects/${projectId}`}>Back to project</Link>
        </Button>
      </div>
    )
  }

  if (isLoading) {
    return (
      <div className="space-y-6 p-6">
        <div className="flex items-center gap-3">
          <Skeleton className="h-4 w-20" data-testid="compare-skeleton" />
          <Skeleton className="h-6 w-48" data-testid="compare-skeleton" />
        </div>
        <div className="grid grid-cols-4 gap-4">
          {[...Array(4)].map((_, i) => (
            <Skeleton key={i} className="h-24" data-testid="compare-skeleton" />
          ))}
        </div>
        <Skeleton className="h-64" data-testid="compare-skeleton" />
      </div>
    )
  }

  const summary = data?.summary ?? { regressed: 0, fixed: 0, added: 0, removed: 0, total: 0 }
  const categories: DiffCategory[] = ['regressed', 'fixed', 'added', 'removed']

  return (
    <TooltipProvider>
      <div className="space-y-6 p-6">
        {/* Header */}
        <div className="flex items-center gap-3">
          <Button asChild variant="ghost" size="sm">
            <Link to={`/projects/${projectId}`}>← Back</Link>
          </Button>
          <div>
            <h1 className="text-xl font-semibold">
              Build #{buildA} vs Build #{buildB}
            </h1>
            <p className="text-sm text-muted-foreground">
              {summary.total} difference{summary.total !== 1 ? 's' : ''} found
            </p>
          </div>
        </div>

        {/* Summary cards */}
        <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
          {categories.map((cat) => (
            <Card key={cat}>
              <CardHeader className="pb-1 pt-4">
                <CardTitle className="text-sm font-medium text-muted-foreground">
                  {CATEGORY_LABELS[cat]}
                </CardTitle>
              </CardHeader>
              <CardContent className="pb-4">
                <p className="text-3xl font-bold">{summary[cat]}</p>
              </CardContent>
            </Card>
          ))}
        </div>

        {/* Filter bar */}
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            variant={activeFilter === 'all' ? 'default' : 'outline'}
            onClick={() => setActiveFilter('all')}
          >
            All ({summary.total})
          </Button>
          {categories.map((cat) => (
            <Button
              key={cat}
              size="sm"
              variant={activeFilter === cat ? 'default' : 'outline'}
              onClick={() => setActiveFilter(cat)}
            >
              {CATEGORY_LABELS[cat]} ({summary[cat]})
            </Button>
          ))}
        </div>

        {/* Diff table */}
        {filteredTests.length === 0 ? (
          <div className="flex flex-col items-center justify-center rounded-lg border border-dashed py-16 text-center">
            <p className="text-sm text-muted-foreground">No differences found between these builds.</p>
          </div>
        ) : (
          <div className="rounded-lg border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Test Name</TableHead>
                  <TableHead className="w-28">Build A</TableHead>
                  <TableHead className="w-28">Build B</TableHead>
                  <TableHead className="w-28">Category</TableHead>
                  <TableHead className="w-28 text-right">Duration Δ</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {filteredTests.map((entry) => (
                  <TableRow key={entry.history_id}>
                    <TableCell>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className="cursor-default font-medium">{entry.test_name}</span>
                        </TooltipTrigger>
                        <TooltipContent>
                          <p className="max-w-xs break-all text-xs">{entry.full_name}</p>
                        </TooltipContent>
                      </Tooltip>
                    </TableCell>
                    <TableCell>
                      {entry.status_a ? (
                        <Badge variant={getStatusVariant(entry.status_a)}>
                          {entry.status_a}
                        </Badge>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell>
                      {entry.status_b ? (
                        <Badge variant={getStatusVariant(entry.status_b)}>
                          {entry.status_b}
                        </Badge>
                      ) : (
                        <span className="text-muted-foreground">—</span>
                      )}
                    </TableCell>
                    <TableCell>
                      <DiffCategoryBadge category={entry.category} />
                    </TableCell>
                    <TableCell className="text-right">
                      <DurationDelta delta={entry.duration_delta} />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>
    </TooltipProvider>
  )
}
