import { ExternalLink, Star } from 'lucide-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { CatalogApp } from '@/api/types'

const ACCENT_CLASSES: Record<CatalogApp['accent'], string> = {
  primary: 'bg-primary/12 text-primary border-primary/25',
  info: 'bg-info/12 text-info border-info/25',
  success: 'bg-success/12 text-success border-success/25',
  attention: 'bg-attention/12 text-attention border-attention/25',
  warning: 'bg-warning/12 text-warning border-warning/25',
  critical: 'bg-critical/12 text-critical border-critical/25',
}

export function CatalogCard({
  app,
  installed,
  onInstall,
}: {
  app: CatalogApp
  installed: boolean
  onInstall: (app: CatalogApp) => void
}) {
  return (
    <div className="group flex flex-col gap-3 rounded-xl border bg-card p-4 shadow-xs transition-colors hover:border-primary/40">
      <div className="flex items-start justify-between gap-2">
        <div
          className={cn(
            'flex size-11 shrink-0 items-center justify-center rounded-lg border text-lg font-semibold',
            ACCENT_CLASSES[app.accent],
          )}
          aria-hidden
        >
          {app.name.slice(0, 1)}
        </div>
        <div className="flex items-center gap-1.5">
          {app.popular && (
            <Badge variant="secondary" className="gap-1">
              <Star className="size-3 text-attention" />
              Popular
            </Badge>
          )}
          {installed && <Badge variant="success">Installed</Badge>}
        </div>
      </div>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2">
          <h3 className="truncate text-sm font-semibold">{app.name}</h3>
          <span className="shrink-0 text-xs text-muted-foreground">{app.category}</span>
        </div>
        <p className="mt-1 line-clamp-2 text-xs leading-relaxed text-muted-foreground">
          {app.tagline}
        </p>
      </div>
      <div className="flex items-center justify-between gap-2">
        <Button
          size="sm"
          variant={installed ? 'outline' : 'default'}
          className="h-8 text-xs"
          onClick={() => onInstall(app)}
        >
          {installed ? 'Install again' : 'Install'}
        </Button>
        <a
          href={app.upstream}
          target="_blank"
          rel="noreferrer"
          className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
        >
          Upstream
          <ExternalLink className="size-3" />
        </a>
      </div>
    </div>
  )
}
