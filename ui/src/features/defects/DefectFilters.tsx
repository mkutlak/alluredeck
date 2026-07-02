import { SearchInput } from '@/components/ui/SearchInput'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { FilterBar } from '@/components/app/FilterBar'
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

// Radix Select forbids an empty-string item value, so the UI uses the 'all'
// sentinel internally and maps it back to '' at the DefectFilterValues boundary.
const ALL_SENTINEL = 'all'

const CATEGORIES: { value: DefectCategory | typeof ALL_SENTINEL; label: string }[] = [
  { value: ALL_SENTINEL, label: 'All categories' },
  { value: 'product_bug', label: 'Product Bug' },
  { value: 'test_bug', label: 'Test Bug' },
  { value: 'infrastructure', label: 'Infrastructure' },
  { value: 'to_investigate', label: 'To Investigate' },
]

const RESOLUTIONS: { value: DefectResolution | typeof ALL_SENTINEL; label: string }[] = [
  { value: ALL_SENTINEL, label: 'All resolutions' },
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
    <FilterBar
      search={
        <SearchInput
          placeholder="Search defects..."
          value={filters.search}
          onChange={(e) => onFilterChange({ ...filters, search: e.target.value })}
          aria-label="Search defects"
        />
      }
      end={
        <>
          <Select
            value={filters.category || ALL_SENTINEL}
            onValueChange={(v) =>
              onFilterChange({
                ...filters,
                category: v === ALL_SENTINEL ? '' : (v as DefectCategory),
              })
            }
          >
            <SelectTrigger className="w-40" aria-label="Filter by category">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {CATEGORIES.map((c) => (
                <SelectItem key={c.value} value={c.value}>
                  {c.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select
            value={filters.resolution || ALL_SENTINEL}
            onValueChange={(v) =>
              onFilterChange({
                ...filters,
                resolution: v === ALL_SENTINEL ? '' : (v as DefectResolution),
              })
            }
          >
            <SelectTrigger className="w-40" aria-label="Filter by resolution">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {RESOLUTIONS.map((r) => (
                <SelectItem key={r.value} value={r.value}>
                  {r.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>

          <Select
            value={filters.sort}
            onValueChange={(v) => onFilterChange({ ...filters, sort: v })}
          >
            <SelectTrigger className="w-36" aria-label="Sort by">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {SORT_OPTIONS.map((s) => (
                <SelectItem key={s.value} value={s.value}>
                  {s.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </>
      }
    />
  )
}
