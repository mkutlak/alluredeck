import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router'
import { fetchReportStability } from '@/api/reports'
import { ApiError } from '@/api/client'
import { queryKeys } from '@/lib/query-keys'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { FlakyBadge } from '@/components/ui/FlakyBadge'
import { CardState } from '@/components/ui/CardState'

interface Props {
  projectId: string
  /** Numeric project_id used to build the flaky-impact link — omit while unresolved. */
  numericProjectId?: number
}

export function FlakyTestsCard({ projectId, numericProjectId }: Props) {
  const {
    data: stability,
    isLoading,
    isError,
    error,
    refetch,
  } = useQuery({
    queryKey: queryKeys.reportStability(projectId),
    queryFn: () => fetchReportStability(projectId),
    staleTime: 30_000,
    // Auxiliary card: settle immediately instead of retrying — a 404 hides the
    // card and other errors surface right away.
    retry: false,
  })

  const flakyTests = stability?.flaky_tests ?? []
  const summary = stability?.summary

  // A 404 means the project has no stability data yet (e.g. no builds) — for an
  // auxiliary card that is "nothing to show", not an error worth a banner.
  const isNotFound = isError && error instanceof ApiError && error.response?.status === 404

  // Hide the card entirely when there is nothing to show — don't render an
  // empty card (or a loud error card for missing data) that would confuse users.
  if (!isLoading && flakyTests.length === 0 && (!isError || isNotFound)) {
    return null
  }

  return (
    <Card>
      <CardHeader className="pb-2">
        <div className="flex items-center justify-between gap-2">
          <CardTitle className="text-sm font-medium">
            Flaky Tests
            {summary && summary.flaky_count > 0 && (
              <Badge variant="secondary" className="ml-2 text-xs">
                {summary.flaky_count}
              </Badge>
            )}
          </CardTitle>
          {numericProjectId != null && (
            <Link
              to={`/projects/${numericProjectId}/analytics`}
              className="text-primary text-xs whitespace-nowrap hover:underline"
              data-testid="flaky-impact-link"
            >
              View flaky impact
            </Link>
          )}
        </div>
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
                  <FlakyBadge retries={test.retries_count} />
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
