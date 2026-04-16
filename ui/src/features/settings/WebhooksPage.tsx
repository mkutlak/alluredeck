import { useState } from 'react'
import { useSearchParams } from 'react-router'
import { Bell, Plus } from 'lucide-react'

import { Button } from '@/components/ui/button'
import { WebhookList } from './components/WebhookList'

export function WebhooksPage() {
  const [searchParams] = useSearchParams()
  const projectId = searchParams.get('project') ?? ''

  const [createOpen, setCreateOpen] = useState(false)

  if (!projectId) {
    return (
      <div className="space-y-6 p-6">
        <div>
          <h1 className="text-2xl font-bold">Webhooks</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Manage webhook notifications for project events.
          </p>
        </div>
        <div className="flex items-center gap-2 rounded-md border border-dashed p-6">
          <Bell className="text-muted-foreground" size={20} />
          <p className="text-muted-foreground text-sm">
            Select a project from the URL to manage its webhooks.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="space-y-6 p-6">
      <div className="flex items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-bold">Webhooks</h1>
          <p className="text-muted-foreground mt-1 text-sm">
            Receive notifications when reports are generated for{' '}
            <span className="font-medium">{projectId}</span>.
          </p>
        </div>
        <Button onClick={() => setCreateOpen(true)} aria-label="Add Webhook">
          <Plus size={16} className="mr-1" />
          Add Webhook
        </Button>
      </div>

      <WebhookList
        projectId={projectId}
        createOpen={createOpen}
        onCreateOpenChange={setCreateOpen}
      />
    </div>
  )
}
