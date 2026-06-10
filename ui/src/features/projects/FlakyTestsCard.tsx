import { useQuery } from '@tanstack/react-query'
import { fetchReportStability } from '@/api/reports'
import { queryKeys } from '@/lib/query-keys'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { CardState } from '@/components/ui/CardState'

interface Props {
  projectId: string
}

export function FlakyTestsCard({ projectId }: Props) {
  const { data: stability, isLoading, isError, error, refetch } = useQuery({
    queryKey: queryKeys.reportStability(projectId),
    queryFn: () => fetchReportStability(projectId),
    staleTime: 30_000,
  })

  const flakyTests = stability?.flaky_tests ?? []
  const summary = stability?.summary

  // Hide the card entirely when there are no flaky tests and no error — don't
  // show an empty card that would confuse users.
  if (!isLoading && !isError && flakyTests.length === 0) {
    return null
  }

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium">
          Flaky Tests
          {summary && summary.flaky_count > 0 && (
            <Badge variant="secondary" className="ml-2 text-xs">
              {summary.flaky_count}
            </Badge>
          )}
        </CardTitle>
      </CardHeader>
      <CardContent>
        <CardState
          isLoading={isLoading}
          isError={isError}
          error={error}
          isEmpty={flakyTests.length === 0}
          refetch={refetch}
          skeletonRows={3}
          emptyMessage="No flaky tests detected"
        >
          <div className="space-y-2">
            {flakyTests.map((test) => (
              <div key={test.full_name} className="flex items-center justify-between gap-2">
                <span className="truncate text-sm" title={test.full_name}>
                  {test.name}
                </span>
                <div className="flex shrink-0 gap-1">
                  {test.retries_count > 0 && (
                    <Badge variant="broken" className="text-xs">
                      {test.retries_count}x
                    </Badge>
                  )}
                  <Badge
                    variant={test.status === 'passed' ? 'secondary' : 'destructive'}
                    className="text-xs"
                  >
                    {test.status}
                  </Badge>
                </div>
              </div>
            ))}
          </div>
        </CardState>
      </CardContent>
    </Card>
  )
}
