import { useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { ArrowLeft, Download, Folder, LockKeyhole, Share2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatBytes } from '@/lib/format'
import type { FileEntry } from '@/api/types'

type ShareInfo = { shareName: string; path: string; expiresAt: string; passwordProtected: boolean }
type Entries = { shareName: string; path: string; entries: FileEntry[] }

export function PublicFileSharePage() {
  const { token = '' } = useParams()
  const [password, setPassword] = useState('')
  const [passwordInput, setPasswordInput] = useState('')
  const [info, setInfo] = useState<ShareInfo | null>(null)
  const [entries, setEntries] = useState<Entries | null>(null)
  const [path, setPath] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  async function request<T>(url: string, secret = password): Promise<T> {
    const response = await fetch(`/api/v1${url}`, { headers: secret ? { 'X-Share-Password': secret } : {}, cache: 'no-store' })
    const body = await response.json() as T & { error?: string }
    if (!response.ok) throw new Error(body.error || 'This share link is unavailable.')
    return body
  }

  async function openShare(secret = password) {
    setLoading(true); setError('')
    try {
      const current = await request<ShareInfo>(`/public/file-share-links/${encodeURIComponent(token)}`, secret)
      setInfo(current); setPassword(secret)
      const listing = await request<Entries>(`/public/file-share-links/${encodeURIComponent(token)}/files`, secret)
      setEntries(listing); setPath('')
    } catch (reason) { setError(reason instanceof Error ? reason.message : 'Could not open this share.') }
    finally { setLoading(false) }
  }

  async function browse(next: string) {
    setLoading(true); setError('')
    try { const listing = await request<Entries>(`/public/file-share-links/${encodeURIComponent(token)}/files?path=${encodeURIComponent(next)}`); setEntries(listing); setPath(next) }
    catch (reason) { setError(reason instanceof Error ? reason.message : 'Could not browse this folder.') }
    finally { setLoading(false) }
  }

  async function download(entry: FileEntry) {
    setError('')
    try {
      const response = await fetch(`/api/v1/public/file-share-links/${encodeURIComponent(token)}/download?path=${encodeURIComponent(path)}&name=${encodeURIComponent(entry.name)}`, { headers: password ? { 'X-Share-Password': password } : {}, cache: 'no-store' })
      if (!response.ok) { const body = await response.json() as { error?: string }; throw new Error(body.error || 'Download failed.') }
      const blob = await response.blob(); const url = URL.createObjectURL(blob)
      const anchor = document.createElement('a'); anchor.href = url; anchor.download = entry.name; anchor.click(); URL.revokeObjectURL(url)
    } catch (reason) { setError(reason instanceof Error ? reason.message : 'Download failed.') }
  }

  function submitPassword(event: FormEvent) { event.preventDefault(); void openShare(passwordInput) }
  const parentPath = path.includes('/') ? path.slice(0, path.lastIndexOf('/')) : ''

  return <main className="flex min-h-dvh items-center justify-center bg-muted/30 p-4"><Card className="w-full max-w-2xl">
    <CardHeader><div className="mb-2 flex size-10 items-center justify-center rounded-lg bg-primary/10 text-primary">{info?.passwordProtected ? <LockKeyhole className="size-5" /> : <Share2 className="size-5" />}</div><CardTitle>{info ? `Shared folder: ${info.shareName}` : 'Read-only folder share'}</CardTitle><p className="text-sm text-muted-foreground">{info ? `Browse and download files. This link expires ${new Date(info.expiresAt).toLocaleString()}.` : 'This private link grants read-only access to one folder.'}</p></CardHeader>
    <CardContent className="space-y-4">
      {!info ? <form onSubmit={submitPassword} className="flex flex-col gap-2 sm:flex-row"><input aria-label="Share link password" type="password" autoComplete="current-password" value={passwordInput} onChange={(event) => setPasswordInput(event.target.value)} className="h-9 flex-1 rounded-md border bg-background px-3 text-sm" placeholder="Password, if required" /><Button disabled={loading}>{loading ? 'Opening…' : 'Open shared folder'}</Button></form> : null}
      {error ? <p role="alert" className="text-sm text-destructive">{error}</p> : null}
      {info && entries ? <>
        <div className="flex items-center gap-2 rounded-md border bg-muted/30 px-3 py-2 text-xs text-muted-foreground"><span className="min-w-0 flex-1 truncate">/{path}</span>{path ? <Button size="sm" variant="outline" onClick={() => void browse(parentPath)}><ArrowLeft />Up</Button> : null}</div>
        {loading ? <p role="status" className="text-sm text-muted-foreground">Loading folder…</p> : null}
        {entries.entries.length ? <ul className="divide-y rounded-md border">{entries.entries.map((entry) => <li key={entry.id} className="flex items-center gap-3 px-3 py-2.5"><span className="flex size-8 items-center justify-center rounded bg-muted">{entry.type === 'dir' ? <Folder className="size-4" /> : <Download className="size-4" />}</span><button className="min-w-0 flex-1 truncate text-left text-sm font-medium hover:underline" onClick={() => entry.type === 'dir' ? void browse(path ? `${path}/${entry.name}` : entry.name) : void download(entry)}>{entry.name}</button><span className="text-xs text-muted-foreground">{entry.type === 'file' ? formatBytes(entry.sizeBytes) : 'Folder'}</span>{entry.type === 'file' ? <Button size="icon" variant="ghost" aria-label={`Download ${entry.name}`} onClick={() => void download(entry)}><Download /></Button> : null}</li>)}</ul> : <p className="rounded-md border p-6 text-center text-sm text-muted-foreground">This folder is empty.</p>}
      </> : null}
    </CardContent>
  </Card></main>
}
