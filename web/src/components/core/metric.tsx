import { cn } from '@/lib/utils'

export function Metric({
  label,
  value,
  sub,
  icon,
  className,
}: {
  label: string
  value: React.ReactNode
  sub?: React.ReactNode
  icon?: React.ReactNode
  className?: string
}) {
  return (
    <div className={cn('flex flex-col gap-1', className)}>
      <span className="flex items-center gap-1.5 text-xs font-medium tracking-wide text-muted-foreground uppercase">
        {icon}
        {label}
      </span>
      <span className="tnum text-xl font-semibold text-foreground">{value}</span>
      {sub != null && <span className="text-xs text-muted-foreground">{sub}</span>}
    </div>
  )
}
