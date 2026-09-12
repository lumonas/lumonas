import { formatBytes, formatPercent } from '@/lib/format'
import { cn } from '@/lib/utils'

export function StorageUsage({
  usedBytes,
  totalBytes,
  label,
  className,
  compact = false,
}: {
  usedBytes: number
  totalBytes: number
  label?: string
  className?: string
  compact?: boolean
}) {
  const percent = totalBytes > 0 ? (usedBytes / totalBytes) * 100 : 0
  const tone =
    percent >= 95 ? 'critical' : percent >= 85 ? 'attention' : percent >= 70 ? 'warning' : 'primary'
  return (
    <div className={cn('flex flex-col gap-1.5', className)}>
      {!compact && (
        <div className="flex items-baseline justify-between gap-2 text-sm">
          <span className="truncate text-muted-foreground">
            {label ? `${label} · ` : ''}
            <span className="tnum font-medium text-foreground">
              {formatBytes(usedBytes)}
            </span>{' '}
            of {formatBytes(totalBytes)}
          </span>
          <span className="tnum shrink-0 text-xs text-muted-foreground">
            {formatPercent(percent)}
          </span>
        </div>
      )}
      <div
        className="h-2 w-full overflow-hidden rounded-full bg-secondary"
        role="progressbar"
        aria-valuenow={Math.round(percent)}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-label={label ?? 'Storage usage'}
      >
        <div
          className={cn(
            'h-full rounded-full transition-[width] duration-500',
            tone === 'primary' && 'bg-primary',
            tone === 'warning' && 'bg-warning',
            tone === 'attention' && 'bg-attention',
            tone === 'critical' && 'bg-critical',
          )}
          style={{ width: `${Math.min(100, percent)}%` }}
        />
      </div>
    </div>
  )
}
