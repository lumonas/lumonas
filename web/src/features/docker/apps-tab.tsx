import { useMemo, useState } from 'react'
import { ClipboardPaste, Search } from 'lucide-react'
import { Camera, Copy, ExternalLink, Smartphone } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { toast } from 'sonner'
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
import type { DockerStack } from '@/api/types'

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

      <ImmichPhoneSetup stack={stacks?.find((stack) => stack.catalogId === 'immich')} onInstall={() => {
        const app = (apps ?? []).find((item) => item.id === 'immich')
        if (app) setInstallApp(app)
      }} />

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

function ImmichPhoneSetup({ stack, onInstall }: { stack?: DockerStack; onInstall: () => void }) {
  const [host, setHost] = useState(() => typeof window === 'undefined' ? '' : window.location.hostname)
  const [expanded, setExpanded] = useState(false)
  const port = stack?.ports.find((entry) => entry.label?.toLowerCase().includes('web'))?.host ?? stack?.ports[0]?.host ?? 2283
  const cleanHost = host.trim().replace(/^https?:\/\//, '').replace(/\/$/, '')
  const serverURL = cleanHost ? `http://${cleanHost}:${port}` : ''
  return <div className="rounded-xl border bg-card p-4">
    <div className="flex flex-wrap items-start justify-between gap-3">
      <div className="flex items-start gap-3"><div className="flex size-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary"><Camera className="size-5" /></div><div><h3 className="text-sm font-semibold">Phone photo backup</h3><p className="mt-1 max-w-2xl text-xs text-muted-foreground">{stack ? 'Connect the Immich mobile app to your NAS and enable automatic camera uploads.' : 'Install Immich, then set up automatic camera uploads from your phone.'}</p></div></div>
      {stack ? <button type="button" className="text-xs font-medium text-primary hover:underline" onClick={() => setExpanded((value) => !value)}>{expanded ? 'Hide setup' : 'Set up phone backup'}</button> : <Button size="sm" variant="outline" onClick={onInstall}>Install Immich</Button>}
    </div>
    {stack && expanded ? <div className="mt-4 grid gap-4 border-t pt-4 md:grid-cols-[auto_1fr]">
      <div className="flex flex-col items-center gap-2"><div className="rounded bg-white p-2"><QRCodeSVG value={serverURL} size={144} /></div><a href={serverURL} target="_blank" rel="noreferrer" className="inline-flex items-center gap-1 text-xs text-primary">Open Immich <ExternalLink className="size-3" /></a></div>
      <div className="space-y-3"><div className="space-y-1.5"><Label htmlFor="immich-phone-host">NAS address on your home network</Label><div className="flex gap-2"><Input id="immich-phone-host" value={host} onChange={(event) => setHost(event.target.value)} placeholder="192.168.1.20" /><Button type="button" variant="outline" size="icon" aria-label="Copy Immich address" onClick={() => { void navigator.clipboard.writeText(serverURL).then(() => toast.success('Immich address copied')).catch(() => toast.error('Could not copy address')) }}><Copy /></Button></div><p className="text-[11px] text-muted-foreground">The address must be reachable from your phone on the same network. The QR code contains only this address.</p></div>
        <ol className="space-y-2 text-xs text-muted-foreground"><li className="flex gap-2"><span className="font-semibold text-foreground">1.</span><span>Install Immich from the <a className="text-primary underline" href="https://immich.app/docs/install/mobile-app/" target="_blank" rel="noreferrer">iOS or Android app store</a>.</span></li><li className="flex gap-2"><span className="font-semibold text-foreground">2.</span><span>Scan the QR code or enter <code className="rounded bg-muted px-1 py-0.5">{serverURL}</code>, then sign in with your Immich account.</span></li><li className="flex gap-2"><span className="font-semibold text-foreground">3.</span><span>Open Backup settings, choose Camera as the album, and enable automatic backup. Select Wi-Fi only if you want to avoid mobile data usage.</span></li></ol>
        <p className="flex items-center gap-1.5 text-[11px] text-muted-foreground"><Smartphone className="size-3.5" />Uploads are stored in the Photos share configured for Immich.</p>
      </div>
    </div> : null}
  </div>
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
