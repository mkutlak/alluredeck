import { useParams, useSearchParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { fetchTestHistory } from '@/api/test-history'
import { queryKeys } from '@/lib/query-keys'
import { formatDate, formatDuration, getStatusVariant } from '@/lib/utils'
import type { TestHistoryEntry } from '@/types/api'
import { Badge } from '@/components/ui/badge'
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

export function TestHistoryPage() {
  const { id: projectId } = useParams<{ id: string }>()
  const [searchParams] = useSearchParams()

  const historyId = searchParams.get('history_id') ?? ''
  const branch = searchParams.get('branch') ?? undefined

  if (!historyId) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
        <p className="text-lg font-medium text-destructive">Missing test history ID</p>
        <p className="text-sm text-muted-foreground">
          A <code>history_id</code> query parameter is required.
        </p>
      </div>
    )
  }

  return (
    <TestHistoryContent projectId={projectId ?? ''} historyId={historyId} branch={branch} />
  )
}

interface TestHistoryContentProps {
  projectId: string
  historyId: string
  branch: string | undefined
}

function TestHistoryContent({ projectId, historyId, branch }: TestHistoryContentProps) {
  const { data, isLoading, isError } = useQuery({
    queryKey: queryKeys.tests.history(projectId, historyId, branch),
    queryFn: () => fetchTestHistory(projectId, historyId, branch),
    enabled: !!projectId && !!historyId,
  })

  if (isLoading) {
    return (
      <div className="space-y-6 p-6">
        <Skeleton className="h-8 w-64" data-testid="history-skeleton" />
        <Skeleton className="h-4 w-32" data-testid="history-skeleton" />
        <Skeleton className="h-64" data-testid="history-skeleton" />
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
        <p className="text-lg font-medium text-destructive">Failed to load test history</p>
        <p className="text-sm text-muted-foreground">
          There was a problem fetching the history for this test.
        </p>
      </div>
    )
  }

  const history = data?.history ?? []

  return (
    <div className="space-y-6 p-6">
      {/* Header */}
      <div className="flex flex-wrap items-start gap-3">
        <div className="flex-1 space-y-1">
          <h1 className="text-xl font-semibold">Test History</h1>
          <h2 className="font-mono text-sm text-muted-foreground">{historyId}</h2>
        </div>
        {branch !== undefined && (
          <Badge variant="outline" className="mt-1">
            {branch}
          </Badge>
        )}
      </div>

      {/* Trend summary */}
      <Card>
        <CardHeader className="pb-1 pt-4">
          <CardTitle className="text-sm font-medium text-muted-foreground">Trend</CardTitle>
        </CardHeader>
        <CardContent className="pb-4">
          <p className="text-sm text-muted-foreground">
            {history.length} build{history.length !== 1 ? 's' : ''} shown
          </p>
        </CardContent>
      </Card>

      {/* Table or empty state */}
      {history.length === 0 ? (
        <div className="flex flex-col items-center justify-center rounded-lg border border-dashed py-16 text-center">
          <p className="text-sm text-muted-foreground">No history found for this test.</p>
        </div>
      ) : (
        <div className="rounded-lg border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-24">Build #</TableHead>
                <TableHead className="w-28">Status</TableHead>
                <TableHead className="w-28">Duration</TableHead>
                <TableHead>Date</TableHead>
                <TableHead className="w-28">Commit</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {history.map((entry: TestHistoryEntry) => (
                <TableRow key={entry.build_id}>
                  <TableCell className="font-medium">#{entry.build_order}</TableCell>
                  <TableCell>
                    <Badge variant={getStatusVariant(entry.status)}>{entry.status}</Badge>
                  </TableCell>
                  <TableCell>{formatDuration(entry.duration_ms)}</TableCell>
                  <TableCell className="text-muted-foreground">
                    {formatDate(entry.created_at)}
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {entry.ci_commit_sha ? entry.ci_commit_sha.slice(0, 7) : '—'}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
