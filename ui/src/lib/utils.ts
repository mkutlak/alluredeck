import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}

const _dateFormatter = new Intl.DateTimeFormat('en-US', {
  year: 'numeric',
  month: 'short',
  day: '2-digit',
  hour: '2-digit',
  minute: '2-digit',
})

export function formatDate(dateStr: string | Date | number): string {
  const date =
    typeof dateStr === 'string' || typeof dateStr === 'number' ? new Date(dateStr) : dateStr
  return _dateFormatter.format(date)
}

export function formatDuration(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  if (m < 60) return s > 0 ? `${m}m ${s}s` : `${m}m`
  const h = Math.floor(m / 60)
  const rm = m % 60
  return rm > 0 ? `${h}h ${rm}m` : `${h}h`
}

export function calcPassRate(passed: number, total: number): number {
  if (total === 0) return 0
  return Math.round((passed / total) * 100)
}

export function getStatusVariant(
  status: string,
): 'passed' | 'failed' | 'broken' | 'skipped' | 'default' {
  switch (status.toLowerCase()) {
    case 'passed':
      return 'passed'
    case 'failed':
      return 'failed'
    case 'broken':
      return 'broken'
    case 'skipped':
      return 'skipped'
    default:
      return 'default'
  }
}

export function truncate(str: string, maxLen = 40): string {
  return str.length > maxLen ? str.slice(0, maxLen - 1) + '…' : str
}

export function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  const value = bytes / Math.pow(1024, i)
  return `${value % 1 === 0 ? value : value.toFixed(1)} ${units[i]}`
}
