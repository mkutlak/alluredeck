import { useState } from 'react'
import { Link } from 'react-router'
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
import { Skeleton } from '@/components/ui/skeleton'
import { useAdminResults } from '../hooks/useAdminResults'
import { DeleteResultDialog } from './DeleteResultDialog'
import { DeleteResultsBulkDialog } from './DeleteResultsBulkDialog'

export function ResultsCard() {
  const [deletingSlug, setDeletingSlug] = useState<string | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set())
  const [confirmBulkDeleteOpen, setConfirmBulkDeleteOpen] = useState(false)

  const { data: results = [], isLoading, doClean, doCleanBulk } = useAdminResults()

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
              {results.map((entry) => (
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
                      {entry.slug || String(entry.project_id)}
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
                      label={entry.slug || String(entry.project_id)}
                      onConfirm={() => handleSingleConfirm(entry.slug)}
                    />
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
