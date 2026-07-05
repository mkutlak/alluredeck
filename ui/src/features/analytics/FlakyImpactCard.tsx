import { useQuery } from '@tanstack/react-query'
import { NavLink } from 'react-router'
import { flakyImpactOptions } from '@/lib/queries'
import { formatDuration } from '@/lib/utils'
import { STATUS_TEXT_CLASSES } from '@/lib/status-colors'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { CardState } from '@/components/ui/CardState'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

interface Props {
  projectId: string
  /** Numeric project_id used to link to a build's report — omit while unresolved. */
  numericProjectId?: number
  branch?: string
}

const BUILDS = 20
const LIMIT = 10

/**
 * Flake rate is the inverse of a pass-rate signal — higher is worse — so the
 * thresholds are simple and inverted relative to getPassRateColorClass.
 */
function getFlakeRateColorClass(rate: number): string {
  if (rate >= 30) return STATUS_TEXT_CLASSES.failed
  if (rate >= 10) return STATUS_TEXT_CLASSES.broken
  return STATUS_TEXT_CLASSES.passed
}

export function FlakyImpactCard({ projectId, numericProjectId, branch }: Props) {
  const { data, isLoading, isError, error, refetch } = useQuery(
    flakyImpactOptions(projectId, { builds: BUILDS, limit: LIMIT, branch }),
  )

  const tests = data?.tests ?? []

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium">Flaky Impact</CardTitle>
      </CardHeader>
      <CardContent>
        <CardState
          isLoading={isLoading}
          isError={isError}
          error={error}
          isEmpty={tests.length === 0}
          refetch={refetch}
          emptyMessage="No flaky tests detected in recent builds"
        >
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Test</TableHead>
                <TableHead className="text-right">Flake rate</TableHead>
                <TableHead className="text-right">CI time wasted</TableHead>
                <TableHead className="text-right">Retries</TableHead>
                <TableHead className="text-right">Builds affected</TableHead>
                <TableHead className="text-right">Last seen</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {tests.map((test) => {
                const flakeRate = test.runs > 0 ? (test.builds_affected / test.runs) * 100 : 0
                return (
                  <TableRow key={test.full_name}>
                    <TableCell
                      className="max-w-xs truncate font-mono text-xs"
                      title={test.full_name}
                    >
                      {test.full_name}
                    </TableCell>
                    <TableCell
                      className={`text-right text-xs ${getFlakeRateColorClass(flakeRate)}`}
                    >
                      {flakeRate.toFixed(1)}%
                    </TableCell>
                    <TableCell className="text-right text-xs">
                      {formatDuration(test.wasted_ms)}
                    </TableCell>
                    <TableCell className="text-right text-xs">{test.retry_sum}</TableCell>
                    <TableCell className="text-right text-xs">
                      {test.builds_affected}/{test.runs}
                    </TableCell>
                    <TableCell className="text-right text-xs">
                      {numericProjectId != null ? (
                        <NavLink
                          to={`/projects/${numericProjectId}/reports/${test.last_seen_build_order}`}
                          className="hover:underline"
                        >
                          #{test.last_seen_build_order}
                        </NavLink>
                      ) : (
                        `#${test.last_seen_build_order}`
                      )}
                    </TableCell>
                  </TableRow>
                )
              })}
            </TableBody>
          </Table>
        </CardState>
      </CardContent>
    </Card>
  )
}
