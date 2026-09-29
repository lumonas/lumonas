import { useNavigate } from 'react-router-dom'
import { ArrowUpCircle, Container } from 'lucide-react'
import { useDockerStacks, useStackAction } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { EmptyState } from '@/components/core/empty-state'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { StateChip } from '@/features/docker/docker-meta'
import { Button } from '@/components/ui/button'
import { formatBytes, timeAgo } from '@/lib/format'
import type { DockerStack } from '@/api/types'

export function StacksTab() {
  const navigate = useNavigate()
  const { data: stacks, isLoading } = useDockerStacks()
  const stackAction = useStackAction()

  const columns: Column<DockerStack>[] = [
    {
      id: 'name',
      header: 'Stack',
      sortValue: (s) => s.name,
      searchValue: (s) => `${s.name} ${s.category} ${s.images.join(' ')}`,
      cell: (s) => (
        <div className="flex flex-col">
          <span className="font-mono text-[13px] font-medium">{s.name}</span>
          <span className="text-xs text-muted-foreground">{s.category}</span>
        </div>
      ),
    },
    {
      id: 'state',
      header: 'State',
      sortValue: (s) => s.state,
      cell: (s) => <StateChip state={s.state} kind="stack" />,
    },
    {
      id: 'status',
      header: 'Health',
      sortValue: (s) => s.status,
      cell: (s) => <HealthBadge state={s.status} />,
    },
    {
      id: 'image',
      header: 'Image',
      cell: (s) => (
        <span className="font-mono text-xs text-muted-foreground">
          {s.images[0]}
          {s.images.length > 1 ? ` +${s.images.length - 1}` : ''}
        </span>
      ),
    },
    {
      id: 'cpu',
      header: 'CPU',
      className: 'tnum',
      sortValue: (s) => s.cpuPercent,
      cell: (s) => `${s.cpuPercent}%`,
    },
    {
      id: 'ram',
      header: 'Memory',
      className: 'tnum',
      sortValue: (s) => s.ramUsedBytes,
      cell: (s) => formatBytes(s.ramUsedBytes),
    },
    {
      id: 'deployed',
      header: 'Deployed',
      className: 'tnum',
      sortValue: (s) => s.lastDeploy,
      cell: (s) => <span className="text-muted-foreground">{timeAgo(s.lastDeploy)}</span>,
    },
    {
      id: 'update',
      header: 'Update',
      cell: (s) =>
        s.updateAvailable ? (
          <Button
            size="sm"
            variant="outline"
            className="h-7 gap-1.5 text-xs"
            onClick={(e) => {
              e.stopPropagation()
              stackAction.mutate({ id: s.id, action: 'update', body: { backup: true } })
            }}
          >
            <ArrowUpCircle className="text-primary" />
            {s.updateAvailable.latest}
          </Button>
        ) : (
          <span className="text-xs text-muted-foreground">—</span>
        ),
    },
  ]

  return (
    <ResourceTable
      columns={columns}
      rows={stacks ?? []}
      loading={isLoading}
      onRowClick={(stack) => navigate(`/docker?tab=stacks&stack=${stack.id}`)}
      emptyState={
        <EmptyState
          icon={<Container />}
          title="No stacks installed"
          description="Install an app from the Apps tab or paste a Compose file to get started."
          className="border-0"
        />
      }
    />
  )
}
