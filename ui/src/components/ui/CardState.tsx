import { AlertCircle, RefreshCw } from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { extractErrorMessage } from '@/api/client'

interface CardStateProps {
  isLoading: boolean
  isError: boolean
  error?: unknown
  isEmpty: boolean
  refetch: () => void
  /** Shown when the card has no data but no error either. */
  emptyMessage?: string
  /** Number of skeleton rows to show while loading. */
  skeletonRows?: number
  /** Content to render when data is available. */
  children: React.ReactNode
}

/**
 * Wraps a card body with four distinct states:
 *  1. Loading  — skeleton rows
 *  2. Error    — visually distinct alert with a Retry button
 *  3. Empty    — muted "no data" message
 *  4. Content  — renders children
 *
 * Usage:
 *   <CardState isLoading={isLoading} isError={isError} error={error}
 *              isEmpty={items.length === 0} refetch={refetch}>
 *     <MyContent items={items} />
 *   </CardState>
 */
export function CardState({
  isLoading,
  isError,
  error,
  isEmpty,
  refetch,
  emptyMessage = 'No data available',
  skeletonRows = 5,
  children,
}: CardStateProps) {
  if (isLoading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: skeletonRows }).map((_, i) => (
          <Skeleton key={i} className="h-8 w-full" />
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex flex-col items-center gap-2 py-4 text-center">
        <AlertCircle className="text-destructive h-5 w-5" aria-hidden="true" />
        <p className="text-destructive text-sm font-medium">Couldn&apos;t load data</p>
        {error !== undefined && (
          <p className="text-muted-foreground max-w-xs text-xs">{extractErrorMessage(error)}</p>
        )}
        <Button
          variant="outline"
          size="sm"
          className="mt-1 h-7 gap-1 text-xs"
          onClick={() => refetch()}
        >
          <RefreshCw className="h-3 w-3" />
          Retry
        </Button>
      </div>
    )
  }

  if (isEmpty) {
    return (
      <p className="text-muted-foreground py-4 text-center text-sm">{emptyMessage}</p>
    )
  }

  return <>{children}</>
}
