import { ArrowUpDown } from 'lucide-react'
import { TableHead } from '@/components/ui/table'
import type { SortField } from './sort'

interface SortableHeaderProps {
  field: SortField
  label: string
  onSort: (field: SortField) => void
}

export function SortableHeader({ field, label, onSort }: SortableHeaderProps) {
  return (
    <TableHead>
      <button className="flex items-center gap-1" onClick={() => onSort(field)}>
        {label}
        <ArrowUpDown className="h-3.5 w-3.5" />
      </button>
    </TableHead>
  )
}
