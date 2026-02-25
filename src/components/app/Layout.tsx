import { Outlet, useMatch, useParams } from 'react-router-dom'
import { TooltipProvider } from '@/components/ui/tooltip'
import { TopBar } from './TopBar'
import { ProjectTabBar } from './ProjectTabBar'

export function Layout() {
  const { id: projectId } = useParams<{ id: string }>()
  // Hide tab bar on the report viewer page (has reportId param)
  const isReportViewer = useMatch('/projects/:id/reports/:reportId')
  const showTabs = !!projectId && !isReportViewer

  return (
    <TooltipProvider>
      <div className="flex h-screen flex-col overflow-hidden">
        <TopBar />
        {showTabs && <ProjectTabBar />}
        <main className="flex-1 overflow-y-auto p-6">
          <div className="mx-auto max-w-[1440px]">
            <Outlet />
          </div>
        </main>
      </div>
    </TooltipProvider>
  )
}
