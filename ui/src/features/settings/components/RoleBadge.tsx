import { Badge } from '@/components/ui/badge'
import type { APIKey } from '@/types/api'

export function RoleBadge({ role }: { role: APIKey['role'] }) {
  if (role === 'admin') {
    return (
      <Badge className="border-transparent bg-blue-100 text-blue-700 hover:bg-blue-100/80 dark:bg-blue-900/30 dark:text-blue-400">
        admin
      </Badge>
    )
  }
  return <Badge variant="secondary">viewer</Badge>
}
