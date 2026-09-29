import { useState, type FormEvent } from 'react'
import { Copy, Link2, Plus, Share2, Trash2 } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { toast } from 'sonner'
import { useCreateFileRequestLink, useFileRequestLinks, useRevokeFileRequestLink } from '@/api/queries'
import { Button } from '@/components/ui/button'
import { Card } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { formatBytes } from '@/lib/format'
import { useCurrentTime } from '@/hooks/useCurrentTime'
import type { FileRequestLink } from '@/api/types'
import { useCreateFileShareLink, useFileShareLinks, useRevokeFileShareLink } from '@/api/queries'

export function FileRequestManager({ shareId, path }: { shareId: string | null; path: string }) {
  const [open, setOpen] = useState(false)
  const [hours, setHours] = useState('72')
  const [maxFiles, setMaxFiles] = useState('10')
  const [maxGiB, setMaxGiB] = useState('5')
  const [createdURL, setCreatedURL] = useState('')
  const links = useFileRequestLinks(shareId)
  const create = useCreateFileRequestLink()
  const revoke = useRevokeFileRequestLink()

  function submit(event: FormEvent) {
    event.preventDefault()
    if (!shareId) return
    create.mutate({ shareId, path, expiresInHours: Number(hours), maxFiles: Number(maxFiles), maxBytes: Math.round(Number(maxGiB) * 1024 ** 3) }, {
      onSuccess: async (result) => {
        const absolute = new URL(result.url, window.location.origin).toString()
        setCreatedURL(absolute)
        try { await navigator.clipboard.writeText(absolute); toast.success('Upload link created and copied') }
        catch { toast.success('Upload link created') }
      },
      onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not create upload link'),
    })
  }

  async function copyLink() {
    try { await navigator.clipboard.writeText(createdURL); toast.success('Link copied') }
    catch { toast.error('Could not copy link') }
  }

  return <>
    <Button size="sm" variant="outline" disabled={!shareId} onClick={() => { setCreatedURL(''); setOpen(true) }}><Link2 />Request files</Button>
    <ReadOnlyShareManager shareId={shareId} path={path} />
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="max-w-xl">
        <DialogHeader><DialogTitle>Request files</DialogTitle><DialogDescription>Create a private upload link for this folder. Anyone with the link can upload until it expires or reaches its limits; the folder contents stay private.</DialogDescription></DialogHeader>
        <form className="space-y-4" onSubmit={submit}>
          <div className="rounded-md border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">Destination: <span className="font-medium text-foreground">{path === '/' ? 'Share root' : path}</span></div>
          <div className="grid gap-3 sm:grid-cols-3">
            <label className="space-y-1 text-xs text-muted-foreground"><span>Expires in hours</span><Input aria-label="Link expires in hours" type="number" min="1" max="720" required value={hours} onChange={(event) => setHours(event.target.value)} /></label>
            <label className="space-y-1 text-xs text-muted-foreground"><span>Maximum files</span><Input aria-label="Maximum uploaded files" type="number" min="1" max="1000" required value={maxFiles} onChange={(event) => setMaxFiles(event.target.value)} /></label>
            <label className="space-y-1 text-xs text-muted-foreground"><span>Total size (GiB)</span><Input aria-label="Maximum total upload GiB" type="number" min="0.001" max="1024" step="0.001" required value={maxGiB} onChange={(event) => setMaxGiB(event.target.value)} /></label>
          </div>
          {createdURL ? <div className="flex flex-col gap-3 rounded-md border border-primary/30 bg-primary/5 p-3 sm:flex-row sm:items-center"><div className="rounded bg-white p-2"><QRCodeSVG value={createdURL} size={112} /></div><div className="min-w-0 flex-1 space-y-2"><Label htmlFor="new-file-request-link">New upload link</Label><div className="flex gap-2"><Input id="new-file-request-link" readOnly value={createdURL} className="font-mono text-xs" /><Button type="button" variant="outline" size="icon" aria-label="Copy upload link" onClick={() => void copyLink()}><Copy /></Button></div><p className="text-xs text-muted-foreground">This secret link is only shown now. Copy it or scan the QR code before closing.</p></div></div> : null}
          <DialogFooter><Button type="button" variant="ghost" onClick={() => setOpen(false)}>Close</Button><Button type="submit" disabled={create.isPending || !shareId}><Plus />{create.isPending ? 'Creating…' : 'Create link'}</Button></DialogFooter>
        </form>
        <div className="space-y-2 border-t pt-3"><p className="text-sm font-medium">Active and expired links</p>
          {links.isLoading ? <p className="text-xs text-muted-foreground">Loading upload links…</p> : null}
          {links.isError ? <p role="alert" className="text-xs text-destructive">Could not load upload links.</p> : null}
          {(links.data ?? []).length === 0 && !links.isLoading ? <p className="text-xs text-muted-foreground">No upload links have been created for this share.</p> : null}
          {(links.data ?? []).map((item) => <FileRequestRow key={item.id} item={item} onRevoke={() => revoke.mutate({ id: item.id, shareId: item.shareId })} disabled={revoke.isPending} />)}
        </div>
      </DialogContent>
    </Dialog>
  </>
}

