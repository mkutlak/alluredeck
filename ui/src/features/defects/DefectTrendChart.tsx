interface DefectTrendChartProps {
  projectId: string
}

export function DefectTrendChart({ projectId: _projectId }: DefectTrendChartProps) {
  return (
    <div className="flex items-center justify-center rounded-lg border border-dashed p-8">
      <p className="text-muted-foreground text-sm">Defect trends coming soon</p>
    </div>
  )
}
