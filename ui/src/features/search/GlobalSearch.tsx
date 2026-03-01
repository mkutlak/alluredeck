import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { Search, FolderOpen, FlaskConical } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { useDebounce } from '@/hooks/useDebounce'
import { search } from '@/api/search'

export function GlobalSearch() {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const navigate = useNavigate()

  const debouncedQuery = useDebounce(query, 300)

  const { data, isFetching } = useQuery({
    queryKey: ['search', debouncedQuery],
    queryFn: () => search({ q: debouncedQuery }),
    enabled: debouncedQuery.length >= 2,
    staleTime: 10_000,
  })

  const handleKeyDown = useCallback(
    (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        setOpen((prev) => !prev)
      }
    },
    [],
  )

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [handleKeyDown])

  const handleSelect = (path: string) => {
    setOpen(false)
    setQuery('')
    navigate(path)
  }

  const projects = data?.data.projects ?? []
  const tests = data?.data.tests ?? []
  const hasQuery = debouncedQuery.length >= 2
  const hasResults = projects.length > 0 || tests.length > 0

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        className="gap-2 text-muted-foreground"
        onClick={() => setOpen(true)}
        aria-label="Search"
      >
        <Search size={14} />
        <span className="hidden sm:inline">Search</span>
        <kbd className="pointer-events-none hidden select-none rounded border bg-muted px-1.5 py-0.5 font-mono text-[10px] font-medium text-muted-foreground sm:inline">
          ⌘K
        </kbd>
      </Button>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="max-w-lg gap-0 p-0">
          <DialogHeader className="border-b px-4 py-3">
            <DialogTitle className="sr-only">Search</DialogTitle>
            <DialogDescription className="sr-only">
              Search across projects and test names
            </DialogDescription>
            <div className="flex items-center gap-2">
              <Search size={16} className="text-muted-foreground" />
              <Input
                placeholder="Search projects and tests..."
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                className="border-0 p-0 shadow-none focus-visible:ring-0"
                autoFocus
              />
            </div>
          </DialogHeader>

          <div className="max-h-80 overflow-y-auto p-2">
            {isFetching && hasQuery && (
              <p className="px-2 py-4 text-center text-sm text-muted-foreground">
                Searching...
              </p>
            )}

            {!isFetching && hasQuery && !hasResults && (
              <p className="px-2 py-4 text-center text-sm text-muted-foreground">
                No results found
              </p>
            )}

            {!hasQuery && (
              <p className="px-2 py-4 text-center text-sm text-muted-foreground">
                Type at least 2 characters to search
              </p>
            )}

            {projects.length > 0 && (
              <div className="mb-2">
                <p className="px-2 py-1 text-xs font-medium text-muted-foreground">
                  Projects
                </p>
                {projects.map((p) => (
                  <button
                    key={p.project_id}
                    type="button"
                    className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-accent"
                    onClick={() => handleSelect(`/projects/${p.project_id}`)}
                  >
                    <FolderOpen size={14} className="shrink-0 text-muted-foreground" />
                    <span>{p.project_id}</span>
                  </button>
                ))}
              </div>
            )}

            {tests.length > 0 && (
              <div>
                <p className="px-2 py-1 text-xs font-medium text-muted-foreground">
                  Tests
                </p>
                {tests.map((t, i) => (
                  <button
                    key={`${t.project_id}-${t.test_name}-${i}`}
                    type="button"
                    className="flex w-full items-center gap-2 rounded px-2 py-1.5 text-left text-sm hover:bg-accent"
                    onClick={() => handleSelect(`/projects/${t.project_id}`)}
                  >
                    <FlaskConical size={14} className="shrink-0 text-muted-foreground" />
                    <div className="min-w-0 flex-1">
                      <span className="block truncate">{t.test_name}</span>
                      <span className="block truncate text-xs text-muted-foreground">
                        {t.project_id}
                      </span>
                    </div>
                    <Badge variant="outline" className="shrink-0 text-xs">
                      {t.status}
                    </Badge>
                  </button>
                ))}
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
