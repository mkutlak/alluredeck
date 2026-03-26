import { useCallback } from 'react'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'

export interface DateRangePickerProps {
  from: string | undefined
  to: string | undefined
  onRangeChange: (from: string | undefined, to: string | undefined) => void
}

export function DateRangePicker({ from, to, onRangeChange }: DateRangePickerProps) {
  const handleFromChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value || undefined
      onRangeChange(value, to)
    },
    [to, onRangeChange],
  )

  const handleToChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value || undefined
      onRangeChange(from, value)
    },
    [from, onRangeChange],
  )

  const handleClear = useCallback(() => {
    onRangeChange(undefined, undefined)
  }, [onRangeChange])

  const hasRange = from !== undefined || to !== undefined

  return (
    <div className="flex items-end gap-2">
      <div className="flex flex-col gap-1">
        <Label htmlFor="date-from">From</Label>
        <Input
          id="date-from"
          type="date"
          value={from ?? ''}
          onChange={handleFromChange}
          className="w-36"
        />
      </div>
      <div className="flex flex-col gap-1">
        <Label htmlFor="date-to">To</Label>
        <Input
          id="date-to"
          type="date"
          value={to ?? ''}
          onChange={handleToChange}
          className="w-36"
        />
      </div>
      {hasRange && (
        <Button variant="ghost" size="sm" onClick={handleClear}>
          Clear
        </Button>
      )}
    </div>
  )
}
