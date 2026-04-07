import { Outlet } from 'react-router'
import { TooltipProvider } from '@/components/ui/tooltip'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { SearchCommand } from '@/features/search'
import { usePreferencesSync } from '@/hooks/usePreferencesSync'
import { AppSidebar } from './AppSidebar'
import { TopBar } from './TopBar'

export function Layout() {
  usePreferencesSync()

  return (
    <TooltipProvider>
      <SearchCommand>
        <SidebarProvider className="h-svh min-h-0 flex-col overflow-hidden">
          <TopBar />
          <div className="flex min-h-0 flex-1">
            <AppSidebar />
            <SidebarInset>
              <div className="flex-1 overflow-auto p-6">
                <Outlet />
              </div>
            </SidebarInset>
          </div>
        </SidebarProvider>
      </SearchCommand>
    </TooltipProvider>
  )
}
