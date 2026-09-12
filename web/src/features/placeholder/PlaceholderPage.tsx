import { Link, useLocation } from 'react-router-dom'
import { Compass } from 'lucide-react'
import { EmptyState } from '@/components/core/empty-state'
import { PageHeader } from '@/components/core/page-header'
import { Button } from '@/components/ui/button'
import { NAV_ITEMS } from '@/components/layout/nav'

const DESCRIPTIONS: Record<string, string> = {}

export function PlaceholderPage() {
  const { pathname } = useLocation()
  const item = NAV_ITEMS.find((n) => n.to === pathname)
  const label = item?.label ?? 'Coming soon'

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={label} description={DESCRIPTIONS[pathname]} />
      <EmptyState
        icon={item ? <item.Icon /> : <Compass />}
        title={`${label} is planned`}
        description="This screen is specified in the design docs and scheduled for a later milestone. The shell, design system and live mock backend are ready for it."
        action={
          <Button variant="outline" asChild>
            <Link to="/">Back to overview</Link>
          </Button>
        }
        className="min-h-72"
      />
    </div>
  )
}
