import { useState } from 'react'
import { Navigate } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Inbox } from 'lucide-react'
import { useAuthStore, selectIsAdmin } from '@/store/auth'
import { getConfig } from '@/api/system'
import {
  approveProposal,
  listDefectProposals,
  listFlakyProposals,
  listKnownIssueProposals,
  rejectProposal,
} from '@/api/proposals'
import { queryKeys } from '@/lib/query-keys'
import { formatDate } from '@/lib/utils'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Input } from '@/components/ui/input'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import type { DefectProposal, FlakyProposal, KnownIssueProposal, ProposalType } from '@/types/proposals'

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------
type TabType = 'defect' | 'known_issue' | 'flaky'

interface ApproveTarget {
  type: ProposalType
  id: number
}

interface RejectTarget {
  type: ProposalType
  id: number
}

// ---------------------------------------------------------------------------
// Status badge helper
// ---------------------------------------------------------------------------
function ProposalStatusBadge({ status }: { status: string }) {
  const variant =
    status === 'approved'
      ? 'passed'
      : status === 'rejected'
        ? 'destructive'
        : 'secondary'
  return <Badge variant={variant}>{status}</Badge>
}

// ---------------------------------------------------------------------------
// Approve confirmation dialog
// ---------------------------------------------------------------------------
interface ApproveDialogProps {
  target: ApproveTarget | null
  onOpenChange: (open: boolean) => void
  onConfirm: (target: ApproveTarget) => void
}

function ApproveDialog({ target, onOpenChange, onConfirm }: ApproveDialogProps) {
  return (
    <AlertDialog open={target !== null} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Approve proposal?</AlertDialogTitle>
          <AlertDialogDescription>
            This will mark the proposal as approved and apply its suggested change.
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction onClick={() => target && onConfirm(target)}>
            Approve
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  )
}

// ---------------------------------------------------------------------------
// Reject dialog (requires reason)
// ---------------------------------------------------------------------------
interface RejectDialogProps {
  target: RejectTarget | null
  onOpenChange: (open: boolean) => void
  onConfirm: (target: RejectTarget, reason: string) => void
}

