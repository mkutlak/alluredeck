import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'
import type { APIKey, APIKeyCreated } from '@/types/api'
import { MAX_KEYS, useAPIKeys } from './hooks/useAPIKeys'
import { APIKeyList } from './components/APIKeyList'
import { APIKeyFormDialog } from './components/APIKeyFormDialog'
import { CreatedKeyDialog } from './components/CreatedKeyDialog'
import { DeleteAPIKeyDialog } from './components/DeleteAPIKeyDialog'

export function APIKeysPage() {
  const [createOpen, setCreateOpen] = useState(false)
  const [createdKey, setCreatedKey] = useState<APIKeyCreated | null>(null)
  const [deletingKey, setDeletingKey] = useState<APIKey | null>(null)

  const { data: keys = [], isLoading, atLimit } = useAPIKeys()

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">API Keys</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Create API keys for programmatic access from CI/CD pipelines.
          </p>
        </div>
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <span>
                <Button
                  onClick={() => setCreateOpen(true)}
                  disabled={atLimit}
                  aria-label="Create API Key"
                >
                  Create API Key
                </Button>
              </span>
            </TooltipTrigger>
            {atLimit && <TooltipContent>Maximum {MAX_KEYS} API keys allowed</TooltipContent>}
          </Tooltip>
        </TooltipProvider>
      </div>

      {isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-10 w-full" />
          <Skeleton className="h-10 w-full" />
        </div>
      ) : keys.length === 0 ? (
        <p className="text-muted-foreground py-8 text-center text-sm">
          No API keys yet. Create one to get started.
        </p>
      ) : (
        <APIKeyList keys={keys} onDelete={setDeletingKey} />
      )}

      <APIKeyFormDialog
        open={createOpen}
        onOpenChange={setCreateOpen}
        onCreated={(apiKey) => {
          setCreateOpen(false)
          setCreatedKey(apiKey)
        }}
      />

      <CreatedKeyDialog apiKey={createdKey} onClose={() => setCreatedKey(null)} />

      <DeleteAPIKeyDialog apiKey={deletingKey} onClose={() => setDeletingKey(null)} />
    </div>
  )
}
