import { useCallback } from 'react'
import { Label } from '@/components/ui/label'

export interface BuildCountSelectorProps {
  value: number
  onChange: (value: number) => void
}

const BUILD_OPTIONS = Array.from({ length: 10 }, (_, i) => i + 1)

export function BuildCountSelector({ value, onChange }: BuildCountSelectorProps) {
  const handleChange = useCallback(
    (e: React.ChangeEvent<HTMLSelectElement>) => {
      onChange(Number(e.target.value))
    },
    [onChange],
  )

  return (
    <div className="flex flex-col gap-1">
      <Label htmlFor="build-count">Builds</Label>
      <select
        id="build-count"
        aria-label="Builds"
        value={String(value)}
        onChange={handleChange}
        className="border-input bg-background ring-offset-background focus-visible:ring-ring h-9 rounded-md border px-3 py-1 text-sm focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:outline-none"
      >
        {BUILD_OPTIONS.map((n) => (
          <option key={n} value={String(n)}>
            {n} {n === 1 ? 'build' : 'builds'}
          </option>
        ))}
      </select>
    </div>
  )
}
