import { ChevronLeft, ChevronRight } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { Pagination, PaginationContent, PaginationItem } from '@/components/ui/pagination'

interface ReportPaginationProps {
  page: number
  totalPages: number
  onPageChange: (updater: (p: number) => number) => void
}

export function ReportPagination({ page, totalPages, onPageChange }: ReportPaginationProps) {
  return (
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
  )
}
