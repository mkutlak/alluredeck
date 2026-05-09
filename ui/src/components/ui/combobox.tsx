import * as React from 'react'
import { Check, ChevronsUpDown } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from '@/components/ui/command'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'

export type ComboboxOption = { value: string; label: string }

export type ComboboxProps = {
  options: ComboboxOption[]
  value: string | null
  onChange: (v: string | null) => void
  placeholder?: string
  searchPlaceholder?: string
  emptyText?: string
  allowClear?: boolean
  className?: string
  disabled?: boolean
}

export function Combobox({
  options,
  value,
  onChange,
  placeholder = 'Select…',
  searchPlaceholder = 'Search…',
  emptyText = 'No results found.',
  allowClear = false,
  className,
  disabled = false,
}: ComboboxProps): React.JSX.Element {
  const [open, setOpen] = React.useState<boolean>(false)

  const selectedLabel = value !== null ? (options.find((o) => o.value === value)?.label ?? value) : null

  function handleSelect(optionValue: string) {
    onChange(optionValue)
    setOpen(false)
  }

  function handleClear() {
    onChange(null)
    setOpen(false)
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          variant="outline"
          role="combobox"
          aria-expanded={open}
          disabled={disabled}
          className={cn('w-full justify-between', className)}
        >
          <span className={cn(!selectedLabel && 'text-muted-foreground')}>
            {selectedLabel ?? placeholder}
          </span>
          <ChevronsUpDown className="ml-2 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent className="w-[var(--radix-popover-trigger-width)] p-0">
        <Command>
          <CommandInput placeholder={searchPlaceholder} />
          <CommandList>
            <CommandEmpty>{emptyText}</CommandEmpty>
            <CommandGroup>
              {allowClear && value !== null && (
                <CommandItem onSelect={handleClear}>
                  <span className="text-muted-foreground">Clear selection</span>
                </CommandItem>
              )}
              {options.map((option) => (
                <CommandItem
                  key={option.value}
                  value={option.value}
                  onSelect={handleSelect}
                >
                  <Check
                    className={cn('mr-2', value === option.value ? 'opacity-100' : 'opacity-0')}
                  />
                  {option.label}
                </CommandItem>
              ))}
            </CommandGroup>
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  )
}
