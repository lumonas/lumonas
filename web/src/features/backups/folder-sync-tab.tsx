import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiDelete, apiGet, apiPost } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { Share, StorageMount } from '@/api/types'
import { toast } from 'sonner'
import { NASImportWizard } from '@/features/backups/nas-import-wizard'

type Endpoint = { kind: 'share'; shareId: string; path?: string } | { kind: 'mount'; mountPath: string; path?: string } | { kind: 'destination'; destinationId: string; prefix?: string }
type Task = { id: string; name: string; direction?: 'one-way' | 'two-way'; source: Endpoint; destination: Endpoint; mode: 'copy' | 'mirror'; deepCheck: boolean; ignorePatterns?: string[]; mirrorApproved: boolean; enabled: boolean; onUsbAttach?: boolean; scheduleKind?: 'manual' | 'daily' | 'weekly'; timeOfDay?: string; weekday?: string; updatedAt: string }
type Change = { path: string; action: string; bytes: number; from?: string; to?: string; archivePath?: string; reason?: string }
type Preview = { plan: { changes: Change[]; files: number; bytes: number; deletes: number; conflicts: number }; planHash: string; requiresConfirmation: boolean }
type Run = { id: string; state: string; startedAt: string; files: number; bytes: number; deleted: number; conflicts?: number; error?: string }
type DestinationProfile = { id: string; name: string; type: 'local' | 'sftp' | 's3'; enabled: boolean }

