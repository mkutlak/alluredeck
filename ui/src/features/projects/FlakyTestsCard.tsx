import { useQuery } from '@tanstack/react-query'
import { fetchReportStability } from '@/api/reports'
import { queryKeys } from '@/lib/query-keys'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'

interface Props {
  projectId: string
}

export function FlakyTestsCard({ projectId }: Props) {
  const { data: stability, isLoading } = useQuery({
    queryKey: queryKeys.reportStability(projectId),
    queryFn: () => fetchReportStability(projectId),
    staleTime: 30_000,
  })

  const flakyTests = stability?.flaky_tests ?? []
  const summary = stability?.summary

  if (!isLoading && flakyTests.length === 0) {
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
        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-8 w-full" />
            ))}
          </div>
        ) : (
          <div className="space-y-2">
            {flakyTests.map((test, i) => (
              <div key={i} className="flex items-center justify-between gap-2">
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
        )}
      </CardContent>
    </Card>
  )
}
