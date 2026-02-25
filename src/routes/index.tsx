import { Navigate, Route, Routes } from 'react-router'
import { AuthGuard } from '@/features/auth/AuthGuard'
import { LoginPage } from '@/features/auth/LoginPage'
import { Layout } from '@/components/app/Layout'
import { ProjectsPage } from '@/features/projects/ProjectsPage'
import { OverviewTab } from '@/features/projects/OverviewTab'
import { AnalyticsTab } from '@/features/analytics/AnalyticsTab'
import { HistoryTab } from '@/features/reports/HistoryTab'
import { ReportViewerPage } from '@/features/reports/ReportViewerPage'

function NotFound() {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
      <p className="text-6xl font-bold text-muted-foreground/30">404</p>
      <p className="text-lg font-medium">Page not found</p>
      <p className="text-sm text-muted-foreground">The page you're looking for doesn't exist.</p>
    </div>
  )
}

export function AppRoutes() {
  return (
    <Routes>
      <Route path="/login" element={<LoginPage />} />

      <Route
        element={
          <AuthGuard>
            <Layout />
          </AuthGuard>
        }
      >
        <Route index element={<ProjectsPage />} />
        <Route path="projects/:id" element={<OverviewTab />} />
        <Route path="projects/:id/analytics" element={<AnalyticsTab />} />
        <Route path="projects/:id/history" element={<HistoryTab />} />
        <Route path="projects/:id/reports/:reportId" element={<ReportViewerPage />} />
        <Route path="*" element={<NotFound />} />
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
