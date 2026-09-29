import { useState } from 'react'
import { Archive } from 'lucide-react'
import { useDeleteShare, usePrincipals, usePreviewShareRelocation, useShare, useShareAccess, useShareProtocol, useShareSettings, useShareStorageResources, useStartShareRelocation } from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { DangerZone } from '@/components/core/danger-zone'
import { HealthBadge } from '@/components/core/health-badge'
import { ResourceDrawer } from '@/components/core/resource-drawer'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { formatBytes } from '@/lib/format'
import { cn } from '@/lib/utils'
import type { AccessLevel, Share, ShareProtocolType, ShareRelocationPreview } from '@/api/types'

const PROTOCOL_LABELS: Record<ShareProtocolType, string> = {
  smb: 'SMB — Windows, macOS, Linux file sharing',
  nfs: 'NFS — Linux / Unix mounts',
  sftp: 'SFTP — encrypted file transfer',
  rsync: 'rsync — efficient sync target',
  timemachine: 'Time Machine — macOS backups',
}

const LEVELS: { value: AccessLevel; label: string }[] = [
  { value: 'none', label: 'No access' },
  { value: 'read', label: 'Read only' },
  { value: 'write', label: 'Read & write' },
]

const SMB_AUDIT_OPERATIONS = [
  ['create_file', 'Create or overwrite files'], ['mkdirat', 'Create folders'],
  ['renameat', 'Rename or move'], ['unlinkat', 'Delete files'], ['rmdir', 'Delete folders'],
  ['pwrite', 'Write file data'], ['ftruncate', 'Change file size'],
] as const

export function ShareDrawer({
  shareId,
  onOpenChange,
}: {
  shareId: string | null
  onOpenChange: (open: boolean) => void
}) {
  const { data: share } = useShare(shareId)
  const { data: principals } = usePrincipals()
  const deleteShare = useDeleteShare()

  if (!share) return null

  return (
    <ResourceDrawer
      open={shareId != null}
      onOpenChange={onOpenChange}
      title={
        <span className="flex items-center gap-2.5">
          {share.name}
          <HealthBadge state={share.status} />
        </span>
      }
      description={
        <span className="text-sm">
          {share.resourceLabel}
          <span className="ml-2 font-mono text-xs text-muted-foreground">{share.relativePath}</span>
        </span>
      }
    >
      <Tabs defaultValue="access">
        <TabsList className="w-full justify-start overflow-x-auto">
          <TabsTrigger value="access">Access</TabsTrigger>
          <TabsTrigger value="protocols">Protocols</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
          <TabsTrigger value="location">Location</TabsTrigger>
        </TabsList>

        <TabsContent value="access">
          <AccessTab key={share.id} share={share} principals={principals ?? []} />
        </TabsContent>

        <TabsContent value="protocols">
          <ProtocolsTab key={`proto-${share.id}`} share={share} />
        </TabsContent>

        <TabsContent value="settings">
          <SettingsTab key={`settings-${share.id}`} share={share} />
        </TabsContent>

        <TabsContent value="location">
          <ShareRelocationTab key={`location-${share.id}`} share={share} />
        </TabsContent>
      </Tabs>

      <DangerZone
        identity={
          <div className="flex flex-col gap-1 text-sm">
            <span className="font-medium">{share.name}</span>
            <span className="font-mono text-xs text-muted-foreground">
              {share.resourceLabel}{share.relativePath} · {formatBytes(share.usedBytes ?? 0)} used
            </span>
          </div>
        }
        actions={[
          {
            id: 'delete-share',
            label: 'Remove share',
            description:
              'The share definition and its protocol exposure are removed. Files at this location are not deleted — the folder stays on disk.',
            match: share.name,
          },
        ]}
        onAction={(actionId) => {
          if (actionId === 'delete-share') {
            deleteShare.mutate(share.id, { onSuccess: () => onOpenChange(false) })
          }
        }}
      />
    </ResourceDrawer>
  )
}

