import { useState, type FormEvent } from 'react'
import { CheckCircle2, Cloud, HardDrive, KeyRound, LockKeyhole, Plus, Server, Upload, RotateCcw } from 'lucide-react'
import { useBackupDestinationRestoreCheck, useBackupDestinations, useSaveBackupDestination, type BackupDestinationInput } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { timeAgo } from '@/lib/format'
import type { BackupDestination } from '@/api/types'

const TYPE_ICONS: Record<BackupDestination['type'], React.ElementType> = {
  usb: HardDrive,
  s3: Upload,
  sftp: Server,
  nas: Server,
  cloud: Cloud,
}

export function DestinationsTab() {
  const [open, setOpen] = useState(false)
  const [type, setType] = useState<BackupDestinationInput['type']>('local')
  const [name, setName] = useState('')
  const [target, setTarget] = useState('')
  const [credentialA, setCredentialA] = useState('')
  const [credentialB, setCredentialB] = useState('')
  const [rcloneConfig, setRcloneConfig] = useState('')
  const [immutableDays, setImmutableDays] = useState(0)
  const [providerObjectLock, setProviderObjectLock] = useState(false)
  const { data: destinations } = useBackupDestinations()
  const save = useSaveBackupDestination()
  const restoreCheck = useBackupDestinationRestoreCheck()

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    let credentials: Record<string, string> | undefined
    if (type === 's3') credentials = { accessKey: credentialA, secretKey: credentialB }
    if (type === 'sftp') credentials = { username: credentialA, password: credentialB }
    if (type === 'rclone') credentials = { rcloneConfig }
    save.mutate(
      {
        name: name.trim(),
        type,
        target: target.trim(),
        enabled: true,
        retention: { generations: 20, daily: 30, monthly: 12, immutableDays, providerObjectLock },
        credentials,
      },
      {
        onSuccess: () => {
          setOpen(false)
          setName('')
          setTarget('')
          setCredentialA('')
          setCredentialB('')
          setRcloneConfig('')
          setImmutableDays(0)
          setProviderObjectLock(false)
        },
      },
    )
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-3">
        {(destinations ?? []).map((destination) => {
          const Icon = TYPE_ICONS[destination.type]
          return (
            <Card key={destination.id} className="flex flex-col">
              <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
                <CardTitle className="flex items-center gap-2 text-sm font-medium">
                  <Icon className="size-4 text-muted-foreground" />
                  {destination.label}
                </CardTitle>
                <HealthBadge state={destination.status} />
              </CardHeader>
              <CardContent className="flex flex-1 flex-col gap-2">
                {destination.target ? (
                  <p className="truncate font-mono text-xs text-muted-foreground">
                    {destination.target}
                  </p>
                ) : null}
                <p className="text-xs text-muted-foreground">{destination.detail}</p>
                {(destination.immutableDays ?? 0) > 0 && (
                  <span className="flex items-center gap-1 text-xs text-success">
                    <LockKeyhole className="size-3.5" />
                    LumoNAS prune protection: {destination.immutableDays} days
                  </span>
                )}
                {destination.providerObjectLock ? <span className="flex items-center gap-1 text-xs text-success"><LockKeyhole className="size-3.5" />S3 compliance lock: checked on each upload</span> : null}
                <div className="mt-auto flex flex-wrap items-center gap-2 pt-2">
                  {destination.encrypted && (
                    <span className="flex items-center gap-1 text-xs text-success">
                      <CheckCircle2 className="size-3.5" />
                      Encrypted
                    </span>
                  )}
                  {destination.lastVerifiedAt ? (
                    <span className="text-xs text-muted-foreground">
                      verified {timeAgo(destination.lastVerifiedAt)}
                    </span>
                  ) : (
                    <span className="text-xs text-muted-foreground">never verified</span>
                  )}
                </div>
                <Button size="sm" variant="outline" className="mt-1 self-start" disabled={restoreCheck.isPending || !destination.enabled} onClick={() => restoreCheck.mutate(destination.id)}>
                  <RotateCcw className="mr-1.5 size-3.5" />{restoreCheck.isPending ? 'Checking…' : 'Run restore check'}
                </Button>
                <p className="text-[11px] text-muted-foreground">Downloads and extracts into temporary storage; it does not start services.</p>
              </CardContent>
            </Card>
          )
        })}

        <Card className="flex flex-col items-center justify-center gap-2 border-dashed p-6 text-center">
          <Plus className="size-5 text-muted-foreground" />
          <p className="text-sm font-medium">Add destination</p>
          <p className="max-w-[28ch] text-xs text-muted-foreground">
            USB, another NAS, SFTP, S3, or Google Drive, Dropbox, OneDrive, Box, Backblaze B2, Azure Blob, and WebDAV through rclone.
          </p>
          <Button size="sm" variant="outline" className="mt-1" onClick={() => setOpen(true)}>
            Add destination
          </Button>
        </Card>
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
            <KeyRound className="size-4" />
            Recovery key
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <p className="flex items-center gap-2 text-sm">
              <CheckCircle2 className="size-4 text-success" />
              Configured and verified
            </p>
            <p className="mt-0.5 max-w-xl text-xs text-muted-foreground">
              The recovery key unlocks config backups without this NAS. Recovery never depends on
              a single device (TPM-only recovery is avoided on purpose). Store at least one copy
              away from the NAS.
            </p>
          </div>
          <p className="text-right text-xs text-muted-foreground">Exported during first-time setup</p>
        </CardContent>
      </Card>

      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Add backup destination</DialogTitle>
            <DialogDescription>Credentials are encrypted with the recovery key before storage.</DialogDescription>
          </DialogHeader>
          <form className="space-y-4" onSubmit={submit}>
            <div className="space-y-2">
              <Label htmlFor="backup-destination-type">Type</Label>
              <select id="backup-destination-type" value={type} onChange={(event) => setType(event.target.value as BackupDestinationInput['type'])} className="flex h-9 w-full rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground">
                <option value="local">Local / USB / NAS path</option>
                <option value="sftp">SFTP</option>
                <option value="s3">S3-compatible</option>
                <option value="rclone">Google Drive / Dropbox / OneDrive / other cloud</option>
              </select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="backup-destination-name">Name</Label>
              <Input id="backup-destination-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="Offsite backup" required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="backup-destination-target">Target</Label>
              <Input id="backup-destination-target" value={target} onChange={(event) => setTarget(event.target.value)} placeholder={type === 'local' ? '/mnt/backup' : type === 'sftp' ? 'sftp://host/path' : type === 'rclone' ? 'remote-name:backup-prefix' : 'https://s3.example/bucket'} required />
            </div>
            {type === 'rclone' ? <div className="space-y-2"><Label htmlFor="backup-rclone-config">rclone remote configuration</Label><textarea id="backup-rclone-config" value={rcloneConfig} onChange={(event) => setRcloneConfig(event.target.value)} rows={7} spellCheck={false} autoComplete="off" className="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs text-foreground" placeholder={'[remote-name]\ntype = drive\ntoken = {…}'} required /><p className="text-xs text-muted-foreground">Paste the matching remote block from your rclone config file. It is encrypted with the recovery key. Remote providers are allowlisted.</p></div> : null}
            {type !== 'local' && type !== 'rclone' ? (
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-2"><Label htmlFor="backup-credential-a">{type === 's3' ? 'Access key' : 'Username'}</Label><Input id="backup-credential-a" value={credentialA} onChange={(event) => setCredentialA(event.target.value)} autoComplete="off" /></div>
                <div className="space-y-2"><Label htmlFor="backup-credential-b">{type === 's3' ? 'Secret key' : 'Password'}</Label><Input id="backup-credential-b" type="password" value={credentialB} onChange={(event) => setCredentialB(event.target.value)} autoComplete="new-password" /></div>
              </div>
            ) : null}
            <div className="space-y-2 rounded-lg border bg-muted/30 p-3">
              <Label htmlFor="backup-immutable-days" className="flex items-center gap-2">
                <LockKeyhole className="size-3.5" /> Protect copies from LumoNAS pruning (days)
              </Label>
              <Input id="backup-immutable-days" type="number" min="0" max="3650" value={immutableDays} onChange={(event) => setImmutableDays(Math.max(0, Number(event.target.value) || 0))} />
              <p className="text-xs text-muted-foreground">LumoNAS keeps recent copies out of its own cleanup selection. This setting alone does not stop deletion at the storage provider.</p>
              <label className={`flex items-start gap-2 border-t pt-3 text-xs ${type === 's3' ? 'text-foreground' : 'text-muted-foreground'}`}><input type="checkbox" checked={providerObjectLock} disabled={type !== 's3'} onChange={(event) => setProviderObjectLock(event.target.checked)} className="mt-0.5" /><span><strong className="font-medium">Require S3 COMPLIANCE Object Lock</strong><span className="mt-1 block text-muted-foreground">Requires a bucket created with Object Lock. LumoNAS checks bucket support before upload and confirms the per-object retention after every copy. Requires permission to read bucket lock configuration and object retention.</span></span></label>
            </div>
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={save.isPending || !name.trim() || !target.trim() || (type === 'rclone' && !rcloneConfig.trim()) || (providerObjectLock && (type !== 's3' || immutableDays < 1))}>Save destination</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
