import { FolderOpen } from 'lucide-react'

interface DragOverlayCardProps {
  slug: string
}

export function DragOverlayCard({ slug }: DragOverlayCardProps) {
  return (
    <div className="flex items-center gap-2 rounded-lg border bg-background px-3 py-2 shadow-lg rotate-[2deg]">
      <FolderOpen size={14} className="text-muted-foreground shrink-0" />
      <span className="text-sm font-medium">{slug}</span>
    </div>
  )
}
