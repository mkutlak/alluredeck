import { Navigate } from 'react-router'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import { JobsCard } from './components/JobsCard'
import { ResultsCard } from './components/ResultsCard'

export function AdminPage() {
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
