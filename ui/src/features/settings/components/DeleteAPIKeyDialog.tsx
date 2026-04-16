import { useMutation, useQueryClient } from '@tanstack/react-query'
import { deleteAPIKey } from '@/api/api-keys'
import { queryKeys } from '@/lib/query-keys'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { toast } from '@/components/ui/use-toast'
import type { APIKey } from '@/types/api'

interface DeleteAPIKeyDialogProps {
  apiKey: APIKey | null
  onClose: () => void
}

export function DeleteAPIKeyDialog({ apiKey, onClose }: DeleteAPIKeyDialogProps) {
  const queryClient = useQueryClient()

  const { mutate: doDelete, isPending } = useMutation({
    mutationFn: (id: number) => deleteAPIKey(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys })
      toast({ title: 'API key deleted' })
      onClose()
    },
    onError: () => {
      toast({ title: 'Failed to delete API key', variant: 'destructive' })
    },
  })

  return (
    <AlertDialog open={apiKey !== null} onOpenChange={(open) => !open && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete API Key?</AlertDialogTitle>
          <AlertDialogDescription>
            Delete API key &quot;{apiKey?.name}&quot; ({apiKey?.prefix}…)? This cannot be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction disabled={isPending} onClick={() => apiKey && doDelete(apiKey.id)}>
            Delete
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}
