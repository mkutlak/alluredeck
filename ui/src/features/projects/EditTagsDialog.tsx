import { useState, useRef } from 'react'
import { X } from 'lucide-react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Badge } from '@/components/ui/badge'
import { updateProjectTags, getTags } from '@/api/projects'
import { extractErrorMessage } from '@/api/client'
import { queryKeys } from '@/lib/query-keys'

interface Props {
  projectId: string
  currentTags: string[]
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function EditTagsDialog({ projectId, currentTags, open, onOpenChange }: Props) {
  const [tags, setTags] = useState<string[]>(currentTags)
  const [input, setInput] = useState('')
  const [error, setError] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)
  const qc = useQueryClient()

  const { data: allTagsResp } = useQuery({
    queryKey: queryKeys.tags,
    queryFn: getTags,
    staleTime: 60_000,
  })
  const allTags = allTagsResp?.data ?? []
  const suggestions = allTags.filter((t) => !tags.includes(t) && t.startsWith(input.trim()))

  const { mutate, isPending } = useMutation({
    mutationFn: () => updateProjectTags(projectId, tags),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.dashboard() })
      void qc.invalidateQueries({ queryKey: queryKeys.tags })
      onOpenChange(false)
    },
    onError: (e) => setError(extractErrorMessage(e)),
  })

  function addTag(tag: string) {
    const trimmed = tag.trim()
    if (!trimmed) return
    if (!/^[a-zA-Z0-9_-]+$/.test(trimmed)) {
      setError(
        'Tag contains invalid characters: only letters, numbers, hyphens, and underscores are allowed.',
      )
      return
    }
    if (trimmed.length > 50) {
      setError('Tag must be 50 characters or fewer.')
      return
    }
    if (tags.length >= 20) {
      setError('Maximum 20 tags per project.')
      return
    }
    if (tags.includes(trimmed)) return
    setTags((prev) => [...prev, trimmed])
    setInput('')
    setError('')
  }

  function removeTag(tag: string) {
    setTags((prev) => prev.filter((t) => t !== tag))
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault()
      addTag(input)
    } else if (e.key === 'Backspace' && input === '' && tags.length > 0) {
      setTags((prev) => prev.slice(0, -1))
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Edit tags — {projectId}</DialogTitle>
          <DialogDescription>
            Add tags to categorize this project. Press Enter or comma to add a tag.
          </DialogDescription>
        </DialogHeader>

        <div className="space-y-3">
          {/* Current tags */}
          {tags.length > 0 && (
            <div className="flex flex-wrap gap-1.5">
              {tags.map((tag) => (
                <Badge key={tag} variant="secondary" className="gap-1 pr-1">
                  {tag}
                  <button
                    type="button"
                    onClick={() => removeTag(tag)}
                    className="hover:bg-muted-foreground/20 ml-0.5 rounded-full"
                    aria-label={`Remove tag ${tag}`}
                  >
                    <X size={10} />
                  </button>
                </Badge>
              ))}
            </div>
          )}

          {/* Input */}
          <div className="relative">
            <Input
              ref={inputRef}
              placeholder="Type a tag and press Enter…"
              value={input}
              onChange={(e) => {
                setInput(e.target.value)
                setError('')
              }}
              onKeyDown={handleKeyDown}
            />
            {/* Autocomplete suggestions */}
            {input.trim() && suggestions.length > 0 && (
              <ul className="bg-popover absolute z-10 mt-1 w-full rounded-md border py-1 shadow-md">
                {suggestions.slice(0, 6).map((s) => (
                  <li key={s}>
                    <button
                      type="button"
                      className="hover:bg-accent w-full px-3 py-1.5 text-left text-sm"
                      onMouseDown={(e) => {
                        e.preventDefault()
                        addTag(s)
                        inputRef.current?.focus()
                      }}
                    >
                      {s}
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>

          {error && <p className="text-destructive text-sm">{error}</p>}
        </div>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={isPending}>
            Cancel
          </Button>
          <Button onClick={() => mutate()} disabled={isPending}>
            {isPending ? 'Saving…' : 'Save tags'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
