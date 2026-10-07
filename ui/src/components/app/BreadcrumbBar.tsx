import { Link, useLocation, useParams } from 'react-router'
import { FileText, Folder } from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { useProjectFromParam } from '@/lib/resolveProject'
import { buildBreadcrumbs, LOADING_CRUMB, type BreadcrumbParams } from './breadcrumbs'

const CRUMB_ICONS = {
  folder: <Folder size={14} className="text-muted-foreground" />,
  file: <FileText size={14} />,
}

export function BreadcrumbBar() {
  const location = useLocation()
  const params = useParams<keyof BreadcrumbParams>()
  const resolved = useProjectFromParam(params.id ?? params.groupId)
  const crumbs = buildBreadcrumbs(location.pathname, params, resolved)

  if (!crumbs) return null

  const isLoadingState = crumbs.length === 1 && crumbs[0]?.label === LOADING_CRUMB

  return (
    <nav
      aria-label="Breadcrumb"
      className="bg-background flex h-10 shrink-0 items-center gap-1.5 border-b px-4 text-sm"
    >
      {isLoadingState || resolved.isLoading ? (
        <>
          <Skeleton className="h-4 w-16" data-testid="breadcrumb-skeleton" />
          <span className="text-muted-foreground">/</span>
          <Skeleton className="h-4 w-24" data-testid="breadcrumb-skeleton" />
        </>
      ) : (
        // Crumbs truncate rather than wrap: a long group name used to spill three lines
        // out of the 40px bar on a phone. The short root and the current page do not shrink.
        <ol className="flex min-w-0 items-center gap-1.5">
          {crumbs.map((crumb, i) => {
            const isLast = i === crumbs.length - 1
            return (
              <li
                key={i}
                className={cn(
                  'flex min-w-0 items-center gap-1.5',
                  (i === 0 || isLast) && 'shrink-0',
                  isLast && 'max-w-[70%]',
                )}
              >
                {i > 0 && <span className="text-muted-foreground shrink-0 select-none">/</span>}
                {crumb.icon && (
                  <span className="flex shrink-0 items-center">{CRUMB_ICONS[crumb.icon]}</span>
                )}
                {crumb.href && !isLast ? (
                  <Link
                    to={crumb.href}
                    title={crumb.label}
                    className="text-fact hover:text-foreground min-w-0 truncate transition-colors"
                  >
                    {crumb.label}
                  </Link>
                ) : (
                  <span
                    title={crumb.label}
                    className={cn('min-w-0 truncate', isLast ? 'font-medium' : 'text-fact')}
                  >
                    {crumb.label}
                  </span>
                )}
              </li>
            )
          })}
        </ol>
      )}
    </nav>
  )
}
