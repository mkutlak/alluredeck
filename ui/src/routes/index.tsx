import { lazy, Suspense } from 'react'
import { Navigate, Route, Routes } from 'react-router'
import { AuthGuard } from '@/features/auth/AuthGuard'
import { LoginPage } from '@/features/auth/LoginPage'
import { Layout } from '@/components/app/Layout'
import { ErrorBoundary } from '@/components/app/ErrorBoundary'
import { ProjectGuard } from '@/components/app/ProjectGuard'

const DashboardPage = lazy(() =>
  import('@/features/dashboard').then((m) => ({ default: m.DashboardPage })),
)
const OverviewTab = lazy(() =>
  import('@/features/projects/OverviewTab').then((m) => ({ default: m.OverviewTab })),
)
const AnalyticsTab = lazy(() =>
  import('@/features/analytics/AnalyticsTab').then((m) => ({ default: m.AnalyticsTab })),
)
const KnownIssuesTab = lazy(() =>
  import('@/features/known-issues/KnownIssuesTab').then((m) => ({ default: m.KnownIssuesTab })),
)
const TimelineTab = lazy(() =>
  import('@/features/timeline/TimelineTab').then((m) => ({ default: m.TimelineTab })),
)
const ReportViewerPage = lazy(() =>
  import('@/features/reports/ReportViewerPage').then((m) => ({ default: m.ReportViewerPage })),
)
const ComparePage = lazy(() =>
  import('@/features/compare/ComparePage').then((m) => ({ default: m.ComparePage })),
)
const AdminPage = lazy(() => import('@/features/admin').then((m) => ({ default: m.AdminPage })))
const APIKeysPage = lazy(() =>
  import('@/features/settings/APIKeysPage').then((m) => ({ default: m.APIKeysPage })),
)
const WebhooksPage = lazy(() =>
  import('@/features/settings/WebhooksPage').then((m) => ({
    default: m.WebhooksPage,
  })),
)
const UsersPage = lazy(() =>
  import('@/features/settings/users/UsersPage').then((m) => ({ default: m.UsersPage })),
)
const ProfilePage = lazy(() =>
  import('@/features/settings/users/ProfilePage').then((m) => ({ default: m.ProfilePage })),
)
const TestHistoryPage = lazy(() =>
  import('@/features/tests/TestHistoryPage').then((m) => ({ default: m.TestHistoryPage })),
)
const AttachmentsTab = lazy(() =>
  import('@/features/attachments').then((m) => ({ default: m.AttachmentsTab })),
)
const ProjectDefectsView = lazy(() =>
  import('@/features/defects/ProjectDefectsView').then((m) => ({ default: m.ProjectDefectsView })),
)
const BuildDefectsView = lazy(() =>
  import('@/features/defects/BuildDefectsView').then((m) => ({ default: m.BuildDefectsView })),
)
const TraceViewerPage = lazy(() => import('@/features/trace/TraceViewerPage'))

function PageLoader() {
  return (
    <div className="flex h-64 items-center justify-center">
      <div className="border-primary h-6 w-6 animate-spin rounded-full border-2 border-t-transparent" />
    </div>
  )
}

function NotFound() {
  return (
    <div className="flex flex-col items-center justify-center gap-3 py-24 text-center">
      <p className="text-muted-foreground/30 text-6xl font-bold">404</p>
      <p className="text-lg font-medium">Page not found</p>
      <p className="text-muted-foreground text-sm">The page you're looking for doesn't exist.</p>
    </div>
  )
}

export function AppRoutes() {
  return (
    <ErrorBoundary>
      <Routes>
        <Route path="/login" element={<LoginPage />} />

        <Route
          element={
            <AuthGuard>
              <Layout />
            </AuthGuard>
          }
        >
          <Route
            index
            element={
              <Suspense fallback={<PageLoader />}>
                <DashboardPage />
              </Suspense>
            }
          />
          <Route path="projects/:id" element={<ProjectGuard />}>
            <Route
              index
              element={
                <Suspense fallback={<PageLoader />}>
                  <OverviewTab />
                </Suspense>
              }
            />
            <Route
              path="analytics"
              element={
                <Suspense fallback={<PageLoader />}>
                  <AnalyticsTab />
                </Suspense>
              }
            />
            <Route
              path="known-issues"
              element={
                <Suspense fallback={<PageLoader />}>
                  <KnownIssuesTab />
                </Suspense>
              }
            />
            <Route
              path="defects"
              element={
                <Suspense fallback={<PageLoader />}>
                  <ProjectDefectsView />
                </Suspense>
              }
            />
            <Route
              path="builds/:buildId/defects"
              element={
                <Suspense fallback={<PageLoader />}>
                  <BuildDefectsView />
                </Suspense>
              }
            />
            <Route
              path="timeline"
              element={
                <Suspense fallback={<PageLoader />}>
                  <TimelineTab />
                </Suspense>
              }
            />
            <Route
              path="attachments"
              element={
                <Suspense fallback={<PageLoader />}>
                  <AttachmentsTab />
                </Suspense>
              }
            />
            <Route
              path="compare"
              element={
                <Suspense fallback={<PageLoader />}>
                  <ComparePage />
                </Suspense>
              }
            />
            <Route
              path="tests"
              element={
                <Suspense fallback={<PageLoader />}>
                  <TestHistoryPage />
                </Suspense>
              }
            />
            <Route
              path="reports/:reportId"
              element={
                <Suspense fallback={<PageLoader />}>
                  <ReportViewerPage />
                </Suspense>
              }
            />
            <Route
              path="trace/:source"
              element={
                <Suspense fallback={<PageLoader />}>
                  <TraceViewerPage />
                </Suspense>
              }
            />
          </Route>
          <Route
            path="admin"
            element={
              <Suspense fallback={<PageLoader />}>
                <AdminPage />
              </Suspense>
            }
          />
          <Route
            path="settings/api-keys"
            element={
              <Suspense fallback={<PageLoader />}>
                <APIKeysPage />
              </Suspense>
            }
          />
          <Route
            path="settings/webhooks"
            element={
              <Suspense fallback={<PageLoader />}>
                <WebhooksPage />
              </Suspense>
            }
          />
          <Route
            path="settings/users"
            element={
              <Suspense fallback={<PageLoader />}>
                <UsersPage />
              </Suspense>
            }
          />
          <Route
            path="settings/profile"
            element={
              <Suspense fallback={<PageLoader />}>
                <ProfilePage />
              </Suspense>
            }
          />
          <Route path="dashboard" element={<Navigate to="/" replace />} />
          <Route path="*" element={<NotFound />} />
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </ErrorBoundary>
  )
}
