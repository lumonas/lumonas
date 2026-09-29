import { AlertTriangle, CircleAlert, Info } from 'lucide-react'
import { cn } from '@/lib/utils'

const TONES = {
  info: { border: 'border-info/40', bg: 'bg-info/5', text: 'text-info', Icon: Info },
  attention: {
    border: 'border-attention/40',
    bg: 'bg-attention/5',
    text: 'text-attention',
    Icon: CircleAlert,
  },
  warning: {
    border: 'border-warning/40',
    bg: 'bg-warning/5',
    text: 'text-warning',
    Icon: AlertTriangle,
  },
  critical: {
    border: 'border-critical/40',
    bg: 'bg-critical/5',
    text: 'text-critical',
    Icon: AlertTriangle,
  },
} as const

export function AlertBanner({
  tone = 'info',
  title,
  children,
  action,
  className,
}: {
  tone?: keyof typeof TONES
  title: string
  children?: React.ReactNode
  action?: React.ReactNode
  className?: string
}) {
  const { border, bg, text, Icon } = TONES[tone]
  return (
    <div
      role={tone === 'critical' || tone === 'warning' ? 'alert' : 'status'}
      className={cn('flex items-start gap-3 rounded-lg border p-3.5', border, bg, className)}
    >
      <Icon className={cn('mt-0.5 size-4 shrink-0', text)} />
      <div className="min-w-0 flex-1">
        <p className={cn('text-sm font-medium', text)}>{title}</p>
        {children ? (
          <div className="mt-0.5 text-sm text-muted-foreground">{children}</div>
        ) : null}
      </div>
      {action ? <div className="shrink-0 self-center">{action}</div> : null}
    </div>
  )
}
