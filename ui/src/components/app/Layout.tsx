import { Outlet } from 'react-router'
import { TooltipProvider } from '@/components/ui/tooltip'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { SearchCommand } from '@/features/search'
import { AppSidebar } from './AppSidebar'
import { TopBar } from './TopBar'

export function Layout() {
  return (
    <TooltipProvider>
      <SearchCommand>
        <SidebarProvider className="flex-col">
          <TopBar />
          <div className="flex flex-1 overflow-hidden">
            <AppSidebar />
            <SidebarInset>
              <div className="flex-1 overflow-y-auto p-6">
                <div className="mx-auto max-w-[1440px]">
                  <Outlet />
                </div>
              </div>
            </SidebarInset>
          </div>
        </SidebarProvider>
      </SearchCommand>
    </TooltipProvider>
  )
}
