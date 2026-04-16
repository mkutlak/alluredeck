import type { AdminJobStatus } from '@/types/api'

export function jobStatusVariant(
  status: AdminJobStatus,
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case 'running':
      return 'default'
    case 'completed':
      return 'secondary'
    case 'failed':
      return 'destructive'
    case 'retrying':
      return 'outline'
    case 'cancelled':
      return 'outline'
    default:
      return 'outline'
  }
}

export function isTerminalStatus(status: AdminJobStatus): boolean {
  return status === 'completed' || status === 'failed' || status === 'cancelled'
}
