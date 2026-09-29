import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowDownToLine, HardDrive, RefreshCw } from 'lucide-react'
import { apiDelete, apiGet, apiPost } from '@/api/client'
import { AlertBanner } from '@/components/core/alert-banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { Share, StorageMount } from '@/api/types'
import { formatBytes } from '@/lib/format'
import { toast } from 'sonner'

type Endpoint = { kind: 'share'; shareId: string; path?: string } | { kind: 'mount'; mountPath: string; path?: string } | { kind: 'destination'; destinationId: string; prefix: string }
type ImportTask = { id: string; name: string; source: Endpoint; destination: Endpoint; direction: 'one-way'; mode: 'copy'; deepCheck: true; enabled: boolean }
type ImportPreview = { plan: { changes: { path: string; action: string; bytes: number }[]; files: number; bytes: number; deletes: number }; planHash: string }
type RemoteProfile = { id: string; name: string; type: 'sftp' | 's3'; enabled: boolean }

export function NASImportWizard() {
  const client = useQueryClient()
  const sources = useQuery({ queryKey: ['backups', 'destinations'], queryFn: () => apiGet<RemoteProfile[]>('/backups/destinations') })
  const shares = useQuery({ queryKey: ['shares'], queryFn: () => apiGet<Share[]>('/shares') })
  const mounts = useQuery({ queryKey: ['storage', 'mounts'], queryFn: () => apiGet<{ entries: StorageMount[] }>('/storage/mounts') })
  const [name, setName] = useState('NAS import')
  const [sourceId, setSourceId] = useState('')
  const [prefix, setPrefix] = useState('')
  const [destinationId, setDestinationId] = useState('')
  const [relativePath, setRelativePath] = useState('')
  const [preview, setPreview] = useState<{ task: ImportTask; plan: ImportPreview } | null>(null)
  const [confirmed, setConfirmed] = useState(false)
  const remoteSources = sources.data?.filter((item) => item.enabled && (item.type === 'sftp' || item.type === 's3')) ?? []
  const localShares = shares.data ?? []
  const localMounts = mounts.data?.entries.filter((item) => item.enabled) ?? []

  const prepare = useMutation({
    mutationFn: async () => {
      if (!sourceId || !destinationId) throw new Error('Choose a remote source and local destination')
      const source: Endpoint = { kind: 'destination', destinationId: sourceId, prefix: prefix.trim() }
      const destination: Endpoint = destinationId.startsWith('share:')
        ? { kind: 'share', shareId: destinationId.slice(6), path: relativePath.trim() || undefined }
        : { kind: 'mount', mountPath: destinationId.slice(6), path: relativePath.trim() || undefined }
      const task = await apiPost<ImportTask>('/folder-sync/tasks', {
        name: name.trim() || 'NAS import', direction: 'one-way', source, destination,
        mode: 'copy', deepCheck: true, mirrorApproved: false, scheduleKind: 'manual', enabled: false,
      })
      let plan: ImportPreview
      try {
        plan = await apiPost<ImportPreview>(`/folder-sync/tasks/${encodeURIComponent(task.id)}/preview`)
      } catch (error) {
        await apiDelete(`/folder-sync/tasks/${encodeURIComponent(task.id)}`).catch(() => undefined)
        throw error
      }
      return { task, plan }
    },
    onSuccess: async (value) => {
      setPreview(value)
      setConfirmed(false)
      toast.success('Import preview is ready')
      await client.invalidateQueries({ queryKey: ['folder-sync'] })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not preview the NAS import'),
  })

  const run = useMutation({
    mutationFn: async () => {
      if (!preview || !confirmed) throw new Error('Review the plan and confirm the import')
      return apiPost(`/folder-sync/tasks/${encodeURIComponent(preview.task.id)}/run`, { planHash: preview.plan.planHash, confirmMirror: false })
    },
    onSuccess: async () => {
      toast.success('NAS import started')
      setPreview(null)
      setConfirmed(false)
      await Promise.all([
        client.invalidateQueries({ queryKey: ['folder-sync'] }),
        client.invalidateQueries({ queryKey: ['jobs'] }),
      ])
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not start the NAS import'),
  })

  return <Card>
    <CardHeader>
      <CardTitle className="flex items-center gap-2 text-sm"><ArrowDownToLine className="size-4" />Import from another NAS</CardTitle>
      <CardDescription>Copy files from a configured SFTP or S3 source into a local LumoNAS share or mounted volume. The source is unchanged, destination-only files are preserved, and every imported file is hash-checked.</CardDescription>
    </CardHeader>
    <CardContent className="space-y-4">
      <AlertBanner tone="info" title="Prepare the source NAS">
        Add its SFTP or S3 credentials under Backup destinations first. Configure the NAS to expose only the folder you intend to import. User ownership and NAS-specific ACLs are not copied; destination share permissions remain in effect.
      </AlertBanner>
      <form className="grid gap-3 sm:grid-cols-2" onSubmit={(event) => { event.preventDefault(); prepare.mutate() }}>
        <label className="space-y-1 text-xs font-medium text-muted-foreground">Import name
          <input aria-label="NAS import name" value={name} onChange={(event) => setName(event.target.value)} maxLength={80} required className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-sm text-foreground" />
        </label>
        <label className="space-y-1 text-xs font-medium text-muted-foreground">Remote source profile
          <select aria-label="NAS import source" value={sourceId} onChange={(event) => { setSourceId(event.target.value); setPreview(null) }} required className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-sm text-foreground">
            <option value="">Choose SFTP or S3 source</option>{remoteSources.map((source) => <option key={source.id} value={source.id}>{source.name} · {source.type.toUpperCase()}</option>)}
          </select>
        </label>
        <label className="space-y-1 text-xs font-medium text-muted-foreground">Remote folder / prefix
          <input aria-label="NAS import remote prefix" value={prefix} onChange={(event) => { setPrefix(event.target.value); setPreview(null) }} placeholder="Photos/2026" className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-sm text-foreground" />
        </label>
        <label className="space-y-1 text-xs font-medium text-muted-foreground">Local destination
          <select aria-label="NAS import destination" value={destinationId} onChange={(event) => { setDestinationId(event.target.value); setPreview(null) }} required className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-sm text-foreground">
            <option value="">Choose a share or mounted volume</option>
            <optgroup label="Managed shares">{localShares.map((share) => <option key={share.id} value={`share:${share.id}`}>{share.name}</option>)}</optgroup>
            <optgroup label="Mounted volumes">{localMounts.map((mount) => <option key={mount.mountPath} value={`mount:${mount.mountPath}`}>{mount.mountPath}</option>)}</optgroup>
          </select>
        </label>
        <label className="space-y-1 text-xs font-medium text-muted-foreground sm:col-span-2">Folder inside destination
          <input aria-label="NAS import destination path" value={relativePath} onChange={(event) => { setRelativePath(event.target.value); setPreview(null) }} placeholder="Imported/Photos" className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-sm text-foreground" />
          <span className="block pt-1">Leave blank to copy into the selected share or volume root.</span>
        </label>
        <div className="sm:col-span-2"><Button disabled={prepare.isPending || !sourceId || !destinationId}><RefreshCw />{prepare.isPending ? 'Scanning and hashing source…' : 'Scan and preview import'}</Button></div>
      </form>
      {sources.isError || shares.isError || mounts.isError ? <p role="alert" className="text-sm text-destructive">Could not load import sources or destinations.</p> : null}
      {preview ? <div className="space-y-3 rounded-lg border p-3">
        <div><p className="text-sm font-medium">Review import plan</p><p className="text-xs text-muted-foreground">{preview.plan.plan.files.toLocaleString()} file copies · {formatBytes(preview.plan.plan.bytes)} · {preview.plan.plan.deletes} deletions</p></div>
        <p className="text-xs text-muted-foreground">No destination files will be deleted. Existing files with matching paths may be updated after verification. The remote NAS remains unchanged.</p>
        {preview.plan.plan.changes.length ? <div className="max-h-40 overflow-auto rounded border">{preview.plan.plan.changes.slice(0, 100).map((change) => <div key={`${change.action}:${change.path}`} className="flex justify-between gap-3 border-b px-2 py-1 text-xs last:border-0"><span className="truncate font-mono">{change.path}</span><span>{change.action}</span></div>)}</div> : <p className="rounded border border-dashed p-3 text-center text-xs text-muted-foreground">Everything in this source is already present.</p>}
        <label className="flex items-start gap-2 text-xs"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} className="mt-0.5" /><span>I reviewed the source, destination, and planned file changes. Destination-only files will be preserved.</span></label>
        <Button onClick={() => run.mutate()} disabled={!confirmed || run.isPending || preview.plan.plan.files === 0}><HardDrive />{run.isPending ? 'Starting import…' : 'Import files'}</Button>
      </div> : null}
    </CardContent>
  </Card>
}
