import { useState } from 'react'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, GitCommitHorizontal, Layers } from 'lucide-react'

import { pipelineRunsOptions } from '@/lib/queries'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Pagination, PaginationContent, PaginationItem } from '@/components/ui/pagination'
import { BranchSelector } from '@/features/projects/BranchSelector'
import { PipelineRunCard } from './PipelineRunCard'

interface PipelineRunsTabProps {
  projectId: string
  childIds: string[]
}

export function PipelineRunsTab({ projectId, childIds }: PipelineRunsTabProps) {
  const [page, setPage] = useState(1)
  const [selectedBranch, setSelectedBranch] = useState<string | undefined>(undefined)

  // Use the first child's projectId for the branch selector (children share CI branches)
  const branchProjectId = childIds[0] ?? projectId

  const { data, isLoading } = useQuery({
    ...pipelineRunsOptions(projectId, page, selectedBranch),
    placeholderData: keepPreviousData,
  })

  const runs = data?.data ?? []
  const pagination = data?.pagination
  const totalPages = Math.max(1, pagination?.total_pages ?? 1)

  return (
    <div className="space-y-6">
      {/* Header */}
      <div>
        <h1 className="font-mono text-2xl font-semibold">{projectId}</h1>
        <p className="text-muted-foreground flex items-center gap-1 text-sm">
          <Layers size={14} />
          Parent project — {childIds.length} {childIds.length === 1 ? 'suite' : 'suites'}
        </p>
      </div>

      {/* Branch filter */}
      <div className="flex items-center gap-2">
        <span className="text-muted-foreground text-xs">Choose branch:</span>
        <BranchSelector
          projectId={branchProjectId}
          selectedBranch={selectedBranch}
          onBranchChange={(branch) => {
            setSelectedBranch(branch)
            setPage(1)
          }}
        />
      </div>

      {/* Run cards */}
      {isLoading ? (
        <div className="space-y-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-24 w-full" />
          ))}
        </div>
      ) : runs.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed py-16 text-center">
          <GitCommitHorizontal size={36} className="text-muted-foreground/40" />
          <div>
            <p className="font-medium">No pipeline runs found</p>
            <p className="text-muted-foreground text-sm">
              Pipeline runs appear here when child suites upload results with CI metadata.
            </p>
          </div>
        </div>
      ) : (
        <div className="space-y-3">
          {runs.map((run) => (
            <PipelineRunCard key={run.commit_sha} run={run} />
          ))}
        </div>
      )}

      {/* Pagination */}
      {runs.length > 0 && (
        <Pagination>
          <PaginationContent>
            <PaginationItem>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPage((p) => Math.max(1, p - 1))}
                disabled={page <= 1}
              >
                <ChevronLeft size={14} />
                Previous
              </Button>
            </PaginationItem>
            <PaginationItem>
              <span className="text-muted-foreground px-4 text-sm">
                Page {page} of {totalPages}
              </span>
            </PaginationItem>
            <PaginationItem>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
                disabled={page >= totalPages}
              >
                Next
                <ChevronRight size={14} />
              </Button>
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      )}
    </div>
  )
}
