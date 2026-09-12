import { cn } from '@/lib/utils'
import type { ContainerState, DockerStack } from '@/api/types'
import { CONTAINER_STATE_META, STACK_STATE_META } from '@/features/docker/state-meta'

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
