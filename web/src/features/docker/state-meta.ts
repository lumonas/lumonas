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
