import { useNavigate } from 'react-router-dom'
import { useDisks } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { EmptyState } from '@/components/core/empty-state'
import { Badge } from '@/components/ui/badge'
import { HardDrive } from 'lucide-react'
import { formatBytes, formatPercent } from '@/lib/format'
import { ROLE_LABELS } from '@/features/storage/roles'
import { cn } from '@/lib/utils'
import type { Disk } from '@/api/types'

function tempClass(temp: number | null): string {
  if (temp == null) return ''
  if (temp >= 45) return 'text-critical'
  if (temp >= 41) return 'text-attention'
  return ''
}

export function DisksTab() {
  const navigate = useNavigate()
  const { data: disks, isLoading } = useDisks()

  const columns: Column<Disk>[] = [
    {
      id: 'name',
      header: 'Name',
      sortValue: (d) => d.name,
      searchValue: (d) => `${d.name} ${d.model} ${d.serial} ${d.wwn ?? ''}`,
      cell: (d) => (
        <div className="flex flex-col">
          <span className="font-mono text-[13px] font-medium">{d.name}</span>
          <span className="text-xs text-muted-foreground">{d.model}</span>
        </div>
      ),
    },
    {
      id: 'role',
      header: 'Role',
      sortValue: (d) => ROLE_LABELS[d.role],
      cell: (d) => (
        <Badge variant={d.role === 'parity' ? 'default' : 'secondary'}>
          {ROLE_LABELS[d.role]}
        </Badge>
      ),
    },
    {
      id: 'capacity',
      header: 'Capacity',
      className: 'tnum',
      sortValue: (d) => d.sizeBytes,
      cell: (d) => formatBytes(d.sizeBytes),
    },
    {
      id: 'used',
      header: 'Used',
      className: 'tnum',
      sortValue: (d) => (d.usedBytes ?? 0) / d.sizeBytes,
      cell: (d) => {
        if (d.usedBytes == null) return '—'
        const percent = (d.usedBytes / d.sizeBytes) * 100
        return (
          <span>
            {formatBytes(d.usedBytes)}{' '}
            <span className="text-xs text-muted-foreground">({formatPercent(percent)})</span>
          </span>
        )
      },
    },
    {
      id: 'temp',
      header: 'Temp',
      className: 'tnum',
      sortValue: (d) => d.temperatureC ?? -1,
      cell: (d) => (
        <span className={cn('tnum', tempClass(d.temperatureC))}>
          {d.temperatureC != null ? `${d.temperatureC}°C` : '—'}
        </span>
      ),
    },
    {
      id: 'health',
      header: 'Health',
      sortValue: (d) => d.health,
      cell: (d) => <HealthBadge state={d.health} />,
    },
    {
      id: 'serial',
      header: 'Serial',
      advanced: true,
      cell: (d) => <span className="font-mono text-xs">{d.serial}</span>,
    },
    {
      id: 'wwn',
      header: 'WWN',
      advanced: true,
      cell: (d) => <span className="font-mono text-xs">{d.wwn ?? '—'}</span>,
    },
    {
      id: 'hours',
      header: 'Power-on',
      advanced: true,
      className: 'tnum',
      sortValue: (d) => d.smart.powerOnHours,
      cell: (d) => `${d.smart.powerOnHours.toLocaleString()} h`,
    },
    {
      id: 'standby',
      header: 'Standby',
      advanced: true,
      cell: (d) => (d.standby ? 'Yes' : 'No'),
    },
  ]

  return (
    <ResourceTable
      columns={columns}
      rows={disks ?? []}
      loading={isLoading}
      onRowClick={(disk) => navigate(`/storage?disk=${disk.id}`)}
      emptyState={
        <EmptyState
          icon={<HardDrive />}
          title="No disks discovered"
          description="Disks detected on this system will appear here."
          className="border-0"
        />
      }
    />
  )
}
