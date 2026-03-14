import { ChevronLeft, ChevronRight } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Pagination, PaginationContent, PaginationItem } from '@/components/ui/pagination'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { PER_PAGE_OPTIONS } from '@/store/ui'

interface ReportPaginationProps {
  page: number
  totalPages: number
  onPageChange: (updater: (p: number) => number) => void
  perPage: number
  onPerPageChange: (perPage: number) => void
}

export function ReportPagination({
  page,
  totalPages,
  onPageChange,
  perPage,
  onPerPageChange,
}: ReportPaginationProps) {
  return (
    <div className="flex items-center justify-between">
      <Pagination>
        <PaginationContent>
          <PaginationItem>
            <Button
              variant="outline"
              size="sm"
              onClick={() => onPageChange((p) => Math.max(1, p - 1))}
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
              onClick={() => onPageChange((p) => Math.min(totalPages, p + 1))}
              disabled={page >= totalPages}
            >
              Next
              <ChevronRight size={14} />
            </Button>
          </PaginationItem>
        </PaginationContent>
      </Pagination>
      <div className="flex items-center gap-2">
        <span className="text-muted-foreground text-sm">Rows per page:</span>
        <Select value={String(perPage)} onValueChange={(v) => onPerPageChange(Number(v))}>
          <SelectTrigger className="w-20" aria-label="Rows per page">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {PER_PAGE_OPTIONS.map((opt) => (
              <SelectItem key={opt} value={String(opt)}>
                {opt}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
    </div>
  )
}
