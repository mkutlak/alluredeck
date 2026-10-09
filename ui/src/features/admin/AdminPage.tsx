import { Navigate } from 'react-router'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import { JobsCard } from './components/JobsCard'
import { ResultsCard } from './components/ResultsCard'
import { env } from '@/lib/env'
import { useDocumentTitle } from '@/lib/useDocumentTitle'

export function AdminPage() {
  useDocumentTitle(`System Monitor — ${env.appTitle}`)
  const isAdmin = useAuthStore(selectIsAdmin)

  if (!isAdmin) {
    return <Navigate to="/" replace />
  }

  return (
    <div className="space-y-6 p-6">
      <h1 className="text-2xl font-bold">System Monitor</h1>
      <JobsCard />
      <ResultsCard />
    </div>
  )
}
