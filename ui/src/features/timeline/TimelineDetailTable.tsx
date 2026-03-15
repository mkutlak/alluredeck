import { useState, useMemo } from 'react'
import type { TimelineTestCase } from '@/types/api'
import type { StatusColorMap } from '@/hooks/useStatusColors'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDuration, getStatusVariant } from '@/lib/utils'
import { useDebounce } from '@/hooks/useDebounce'

type SortField = 'name' | 'duration' | 'status' | 'thread'
type SortDir = 'asc' | 'desc'

export interface TimelineDetailTableProps {
  testCases: TimelineTestCase[]
  statusColors: StatusColorMap
  onTestClick: (tc: TimelineTestCase) => void
}

export function TimelineDetailTable({
  testCases,
  statusColors: _statusColors,
  onTestClick,
}: TimelineDetailTableProps) {
  const [search, setSearch] = useState('')
  const [sortField, setSortField] = useState<SortField>('duration')
  const [sortDir, setSortDir] = useState<SortDir>('desc')
  const debouncedSearch = useDebounce(search, 300)

  const filtered = useMemo(() => {
    if (!debouncedSearch) return testCases
    const q = debouncedSearch.toLowerCase()
    return testCases.filter(
      (tc) => tc.name.toLowerCase().includes(q) || tc.full_name.toLowerCase().includes(q),
    )
  }, [testCases, debouncedSearch])

  const sorted = useMemo(() => {
    const copy = [...filtered]
    copy.sort((a, b) => {
      let cmp = 0
      switch (sortField) {
        case 'name':
          cmp = a.name.localeCompare(b.name)
          break
        case 'duration':
          cmp = a.duration - b.duration
          break
        case 'status':
          cmp = a.status.localeCompare(b.status)
          break
        case 'thread':
          cmp = (a.thread || '').localeCompare(b.thread || '')
          break
      }
      return sortDir === 'asc' ? cmp : -cmp
    })
    return copy
  }, [filtered, sortField, sortDir])

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'))
    } else {
      setSortField(field)
      setSortDir(field === 'duration' ? 'desc' : 'asc')
    }
  }

  const sortIndicator = (field: SortField) =>
    sortField === field ? (sortDir === 'asc' ? ' ↑' : ' ↓') : ''

  return (
    <div data-testid="timeline-detail-table" className="space-y-2">
      <Input
        placeholder="Search tests..."
        value={search}
        onChange={(e) => setSearch(e.target.value)}
        className="max-w-xs"
      />
      {sorted.length === 0 && debouncedSearch ? (
        <p className="text-muted-foreground py-8 text-center text-sm">No tests match your search</p>
      ) : sorted.length > 0 ? (
        <div className="max-h-[400px] overflow-auto rounded border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead className="cursor-pointer" onClick={() => handleSort('name')}>
                  Name{sortIndicator('name')}
                </TableHead>
                <TableHead className="cursor-pointer" onClick={() => handleSort('status')}>
                  Status{sortIndicator('status')}
                </TableHead>
                <TableHead className="cursor-pointer" onClick={() => handleSort('duration')}>
                  Duration{sortIndicator('duration')}
                </TableHead>
                <TableHead className="cursor-pointer" onClick={() => handleSort('thread')}>
                  Worker{sortIndicator('thread')}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {sorted.map((tc) => (
                <TableRow
                  key={`${tc.full_name}-${tc.start}`}
                  className="cursor-pointer"
                  onClick={() => onTestClick(tc)}
                >
                  <TableCell className="max-w-[300px] truncate text-xs font-medium">
                    {tc.name}
                  </TableCell>
                  <TableCell>
                    <Badge variant={getStatusVariant(tc.status)} className="text-xs">
                      {tc.status}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs">{formatDuration(tc.duration)}</TableCell>
                  <TableCell className="text-muted-foreground text-xs">
                    {tc.thread || '—'}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      ) : null}
    </div>
  )
}
