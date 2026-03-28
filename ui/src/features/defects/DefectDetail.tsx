import { useMutation, useQueryClient } from '@tanstack/react-query'
import { updateDefect } from '@/api/defects'
import { extractErrorMessage } from '@/api/client'
import { queryKeys } from '@/lib/query-keys'
import { toast } from '@/components/ui/use-toast'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { DefectCategory, DefectListRow, DefectResolution } from '@/types/api'

interface DefectDetailProps {
  defect: DefectListRow
  projectId: string
}

const CATEGORY_OPTIONS: { value: DefectCategory; label: string }[] = [
  { value: 'product_bug', label: 'Product Bug' },
  { value: 'test_bug', label: 'Test Bug' },
  { value: 'infrastructure', label: 'Infrastructure' },
  { value: 'to_investigate', label: 'To Investigate' },
]

const RESOLUTION_ACTIONS: { value: DefectResolution; label: string }[] = [
  { value: 'muted', label: 'Mute' },
  { value: 'wont_fix', label: "Won't Fix" },
  { value: 'open', label: 'Reopen' },
]

export function DefectDetail({ defect, projectId }: DefectDetailProps) {
  const queryClient = useQueryClient()

  const mutation = useMutation({
    mutationFn: (data: { category?: DefectCategory; resolution?: DefectResolution }) =>
      updateDefect(projectId, defect.id, data),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.defects(projectId) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.defectProjectSummary(projectId) })
      toast({ title: 'Defect updated' })
    },
    onError: (err) => {
      toast({
        title: 'Update failed',
        description: extractErrorMessage(err),
        variant: 'destructive',
      })
    },
  })

  return (
    <div className="border-muted space-y-4 border-t px-4 py-4" data-testid="defect-detail">
      {/* Metadata grid */}
      <div className="grid grid-cols-2 gap-x-6 gap-y-2 text-sm md:grid-cols-4">
        <div>
          <span className="text-muted-foreground">First seen</span>
          <p className="font-medium">Build #{defect.first_seen_build_order}</p>
        </div>
        <div>
          <span className="text-muted-foreground">Last seen</span>
          <p className="font-medium">Build #{defect.last_seen_build_order}</p>
        </div>
        <div>
          <span className="text-muted-foreground">Total occurrences</span>
          <p className="font-medium">{defect.occurrence_count}</p>
        </div>
        <div>
          <span className="text-muted-foreground">Clean builds</span>
          <p className="font-medium">{defect.consecutive_clean_builds}</p>
        </div>
      </div>

      {/* Actions row */}
      <div className="flex flex-wrap items-center gap-3">
        <label className="text-muted-foreground text-sm">Category:</label>
        <select
          value={defect.category}
          onChange={(e) => mutation.mutate({ category: e.target.value as DefectCategory })}
          disabled={mutation.isPending}
          className="border-input bg-background h-8 rounded-md border px-2 text-sm"
          aria-label="Change category"
        >
          {CATEGORY_OPTIONS.map((c) => (
            <option key={c.value} value={c.value}>
              {c.label}
            </option>
          ))}
        </select>

        <div className="flex items-center gap-1">
          {RESOLUTION_ACTIONS.filter((a) => a.value !== defect.resolution).map((action) => (
            <Button
              key={action.value}
              size="sm"
              variant="outline"
              disabled={mutation.isPending}
              onClick={() => mutation.mutate({ resolution: action.value })}
            >
              {action.label}
            </Button>
          ))}
        </div>
      </div>

      {/* Known issue link */}
      {defect.known_issue && (
        <div className="flex items-center gap-2">
          <Badge variant="secondary">Linked Issue #{defect.known_issue.id}</Badge>
          {defect.known_issue.ticket_url && (
            <a
              href={defect.known_issue.ticket_url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-primary text-sm hover:underline"
            >
              {defect.known_issue.ticket_url}
            </a>
          )}
        </div>
      )}

      {/* Sample stack trace */}
      {defect.sample_trace && (
        <div>
          <p className="text-muted-foreground mb-1 text-sm">Sample stack trace</p>
          <pre className="bg-muted max-h-64 overflow-auto rounded-md p-3 font-mono text-xs">
            {defect.sample_trace}
          </pre>
        </div>
      )}
    </div>
  )
}
