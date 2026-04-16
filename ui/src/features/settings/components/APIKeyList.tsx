import { Trash2 } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { formatDate } from '@/lib/utils'
import type { APIKey } from '@/types/api'
import { RoleBadge } from './RoleBadge'

function isExpired(expiresAt: string | null): boolean {
  if (!expiresAt) return false
  return new Date(expiresAt) < new Date()
}

interface APIKeyListProps {
  keys: APIKey[]
  onDelete: (key: APIKey) => void
}

export function APIKeyList({ keys, onDelete }: APIKeyListProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Name</TableHead>
          <TableHead>Prefix</TableHead>
          <TableHead>Role</TableHead>
          <TableHead>Expires</TableHead>
          <TableHead>Last Used</TableHead>
          <TableHead>Created</TableHead>
          <TableHead className="w-10" />
        </TableRow>
      </TableHeader>
      <TableBody>
        {keys.map((key) => {
          const expired = isExpired(key.expires_at)
          return (
            <TableRow key={key.id} className={expired ? 'opacity-50' : undefined}>
              <TableCell className="font-medium">
                <span className="flex items-center gap-2">
                  {key.name}
                  {expired && (
                    <Badge variant="destructive" className="text-xs">
                      Expired
                    </Badge>
                  )}
                </span>
              </TableCell>
              <TableCell>
                <code className="font-mono text-sm">{key.prefix}</code>
              </TableCell>
              <TableCell>
                <RoleBadge role={key.role} />
              </TableCell>
              <TableCell className="text-muted-foreground text-sm">
                {key.expires_at ? formatDate(key.expires_at) : '—'}
              </TableCell>
              <TableCell className="text-muted-foreground text-sm">
                {key.last_used ? formatDate(key.last_used) : '—'}
              </TableCell>
              <TableCell className="text-muted-foreground text-sm">
                {key.created_at ? formatDate(key.created_at) : '—'}
              </TableCell>
              <TableCell>
                <Button
                  size="icon"
                  variant="ghost"
                  onClick={() => onDelete(key)}
                  aria-label={`Delete API key ${key.name}`}
                >
                  <Trash2 size={16} />
                </Button>
              </TableCell>
            </TableRow>
          )
        })}
      </TableBody>
    </Table>
  )
}
