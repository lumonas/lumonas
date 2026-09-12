import { useState, type FormEvent } from 'react'
import { CheckCircle2, HardDrive, KeyRound, Plus, Server, Upload } from 'lucide-react'
import { useBackupDestinations, useSaveBackupDestination, type BackupDestinationInput } from '@/api/queries'
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
}

export function DestinationsTab() {
  const [open, setOpen] = useState(false)
  const [type, setType] = useState<BackupDestinationInput['type']>('local')
  const [name, setName] = useState('')
  const [target, setTarget] = useState('')
  const [credentialA, setCredentialA] = useState('')
  const [credentialB, setCredentialB] = useState('')
  const { data: destinations } = useBackupDestinations()
  const save = useSaveBackupDestination()

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    let credentials: Record<string, string> | undefined
    if (type === 's3') credentials = { accessKey: credentialA, secretKey: credentialB }
    if (type === 'sftp') credentials = { username: credentialA, password: credentialB }
    save.mutate(
      {
        name: name.trim(),
        type,
        target: target.trim(),
        enabled: true,
        retention: { generations: 20, daily: 30, monthly: 12 },
        credentials,
      },
      {
        onSuccess: () => {
          setOpen(false)
          setName('')
          setTarget('')
          setCredentialA('')
          setCredentialB('')
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
              </CardContent>
            </Card>
          )
        })}

        <Card className="flex flex-col items-center justify-center gap-2 border-dashed p-6 text-center">
          <Plus className="size-5 text-muted-foreground" />
          <p className="text-sm font-medium">Add destination</p>
          <p className="max-w-[28ch] text-xs text-muted-foreground">
            USB, another NAS, SFTP or S3 — cloud is optional, never required.
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
              </select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="backup-destination-name">Name</Label>
              <Input id="backup-destination-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="Offsite backup" required />
            </div>
            <div className="space-y-2">
              <Label htmlFor="backup-destination-target">Target</Label>
              <Input id="backup-destination-target" value={target} onChange={(event) => setTarget(event.target.value)} placeholder={type === 'local' ? '/mnt/backup' : type === 'sftp' ? 'sftp://host/path' : 'https://s3.example/bucket'} required />
            </div>
            {type !== 'local' ? (
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-2"><Label htmlFor="backup-credential-a">{type === 's3' ? 'Access key' : 'Username'}</Label><Input id="backup-credential-a" value={credentialA} onChange={(event) => setCredentialA(event.target.value)} autoComplete="off" /></div>
                <div className="space-y-2"><Label htmlFor="backup-credential-b">{type === 's3' ? 'Secret key' : 'Password'}</Label><Input id="backup-credential-b" type="password" value={credentialB} onChange={(event) => setCredentialB(event.target.value)} autoComplete="new-password" /></div>
              </div>
            ) : null}
            <DialogFooter>
              <Button type="button" variant="ghost" onClick={() => setOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={save.isPending || !name.trim() || !target.trim()}>Save destination</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
