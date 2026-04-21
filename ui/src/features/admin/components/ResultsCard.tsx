import { useState } from 'react'
import { Link } from 'react-router'
import { ChevronRight } from 'lucide-react'
import { useQuery } from '@tanstack/react-query'
import { projectIndexOptions } from '@/lib/queries/projects'
import { formatProjectLabel } from '@/lib/projectLabel'
import { formatBytes, formatDate } from '@/lib/utils'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Checkbox } from '@/components/ui/checkbox'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { useAdminResults } from '../hooks/useAdminResults'
import { DeleteResultDialog } from './DeleteResultDialog'
import { DeleteResultsBulkDialog } from './DeleteResultsBulkDialog'
import { groupByParent } from '../utils/groupByParent'

export function ResultsCard() {
  const [deletingSlug, setDeletingSlug] = useState<string | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [confirmBulkDeleteOpen, setConfirmBulkDeleteOpen] = useState(false)
  const [expandedGroups, setExpandedGroups] = useState<Set<number>>(() => new Set())

  const { data: results = [], isLoading, doClean, doCleanBulk } = useAdminResults()
  const { data: projectsData } = useQuery(projectIndexOptions())
  const projects = projectsData?.data

  const allSelected = results.length > 0 && results.every((r) => selectedIds.has(r.project_id))
  const someSelected = results.some((r) => selectedIds.has(r.project_id))

  const toggleSelectAll = () => {
    if (allSelected) {
      setSelectedIds(new Set())
    } else {
      setSelectedIds(new Set(results.map((r) => r.project_id)))
    }
  }

  const toggleResult = (projectId: number) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(projectId)) {
        next.delete(projectId)
      } else {
        next.add(projectId)
      }
      return next
    })
  }

  const toggleGroup = (parentId: number) => {
    setExpandedGroups((prev) => {
      const next = new Set(prev)
      if (next.has(parentId)) {
        next.delete(parentId)
      } else {
        next.add(parentId)
      }
      return next
    })
  }

  const handleBulkConfirm = () => {
    doCleanBulk(Array.from(selectedIds), {
      onSuccess: () => {
        setSelectedIds(new Set())
        setConfirmBulkDeleteOpen(false)
      },
    })
  }

  const handleSingleConfirm = (slug: string) => {
    doClean(slug, {
      onSuccess: () => {
        setDeletingSlug(null)
      },
    })
  }

  const groups = groupByParent(results, projects)

  // Auto-expand all groups on first render
  if (expandedGroups.size === 0 && groups.some((g) => g.parentId != null)) {
    const parentIds = groups.filter((g) => g.parentId != null).map((g) => g.parentId!)
    if (parentIds.length > 0) {
      setExpandedGroups(new Set(parentIds))
    }
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Pending Results</CardTitle>
            <CardDescription>Projects with unprocessed result files</CardDescription>
          </div>
          {someSelected && (
            <DeleteResultsBulkDialog
              open={confirmBulkDeleteOpen}
              onOpenChange={setConfirmBulkDeleteOpen}
              count={selectedIds.size}
              onConfirm={handleBulkConfirm}
            />
          )}
        </div>
      </CardHeader>
      <CardContent>
        {isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-8 w-full" />
            <Skeleton className="h-8 w-full" />
          </div>
        ) : results.length === 0 ? (
          <p className="text-muted-foreground py-4 text-center text-sm">No unprocessed results</p>
        ) : (
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="w-10">
                  <Checkbox
                    checked={allSelected ? true : someSelected ? 'indeterminate' : false}
                    onCheckedChange={toggleSelectAll}
                    aria-label="Select all pending results"
                    disabled={results.length === 0}
                  />
                </TableHead>
                <TableHead>Project</TableHead>
                <TableHead>Files</TableHead>
                <TableHead>Total Size</TableHead>
                <TableHead>Last Modified</TableHead>
                <TableHead className="text-right">Actions</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {groups.map((group) => {
                if (group.parentId == null) {
                  const entry = group.items[0]
                  return (
                    <TableRow key={entry.project_id}>
                      <TableCell>
                        <Checkbox
                          checked={selectedIds.has(entry.project_id)}
                          onCheckedChange={() => toggleResult(entry.project_id)}
                          aria-label={`Select ${entry.slug}`}
                        />
                      </TableCell>
                      <TableCell>
                        <Link
                          to={`/projects/${entry.project_id}`}
                          className="font-medium hover:underline"
                        >
                          {(() => {
                            const matched = projects?.find(
                              (p) => p.project_id === entry.project_id,
                            )
                            return matched
                              ? formatProjectLabel(matched, projects)
                              : entry.slug
                          })()}
                        </Link>
                      </TableCell>
                      <TableCell>{entry.file_count}</TableCell>
                      <TableCell className="text-muted-foreground text-sm">
                        {formatBytes(entry.total_size)}
                      </TableCell>
                      <TableCell className="text-muted-foreground text-sm">
                        {formatDate(entry.last_modified)}
                      </TableCell>
                      <TableCell className="text-right">
                        <DeleteResultDialog
                          open={deletingSlug === entry.slug}
                          onOpenChange={(open) => setDeletingSlug(open ? entry.slug : null)}
                          label={entry.slug}
                          onConfirm={() => handleSingleConfirm(entry.slug)}
                        />
                      </TableCell>
                    </TableRow>
                  )
                }

                const isExpanded = expandedGroups.has(group.parentId)
                return [
                  <TableRow
                    key={`group-${group.parentId}`}
                    className="bg-muted/30 cursor-pointer"
                    onClick={() => toggleGroup(group.parentId!)}
                  >
                    <TableCell />
                    <TableCell colSpan={5}>
                      <div className="flex items-center gap-2">
                        <ChevronRight
                          className={`h-4 w-4 shrink-0 transition-transform ${isExpanded ? 'rotate-90' : ''}`}
                        />
                        <span className="font-semibold">{group.parentLabel}</span>
                        <Badge variant="secondary" className="text-xs">
                          {group.items.length}
                        </Badge>
                      </div>
                    </TableCell>
                  </TableRow>,
                  ...group.items.map((entry) => (
                    <TableRow
                      key={entry.project_id}
                      className={isExpanded ? '' : 'hidden'}
                    >
                      <TableCell>
                        <Checkbox
                          checked={selectedIds.has(entry.project_id)}
                          onCheckedChange={() => toggleResult(entry.project_id)}
                          aria-label={`Select ${entry.slug}`}
                        />
                      </TableCell>
                      <TableCell className="pl-10">
                        <Link
                          to={`/projects/${entry.project_id}`}
                          className="font-medium hover:underline"
                        >
                          {(() => {
                            const matched = projects?.find(
                              (p) => p.project_id === entry.project_id,
                            )
                            return matched
                              ? (matched.display_name || matched.slug)
                              : entry.slug
                          })()}
                        </Link>
                      </TableCell>
                      <TableCell>{entry.file_count}</TableCell>
                      <TableCell className="text-muted-foreground text-sm">
                        {formatBytes(entry.total_size)}
                      </TableCell>
                      <TableCell className="text-muted-foreground text-sm">
                        {formatDate(entry.last_modified)}
                      </TableCell>
                      <TableCell className="text-right">
                        <DeleteResultDialog
                          open={deletingSlug === entry.slug}
                          onOpenChange={(open) => setDeletingSlug(open ? entry.slug : null)}
                          label={entry.slug}
                          onConfirm={() => handleSingleConfirm(entry.slug)}
                        />
                      </TableCell>
                    </TableRow>
                  )),
                ]
              })}
            </TableBody>
          </Table>
        )}
      </CardContent>
    </Card>
  )
}
