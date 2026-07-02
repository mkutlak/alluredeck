import * as React from 'react'
import { Search } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Input } from '@/components/ui/input'

export interface SearchInputProps extends Omit<
  React.InputHTMLAttributes<HTMLInputElement>,
  'aria-label'
> {
  'aria-label': string
}

const SearchInput = React.forwardRef<HTMLInputElement, SearchInputProps>(
  ({ className, ...props }, ref) => {
    return (
      <div className="relative w-64">
        <Search
          className="text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 h-4 w-4 -translate-y-1/2"
          aria-hidden="true"
        />
        <Input ref={ref} className={cn('pl-8', className)} {...props} />
      </div>
    )
  },
)
SearchInput.displayName = 'SearchInput'

export { SearchInput }