function ReadOnlyShareManager({ shareId, path }: { shareId: string | null; path: string }) {
  const [open, setOpen] = useState(false)
  const [hours, setHours] = useState('72')
  const [password, setPassword] = useState('')
  const [createdURL, setCreatedURL] = useState('')
  const links = useFileShareLinks(shareId)
  const create = useCreateFileShareLink()
  const revoke = useRevokeFileShareLink()
  const now = useCurrentTime()

  function submit(event: FormEvent) {
    event.preventDefault()
    if (!shareId) return
    create.mutate({ shareId, path, expiresInHours: Number(hours), ...(password ? { password } : {}) }, {
      onSuccess: async (result) => {
        const absolute = new URL(result.url, window.location.origin).toString()
        setCreatedURL(absolute)
        setPassword('')
        try { await navigator.clipboard.writeText(absolute); toast.success('Read-only link created and copied') }
        catch { toast.success('Read-only link created') }
      },
      onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not create share link'),
    })
  }

  return <>
    <Button size="sm" variant="outline" disabled={!shareId} onClick={() => { setCreatedURL(''); setOpen(true) }}><Share2 />Share folder</Button>
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="max-w-xl">
        <DialogHeader><DialogTitle>Share folder read-only</DialogTitle><DialogDescription>Anyone with the link can browse and download files in this folder until it expires or you revoke it. Uploading and changing files are never allowed.</DialogDescription></DialogHeader>
        <form className="space-y-3" onSubmit={submit}>
          <div className="rounded-md border bg-muted/30 px-3 py-2 text-xs text-muted-foreground">Shared folder: <span className="font-medium text-foreground">{path === '/' ? 'Share root' : path}</span></div>
          <label className="block space-y-1 text-xs text-muted-foreground">Expires in hours<Input aria-label="Read-only link expires in hours" type="number" min="1" max="720" required value={hours} onChange={(event) => setHours(event.target.value)} /></label>
          <label className="block space-y-1 text-xs text-muted-foreground">Password (optional, at least 12 characters)<Input aria-label="Read-only link password" type="password" autoComplete="new-password" minLength={12} maxLength={128} value={password} onChange={(event) => setPassword(event.target.value)} /></label>
          {createdURL ? <div className="space-y-2 rounded-md border border-primary/30 bg-primary/5 p-3"><Label htmlFor="new-readonly-share-link">New read-only link</Label><div className="flex gap-2"><Input id="new-readonly-share-link" readOnly value={createdURL} className="font-mono text-xs" /><Button type="button" variant="outline" size="icon" aria-label="Copy read-only share link" onClick={() => void navigator.clipboard.writeText(createdURL).then(() => toast.success('Link copied')).catch(() => toast.error('Could not copy link'))}><Copy /></Button></div><p className="text-xs text-muted-foreground">The secret link is shown only now. Save it before closing.</p></div> : null}
          <DialogFooter><Button type="button" variant="ghost" onClick={() => setOpen(false)}>Close</Button><Button type="submit" disabled={create.isPending || !shareId}><Plus />{create.isPending ? 'Creating…' : 'Create read-only link'}</Button></DialogFooter>
        </form>
        <div className="space-y-2 border-t pt-3"><p className="text-sm font-medium">Read-only links</p>
          {links.isLoading ? <p className="text-xs text-muted-foreground">Loading links…</p> : null}
          {links.isError ? <p role="alert" className="text-xs text-destructive">Could not load read-only links.</p> : null}
          {(links.data ?? []).map((item) => {
            const expired = Boolean(item.revokedAt) || Date.parse(item.expiresAt) <= now
            return <Card key={item.id} className="flex items-center justify-between gap-3 p-3"><div className="min-w-0"><p className="truncate text-xs font-medium">{item.path || 'Share root'}</p><p className="text-xs text-muted-foreground">{item.downloads} downloads · expires {new Date(item.expiresAt).toLocaleString()}</p><p className="text-[11px] text-muted-foreground">{item.revokedAt ? 'Revoked' : expired ? 'Expired' : 'Active · secret hidden after creation'}</p></div>{!expired ? <Button size="icon" variant="ghost" aria-label="Revoke read-only link" disabled={revoke.isPending} onClick={() => revoke.mutate({ id: item.id, shareId: item.shareId })}><Trash2 className="text-destructive" /></Button> : null}</Card>
          })}
          {(links.data ?? []).length === 0 && !links.isLoading ? <p className="text-xs text-muted-foreground">No read-only links for this share.</p> : null}
        </div>
      </DialogContent>
    </Dialog>
  </>
}

function FileRequestRow({ item, onRevoke, disabled }: { item: FileRequestLink; onRevoke: () => void; disabled: boolean }) {
  const now = useCurrentTime()
  const expired = Boolean(item.revokedAt) || Date.parse(item.expiresAt) <= now || item.receivedFiles >= item.maxFiles || item.receivedBytes >= item.maxBytes
  return <Card className="flex items-center justify-between gap-3 p-3">
    <div className="min-w-0"><p className="truncate text-xs font-medium">{item.path || 'Share root'}</p><p className="text-xs text-muted-foreground">{item.receivedFiles}/{item.maxFiles} files · {formatBytes(item.receivedBytes)} of {formatBytes(item.maxBytes)} · expires {new Date(item.expiresAt).toLocaleString()}</p><p className="text-[11px] text-muted-foreground">{expired ? item.revokedAt ? 'Revoked' : 'Expired or limit reached' : 'Active · link is hidden after creation'}</p></div>
    {!expired ? <Button size="icon" variant="ghost" aria-label="Revoke upload link" disabled={disabled} onClick={onRevoke}><Trash2 className="text-destructive" /></Button> : null}
  </Card>
}
