import { useState } from 'react'
import { NavLink, useLocation } from 'react-router-dom'
import { Ellipsis } from 'lucide-react'
import { MOBILE_MORE, MOBILE_PRIMARY, type NavItem } from '@/components/layout/nav'
import { Sheet, SheetContent, SheetHeader, SheetTitle } from '@/components/ui/sheet'
import { cn } from '@/lib/utils'

export function MobileNav({ className }: { className?: string }) {
  const [moreOpen, setMoreOpen] = useState(false)
  const location = useLocation()
  const moreActive = MOBILE_MORE.some(({ to }) => location.pathname === to || location.pathname.startsWith(`${to}/`))

  const linkClass = ({ isActive }: { isActive: boolean }) =>
    cn(
      'flex flex-1 flex-col items-center gap-1 rounded-md py-2 text-[11px] font-medium outline-none focus-visible:ring-2 focus-visible:ring-ring',
      isActive ? 'text-primary' : 'text-muted-foreground hover:text-foreground',
    )

  return (
    <>
      <nav
        aria-label="Mobile"
        className={cn(
          'fixed inset-x-0 bottom-0 z-40 flex items-stretch border-t bg-background/95 px-2 pb-[env(safe-area-inset-bottom)] backdrop-blur md:hidden',
          className,
        )}
      >
        {MOBILE_PRIMARY.map(({ to, label, Icon }) => (
          <NavLink key={to} to={to} end={to === '/'} className={linkClass}>
            <Icon className="size-5" />
            {label}
          </NavLink>
        ))}
        <button
          type="button"
          onClick={() => setMoreOpen(true)}
          aria-expanded={moreOpen}
          aria-current={moreActive ? 'page' : undefined}
          className={cn(
            'flex flex-1 flex-col items-center gap-1 rounded-md py-2 text-[11px] font-medium outline-none focus-visible:ring-2 focus-visible:ring-ring',
            moreActive || moreOpen ? 'text-primary' : 'text-muted-foreground hover:text-foreground',
          )}
        >
          <Ellipsis className="size-5" />
          More
        </button>
      </nav>

      <Sheet open={moreOpen} onOpenChange={setMoreOpen}>
        <SheetContent side="bottom" className="rounded-t-2xl">
          <SheetHeader className="pb-0">
            <SheetTitle className="text-sm text-muted-foreground">All sections</SheetTitle>
          </SheetHeader>
          <div className="grid grid-cols-2 gap-1 p-4 pb-6">
            {MOBILE_MORE.map(({ to, label, Icon }: NavItem) => (
              <NavLink
                key={to}
                to={to}
                onClick={() => setMoreOpen(false)}
                className={({ isActive }) => cn('flex items-center gap-3 rounded-lg border bg-background px-3 py-3 text-sm font-medium text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring', isActive && 'border-primary/50 bg-primary/5 text-primary')}
              >
                <Icon className="size-4 text-muted-foreground" />
                {label}
              </NavLink>
            ))}
          </div>
        </SheetContent>
      </Sheet>
    </>
  )
}
