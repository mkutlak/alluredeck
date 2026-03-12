import { useQuery } from '@tanstack/react-query'
import { fetchReportCategories } from '@/api/reports'
import { queryKeys } from '@/lib/query-keys'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'

interface Props {
  projectId: string
}

export function CategoriesCard({ projectId }: Props) {
  const { data: categories, isLoading } = useQuery({
    queryKey: queryKeys.reportCategories(projectId),
    queryFn: () => fetchReportCategories(projectId),
    staleTime: 30_000,
  })

  if (!isLoading && (!categories || categories.length === 0)) {
    return null
  }

  return (
    <Card>
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium">Failure Categories</CardTitle>
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
            {(categories ?? []).map((cat) => (
              <div key={cat.name} className="flex items-center justify-between gap-2">
                <span className="truncate text-sm">{cat.name}</span>
                <div className="flex shrink-0 gap-1">
                  {cat.matchedStatistic && cat.matchedStatistic.failed > 0 && (
                    <Badge variant="destructive" className="text-xs">
                      {cat.matchedStatistic.failed}f
                    </Badge>
                  )}
                  {cat.matchedStatistic && cat.matchedStatistic.broken > 0 && (
                    <Badge variant="broken" className="text-xs">
                      {cat.matchedStatistic.broken}b
                    </Badge>
                  )}
                  {cat.matchedStatistic && cat.matchedStatistic.total > 0 && (
                    <Badge variant="secondary" className="text-xs">
                      {cat.matchedStatistic.total}
                    </Badge>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
