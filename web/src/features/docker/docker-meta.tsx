import { cn } from '@/lib/utils'
import type { ContainerState, DockerStack, RiskFlag } from '@/api/types'

export const CONTAINER_STATE_META: Record<ContainerState, { label: string; dot: string }> = {
  running: { label: 'Running', dot: 'bg-success' },
  exited: { label: 'Exited', dot: 'bg-offline' },
  restarting: { label: 'Restarting', dot: 'bg-info' },
  unhealthy: { label: 'Unhealthy', dot: 'bg-critical' },
  created: { label: 'Created', dot: 'bg-muted-foreground' },
}

export const STACK_STATE_META: Record<DockerStack['state'], { label: string; dot: string }> = {
  running: { label: 'Running', dot: 'bg-success' },
  stopped: { label: 'Stopped', dot: 'bg-offline' },
  deploying: { label: 'Deploying', dot: 'bg-info' },
  unhealthy: { label: 'Unhealthy', dot: 'bg-critical' },
}

export const RISK_LABELS: Record<RiskFlag, string> = {
  privileged: 'Privileged container — has full access to the host',
  docker_socket: 'Docker socket mounted — can control the host Docker daemon',
  host_root_bind: 'Host root filesystem is mounted into the container',
  host_pid: 'Shares the host process namespace',
  host_network: 'Uses host networking — ports are exposed directly on the NAS',
  devices: 'Has direct access to host hardware devices',
}

export function StateChip({
  state,
  kind = 'container',
  className,
}: {
  state: ContainerState | DockerStack['state']
  kind?: 'container' | 'stack'
  className?: string
}) {
  const meta =
    kind === 'container'
      ? CONTAINER_STATE_META[state as ContainerState]
      : STACK_STATE_META[state as DockerStack['state']]
  return (
    <span className={cn('inline-flex items-center gap-1.5 text-xs font-medium', className)}>
      <span className={cn('size-1.5 rounded-full', meta.dot)} aria-hidden />
      {meta.label}
    </span>
  )
}
