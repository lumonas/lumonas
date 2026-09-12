import { CircleAlert, CircleCheck, CircleMinus, CircleX, Lightbulb, TriangleAlert } from 'lucide-react'
import { useHealthComponents } from '@/api/queries'
import { cn } from '@/lib/utils'
import type { HealthState } from '@/api/types'

const STATUS_CONFIG: Record<HealthState, { icon: typeof CircleCheck; color: string; bg: string }> = {
  healthy: { icon: CircleCheck, color: 'text-success', bg: 'bg-success/10' },
  attention: { icon: CircleAlert, color: 'text-attention', bg: 'bg-attention/10' },
  warning: { icon: TriangleAlert, color: 'text-warning', bg: 'bg-warning/10' },
  critical: { icon: CircleX, color: 'text-critical', bg: 'bg-critical/10' },
  offline: { icon: CircleMinus, color: 'text-offline', bg: 'bg-offline/10' },
}

export function HealthExplanation() {
  const { data } = useHealthComponents()

  if (!data) return null

  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-center justify-between">
        <span className="text-sm font-medium">Health score</span>
        <span className={cn('text-2xl font-bold', data.score >= 80 ? 'text-success' : data.score >= 50 ? 'text-warning' : 'text-critical')}>
          {data.score}
        </span>
      </div>
      <div className="flex flex-col gap-2">
        {data.components.map((comp) => {
          const config = STATUS_CONFIG[comp.status]
          const Icon = config.icon
          return (
            <div key={comp.id} className={cn('rounded-lg border p-3', config.bg)}>
              <div className="flex items-start gap-2.5">
                <Icon className={cn('mt-0.5 size-4 shrink-0', config.color)} />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center justify-between gap-2">
                    <span className="text-sm font-medium">{comp.label}</span>
                    <span className={cn('text-xs font-medium capitalize', config.color)}>{comp.status}</span>
                  </div>
                  {comp.message && <p className="mt-1 text-xs text-muted-foreground">{comp.message}</p>}
                  {comp.recommended && (
                    <div className="mt-2 flex items-start gap-1.5 rounded-md bg-background/50 px-2 py-1.5">
                      <Lightbulb className="size-3.5 shrink-0 text-info" />
                      <p className="text-xs text-muted-foreground">{comp.recommended}</p>
                    </div>
                  )}
                </div>
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
