import { useMemo, useState } from 'react'
import { ClipboardPaste, Search } from 'lucide-react'
import { useCreateStack, useDockerApps, useDockerStacks } from '@/api/queries'
import { CatalogCard } from '@/components/core/catalog-card'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { InstallWizard } from '@/features/docker/install-wizard'
import { cn } from '@/lib/utils'
import type { CatalogApp } from '@/api/types'

export function AppsTab({ initialInstallId }: { initialInstallId?: string | null }) {
  const { data: apps } = useDockerApps()
  const { data: stacks } = useDockerStacks()
  const [query, setQuery] = useState('')
  const [category, setCategory] = useState('All')
  const [installApp, setInstallApp] = useState<CatalogApp | null>(null)
  const [importOpen, setImportOpen] = useState(false)

  const installedIds = useMemo(() => new Set(stacks?.map((s) => s.catalogId) ?? []), [stacks])

  const categories = useMemo(
    () => ['All', ...Array.from(new Set((apps ?? []).map((a) => a.category))).sort()],
    [apps],
  )

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return (apps ?? []).filter((app) => {
      if (category !== 'All' && app.category !== category) return false
      if (!q) return true
      return (
        app.name.toLowerCase().includes(q) ||
        app.tagline.toLowerCase().includes(q) ||
        app.category.toLowerCase().includes(q)
      )
    })
  }, [apps, query, category])

  const initialApp =
    initialInstallId != null ? (apps ?? []).find((a) => a.id === initialInstallId) : undefined

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative min-w-56 flex-1 sm:max-w-xs">
          <Search className="absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search apps…"
            className="pl-8"
            aria-label="Search apps"
          />
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          {categories.map((c) => (
            <button
              key={c}
              type="button"
              onClick={() => setCategory(c)}
              className={cn(
                'rounded-full border px-3 py-1 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
                category === c
                  ? 'border-primary/40 bg-primary/10 text-primary'
                  : 'border-border text-muted-foreground hover:text-foreground',
              )}
            >
              {c}
            </button>
          ))}
        </div>
        <Button variant="outline" size="sm" className="ml-auto" onClick={() => setImportOpen(true)}>
          <ClipboardPaste />
          Import Compose…
        </Button>
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-4">
        {filtered.map((app) => (
          <CatalogCard
            key={app.id}
            app={app}
            installed={installedIds.has(app.id)}
            onInstall={setInstallApp}
          />
        ))}
      </div>

      {filtered.length === 0 && (
        <p className="py-10 text-center text-sm text-muted-foreground">No apps match “{query}”.</p>
      )}

      <InstallWizard
        app={installApp ?? initialApp ?? null}
        onOpenChange={(open) => {
          if (!open) setInstallApp(null)
        }}
      />
      <ImportComposeDialog open={importOpen} onOpenChange={setImportOpen} />
    </div>
  )
}

function ImportComposeDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const createStack = useCreateStack()
  const [name, setName] = useState('')
  const [yaml, setYaml] = useState('')
  const valid = name.trim().length > 0 && yaml.includes('services:')

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-xl">
        <DialogHeader>
          <DialogTitle>Import Compose</DialogTitle>
          <DialogDescription>
            Paste any standard Compose file. It stays the source of truth — unknown options are
            preserved.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-2">
          <Label htmlFor="import-name">Stack name</Label>
          <Input
            id="import-name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="my-app"
            className="font-mono"
          />
        </div>
        <div className="grid gap-2">
          <Label htmlFor="import-yaml">compose.yaml</Label>
          <textarea
            id="import-yaml"
            value={yaml}
            onChange={(e) => setYaml(e.target.value)}
            spellCheck={false}
            placeholder={'services:\n  app:\n    image: …'}
            className="h-56 w-full resize-none rounded-md border border-input bg-background px-3 py-2 font-mono text-xs leading-relaxed shadow-xs outline-none placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
          />
        </div>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            disabled={!valid || createStack.isPending}
            onClick={() =>
              createStack.mutate(
                { name: name.trim(), composeYaml: yaml },
                { onSuccess: () => onOpenChange(false) },
              )
            }
          >
            Import and deploy
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
