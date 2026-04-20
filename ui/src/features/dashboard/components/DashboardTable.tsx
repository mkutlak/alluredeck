import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { DndProjectProvider } from '@/features/projects/components/DndProjectProvider'
import { NoGroupDropZone } from '@/features/projects/components/NoGroupDropZone'
import type { DndProject } from '@/features/projects/hooks/useProjectDnd'
import type { DashboardProjectEntry } from '@/types/api'
import { DashboardProjectRow } from './DashboardProjectRow'
import { SortableHeader } from './DashboardSortControls'
import type { SortField } from './sort'

interface DashboardTableProps {
  rows: DashboardProjectEntry[]
  dndProjects: DndProject[]
  isAdmin: boolean
  search: string
  onSort: (field: SortField) => void
  onDrillDown: (projectId: number) => void
  parentSlugMap?: Map<number, string>
}

export function DashboardTable({
  rows,
  dndProjects,
  isAdmin,
  search,
  onSort,
  onDrillDown,
  parentSlugMap,
}: DashboardTableProps) {
  return (
    <DndProjectProvider projects={dndProjects}>
      <NoGroupDropZone />
      <Table>
        <TableHeader>
          <TableRow>
            <SortableHeader field="name" label="Name" onSort={onSort} />
            <SortableHeader field="type" label="Type" onSort={onSort} />
            <SortableHeader field="pass_rate" label="Pass Rate" onSort={onSort} />
            {isAdmin && <TableHead className="w-10" />}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.length === 0 ? (
            <TableRow>
              <TableCell colSpan={isAdmin ? 4 : 3} className="text-muted-foreground py-8 text-center">
                {search ? 'No projects match your search.' : 'No projects found.'}
              </TableCell>
            </TableRow>
          ) : (
            rows.map((project) => (
              <DashboardProjectRow
                key={project.project_id}
                project={project}
                isAdmin={isAdmin}
                onDrillDown={project.is_group ? () => onDrillDown(project.project_id) : undefined}
                parentSlug={parentSlugMap?.get(project.project_id)}
              />
            ))
          )}
        </TableBody>
      </Table>
    </DndProjectProvider>
  )
}
