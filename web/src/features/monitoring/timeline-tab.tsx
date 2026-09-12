import { useMemo, useState } from 'react'
import { ScrollText } from 'lucide-react'
import { useActivity } from '@/api/queries'
import { EmptyState } from '@/components/core/empty-state'
import { TimelineEvent } from '@/components/core/timeline-event'
import { Card, CardContent } from '@/components/ui/card'
import { cn } from '@/lib/utils'
import type { ActivityCategory } from '@/api/types'

const CATEGORIES: { value: ActivityCategory | 'all'; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'storage', label: 'Storage' },
  { value: 'docker', label: 'Docker' },
  { value: 'backup', label: 'Backups' },
  { value: 'config', label: 'Config' },
  { value: 'security', label: 'Security' },
  { value: 'update', label: 'Updates' },
]

export function TimelineTab() {
  const { data: activity, isLoading } = useActivity()
  const [category, setCategory] = useState<ActivityCategory | 'all'>('all')

  const filtered = useMemo(() => {
    const events = activity ?? []
    if (category === 'all') return events
    return events.filter((event) => event.category === category)
  }, [activity, category])

  return (
    <Card>
      <CardContent className="flex flex-col gap-4 p-5">
        <div className="flex flex-wrap items-center gap-1.5">
          {CATEGORIES.map((c) => (
            <button
              key={c.value}
              type="button"
              onClick={() => setCategory(c.value)}
              className={cn(
                'rounded-full border px-3 py-1 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                category === c.value
                  ? 'border-primary/40 bg-primary/10 text-primary'
                  : 'border-border text-muted-foreground hover:text-foreground',
              )}
            >
              {c.label}
            </button>
          ))}
          <span className="tnum ml-auto text-xs text-muted-foreground">
            {filtered.length} events
          </span>
        </div>

        {isLoading ? (
          <p className="py-6 text-center text-sm text-muted-foreground">Loading…</p>
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={<ScrollText />}
            title="No events"
            description="System events — config changes, deployments, backups, sign-ins — appear here in order."
            className="border-0"
          />
        ) : (
          <div className="flex flex-col gap-3">
            {filtered.map((event) => (
              <TimelineEvent key={event.id} event={event} />
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}
