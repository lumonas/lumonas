import { Link2 } from 'lucide-react'
import { cn } from '@/lib/utils'

export interface DependencyItem {
  id: string
  label: string
  sublabel?: string
}

export function DependencyList({
  items,
  emptyLabel = 'No dependencies',
  className,
}: {
  items: DependencyItem[]
  emptyLabel?: string
  className?: string
}) {
  if (items.length === 0) {
    return <p className={cn('text-sm text-muted-foreground', className)}>{emptyLabel}</p>
  }
  return (
    <ul className={cn('flex flex-col divide-y rounded-lg border', className)}>
      {items.map((item) => (
        <li key={item.id} className="flex items-center gap-2.5 px-3 py-2">
          <Link2 className="size-3.5 shrink-0 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{item.label}</p>
            {item.sublabel ? (
              <p className="truncate text-xs text-muted-foreground">{item.sublabel}</p>
            ) : null}
          </div>
        </li>
      ))}
    </ul>
  )
}
