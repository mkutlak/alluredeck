import { Navigate, Route, Routes } from 'react-router'
import { AuthGuard } from '@/features/auth/AuthGuard'
import { LoginPage } from '@/features/auth/LoginPage'
import { Layout } from '@/components/app/Layout'
import { OverviewTab } from '@/features/projects/OverviewTab'
import { AnalyticsTab } from '@/features/analytics/AnalyticsTab'
import { KnownIssuesTab } from '@/features/known-issues/KnownIssuesTab'
import { TimelineTab } from '@/features/timeline/TimelineTab'
import { ReportViewerPage } from '@/features/reports/ReportViewerPage'
import { DashboardPage } from '@/features/dashboard'

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
        <Route index element={<DashboardPage />} />
        <Route path="projects/:id" element={<OverviewTab />} />
        <Route path="projects/:id/analytics" element={<AnalyticsTab />} />
        <Route path="projects/:id/known-issues" element={<KnownIssuesTab />} />
        <Route path="projects/:id/timeline" element={<TimelineTab />} />
        <Route path="projects/:id/reports/:reportId" element={<ReportViewerPage />} />
        <Route path="dashboard" element={<Navigate to="/" replace />} />
        <Route path="*" element={<NotFound />} />
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
