import { useState } from 'react'
import { useQuery, keepPreviousData } from '@tanstack/react-query'
import { ChevronLeft, ChevronRight, GitCommitHorizontal } from 'lucide-react'
import { Link } from 'react-router'

import { runsFeedOptions } from '@/lib/queries'
import { useUIStore } from '@/store/ui'
import { PageHeader } from '@/components/app/PageHeader'
import { FilterBar } from '@/components/app/FilterBar'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Pagination, PaginationContent, PaginationItem } from '@/components/ui/pagination'
import { FeedBranchSelect } from './FeedBranchSelect'
import { GroupFilter } from './GroupFilter'
import { RunRow } from './RunRow'

export function RunsFeedPage() {
  const [page, setPage] = useState(1)
  const selectedBranch = useUIStore((s) => s.selectedBranch)
  const runsFeedGroupIds = useUIStore((s) => s.runsFeedGroupIds)

  // Reset to page 1 when the branch or group filter changes
  const [prevBranch, setPrevBranch] = useState(selectedBranch)
  if (prevBranch !== selectedBranch) {
    setPrevBranch(selectedBranch)
    setPage(1)
  }
  const [prevGroupIds, setPrevGroupIds] = useState(runsFeedGroupIds)
  if (prevGroupIds !== runsFeedGroupIds) {
    setPrevGroupIds(runsFeedGroupIds)
    setPage(1)
  }

  const groupIds = runsFeedGroupIds.length > 0 ? runsFeedGroupIds : undefined

  const { data, isLoading } = useQuery({
    ...runsFeedOptions(page, selectedBranch, groupIds),
    placeholderData: keepPreviousData,
  })

  const runs = data?.data ?? []
  const pagination = data?.pagination
  const totalPages = Math.max(1, pagination?.total_pages ?? 1)

  return (
    <div className="space-y-6 p-6" data-testid="runs-feed">
      <PageHeader
        title="Runs"
        titleVariant="sans"
        toolbar={
          <FilterBar
            filters={
              <>
                <FeedBranchSelect />
                <GroupFilter />
              </>
            }
          />
        }
      />

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
            <p className="font-medium">No runs found</p>
            <p className="text-muted-foreground text-sm">
              Runs appear here when suites upload results with CI metadata.
            </p>
          </div>
          <Link to="/projects" className="text-primary text-sm hover:underline">
            Browse projects
          </Link>
        </div>
      ) : (
        <div className="space-y-3">
          {runs.map((run) => (
            <RunRow
              key={`${run.group_project_id ?? run.group_slug}-${run.commit_sha}-${run.timestamp}`}
              run={run}
            />
          ))}
        </div>
      )}

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
