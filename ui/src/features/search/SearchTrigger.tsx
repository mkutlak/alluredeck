import { Search } from 'lucide-react'
import { SidebarMenuButton } from '@/components/ui/sidebar'
import { useSearchCommand } from './SearchCommand'

export function SearchTrigger() {
  const { setOpen } = useSearchCommand()

  return (
    <SidebarMenuButton onClick={() => setOpen(true)} className="text-muted-foreground">
      <Search className="size-4" />
      <span className="flex-1">Search</span>
      <kbd className="pointer-events-none hidden select-none rounded border bg-muted px-1.5 py-0.5 font-mono text-[10px] font-medium text-muted-foreground sm:inline">
        ⌘K
      </kbd>
    </SidebarMenuButton>
  )
}