function ShareRelocationTab({ share }: { share: Share }) {
  const { data: resources, isLoading, isError } = useShareStorageResources()
  const previewMove = usePreviewShareRelocation()
  const startMove = useStartShareRelocation()
  const [resourceId, setResourceId] = useState('')
  const [relativePath, setRelativePath] = useState(share.name)
  const [scheduleKind, setScheduleKind] = useState<'manual' | 'daily' | 'weekly'>('manual')
  const [timeOfDay, setTimeOfDay] = useState('02:00')
  const [weekday, setWeekday] = useState('sunday')
  const [preview, setPreview] = useState<ShareRelocationPreview | null>(null)
  const [confirmed, setConfirmed] = useState(false)

  function invalidatePreview() {
    setPreview(null)
    setConfirmed(false)
  }

  function requestPreview() {
    if (!resourceId || !relativePath.trim()) return
    invalidatePreview()
    previewMove.mutate({ shareId: share.id, target: { resourceId, relativePath: relativePath.trim() } }, {
      onSuccess: setPreview,
    })
  }

  function startRelocation() {
    if (!preview || !confirmed) return
    startMove.mutate({ shareId: share.id, target: { resourceId, relativePath: relativePath.trim() }, planHash: preview.planHash, scheduleKind, timeOfDay, weekday }, {
      onSuccess: () => { setPreview(null); setConfirmed(false) },
    })
  }

  return <div className="flex flex-col gap-3">
    <AlertBanner tone="warning" title="Move this share to another storage location">
      LumoNAS copies and verifies every file before changing the share path. The original directory stays on disk so you can recover it manually.
    </AlertBanner>
    <label className="space-y-1 text-xs font-medium text-muted-foreground">Destination storage
      <select aria-label="Relocation destination storage" value={resourceId} onChange={(event) => { setResourceId(event.target.value); invalidatePreview() }} disabled={isLoading || !resources?.length} className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-sm text-foreground">
        <option value="">{isLoading ? 'Loading mounted storage…' : 'Choose a mounted pool or disk'}</option>
        {(resources ?? []).map((resource) => <option key={resource.id} value={resource.id}>{resource.label} · {formatBytes(resource.usedBytes ?? 0)} used</option>)}
      </select>
    </label>
    <label className="space-y-1 text-xs font-medium text-muted-foreground">Folder inside destination
      <Input aria-label="Relocation destination folder" value={relativePath} onChange={(event) => { setRelativePath(event.target.value); invalidatePreview() }} placeholder={share.name} />
    </label>
    <label className="space-y-1 text-xs font-medium text-muted-foreground">Run
      <select aria-label="Relocation schedule" value={scheduleKind} onChange={(event) => setScheduleKind(event.target.value as typeof scheduleKind)} className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-sm text-foreground">
        <option value="manual">Now, after confirmation</option><option value="daily">Schedule daily</option><option value="weekly">Schedule weekly</option>
      </select>
    </label>
    {scheduleKind !== 'manual' ? <label className="space-y-1 text-xs font-medium text-muted-foreground">NAS local time
      <Input aria-label="Relocation schedule time" type="time" value={timeOfDay} onChange={(event) => setTimeOfDay(event.target.value)} />
    </label> : null}
    {scheduleKind === 'weekly' ? <label className="space-y-1 text-xs font-medium text-muted-foreground">Weekday
      <select aria-label="Relocation schedule weekday" value={weekday} onChange={(event) => setWeekday(event.target.value)} className="mt-1 h-9 w-full rounded-md border bg-background px-3 text-sm text-foreground">{['monday','tuesday','wednesday','thursday','friday','saturday','sunday'].map((day) => <option key={day} value={day}>{day}</option>)}</select>
    </label> : null}
    {isError ? <p role="alert" className="text-sm text-destructive">Could not load mounted storage locations.</p> : null}
    {previewMove.isError ? <p role="alert" className="text-sm text-destructive">{previewMove.error instanceof Error ? previewMove.error.message : 'Could not preview this relocation.'}</p> : null}
    <Button variant="outline" onClick={requestPreview} disabled={!resourceId || !relativePath.trim() || previewMove.isPending || startMove.isPending}>{previewMove.isPending ? 'Scanning and hashing files…' : 'Preview relocation'}</Button>
    {preview ? <div className="space-y-3 rounded-lg border p-3">
      <div><p className="text-sm font-medium">Relocation preview</p><p className="mt-1 break-all font-mono text-xs text-muted-foreground">{preview.sourcePath} → {preview.destinationPath}</p></div>
      <p className="text-sm">{preview.fileCount.toLocaleString()} files · {formatBytes(preview.bytes)}</p>
      {preview.requiresDowntime ? <p className="text-xs text-attention">The share will be briefly unavailable while files are copied and checked.</p> : <p className="text-xs text-muted-foreground">The share is currently disabled; it will remain disabled after relocation.</p>}
      <p className="text-xs text-muted-foreground">The destination must be empty. LumoNAS will verify file hashes before switching the share path. Original files will remain untouched.</p>
      <label className="flex items-start gap-2 text-xs"><input type="checkbox" checked={confirmed} onChange={(event) => setConfirmed(event.target.checked)} className="mt-0.5" /><span>I reviewed the destination and understand the share will be unavailable during the move. I will keep the old directory until I verify the new location.</span></label>
      <Button onClick={startRelocation} disabled={!confirmed || startMove.isPending}>{startMove.isPending ? 'Starting…' : scheduleKind === 'manual' ? 'Move share' : 'Schedule move'}</Button>
    </div> : null}
    {startMove.isError ? <p role="alert" className="text-sm text-destructive">{startMove.error instanceof Error ? startMove.error.message : 'Could not start the relocation.'}</p> : null}
    {startMove.data ? <p role="status" className="text-sm text-muted-foreground">{'schedule' in startMove.data ? `Move scheduled ${startMove.data.schedule.schedule}. Manage it in Monitoring → Jobs → Scheduled jobs.` : `Relocation queued as job ${startMove.data.jobId}. Track it in Monitoring → Jobs.`}</p> : null}
  </div>
}

