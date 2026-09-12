import { NavLink } from 'react-router-dom'
import { HealthDot } from '@/components/core/health-badge'
import { Logo } from '@/components/layout/Logo'
import { NAV_ITEMS } from '@/components/layout/nav'
import { useServer } from '@/api/queries'
import { cn } from '@/lib/utils'

export function Sidebar({ className }: { className?: string }) {
  const { data: server } = useServer()
  return (
    <aside
      className={cn(
        'fixed inset-y-0 left-0 z-30 hidden w-60 flex-col border-r bg-background md:flex',
        className,
      )}
    >
      <div className="flex h-14 items-center gap-2.5 border-b px-4">
        <Logo />
        <div className="flex min-w-0 flex-col">
          <span className="text-sm font-semibold tracking-tight">LumoNAS</span>
          <span className="tnum text-[10px] text-muted-foreground">{server?.version}</span>
        </div>
      </div>
      <nav aria-label="Main" className="flex-1 space-y-0.5 overflow-y-auto p-3">
        {NAV_ITEMS.map(({ to, label, Icon }) => (
          <NavLink
            key={to}
            to={to}
            end={to === '/'}
            className={({ isActive }) =>
              cn(
                'flex items-center gap-3 rounded-md px-2.5 py-2 text-sm font-medium outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
                isActive
                  ? 'bg-accent text-accent-foreground'
                  : 'text-muted-foreground hover:bg-secondary hover:text-foreground',
              )
            }
          >
            <Icon className="size-4 shrink-0" />
            {label}
          </NavLink>
        ))}
      </nav>
      <div className="border-t p-3">
        <div className="flex items-center gap-2.5 rounded-md px-2.5 py-2">
          {server && <HealthDot state={server.health} />}
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium">{server?.name ?? '—'}</p>
            <p className="tnum truncate text-xs text-muted-foreground">{server?.ip}</p>
          </div>
        </div>
      </div>
    </aside>
  )
}
