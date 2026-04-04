import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { bulkUpdateDefects } from '@/api/defects'
import { extractErrorMessage } from '@/api/client'
import { queryKeys } from '@/lib/query-keys'
import { toast } from '@/components/ui/use-toast'
import { Button } from '@/components/ui/button'
import type { DefectCategory, DefectResolution } from '@/types/api'

interface DefectBulkActionsProps {
  selectedIds: Set<string>
  projectId: string
  onComplete: () => void
  onCancel: () => void
}

const CATEGORIES: { value: DefectCategory; label: string }[] = [
  { value: 'product_bug', label: 'Product Bug' },
  { value: 'test_bug', label: 'Test Bug' },
  { value: 'infrastructure', label: 'Infrastructure' },
  { value: 'to_investigate', label: 'To Investigate' },
]

const RESOLUTIONS: { value: DefectResolution; label: string }[] = [
  { value: 'open', label: 'Open' },
  { value: 'fixed', label: 'Fixed' },
  { value: 'muted', label: 'Muted' },
  { value: 'wont_fix', label: "Won't Fix" },
]

export function DefectBulkActions({
  selectedIds,
  projectId,
  onComplete,
  onCancel,
}: DefectBulkActionsProps) {
  const queryClient = useQueryClient()
  const [category, setCategory] = useState<DefectCategory | ''>('')
  const [resolution, setResolution] = useState<DefectResolution | ''>('')

  const mutation = useMutation({
    mutationFn: () => {
      const data: { ids: string[]; category?: DefectCategory; resolution?: DefectResolution } = {
        ids: [...selectedIds],
      }
      if (category) data.category = category
      if (resolution) data.resolution = resolution
      return bulkUpdateDefects(projectId, data)
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.defects(projectId) })
      void queryClient.invalidateQueries({ queryKey: queryKeys.defectProjectSummary(projectId) })
      toast({ title: `Updated ${selectedIds.size} defect(s)` })
      onComplete()
    },
    onError: (err) => {
      toast({
        title: 'Bulk update failed',
        description: extractErrorMessage(err),
        variant: 'destructive',
      })
    },
  })

  const canApply = (category !== '' || resolution !== '') && selectedIds.size > 0

  return (
    <div
      className="bg-muted/50 flex flex-wrap items-center gap-3 rounded-lg border px-4 py-2"
      role="toolbar"
      aria-label="Bulk actions"
    >
      <span className="text-sm font-medium">{selectedIds.size} selected</span>

      <select
        value={category}
        onChange={(e) => setCategory(e.target.value as DefectCategory | '')}
        className="border-input bg-background h-8 rounded-md border px-2 text-sm"
        aria-label="Bulk category"
      >
        <option value="">Set category...</option>
        {CATEGORIES.map((c) => (
          <option key={c.value} value={c.value}>
            {c.label}
          </option>
        ))}
      </select>

      <select
        value={resolution}
        onChange={(e) => setResolution(e.target.value as DefectResolution | '')}
        className="border-input bg-background h-8 rounded-md border px-2 text-sm"
        aria-label="Bulk resolution"
      >
        <option value="">Set resolution...</option>
        {RESOLUTIONS.map((r) => (
          <option key={r.value} value={r.value}>
            {r.label}
          </option>
        ))}
      </select>

      <Button
        size="sm"
        disabled={!canApply || mutation.isPending}
        onClick={() => mutation.mutate()}
      >
        Apply
      </Button>
      <Button size="sm" variant="ghost" onClick={onCancel}>
        Cancel
      </Button>
    </div>
  )
}
