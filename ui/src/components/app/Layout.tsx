import { Outlet } from 'react-router'
import { TooltipProvider } from '@/components/ui/tooltip'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { SearchCommand } from '@/features/search'
import { usePreferencesSync } from '@/hooks/usePreferencesSync'
import { AppSidebar } from './AppSidebar'
import { BreadcrumbBar } from './BreadcrumbBar'
import { TopBar } from './TopBar'

export function Layout() {
  usePreferencesSync()

  return (
    <TooltipProvider>
      <SearchCommand>
        <SidebarProvider className="h-svh min-h-0 flex-col overflow-hidden">
          <TopBar />
          <BreadcrumbBar />
          <div className="flex min-h-0 flex-1">
            <AppSidebar />
            {/* min-w-0: a flex item defaults to min-width:auto, so one long unbroken line
                (an error message) would widen the pane and squeeze the sidebar. */}
            <SidebarInset className="min-w-0">
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
