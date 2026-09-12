import { Play, RotateCw, Square } from 'lucide-react'
import { useContainerAction, useDockerContainers } from '@/api/queries'
import { EmptyState } from '@/components/core/empty-state'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { Button } from '@/components/ui/button'
import { StateChip } from '@/features/docker/docker-meta'
import { formatBytes, timeAgo } from '@/lib/format'
import { Container } from 'lucide-react'
import type { DockerContainer } from '@/api/types'

export function ContainersTab() {
  const { data: containers, isLoading } = useDockerContainers()
  const containerAction = useContainerAction()

  const columns: Column<DockerContainer>[] = [
    {
      id: 'name',
      header: 'Container',
      sortValue: (c) => c.name,
      cell: (c) => (
        <div className="flex flex-col">
          <span className="font-mono text-[13px] font-medium">{c.name}</span>
          <span className="font-mono text-xs text-muted-foreground">{c.image}</span>
        </div>
      ),
    },
    {
      id: 'state',
      header: 'State',
      sortValue: (c) => c.state,
      cell: (c) => <StateChip state={c.state} />,
    },
    {
      id: 'cpu',
      header: 'CPU',
      className: 'tnum',
      sortValue: (c) => c.cpuPercent,
      cell: (c) => `${c.cpuPercent}%`,
    },
    {
      id: 'ram',
      header: 'Memory',
      className: 'tnum',
      sortValue: (c) => c.ramUsedBytes,
      cell: (c) => formatBytes(c.ramUsedBytes),
    },
    {
      id: 'restarts',
      header: 'Restarts',
      className: 'tnum',
      sortValue: (c) => c.restarts,
      cell: (c) => (c.restarts > 0 ? <span className="text-attention">{c.restarts}</span> : '0'),
    },
    {
      id: 'uptime',
      header: 'Up since',
      sortValue: (c) => c.startedAt ?? '',
      cell: (c) =>
        c.startedAt ? (
          <span className="text-muted-foreground">{timeAgo(c.startedAt)}</span>
        ) : (
          <span className="text-muted-foreground">—</span>
        ),
    },
    {
      id: 'actions',
      header: '',
      className: 'w-28 text-right',
      cell: (c) => (
        <div className="flex justify-end gap-1">
          {c.state === 'exited' || c.state === 'created' ? (
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={`Start ${c.name}`}
              onClick={() => containerAction.mutate({ id: c.id, action: 'start' })}
            >
              <Play />
            </Button>
          ) : (
            <>
              <Button
                size="icon-sm"
                variant="ghost"
                aria-label={`Restart ${c.name}`}
                onClick={() => containerAction.mutate({ id: c.id, action: 'restart' })}
              >
                <RotateCw />
              </Button>
              <Button
                size="icon-sm"
                variant="ghost"
                aria-label={`Stop ${c.name}`}
                onClick={() => containerAction.mutate({ id: c.id, action: 'stop' })}
              >
                <Square />
              </Button>
            </>
          )}
        </div>
      ),
    },
  ]

  return (
    <ResourceTable
      columns={columns}
      rows={containers ?? []}
      loading={isLoading}
      emptyState={
        <EmptyState
          icon={<Container />}
          title="No containers"
          description="Running containers will appear here."
          className="border-0"
        />
      }
    />
  )
}
