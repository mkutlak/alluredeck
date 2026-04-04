import { useState, useEffect, useCallback, createContext, useContext } from 'react'
import { useNavigate } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { FolderOpen, FlaskConical } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandSeparator,
} from '@/components/ui/command'
import { useDebounce } from '@/hooks/useDebounce'
import { search } from '@/api/search'
import { queryKeys } from '@/lib/query-keys'

type SearchCommandContextValue = {
  open: boolean
  setOpen: (open: boolean) => void
}

const SearchCommandContext = createContext<SearchCommandContextValue>({
  open: false,
  setOpen: () => {
    throw new Error('useSearchCommand must be used within SearchCommand')
  },
})

// eslint-disable-next-line react-refresh/only-export-components -- hook intentionally co-located with its companion component
export function useSearchCommand() {
  return useContext(SearchCommandContext)
}

export function SearchCommand({ children }: { children?: React.ReactNode }) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const navigate = useNavigate()

  const debouncedQuery = useDebounce(query, 300)

  const { data, isFetching } = useQuery({
    queryKey: queryKeys.search(debouncedQuery),
    queryFn: () => search({ q: debouncedQuery }),
    enabled: debouncedQuery.length >= 2,
    staleTime: 10_000,
  })

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
      e.preventDefault()
      setOpen((prev) => !prev)
    }
  }, [])

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [handleKeyDown])

  const handleSelect = (path: string) => {
    setOpen(false)
    setQuery('')
    navigate(path)
  }

  const handleOpenChange = (value: boolean) => {
    setOpen(value)
    if (!value) setQuery('')
  }

  const projects = data?.data.projects ?? []
  const tests = data?.data.tests ?? []
  const hasQuery = debouncedQuery.length >= 2
  const hasResults = projects.length > 0 || tests.length > 0

  return (
    <SearchCommandContext.Provider value={{ open, setOpen }}>
      {children}
      <CommandDialog open={open} onOpenChange={handleOpenChange} shouldFilter={false}>
        <CommandInput
          placeholder="Search projects and tests..."
          value={query}
          onValueChange={setQuery}
        />
        <CommandList>
          {isFetching && hasQuery && <CommandEmpty>Searching...</CommandEmpty>}

          {!isFetching && hasQuery && !hasResults && <CommandEmpty>No results found.</CommandEmpty>}

          {!hasQuery && <CommandEmpty>Type at least 2 characters to search.</CommandEmpty>}

          {projects.length > 0 && (
            <CommandGroup heading="Projects">
              {projects.map((p) => (
                <CommandItem
                  key={p.project_id}
                  value={`project-${p.project_id}`}
                  onSelect={() => handleSelect(`/projects/${p.project_id}`)}
                >
                  <FolderOpen className="text-muted-foreground size-4 shrink-0" />
                  <span>{p.project_id}</span>
                </CommandItem>
              ))}
            </CommandGroup>
          )}

          {projects.length > 0 && tests.length > 0 && <CommandSeparator />}

          {tests.length > 0 && (
            <CommandGroup heading="Tests">
              {tests.map((t, i) => (
                <CommandItem
                  key={`${t.project_id}-${t.test_name}-${i}`}
                  value={`test-${t.project_id}-${t.test_name}-${i}`}
                  onSelect={() => handleSelect(`/projects/${t.project_id}`)}
                >
                  <FlaskConical className="text-muted-foreground size-4 shrink-0" />
                  <div className="min-w-0 flex-1">
                    <span className="block truncate">{t.test_name}</span>
                    <span className="text-muted-foreground block truncate text-xs">
                      {t.project_id}
                    </span>
                  </div>
                  <Badge variant="outline" className="ml-auto shrink-0 text-xs">
                    {t.status}
                  </Badge>
                </CommandItem>
              ))}
            </CommandGroup>
          )}
        </CommandList>
      </CommandDialog>
    </SearchCommandContext.Provider>
  )
}