function RejectDialog({ target, onOpenChange, onConfirm }: RejectDialogProps) {
  const [reason, setReason] = useState('')

  const handleConfirm = () => {
    if (target && reason.trim()) {
      onConfirm(target, reason.trim())
      setReason('')
    }
  }

  return (
    <Dialog
      open={target !== null}
      onOpenChange={(open) => {
        if (!open) setReason('')
        onOpenChange(open)
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Reject proposal</DialogTitle>
          <DialogDescription>
            Provide a reason for rejecting this proposal. This will be stored for audit purposes.
          </DialogDescription>
        </DialogHeader>
        <div className="py-2">
          <Input
            placeholder="Reason for rejection"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            aria-label="Rejection reason"
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant="destructive" onClick={handleConfirm} disabled={!reason.trim()}>
            Reject
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------
function EmptyState({ type }: { type: TabType }) {
  const labels: Record<TabType, string> = {
    defect: 'defect',
    known_issue: 'known issue',
    flaky: 'flaky test',
  }
  return (
    <div className="text-muted-foreground flex flex-col items-center gap-2 py-12 text-center">
      <Inbox className="h-10 w-10 opacity-40" />
      <p className="font-medium">No pending proposals</p>
      <p className="text-sm">
        Pending {labels[type]} proposals submitted by the MCP server will appear here.
      </p>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Defect proposals table
// ---------------------------------------------------------------------------
interface DefectTableProps {
  items: DefectProposal[]
  onApprove: (id: number) => void
  onReject: (id: number) => void
}

function DefectTable({ items, onApprove, onReject }: DefectTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Fingerprint</TableHead>
          <TableHead>Category</TableHead>
          <TableHead>Resolution</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Created</TableHead>
          <TableHead className="text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => (
          <TableRow key={item.id}>
            <TableCell className="font-mono text-xs">{item.fingerprint_hash.slice(0, 12)}…</TableCell>
            <TableCell>{item.proposed_category}</TableCell>
            <TableCell>{item.proposed_resolution ?? '—'}</TableCell>
            <TableCell>
              <ProposalStatusBadge status={item.status} />
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              {formatDate(item.created_at)}
            </TableCell>
            <TableCell className="text-right">
              {item.status === 'pending' && (
                <div className="flex justify-end gap-2">
                  <Button size="sm" variant="outline" onClick={() => onApprove(item.id)}>
                    Approve
                  </Button>
                  <Button size="sm" variant="destructive" onClick={() => onReject(item.id)}>
                    Reject
                  </Button>
                </div>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

// ---------------------------------------------------------------------------
// Known issue proposals table
// ---------------------------------------------------------------------------
interface KnownIssueTableProps {
  items: KnownIssueProposal[]
  onApprove: (id: number) => void
  onReject: (id: number) => void
}

function KnownIssueTable({ items, onApprove, onReject }: KnownIssueTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Error sample</TableHead>
          <TableHead>Pattern</TableHead>
          <TableHead>Category</TableHead>
          <TableHead>Dry-run matches</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Created</TableHead>
          <TableHead className="text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => (
          <TableRow key={item.id}>
            <TableCell className="max-w-xs truncate text-sm" title={item.error_message_sample}>
              {item.error_message_sample}
            </TableCell>
            <TableCell className="font-mono text-xs">{item.regex_pattern}</TableCell>
            <TableCell>{item.proposed_category}</TableCell>
            <TableCell>
              <span className="font-semibold">{item.dry_run_match_count}</span>
              <span className="text-muted-foreground ml-1 text-xs">recent failures</span>
            </TableCell>
            <TableCell>
              <ProposalStatusBadge status={item.status} />
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              {formatDate(item.created_at)}
            </TableCell>
            <TableCell className="text-right">
              {item.status === 'pending' && (
                <div className="flex justify-end gap-2">
                  <Button size="sm" variant="outline" onClick={() => onApprove(item.id)}>
                    Approve
                  </Button>
                  <Button size="sm" variant="destructive" onClick={() => onReject(item.id)}>
                    Reject
                  </Button>
                </div>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

// ---------------------------------------------------------------------------
// Flaky proposals table
// ---------------------------------------------------------------------------
interface FlakyTableProps {
  items: FlakyProposal[]
  onApprove: (id: number) => void
  onReject: (id: number) => void
}

function FlakyTable({ items, onApprove, onReject }: FlakyTableProps) {
  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead>Test name</TableHead>
          <TableHead>History ID</TableHead>
          <TableHead>Status</TableHead>
          <TableHead>Created</TableHead>
          <TableHead className="text-right">Actions</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.map((item) => (
          <TableRow key={item.id}>
            <TableCell className="max-w-xs truncate text-sm" title={item.test_full_name}>
              {item.test_full_name}
            </TableCell>
            <TableCell className="font-mono text-xs">{item.history_id}</TableCell>
            <TableCell>
              <ProposalStatusBadge status={item.status} />
            </TableCell>
            <TableCell className="text-muted-foreground text-sm">
              {formatDate(item.created_at)}
            </TableCell>
            <TableCell className="text-right">
              {item.status === 'pending' && (
                <div className="flex justify-end gap-2">
                  <Button size="sm" variant="outline" onClick={() => onApprove(item.id)}>
                    Approve
                  </Button>
                  <Button size="sm" variant="destructive" onClick={() => onReject(item.id)}>
                    Reject
                  </Button>
                </div>
              )}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  )
}

// ---------------------------------------------------------------------------
// Tab bar
// ---------------------------------------------------------------------------
const TABS: { id: TabType; label: string }[] = [
  { id: 'defect', label: 'Defects' },
  { id: 'known_issue', label: 'Known Issues' },
  { id: 'flaky', label: 'Flaky Tests' },
]

interface TabBarProps {
  active: TabType
  onChange: (tab: TabType) => void
}

function TabBar({ active, onChange }: TabBarProps) {
  return (
    <div className="border-b">
      <nav className="-mb-px flex gap-4">
        {TABS.map(({ id, label }) => (
          <button
            key={id}
            onClick={() => onChange(id)}
            className={
              active === id
                ? 'border-primary text-primary border-b-2 pb-2 text-sm font-medium'
                : 'text-muted-foreground hover:text-foreground border-b-2 border-transparent pb-2 text-sm font-medium transition-colors'
            }
          >
            {label}
          </button>
        ))}
      </nav>
    </div>
  )
}

// ---------------------------------------------------------------------------
// MCP disabled state
// ---------------------------------------------------------------------------
function McpDisabled() {
  return (
    <div className="space-y-6 p-6">
      <h1 className="text-2xl font-bold">Pending Proposals</h1>
      <div className="text-muted-foreground flex flex-col items-center gap-2 py-16 text-center">
        <Inbox className="h-12 w-12 opacity-40" />
        <p className="text-lg font-medium">MCP server is not enabled</p>
        <p className="text-sm">
          Enable the MCP server in your configuration to start receiving AI proposals.
        </p>
      </div>
    </div>
  )
}

// ---------------------------------------------------------------------------
// Main page
// ---------------------------------------------------------------------------
const LIMIT = 20

export function PendingProposalsPage() {
  const isAdmin = useAuthStore(selectIsAdmin)
  const queryClient = useQueryClient()

  const [activeTab, setActiveTab] = useState<TabType>('defect')
  const [cursors, setCursors] = useState<Record<TabType, string | undefined>>({
    defect: undefined,
    known_issue: undefined,
    flaky: undefined,
  })
  const [approveTarget, setApproveTarget] = useState<ApproveTarget | null>(null)
  const [rejectTarget, setRejectTarget] = useState<RejectTarget | null>(null)

  const { data: configResp } = useQuery({
    queryKey: ['config'],
    queryFn: getConfig,
  })
  const configData = configResp?.data

  const cursor = cursors[activeTab]

  // Defect proposals
  const defectQuery = useQuery({
    queryKey: queryKeys.proposals('defect', 0, cursor),
    queryFn: () =>
      listDefectProposals({ project_id: 0, status: 'pending', limit: LIMIT, cursor }),
    enabled: activeTab === 'defect' && configData?.mcp_enabled === true,
  })

  // Known issue proposals
  const knownIssueQuery = useQuery({
    queryKey: queryKeys.proposals('known_issue', 0, cursor),
    queryFn: () =>
      listKnownIssueProposals({ project_id: 0, status: 'pending', limit: LIMIT, cursor }),
    enabled: activeTab === 'known_issue' && configData?.mcp_enabled === true,
  })

  // Flaky proposals
  const flakyQuery = useQuery({
    queryKey: queryKeys.proposals('flaky', 0, cursor),
    queryFn: () =>
      listFlakyProposals({ project_id: 0, status: 'pending', limit: LIMIT, cursor }),
    enabled: activeTab === 'flaky' && configData?.mcp_enabled === true,
  })

  const invalidateActive = () => {
    void queryClient.invalidateQueries({
      queryKey: queryKeys.proposals(activeTab, 0),
    })
  }

  const approveMutation = useMutation({
    mutationFn: ({ type, id }: ApproveTarget) => approveProposal(type, id),
    onSuccess: () => {
      setApproveTarget(null)
      invalidateActive()
    },
  })

  const rejectMutation = useMutation({
    mutationFn: ({ type, id, reason }: RejectTarget & { reason: string }) =>
      rejectProposal(type, id, { reason }),
    onSuccess: () => {
      setRejectTarget(null)
      invalidateActive()
    },
  })

  if (!isAdmin) {
    return <Navigate to="/" replace />
  }

  if (configData !== undefined && !configData.mcp_enabled) {
    return <McpDisabled />
  }

  const activeQuery =
    activeTab === 'defect' ? defectQuery : activeTab === 'known_issue' ? knownIssueQuery : flakyQuery

  const isLoading = activeQuery.isLoading
  const nextCursor = activeQuery.data?.next_cursor

  const handleApprove = (id: number) => {
    setApproveTarget({ type: activeTab, id })
  }

  const handleReject = (id: number) => {
    setRejectTarget({ type: activeTab, id })
  }

  const handleLoadMore = () => {
    if (nextCursor) {
      setCursors((prev) => ({ ...prev, [activeTab]: nextCursor }))
    }
  }

  const renderTableContent = () => {
    if (isLoading) {
      return (
        <div className="space-y-2">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      )
    }

    if (activeTab === 'defect') {
      const items = defectQuery.data?.items ?? []
      if (items.length === 0) return <EmptyState type="defect" />
      return (
        <DefectTable
          items={items}
          onApprove={handleApprove}
          onReject={handleReject}
        />
      )
    }

    if (activeTab === 'known_issue') {
      const items = knownIssueQuery.data?.items ?? []
      if (items.length === 0) return <EmptyState type="known_issue" />
      return (
        <KnownIssueTable
          items={items}
          onApprove={handleApprove}
          onReject={handleReject}
        />
      )
    }

    const items = flakyQuery.data?.items ?? []
    if (items.length === 0) return <EmptyState type="flaky" />
    return (
      <FlakyTable
        items={items}
        onApprove={handleApprove}
        onReject={handleReject}
      />
    )
  }

  return (
    <div className="space-y-6 p-6">
      <h1 className="text-2xl font-bold">Pending Proposals</h1>

      <Card>
        <CardHeader>
          <CardTitle>AI Proposals</CardTitle>
          <CardDescription>
            Review and act on proposals submitted by the MCP server.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <TabBar active={activeTab} onChange={(tab) => { setActiveTab(tab) }} />

          {renderTableContent()}

          {nextCursor && !isLoading && (
            <div className="flex justify-center pt-2">
              <Button variant="outline" size="sm" onClick={handleLoadMore}>
                Load more
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      <ApproveDialog
        target={approveTarget}
        onOpenChange={(open) => { if (!open) setApproveTarget(null) }}
        onConfirm={(target) => approveMutation.mutate(target)}
      />

      <RejectDialog
        target={rejectTarget}
        onOpenChange={(open) => { if (!open) setRejectTarget(null) }}
        onConfirm={(target, reason) => rejectMutation.mutate({ ...target, reason })}
      />
    </div>
  )
}
