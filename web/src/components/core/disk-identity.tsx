import { HardDrive } from 'lucide-react'
import { HealthBadge } from '@/components/core/health-badge'
import { formatBytes } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { Disk } from '@/api/types'

export function DiskIdentity({
  disk,
  showHealth = true,
  className,
}: {
  disk: Disk
  showHealth?: boolean
  className?: string
}) {
  return (
    <div className={cn('flex items-start gap-3', className)}>
      <div className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-lg border bg-secondary/60">
        <HardDrive className="size-4 text-muted-foreground" />
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
          <span className="font-medium text-foreground">{disk.model}</span>
          {showHealth && <HealthBadge state={disk.health} />}
        </div>
        <div className="tnum mt-0.5 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
          <span className="font-mono">serial {disk.serial}</span>
          <span aria-hidden>·</span>
          <span>{formatBytes(disk.sizeBytes)}</span>
          <span aria-hidden>·</span>
          <span className="uppercase">{disk.interface}</span>
        </div>
      </div>
    </div>
  )
}
