import { useQuery } from '@tanstack/react-query'
import { fetchReportEnvironment } from '@/api/reports'
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

interface Props {
  projectId: string
}

export function EnvironmentCard({ projectId }: Props) {
  const { data: entries, isLoading } = useQuery({
    queryKey: ['report-environment', projectId],
    queryFn: () => fetchReportEnvironment(projectId),
    staleTime: 30_000,
  })

  if (!isLoading && (!entries || entries.length === 0)) {
    return null
  }

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium">Environment</CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-6 w-full" />
            ))}
          </div>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-1/3">Name</TableHead>
                <TableHead>Value</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {(entries ?? []).map((entry) => (
                <TableRow key={entry.name}>
                  <TableCell className="font-mono text-xs font-medium">{entry.name}</TableCell>
                  <TableCell className="text-xs text-muted-foreground">
                    {entry.values.join(', ')}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}
