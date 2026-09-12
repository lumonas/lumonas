import { HEALTH, HEALTH_FALLBACK } from '@/components/core/health'
import { cn } from '@/lib/utils'
import type { HealthState } from '@/api/types'

export function HealthDot({ state, className }: { state: HealthState; className?: string }) {
  const { color } = HEALTH[state] ?? HEALTH_FALLBACK
  return (
    <span
      aria-hidden
      className={cn(
        'inline-block size-1.5 shrink-0 rounded-full',
        color === 'success' && 'bg-success',
        color === 'attention' && 'bg-attention',
        color === 'warning' && 'bg-warning',
        color === 'critical' && 'bg-critical',
        color === 'offline' && 'bg-offline',
        className,
      )}
    />
  )
}

export function HealthBadge({
  state,
  variant = 'badge',
  withIcon = false,
  className,
}: {
  state: HealthState
  variant?: 'badge' | 'text'
  withIcon?: boolean
  className?: string
}) {
  const { label, color, Icon } = HEALTH[state] ?? HEALTH_FALLBACK
  if (variant === 'text') {
    return (
      <span className={cn('inline-flex items-center gap-2 text-sm', className)}>
        <HealthDot state={state} />
        <span
          className={cn(
            'font-medium',
            color === 'success' && 'text-success',
            color === 'attention' && 'text-attention',
            color === 'warning' && 'text-warning',
            color === 'critical' && 'text-critical',
            color === 'offline' && 'text-offline',
          )}
        >
          {label}
        </span>
        {withIcon ? <Icon className="size-3.5 opacity-70" /> : null}
      </span>
    )
  }
  return (
    <span
      className={cn(
        'inline-flex w-fit items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium',
        color === 'success' && 'border-success/25 bg-success/10 text-success',
        color === 'attention' && 'border-attention/25 bg-attention/10 text-attention',
        color === 'warning' && 'border-warning/25 bg-warning/10 text-warning',
        color === 'critical' && 'border-critical/25 bg-critical/10 text-critical',
        color === 'offline' && 'border-offline/25 bg-offline/10 text-offline',
        className,
      )}
    >
      {withIcon ? <Icon className="size-3" /> : <HealthDot state={state} />}
      {label}
    </span>
  )
}
