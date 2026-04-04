import { Search } from 'lucide-react'
import type { DefectCategory, DefectResolution } from '@/types/api'

export interface DefectFilterValues {
  category: DefectCategory | ''
  resolution: DefectResolution | ''
  sort: string
  search: string
}

interface DefectFiltersProps {
  filters: DefectFilterValues
  onFilterChange: (filters: DefectFilterValues) => void
}

const CATEGORIES: { value: DefectCategory | ''; label: string }[] = [
  { value: '', label: 'All categories' },
  { value: 'product_bug', label: 'Product Bug' },
  { value: 'test_bug', label: 'Test Bug' },
  { value: 'infrastructure', label: 'Infrastructure' },
  { value: 'to_investigate', label: 'To Investigate' },
]

const RESOLUTIONS: { value: DefectResolution | ''; label: string }[] = [
  { value: '', label: 'All resolutions' },
  { value: 'open', label: 'Open' },
  { value: 'fixed', label: 'Fixed' },
  { value: 'muted', label: 'Muted' },
  { value: 'wont_fix', label: "Won't Fix" },
]

const SORT_OPTIONS = [
  { value: 'last_seen', label: 'Last seen' },
  { value: 'first_seen', label: 'First seen' },
  { value: 'occurrence_count', label: 'Occurrences' },
]

export function DefectFilters({ filters, onFilterChange }: DefectFiltersProps) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <div className="relative">
        <Search
          size={14}
          className="text-muted-foreground absolute top-1/2 left-2.5 -translate-y-1/2"
        />
        <input
          type="text"
          placeholder="Search defects..."
          value={filters.search}
          onChange={(e) => onFilterChange({ ...filters, search: e.target.value })}
          className="border-input bg-background ring-offset-background placeholder:text-muted-foreground focus-visible:ring-ring h-9 rounded-md border py-1 pr-3 pl-8 text-sm focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none"
          aria-label="Search defects"
        />
      </div>

      <select
        value={filters.category}
        onChange={(e) =>
          onFilterChange({ ...filters, category: e.target.value as DefectCategory | '' })
        }
        className="border-input bg-background ring-offset-background focus-visible:ring-ring h-9 rounded-md border px-3 text-sm focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none"
        aria-label="Filter by category"
      >
        {CATEGORIES.map((c) => (
          <option key={c.value} value={c.value}>
            {c.label}
          </option>
        ))}
      </select>

      <select
        value={filters.resolution}
        onChange={(e) =>
          onFilterChange({ ...filters, resolution: e.target.value as DefectResolution | '' })
        }
        className="border-input bg-background ring-offset-background focus-visible:ring-ring h-9 rounded-md border px-3 text-sm focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none"
        aria-label="Filter by resolution"
      >
        {RESOLUTIONS.map((r) => (
          <option key={r.value} value={r.value}>
            {r.label}
          </option>
        ))}
      </select>

      <select
        value={filters.sort}
        onChange={(e) => onFilterChange({ ...filters, sort: e.target.value })}
        className="border-input bg-background ring-offset-background focus-visible:ring-ring h-9 rounded-md border px-3 text-sm focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none"
        aria-label="Sort by"
      >
        {SORT_OPTIONS.map((s) => (
          <option key={s.value} value={s.value}>
            {s.label}
          </option>
        ))}
      </select>
    </div>
  )
}
