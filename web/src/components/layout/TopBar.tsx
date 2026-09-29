import { Circle, Search } from 'lucide-react'
import { useServer } from '@/api/queries'
import { HealthDot } from '@/components/core/health-badge'
import { JobsPopover } from '@/components/layout/JobsPopover'
import { Logo } from '@/components/layout/Logo'
import { NotificationsPopover } from '@/components/layout/NotificationsPopover'
import { UserMenu } from '@/components/layout/UserMenu'
import { Button } from '@/components/ui/button'
import { Kbd } from '@/components/ui/kbd'
import { useUiStore } from '@/stores/ui'
import { useLiveConnection } from '@/stores/live-connection'

export function TopBar() {
  const { data: server } = useServer()
  const setPaletteOpen = useUiStore((s) => s.setPaletteOpen)
  const liveState = useLiveConnection((s) => s.state)
  const liveLabel = {
    connecting: 'Connecting to live updates',
    live: 'Live updates connected',
    reconnecting: 'Live updates reconnecting',
    offline: 'Live updates offline',
  }[liveState]

  return (
    <header className="sticky top-0 z-20 flex h-14 shrink-0 items-center gap-3 border-b bg-background/80 px-4 backdrop-blur md:px-6">
      <div className="flex items-center gap-2 md:hidden">
        <Logo className="size-6" />
      </div>
      <div className="hidden min-w-0 items-center gap-2.5 md:flex">
        {server && <HealthDot state={server.health} />}
        <div className="min-w-0 leading-tight">
          <p className="truncate text-sm font-medium">{server?.name ?? '—'}</p>
        </div>
        <span className="text-xs text-muted-foreground">·</span>
        <span className="tnum truncate text-xs text-muted-foreground">{server?.ip}</span>
      </div>

      <div className="mx-auto w-full max-w-md flex-1 px-2 md:px-0">
        <Button
          variant="outline"
          onClick={() => setPaletteOpen(true)}
          className="h-9 w-full justify-start gap-2 text-muted-foreground"
          aria-label="Open command palette"
        >
          <Search className="size-4" />
          <span className="hidden sm:inline">Search LumoNAS…</span>
          <span className="sm:hidden">Search…</span>
          <span className="ml-auto flex items-center gap-1">
            <Kbd>⌘</Kbd>
            <Kbd>K</Kbd>
          </span>
        </Button>
      </div>

      <div className="flex shrink-0 items-center gap-1">
        <span className="mr-1 inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs text-muted-foreground" role="status" aria-label={liveLabel} title={liveLabel}>
          <Circle className={`size-2 fill-current ${liveState === 'live' ? 'text-success' : liveState === 'offline' ? 'text-critical' : 'text-attention'}`} aria-hidden="true" />
          <span className="hidden lg:inline">{liveState === 'live' ? 'Live' : liveState === 'reconnecting' ? 'Reconnecting' : liveState === 'offline' ? 'Offline' : 'Connecting'}</span>
        </span>
        <JobsPopover />
        <NotificationsPopover />
        <UserMenu />
      </div>
    </header>
  )
}
