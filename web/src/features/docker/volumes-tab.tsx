import { HardDrive } from 'lucide-react'
import { useDockerVolumes } from '@/api/queries'
import { EmptyState } from '@/components/core/empty-state'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { formatBytes } from '@/lib/format'
import type { DockerVolume } from '@/api/types'

export function VolumesTab() {
  const { data: volumes, isLoading } = useDockerVolumes()

  const columns: Column<DockerVolume>[] = [
    {
      id: 'name',
      header: 'Volume',
      sortValue: (v) => v.name,
      cell: (v) => <span className="font-mono text-[13px] font-medium">{v.name}</span>,
    },
    {
      id: 'stack',
      header: 'Stack',
      sortValue: (v) => v.stackName ?? '',
      cell: (v) => v.stackName ?? <span className="text-muted-foreground">—</span>,
    },
    {
      id: 'type',
      header: 'Type',
      sortValue: (v) => (v.bindPath ? 'bind' : 'named'),
      cell: (v) => (
        <span className="text-xs text-muted-foreground">{v.bindPath ? 'Bind mount' : 'Named volume'}</span>
      ),
    },
    {
      id: 'path',
      header: 'Host path',
      advanced: true,
      cell: (v) => (
        <span className="font-mono text-xs text-muted-foreground">{v.bindPath ?? '—'}</span>
      ),
    },
    {
      id: 'used',
      header: 'Used',
      className: 'tnum',
      sortValue: (v) => v.usedBytes,
      cell: (v) => formatBytes(v.usedBytes),
    },
  ]

  return (
    <ResourceTable
      columns={columns}
      rows={volumes ?? []}
      loading={isLoading}
      emptyState={
        <EmptyState
          icon={<HardDrive />}
          title="No volumes"
          description="Storage used by Docker apps will appear here."
          className="border-0"
        />
      }
    />
  )
}
