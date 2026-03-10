import { useState } from 'react'
import { useParams } from 'react-router'
import { Upload, Play, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useAuthStore } from '@/store/auth'
import { SendResultsDialog } from '@/features/reports/SendResultsDialog'
import { GenerateReportDialog } from '@/features/reports/GenerateReportDialog'
import { CleanDialog } from '@/features/reports/CleanDialog'

export function ActionBar() {
  const { id: projectId } = useParams<{ id: string }>()
  const isAdmin = useAuthStore((s) => s.isAdmin)
  const [sendOpen, setSendOpen] = useState(false)
  const [generateOpen, setGenerateOpen] = useState(false)
  const [cleanResultsOpen, setCleanResultsOpen] = useState(false)
  const [cleanHistoryOpen, setCleanHistoryOpen] = useState(false)

  if (!projectId || !isAdmin()) return null

  return (
    <div className="flex items-center gap-2 border-b bg-muted/30 px-6 py-2">
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="sm" variant="outline" onClick={() => setSendOpen(true)}>
              <Upload size={14} />
              Send results
            </Button>
          </TooltipTrigger>
          <TooltipContent>Upload Allure result files</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button size="sm" variant="outline" onClick={() => setGenerateOpen(true)}>
              <Play size={14} />
              Generate report
            </Button>
          </TooltipTrigger>
          <TooltipContent>Generate report from current results</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="sm"
              variant="outline"
              className="text-amber-600 hover:text-amber-700"
              onClick={() => setCleanResultsOpen(true)}
            >
              <Trash2 size={14} />
              Clean results
            </Button>
          </TooltipTrigger>
          <TooltipContent>Delete pending result files</TooltipContent>
        </Tooltip>

        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              size="sm"
              variant="outline"
              className="text-destructive hover:text-destructive"
              onClick={() => setCleanHistoryOpen(true)}
            >
              <Trash2 size={14} />
              Clean history
            </Button>
          </TooltipTrigger>
          <TooltipContent>Delete all report history</TooltipContent>
        </Tooltip>
      </TooltipProvider>

      <SendResultsDialog
        projectId={projectId}
        open={sendOpen}
        onOpenChange={setSendOpen}
      />
      <GenerateReportDialog
        projectId={projectId}
        open={generateOpen}
        onOpenChange={setGenerateOpen}
      />
      <CleanDialog
        projectId={projectId}
        mode="results"
        open={cleanResultsOpen}
        onOpenChange={setCleanResultsOpen}
      />
      <CleanDialog
        projectId={projectId}
        mode="history"
        open={cleanHistoryOpen}
        onOpenChange={setCleanHistoryOpen}
      />
    </div>
  )
}
