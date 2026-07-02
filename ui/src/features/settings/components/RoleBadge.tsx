import { Badge } from '@/components/ui/badge'
import { INFO_BADGE_CLASSES } from '@/lib/status-colors'
import type { APIKey } from '@/types/api'

export function RoleBadge({ role }: { role: APIKey['role'] }) {
  if (role === 'admin') {
    return <Badge className={INFO_BADGE_CLASSES}>admin</Badge>
  }
  return <Badge variant="secondary">viewer</Badge>
}
