import { Link, useLocation, useParams } from 'react-router'
import { FileText, Folder } from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { useProjectFromParam } from '@/lib/resolveProject'
import { buildBreadcrumbs, LOADING_CRUMB, type BreadcrumbParams } from './breadcrumbs'

const CRUMB_ICONS = {
  folder: <Folder size={14} className="text-muted-foreground" />,
  file: <FileText size={14} />,
}

export function BreadcrumbBar() {
  const location = useLocation()
  const params = useParams<keyof BreadcrumbParams>()
  const resolved = useProjectFromParam(params.id)
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
        <ol className="flex items-center gap-1.5">
          {crumbs.map((crumb, i) => {
            const isLast = i === crumbs.length - 1
            return (
              <li key={i} className="flex items-center gap-1.5">
                {i > 0 && <span className="text-muted-foreground select-none">/</span>}
                {crumb.icon && <span className="flex items-center">{CRUMB_ICONS[crumb.icon]}</span>}
                {crumb.href && !isLast ? (
                  <Link
                    to={crumb.href}
                    className="text-muted-foreground hover:text-foreground transition-colors"
                  >
                    {crumb.label}
                  </Link>
                ) : (
                  <span className={isLast ? 'font-medium' : 'text-muted-foreground'}>
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
