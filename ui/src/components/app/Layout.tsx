import { Outlet } from 'react-router'
import { TooltipProvider } from '@/components/ui/tooltip'
import { SidebarInset, SidebarProvider, readSidebarOpen } from '@/components/ui/sidebar'
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
        <SidebarProvider
          defaultOpen={readSidebarOpen()}
          className="h-svh min-h-0 flex-col overflow-hidden"
        >
          {/* The router would treat the hash as a navigation, so the link moves focus itself.
              The content is focusable only until focus leaves it: a permanent tabindex would
              take focus on every click into blank space. z-[60]: above the top bar (z-50),
              below toasts (z-[100]). */}
          <a
            href="#main-content"
            onClick={(e) => {
              e.preventDefault()
              const main = document.getElementById('main-content')
              if (!main) return
              main.tabIndex = -1
              main.focus()
              if (document.activeElement !== main) main.removeAttribute('tabindex')
              main.addEventListener('blur', () => main.removeAttribute('tabindex'), { once: true })
            }}
            className="focus:bg-background focus:text-foreground sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-[60] focus:rounded-md focus:border focus:px-3 focus:py-2 focus:text-sm"
          >
            Skip to content
          </a>
          <TopBar />
          <BreadcrumbBar />
          <div className="flex min-h-0 flex-1">
            <AppSidebar />
            {/* min-w-0: a flex item defaults to min-width:auto, so one long unbroken line
                (an error message) would widen the pane and squeeze the sidebar. */}
            <SidebarInset className="min-w-0">
              <div id="main-content" className="flex-1 overflow-auto p-6 focus:outline-none">
                <Outlet />
              </div>
            </SidebarInset>
          </div>
        </SidebarProvider>
      </SearchCommand>
    </TooltipProvider>
  )
}
