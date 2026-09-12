import { useEffect, useMemo, useRef, useState } from 'react'
import { Pause, Play } from 'lucide-react'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { useSeedLogs } from '@/api/queries'
import { useLogsStore } from '@/stores/logs'
import { cn } from '@/lib/utils'

function formatTime(iso: string): string {
  const date = new Date(iso)
  const ms = String(date.getMilliseconds()).padStart(3, '0')
  return `${String(date.getHours()).padStart(2, '0')}:${String(date.getMinutes()).padStart(2, '0')}:${String(date.getSeconds()).padStart(2, '0')}.${ms}`
}

export function LogStream({
  containers,
  className,
}: {
  containers: string[]
  className?: string
}) {
  const lines = useLogsStore((s) => s.lines)
  const seed = useSeedLogs()
  const [selected, setSelected] = useState('')
  const [follow, setFollow] = useState(true)
  const scrollRef = useRef<HTMLDivElement>(null)
  const nearBottomRef = useRef(true)

  const container = containers.includes(selected) ? selected : (containers[0] ?? '')

  useEffect(() => {
    if (container) seed.mutate(container)
  }, [container, seed])

  const filtered = useMemo(
    () => lines.filter((line) => line.container === container),
    [lines, container],
  )

  useEffect(() => {
    if (follow && nearBottomRef.current && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }, [filtered, follow])

  function handleScroll() {
    const el = scrollRef.current
    if (!el) return
    nearBottomRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 48
  }

  return (
    <div className={cn('flex flex-col gap-3', className)}>
      <div className="flex flex-wrap items-center gap-4">
        {containers.length > 1 && (
          <div className="grid gap-1">
            <Label className="text-xs text-muted-foreground">Container</Label>
            <Select value={container} onValueChange={setSelected}>
              <SelectTrigger className="h-8 w-56 text-xs">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {containers.map((name) => (
                  <SelectItem key={name} value={name}>
                    {name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
        )}
        <div className="flex items-center gap-2">
          <Switch
            id="log-follow"
            checked={follow}
            onCheckedChange={setFollow}
            aria-label="Follow log output"
          />
          <Label htmlFor="log-follow" className="flex items-center gap-1.5 text-xs text-muted-foreground">
            {follow ? <Pause className="size-3" /> : <Play className="size-3" />}
            {follow ? 'Following' : 'Paused'}
          </Label>
        </div>
        <span className="tnum ml-auto text-xs text-muted-foreground">
          {filtered.length} lines
        </span>
      </div>
      <div
        ref={scrollRef}
        onScroll={handleScroll}
        className="h-80 overflow-auto rounded-lg border bg-muted/20 p-3 font-mono text-xs leading-relaxed"
        aria-live="polite"
      >
        {filtered.length === 0 ? (
          <p className="text-muted-foreground">
            Waiting for log output from {container || 'container'}…
          </p>
        ) : (
          filtered.map((line, index) => (
            <div key={index} className="flex gap-3 whitespace-pre-wrap break-all">
              <span className="shrink-0 text-muted-foreground">{formatTime(line.ts)}</span>
              <span
                className={cn(
                  'w-11 shrink-0 uppercase',
                  line.level === 'error' && 'text-critical',
                  line.level === 'warn' && 'text-attention',
                  line.level === 'info' && 'text-muted-foreground',
                )}
              >
                {line.level}
              </span>
              <span className={cn(line.level === 'error' && 'text-critical')}>{line.message}</span>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
