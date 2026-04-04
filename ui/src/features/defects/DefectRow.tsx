import { ChevronDown, ChevronRight } from 'lucide-react'
import { Checkbox } from '@/components/ui/checkbox'
import { Badge } from '@/components/ui/badge'
import { DefectDetail } from './DefectDetail'
import type { DefectCategory, DefectListRow } from '@/types/api'

interface DefectRowProps {
  defect: DefectListRow
  selected: boolean
  onSelect: (id: string) => void
  onToggle: (id: string) => void
  expanded: boolean
  projectId: string
}

const CATEGORY_COLORS: Record<DefectCategory, string> = {
  product_bug: 'bg-red-500 text-white',
  test_bug: 'bg-amber-500 text-white',
  infrastructure: 'bg-indigo-500 text-white',
  to_investigate: 'bg-slate-400 text-white',
}

const CATEGORY_LABELS: Record<DefectCategory, string> = {
  product_bug: 'Product Bug',
  test_bug: 'Test Bug',
  infrastructure: 'Infra',
  to_investigate: 'To Investigate',
}

export function DefectRow({
  defect,
  selected,
  onSelect,
  onToggle,
  expanded,
  projectId,
}: DefectRowProps) {
  return (
    <div className="border-border border-b last:border-b-0">
      <div
        className="hover:bg-muted/50 flex cursor-pointer items-center gap-3 px-4 py-3"
        onClick={() => onToggle(defect.id)}
        role="button"
        aria-expanded={expanded}
        aria-label={`Toggle details for ${defect.normalized_message}`}
      >
        <div onClick={(e) => e.stopPropagation()}>
          <Checkbox
            checked={selected}
            onCheckedChange={() => onSelect(defect.id)}
            aria-label={`Select defect ${defect.normalized_message}`}
          />
        </div>

        <Badge className={CATEGORY_COLORS[defect.category]} data-testid="category-badge">
          {CATEGORY_LABELS[defect.category]}
        </Badge>

        <div className="min-w-0 flex-1">
          <p className="truncate font-mono text-sm">{defect.normalized_message}</p>
        </div>

        <div className="flex items-center gap-2">
          {defect.is_regression && (
            <span
              className="text-destructive text-sm"
              title="Regression"
              data-testid="regression-flag"
            >
              ↩
            </span>
          )}
          {defect.is_new && (
            <span className="text-primary text-sm" title="New defect" data-testid="new-flag">
              ✦
            </span>
          )}
        </div>

        {defect.test_result_count_in_build != null && (
          <span className="text-muted-foreground text-xs whitespace-nowrap">
            {defect.test_result_count_in_build} test
            {defect.test_result_count_in_build !== 1 ? 's' : ''}
          </span>
        )}

        <span className="text-muted-foreground text-xs whitespace-nowrap">
          #{defect.first_seen_build_order}–#{defect.last_seen_build_order}
        </span>

        {expanded ? (
          <ChevronDown size={16} className="text-muted-foreground shrink-0" />
        ) : (
          <ChevronRight size={16} className="text-muted-foreground shrink-0" />
        )}
      </div>

      {expanded && <DefectDetail defect={defect} projectId={projectId} />}
    </div>
  )
}