export function FolderSyncTab() {
  const client = useQueryClient()
  const [name, setName] = useState('')
  const [source, setSource] = useState('')
  const [destination, setDestination] = useState('')
  const [direction, setDirection] = useState<'one-way' | 'two-way'>('one-way')
  const [remotePrefix, setRemotePrefix] = useState('folder-sync')
  const [mode, setMode] = useState<'copy' | 'mirror'>('copy')
  const [deepCheck, setDeepCheck] = useState(false)
  const [ignoreText, setIgnoreText] = useState('')
  const [scheduleKind, setScheduleKind] = useState<'manual' | 'daily' | 'weekly'>('manual')
  const [timeOfDay, setTimeOfDay] = useState('02:00')
  const [weekday, setWeekday] = useState('sunday')
  const [onUsbAttach, setOnUsbAttach] = useState(false)
  const [preview, setPreview] = useState<Record<string, Preview>>({})
  const [selectedHistory, setSelectedHistory] = useState<string | null>(null)
  const shares = useQuery({ queryKey: ['shares'], queryFn: () => apiGet<Share[]>('/shares') })
  const mounts = useQuery({ queryKey: ['storage', 'mounts'], queryFn: () => apiGet<{ entries: StorageMount[] }>('/storage/mounts') })
  const backupDestinations = useQuery({ queryKey: ['backups', 'destinations'], queryFn: () => apiGet<DestinationProfile[]>('/backups/destinations') })
  const tasks = useQuery({ queryKey: ['folder-sync'], queryFn: () => apiGet<Task[]>('/folder-sync/tasks') })
  const create = useMutation({
    mutationFn: () => apiPost<Task>('/folder-sync/tasks', { name: name.trim(), direction, source: endpoint(source), destination: endpoint(destination), mode: direction === 'two-way' ? 'copy' : mode, deepCheck, ignorePatterns: ignoreText.split('\n').map((item) => item.trim()).filter(Boolean), mirrorApproved: direction === 'two-way' ? false : mode === 'mirror', onUsbAttach: direction === 'two-way' ? false : onUsbAttach, scheduleKind, timeOfDay, weekday, enabled: scheduleKind !== 'manual' || onUsbAttach }),
    onSuccess: async () => { setName(''); setOnUsbAttach(false); toast.success('Sync task created'); await client.invalidateQueries({ queryKey: ['folder-sync'] }) },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not create sync task'),
  })
  const remove = useMutation({ mutationFn: (id: string) => apiDelete(`/folder-sync/tasks/${id}`), onSuccess: () => client.invalidateQueries({ queryKey: ['folder-sync'] }), onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not delete task') })
  const run = useMutation({
    mutationFn: async ({ task, selected }: { task: Task; selected?: Preview }) => {
      if ((task.mode === 'mirror' || task.direction === 'two-way') && !selected) return null
      if (selected?.requiresConfirmation && !window.confirm(task.direction === 'two-way'
        ? `Apply the reviewed two-way plan with ${selected.plan.files} transfers, ${selected.plan.conflicts} conflicts, and ${selected.plan.deletes} deletions? Changed files are retained in .lumonas-versions.`
        : `Mirror will delete ${selected.plan.deletes} destination files. Continue with this preview?`)) return null
      return apiPost<Run>(`/folder-sync/tasks/${task.id}/run`, { planHash: selected?.planHash ?? '', confirmMirror: task.mode === 'mirror', confirmTwoWay: task.direction === 'two-way' })
    },
    onSuccess: async (value) => { if (value) toast.success('Folder sync started'); await client.invalidateQueries({ queryKey: ['folder-sync'] }) },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not start sync'),
  })
  const getPreview = useMutation({
    mutationFn: (id: string) => apiPost<Preview>(`/folder-sync/tasks/${id}/preview`),
    onSuccess: (value, id) => setPreview((current) => ({ ...current, [id]: value })),
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not preview sync'),
  })
  const cancel = useMutation({
    mutationFn: (id: string) => apiPost(`/folder-sync/tasks/${id}/runs/cancel`),
    onSuccess: () => toast.success('Cancellation requested'),
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not cancel sync'),
  })
  const history = useQuery({ queryKey: ['folder-sync-runs', selectedHistory], enabled: selectedHistory != null, refetchInterval: selectedHistory ? 5000 : false, queryFn: () => apiGet<Run[]>(`/folder-sync/tasks/${selectedHistory}/runs`) })
  function endpoint(value: string): Endpoint { return value.startsWith('mount:') ? { kind: 'mount', mountPath: value.slice(6) } : value.startsWith('remote:') ? { kind: 'destination', destinationId: value.slice(7), prefix: remotePrefix.trim() } : { kind: 'share', shareId: value } }
  const remoteProfiles = backupDestinations.data?.filter((item) => item.enabled && (item.type === 'sftp' || item.type === 's3')) ?? []
  const sourceIsRemote = source.startsWith('remote:')
  const destinationIsRemote = destination.startsWith('remote:')
  const destinationIsMount = destination.startsWith('mount:')

  return <div className="flex flex-col gap-4">
    <NASImportWizard />
    <Card>
      <CardHeader><CardTitle className="text-sm">Folder sync</CardTitle><CardDescription>Schedule one-way copies to local volumes or SFTP/S3, or merge two managed folders. Two-way runs require a reviewed preview, retain overwritten and deleted versions on both sides, and preserve concurrent edits as conflicts.</CardDescription></CardHeader>
      <CardContent>
        {shares.isError ? <p role="alert" className="text-sm text-destructive">Could not load managed shares.</p> : <form className="grid gap-3 sm:grid-cols-2" onSubmit={(event) => { event.preventDefault(); create.mutate() }}>
          <label className="text-xs text-muted-foreground">Task name<input aria-label="Sync task name" value={name} onChange={(event) => setName(event.target.value)} maxLength={80} required className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground" placeholder="Family photos" /></label>
          <label className="text-xs text-muted-foreground">Sync direction<select aria-label="Sync direction" value={direction} onChange={(event) => { const next = event.target.value as 'one-way' | 'two-way'; setDirection(next); if (next === 'two-way') { if (source.startsWith('remote:')) setSource(''); if (destination.startsWith('remote:')) setDestination(''); setMode('copy'); setOnUsbAttach(false) } }} className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground"><option value="one-way">One-way copy</option><option value="two-way">Two-way merge · managed folders only</option></select></label>
          <label className="text-xs text-muted-foreground">Source<select aria-label="Sync source share" value={source} onChange={(event) => setSource(event.target.value)} required className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground"><option value="">Choose a source</option><optgroup label="Managed shares">{shares.data?.map((share) => <option key={share.id} value={share.id}>{share.name}</option>)}</optgroup><optgroup label="Mounted volumes">{mounts.data?.entries.filter((entry) => entry.enabled).map((entry) => <option key={entry.mountPath} value={`mount:${entry.mountPath}`}>{entry.mountPath}</option>)}</optgroup>{direction === 'one-way' && <optgroup label="SFTP and S3 backup profiles">{remoteProfiles.map((item) => <option key={item.id} value={`remote:${item.id}`}>{item.name} · {item.type.toUpperCase()}</option>)}</optgroup>}</select></label>
          <label className="text-xs text-muted-foreground">{direction === 'two-way' ? 'Second folder' : 'Destination'}<select aria-label="Sync destination share" value={destination} onChange={(event) => setDestination(event.target.value)} required className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground"><option value="">Choose a destination</option><optgroup label="Managed shares">{shares.data?.filter((share) => share.id !== source).map((share) => <option key={share.id} value={share.id}>{share.name}</option>)}</optgroup><optgroup label="Mounted volumes">{mounts.data?.entries.filter((entry) => entry.enabled && `mount:${entry.mountPath}` !== source).map((entry) => <option key={entry.mountPath} value={`mount:${entry.mountPath}`}>{entry.mountPath}</option>)}</optgroup>{direction === 'one-way' && !sourceIsRemote && <optgroup label="SFTP and S3 backup profiles">{remoteProfiles.filter((item) => `remote:${item.id}` !== source).map((item) => <option key={item.id} value={`remote:${item.id}`}>{item.name} · {item.type.toUpperCase()}</option>)}</optgroup>}</select></label>
          {(sourceIsRemote || destinationIsRemote) && <label className="text-xs text-muted-foreground">Remote prefix<input aria-label="Sync remote prefix" value={remotePrefix} onChange={(event) => setRemotePrefix(event.target.value)} className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground" placeholder="folder-sync" /></label>}
          {direction === 'one-way' ? <label className="text-xs text-muted-foreground">Sync behavior<select aria-label="Sync behavior" value={mode} onChange={(event) => setMode(event.target.value as 'copy' | 'mirror')} className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground"><option value="copy">Copy and update only</option><option value="mirror">Mirror (allow deletions)</option></select></label> : <p className="self-end text-xs text-muted-foreground">Two-way keeps recoverable versions of changes and deletions on both folders.</p>}
          <label className="text-xs text-muted-foreground">Schedule<select aria-label="Sync schedule" value={scheduleKind} onChange={(event) => setScheduleKind(event.target.value as typeof scheduleKind)} className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground"><option value="manual">Manual only</option><option value="daily">Daily</option><option value="weekly">Weekly</option></select></label>
          {scheduleKind !== 'manual' && <label className="text-xs text-muted-foreground">Run at<input aria-label="Sync schedule time" type="time" value={timeOfDay} onChange={(event) => setTimeOfDay(event.target.value)} className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground" /></label>}
          {scheduleKind === 'weekly' && <label className="text-xs text-muted-foreground">Day<select aria-label="Sync schedule weekday" value={weekday} onChange={(event) => setWeekday(event.target.value)} className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground">{['monday','tuesday','wednesday','thursday','friday','saturday','sunday'].map((day) => <option key={day} value={day}>{day}</option>)}</select></label>}
          <label className="flex items-center gap-2 text-xs text-muted-foreground sm:col-span-2"><input type="checkbox" checked={deepCheck} onChange={(event) => setDeepCheck(event.target.checked)} />Deep check by hashing every file</label>
          <label className="text-xs text-muted-foreground sm:col-span-2">Ignore patterns<textarea aria-label="Sync ignore patterns" value={ignoreText} onChange={(event) => setIgnoreText(event.target.value)} maxLength={2048} rows={2} className="mt-1 w-full rounded-md border bg-background px-2 py-1.5 font-mono text-sm text-foreground" placeholder="*.tmp\n.cache/**" /><span className="mt-1 block">One relative glob per line. Ignored files stay local and are excluded from deletion plans.</span></label>
          {direction === 'one-way' && destinationIsMount && !sourceIsRemote && <label className="flex items-start gap-2 text-xs text-muted-foreground sm:col-span-2"><input type="checkbox" className="mt-0.5" checked={onUsbAttach} onChange={(event) => setOnUsbAttach(event.target.checked)} /><span>Run when this USB disk is connected<p className="mt-0.5">The destination must be a mounted USB disk. A disk connected at NAS startup is ignored until it is unplugged and reconnected.</p></span></label>}
          <div className="sm:col-span-2"><Button disabled={create.isPending || !source || !destination || source === destination || (sourceIsRemote && destinationIsRemote)}>{create.isPending ? 'Creating…' : 'Create sync task'}</Button></div>
        </form>}
      </CardContent>
    </Card>
    {tasks.isLoading ? <p className="text-sm text-muted-foreground">Loading sync tasks…</p> : tasks.isError ? <p role="alert" className="text-sm text-destructive">Could not load folder sync tasks.</p> : tasks.data?.length ? tasks.data.map((task) => {
      const plan = preview[task.id]
      const requiresReview = task.mode === 'mirror' || task.direction === 'two-way'
      return <Card key={task.id}><CardHeader className="pb-3"><div className="flex flex-wrap items-start justify-between gap-2"><div><CardTitle className="text-sm">{task.name}</CardTitle><CardDescription>{task.direction === 'two-way' ? 'Two-way merge · versions retained on both folders' : task.mode === 'mirror' ? 'Mirror · destination deletions enabled' : 'One-way copy and update · destination files are preserved'}{task.deepCheck ? ' · full content hash' : ''}{task.ignorePatterns?.length ? ` · ${task.ignorePatterns.length} ignore patterns` : ''}{task.scheduleKind && task.scheduleKind !== 'manual' ? ` · ${task.scheduleKind} at ${task.timeOfDay}${task.weekday ? ` on ${task.weekday}` : ''}` : ' · manual schedule'}{task.onUsbAttach ? ' · runs on USB connect' : ''}</CardDescription></div><div className="flex flex-wrap gap-2"><Button size="sm" variant="outline" onClick={() => getPreview.mutate(task.id)} disabled={getPreview.isPending}>Preview</Button><Button size="sm" onClick={() => run.mutate({ task, selected: plan })} disabled={run.isPending || (requiresReview && !plan)}>{requiresReview && !plan ? 'Preview before run' : 'Run now'}</Button><Button size="sm" variant="ghost" onClick={() => { setSelectedHistory(selectedHistory === task.id ? null : task.id) }}>History</Button><Button size="sm" variant="ghost" onClick={() => remove.mutate(task.id)} disabled={remove.isPending}>Delete</Button></div></div></CardHeader>
        {plan && <CardContent className="border-t pt-3"><p className="mb-2 text-xs text-muted-foreground">Preview: {plan.plan.files} file transfers · {plan.plan.bytes.toLocaleString()} bytes · {plan.plan.conflicts} conflicts · {plan.plan.deletes} planned deletions</p><div className="max-h-40 overflow-auto rounded border">{plan.plan.changes.slice(0,100).map((change, index) => <div key={`${change.action}-${change.path}-${index}`} className="flex justify-between gap-3 border-b px-2 py-1 text-xs last:border-0"><span className="min-w-0 truncate font-mono">{change.archivePath ? `${change.path} → ${change.archivePath}` : change.path}</span><span className={change.action === 'delete' ? 'text-destructive' : 'shrink-0 text-muted-foreground'}>{change.action}{change.from && change.to ? ` ${change.from} → ${change.to}` : ''}</span></div>)}{plan.plan.changes.length > 100 && <p className="p-2 text-xs text-muted-foreground">Showing first 100 planned changes.</p>}</div></CardContent>}
        {selectedHistory === task.id && <CardContent className="border-t pt-3">{history.isLoading ? <p className="text-xs text-muted-foreground">Loading run history…</p> : history.data?.length ? <ul className="space-y-1">{history.data.map((item) => <li key={item.id} className="flex flex-wrap items-center justify-between gap-2 text-xs"><span><span className="font-medium">{item.state}</span> · {new Date(item.startedAt).toLocaleString()} · {item.files} files · {item.conflicts ?? 0} conflicts · {item.deleted} deletions{item.error ? ` · ${item.error}` : ''}</span>{item.state === 'running' && <Button size="sm" variant="outline" onClick={() => cancel.mutate(task.id)} disabled={cancel.isPending}>Cancel</Button>}</li>)}</ul> : <p className="text-xs text-muted-foreground">No runs yet.</p>}</CardContent>}
      </Card>
    }) : <Card><CardContent className="py-8 text-center text-sm text-muted-foreground">No folder sync tasks yet. Create one to preview and copy a managed share.</CardContent></Card>}
  </div>
}
