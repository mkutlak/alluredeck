import { useState, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import { fetchProjectDefects, fetchBuildDefects } from '@/api/defects'
import { queryKeys } from '@/lib/query-keys'
import { Button } from '@/components/ui/button'
import { CardState } from '@/components/ui/CardState'
import { DefectFilters, type DefectFilterValues } from './DefectFilters'
import { DefectRow } from './DefectRow'
import { DefectBulkActions } from './DefectBulkActions'

interface DefectListProps {
  projectId: string
  buildId?: number
  defaultResolution?: string
}

export function DefectList({ projectId, buildId, defaultResolution }: DefectListProps) {
  const [filters, setFilters] = useState<DefectFilterValues>({
    category: '',
    resolution: (defaultResolution as DefectFilterValues['resolution']) ?? '',
    sort: 'last_seen',
    search: '',
  })
  const [page, setPage] = useState(1)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())

  const apiFilters: Record<string, unknown> = {
    ...(filters.category ? { category: filters.category } : {}),
    ...(filters.resolution ? { resolution: filters.resolution } : {}),
    ...(filters.sort ? { sort: filters.sort } : {}),
    ...(filters.search ? { search: filters.search } : {}),
    page,
  }

  const queryKey =
    buildId != null
      ? queryKeys.buildDefects(projectId, buildId, apiFilters)
      : queryKeys.defects(projectId, apiFilters)

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey,
    queryFn: () =>
      buildId != null
        ? fetchBuildDefects(projectId, buildId, {
            ...filters,
            category: filters.category || undefined,
            resolution: filters.resolution || undefined,
            page,
          })
        : fetchProjectDefects(projectId, {
            ...filters,
            category: filters.category || undefined,
            resolution: filters.resolution || undefined,
            page,
          }),
    staleTime: 15_000,
  })

  const defects = data?.data ?? []
  const total = data?.pagination?.total ?? 0
  const perPage = data?.pagination?.per_page ?? 25
  const totalPages = data?.pagination?.total_pages ?? Math.max(1, Math.ceil(total / perPage))

  const handleSelect = useCallback((id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }, [])

  const handleToggle = useCallback((id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }, [])

  const clearSelection = useCallback(() => setSelectedIds(new Set()), [])

  return (
    <div className="space-y-4">
      <DefectFilters filters={filters} onFilterChange={setFilters} />

      {selectedIds.size > 0 && (
        <DefectBulkActions
          selectedIds={selectedIds}
          projectId={projectId}
          onComplete={clearSelection}
          onCancel={clearSelection}
        />
      )}

      <CardState
        isLoading={isLoading}
        isError={isError}
        error={error}
        isEmpty={defects.length === 0}
        refetch={refetch}
        skeletonRows={5}
        emptyMessage="No defects found — defects will appear here after builds are processed."
      >
        <div className="rounded-lg border">
          {defects.map((defect) => (
            <DefectRow
              key={defect.id}
              defect={defect}
              selected={selectedIds.has(defect.id)}
              onSelect={handleSelect}
              onToggle={handleToggle}
              expanded={expandedIds.has(defect.id)}
              projectId={projectId}
            />
          ))}
        </div>

        {totalPages > 1 && (
          <div className="mt-4 flex items-center justify-between">
            <p className="text-muted-foreground text-sm">
              Page {page} of {totalPages} ({total} total)
            </p>
            <div className="flex gap-2">
              <Button
                size="sm"
                variant="outline"
                disabled={page <= 1}
                onClick={() => setPage((p) => p - 1)}
              >
                Previous
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={page >= totalPages}
                onClick={() => setPage((p) => p + 1)}
              >
                Next
              </Button>
            </div>
          </div>
        )}
      </CardState>
    </div>
  )
}