function AccessTab({
  share,
  principals,
}: {
  share: Share
  principals: { id: string; name: string; type: string }[]
}) {
  const shareAccess = useShareAccess()

  const rows = [
    ...share.access,
    ...principals
      .filter((p) => !share.access.some((a) => a.principalId === p.id))
      .map((p) => ({ principalId: p.id, level: 'none' as AccessLevel })),
  ]

  return (
    <div className="flex flex-col gap-3">
      <div className="overflow-hidden rounded-lg border">
        <table className="w-full text-sm">
          <thead>
            <tr className="border-b bg-muted/40">
              <th className="px-3 py-2 text-left text-xs font-medium tracking-wide text-muted-foreground uppercase">
                Principal
              </th>
              <th className="px-3 py-2 text-left text-xs font-medium tracking-wide text-muted-foreground uppercase">
                Level
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => {
              const principal = principals.find((p) => p.id === row.principalId)
              const builtIn = row.principalId === 'p-admins'
              return (
                <tr key={row.principalId} className="border-b last:border-b-0">
                  <td className="px-3 py-2">
                    <span className="flex items-center gap-2">
                      <span className="text-[13px] font-medium">
                        {principal?.name ?? row.principalId}
                      </span>
                      {builtIn && <Badge variant="secondary">built-in</Badge>}
                      {principal?.type === 'service' && <Badge variant="info">service</Badge>}
                      {principal?.type === 'group' && (
                        <span className="text-xs text-muted-foreground">group</span>
                      )}
                    </span>
                  </td>
                  <td className="px-3 py-2">
                    <Select
                      value={row.level}
                      disabled={builtIn || shareAccess.isPending}
                      onValueChange={(level) =>
                        shareAccess.mutate({
                          id: share.id,
                          principalId: row.principalId,
                          level: level as AccessLevel,
                        })
                      }
                    >
                      <SelectTrigger className="h-8 w-36 text-xs">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {LEVELS.map((level) => (
                          <SelectItem key={level.value} value={level.value}>
                            {level.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
      <p className="text-xs text-muted-foreground">
        Access is a simple canonical model — NFS hosts and Time Machine quota add protocol-level
        restrictions on top. UID/GID mapping is available under advanced details.
      </p>
    </div>
  )
}

function ProtocolsTab({ share }: { share: Share }) {
  const shareProtocol = useShareProtocol()

  const allProtocols: ShareProtocolType[] = ['smb', 'nfs', 'sftp', 'rsync', 'timemachine']

  return (
    <div className="flex flex-col gap-3">
      {allProtocols.map((protocol) => {
        const config = share.protocols.find((p) => p.protocol === protocol)
        const enabled = config?.enabled ?? false
        return (
          <div key={protocol} className={cn('rounded-xl border p-4', !enabled && 'opacity-70')}>
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="text-sm font-medium">
                  {protocol === 'timemachine' ? 'Time Machine' : protocol.toUpperCase()}
                </p>
                <p className="mt-0.5 text-xs text-muted-foreground">{PROTOCOL_LABELS[protocol]}</p>
              </div>
              <Switch
                checked={enabled}
                onCheckedChange={(next) =>
                  shareProtocol.mutate({ id: share.id, protocol, body: { enabled: next } })
                }
                aria-label={`Toggle ${protocol}`}
              />
            </div>

            {enabled && protocol === 'nfs' && (
              <div className="mt-3 grid gap-3">
                <div className="grid gap-2">
                  <Label className="text-xs text-muted-foreground">Allowed hosts / networks</Label>
                  <Input
                    defaultValue={config?.hosts ?? '192.168.1.0/24'}
                    className="h-8 font-mono text-xs"
                    onBlur={(e) =>
                      shareProtocol.mutate({
                        id: share.id,
                        protocol,
                        body: { hosts: e.target.value },
                      })
                    }
                  />
                </div>
                <div className="flex items-center justify-between gap-4">
                  <Label className="text-xs text-muted-foreground">Read-only export</Label>
                  <Switch
                    checked={config?.readOnly ?? false}
                    onCheckedChange={(next) =>
                      shareProtocol.mutate({ id: share.id, protocol, body: { readOnly: next } })
                    }
                    aria-label="NFS read-only"
                  />
                </div>
              </div>
            )}

            {enabled && protocol === 'timemachine' && (
              <div className="mt-3 grid gap-2">
                <Label className="text-xs text-muted-foreground">Quota</Label>
                <p className="tnum text-sm">
                  {config?.quotaBytes ? formatBytes(config.quotaBytes) : 'Unlimited'}
                </p>
                <p className="text-xs text-muted-foreground">
                  Quota prevents Time Machine from filling the whole Apps SSD.
                </p>
              </div>
            )}

            {enabled && protocol === 'smb' && (
              <div className="mt-3 space-y-3">
                <p className="text-xs text-muted-foreground">Btrfs snapshots of this share appear in Windows Explorer under Previous Versions.</p>
                <div className="flex items-center justify-between gap-4 rounded-lg border p-3">
                  <div><p className="text-sm font-medium">Audit SMB changes</p><p className="text-xs text-muted-foreground">Selected file and folder operations go to the system journal.</p></div>
                  <Switch checked={config?.auditEnabled ?? false} onCheckedChange={(next) => shareProtocol.mutate({ id: share.id, protocol, body: { auditEnabled: next, auditOperations: config?.auditOperations?.length ? config.auditOperations : ['create_file', 'mkdirat', 'renameat', 'unlinkat', 'rmdir'] } })} aria-label="Audit SMB changes" />
                </div>
                {config?.auditEnabled && <fieldset className="grid gap-2 rounded-lg border p-3"><legend className="px-1 text-xs font-medium text-muted-foreground">Operations to record</legend>{SMB_AUDIT_OPERATIONS.map(([operation, label]) => <label key={operation} className="flex items-center gap-2 text-xs"><input type="checkbox" checked={config.auditOperations?.includes(operation) ?? false} onChange={(event) => { const current = config.auditOperations ?? []; const next = event.target.checked ? [...current, operation] : current.filter((value) => value !== operation); if (next.length > 0) shareProtocol.mutate({ id: share.id, protocol, body: { auditOperations: next } }) }} />{label}</label>)}<p className="text-[11px] text-muted-foreground">At least one operation must stay selected. High-volume shares can produce many events.</p></fieldset>}
                <details>
                  <summary className="cursor-pointer text-xs text-muted-foreground select-none">
                    Advanced SMB options
                  </summary>
                  <p className="mt-2 text-xs text-muted-foreground">
                    Veto files, socket options and auxiliary parameters are preserved as-is and can
                    be edited by experts.
                  </p>
                </details>
              </div>
            )}
          </div>
        )
      })}
      <AlertBanner tone="info" title="Plain FTP is not offered">
        FTPS is available for legacy clients. For everything else, SMB/SFTP are safer defaults.
      </AlertBanner>
    </div>
  )
}

function SettingsTab({ share }: { share: Share }) {
  const shareSettings = useShareSettings()
  const [description, setDescription] = useState(share.description ?? '')
  const [recycleBin, setRecycleBin] = useState(share.recycleBin)

  return (
    <div className="flex flex-col gap-5">
      <div className="grid gap-2">
        <Label htmlFor="share-description">Description</Label>
        <Input
          id="share-description"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          onBlur={() => {
            if (description !== (share.description ?? '')) {
              shareSettings.mutate({ id: share.id, description })
            }
          }}
        />
      </div>
      <div className="flex items-center justify-between gap-4">
        <div>
          <Label htmlFor="share-recycle">Recycle bin</Label>
          <p className="text-xs text-muted-foreground">
            Deleted files are kept for 30 days. A recycle bin is not a backup.
          </p>
        </div>
        <Switch
          id="share-recycle"
          checked={recycleBin}
          onCheckedChange={(next) => {
            setRecycleBin(next)
            shareSettings.mutate({ id: share.id, recycleBin: next })
          }}
        />
      </div>
      <div className="flex items-start gap-3 rounded-lg border bg-muted/30 p-3 text-xs text-muted-foreground">
        <Archive className="mt-0.5 size-4 shrink-0" />
        SnapRAID awareness: deleting or moving files here is normal filesystem behavior — the
        Protection screen will show unsynced changes until the next parity sync.
      </div>
    </div>
  )
}
