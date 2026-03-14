import { useQuery } from '@tanstack/react-query'
import { fetchTopErrors } from '@/api/analytics'
import { queryKeys } from '@/lib/query-keys'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Skeleton } from '@/components/ui/skeleton'
import { Badge } from '@/components/ui/badge'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'

interface Props {
  projectId: string
}

const BUILDS = 20
const LIMIT = 10
const MAX_MESSAGE_LENGTH = 80

function truncateMessage(message: string): { text: string; truncated: boolean } {
  if (message.length <= MAX_MESSAGE_LENGTH) {
    return { text: message, truncated: false }
  }
  return { text: message.slice(0, MAX_MESSAGE_LENGTH) + '...', truncated: true }
}

export function ErrorClusterCard({ projectId }: Props) {
  const { data, isLoading } = useQuery({
    queryKey: queryKeys.topErrors(projectId, BUILDS),
    queryFn: () => fetchTopErrors(projectId, BUILDS, LIMIT),
    staleTime: 60_000,
  })

  const errors = data?.data ?? []

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium">Top Failure Messages</CardTitle>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            {Array.from({ length: 5 }).map((_, i) => (
              <Skeleton key={i} className="h-8 w-full" />
            ))}
          </div>
        ) : errors.length === 0 ? (
          <p className="text-muted-foreground py-4 text-center text-sm">
            No failure data available
          </p>
        ) : (
          <TooltipProvider>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-muted-foreground border-b text-xs">
                    <th className="pb-1 text-left font-medium">Failure Message</th>
                    <th className="pb-1 text-right font-medium">Occurrences</th>
                  </tr>
                </thead>
                <tbody>
                  {errors.map((error) => {
                    const { text, truncated } = truncateMessage(error.message)
                    return (
                      <tr key={error.message} className="border-b last:border-0">
                        <td className="py-1.5 pr-2">
                          {truncated ? (
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <span className="cursor-help font-mono text-xs">{text}</span>
                              </TooltipTrigger>
                              <TooltipContent className="max-w-md">
                                <p className="font-mono text-xs">{error.message}</p>
                              </TooltipContent>
                            </Tooltip>
                          ) : (
                            <span className="font-mono text-xs">{text}</span>
                          )}
                        </td>
                        <td className="py-1.5 text-right">
                          <Badge variant="secondary">{error.count}</Badge>
                        </td>
                      </tr>
                    )
                  })}
                </tbody>
              </table>
            </div>
          </TooltipProvider>
        )}
      </CardContent>
    </Card>
  )
}
