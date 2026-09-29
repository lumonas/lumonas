import { useState } from 'react'
import { Copy, KeyRound, Plus, Trash2 } from 'lucide-react'
import { useAPITokens, useCreateAPIToken, useDeleteAPIToken, useShares } from '@/api/queries'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { formatDateTime, timeAgo } from '@/lib/format'
import { useCurrentTime } from '@/hooks/useCurrentTime'

export function APITokensCard() {
  const { data: tokens } = useAPITokens()
  const { data: shares } = useShares()
  const create = useCreateAPIToken()
  const remove = useDeleteAPIToken()
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [backupWrite, setBackupWrite] = useState(false)
  const [replicationReceive, setReplicationReceive] = useState(false)
  const [snapshotReplicationReceive, setSnapshotReplicationReceive] = useState(false)
  const [snapshotReceiveShareId, setSnapshotReceiveShareId] = useState('')
  const [fleetStatus, setFleetStatus] = useState(false)
  const [workstationBackup, setWorkstationBackup] = useState(false)
  const [backupShareId, setBackupShareId] = useState('')
  const [expiryDays, setExpiryDays] = useState('90')
  const [createdToken, setCreatedToken] = useState<string | null>(null)
  const [createdScopes, setCreatedScopes] = useState('')
  const now = useCurrentTime()

  function createToken() {
    const expiresAt = expiryDays === 'never' ? undefined : new Date(Date.now() + Number(expiryDays) * 86_400_000).toISOString()
    const scopes = workstationBackup
      ? [`workstation:backup:${backupShareId}` as const]
      : fleetStatus ? ['fleet:status' as const]
        : snapshotReplicationReceive ? [`replication:snapshot:receive:${snapshotReceiveShareId}` as const]
          : replicationReceive ? ['replication:receive' as const]
          : backupWrite ? ['read' as const, 'backup:write' as const]
            : ['read' as const]
    create.mutate({ name: name.trim(), scopes, expiresAt }, {
      onSuccess: (value) => {
        setCreatedToken(value.token)
        setCreatedScopes(value.summary.scopes.join(' · '))
        setName('')
        setBackupWrite(false)
        setReplicationReceive(false)
        setSnapshotReplicationReceive(false)
        setSnapshotReceiveShareId('')
        setFleetStatus(false)
        setWorkstationBackup(false)
        setBackupShareId('')
        setExpiryDays('90')
      },
    })
  }

  function closeDialog(next: boolean) {
    setOpen(next)
    if (!next && !createdToken) {
      setName('')
      setBackupWrite(false)
      setReplicationReceive(false)
      setSnapshotReplicationReceive(false)
      setSnapshotReceiveShareId('')
      setFleetStatus(false)
      setWorkstationBackup(false)
      setBackupShareId('')
    }
  }

  return <Card>
    <CardHeader className="flex-row items-center justify-between space-y-0 pb-3">
      <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground"><KeyRound className="size-4" />API tokens</CardTitle>
      <Button size="sm" variant="outline" onClick={() => setOpen(true)}><Plus />Create token</Button>
    </CardHeader>
    <CardContent className="space-y-3">
      {(tokens?.length ?? 0) === 0 ? <p className="text-sm text-muted-foreground">No automation tokens. Tokens are scoped, never shown again, and can be revoked at any time.</p> : <ul className="divide-y rounded-lg border">{tokens?.map((token) => {
        const daysLeft = token.expiresAt ? Math.ceil((Date.parse(token.expiresAt) - now) / 86_400_000) : null
        return <li key={token.id} className="flex items-center justify-between gap-3 px-3 py-2.5"><div><p className="text-sm font-medium">{token.name}{daysLeft != null && daysLeft <= 14 ? <span className="ml-2 text-warning">{daysLeft <= 0 ? 'expired' : `expires in ${daysLeft}d`}</span> : null}</p><p className="text-xs text-muted-foreground">{token.scopes.join(' · ')} · created {timeAgo(token.createdAt)}{token.lastUsedAt ? ` · used ${timeAgo(token.lastUsedAt)}` : ' · never used'}{token.expiresAt ? ` · expires ${formatDateTime(token.expiresAt)}` : ' · no expiry'}</p></div><Button size="sm" variant="ghost" className="text-destructive hover:text-destructive" onClick={() => { if (window.confirm(`Revoke API token “${token.name}”? Any automation using it will stop working.`)) remove.mutate(token.id) }} disabled={remove.isPending}><Trash2 />Revoke</Button></li>
      })}</ul>}
      <Dialog open={open} onOpenChange={closeDialog}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{createdToken ? 'Copy this token now' : 'Create API token'}</DialogTitle>
            <DialogDescription>{createdToken ? 'For security it will not be displayed again.' : 'Choose the smallest access this token needs. Receive tokens can write snapshots only into one selected share.'}</DialogDescription>
          </DialogHeader>
          {createdToken ? <div className="space-y-3"><code className="block break-all rounded border bg-muted p-3 text-xs">{createdToken}</code><p className="text-xs text-muted-foreground">Permission: <code>{createdScopes}</code></p><p className="text-xs text-muted-foreground">Created {formatDateTime(new Date().toISOString())}</p></div> : <div className="space-y-4">
            <div className="space-y-2"><Label htmlFor="api-token-name">Name</Label><Input id="api-token-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="Home automation" /></div>
            <div className="space-y-2"><Label htmlFor="api-token-expiry">Expires after</Label><select id="api-token-expiry" className="h-9 w-full rounded-md border bg-background px-3 text-sm" value={expiryDays} onChange={(event) => setExpiryDays(event.target.value)}><option value="30">30 days</option><option value="90">90 days</option><option value="365">1 year</option><option value="never">Never</option></select></div>
            <label className="flex items-start gap-2 rounded-lg border p-3 text-sm"><input type="checkbox" checked={backupWrite} disabled={replicationReceive || snapshotReplicationReceive || fleetStatus || workstationBackup} onChange={(event) => setBackupWrite(event.target.checked)} className="mt-0.5" />Allow encrypted LumoNAS recovery backup automation</label>
            <label className="flex items-start gap-2 rounded-lg border p-3 text-sm"><input type="checkbox" checked={replicationReceive} disabled={backupWrite || fleetStatus || workstationBackup || snapshotReplicationReceive} onChange={(event) => setReplicationReceive(event.target.checked)} className="mt-0.5" />Receive encrypted LumoNAS recovery bundles</label>
            <label className="flex items-start gap-2 rounded-lg border p-3 text-sm"><input type="checkbox" checked={snapshotReplicationReceive} disabled={backupWrite || replicationReceive || fleetStatus || workstationBackup} onChange={(event) => setSnapshotReplicationReceive(event.target.checked)} className="mt-0.5" />Receive Btrfs snapshots into one share</label>
            {snapshotReplicationReceive ? <div className="space-y-2"><Label htmlFor="snapshot-receive-share">Destination share on this NAS</Label><select id="snapshot-receive-share" value={snapshotReceiveShareId} onChange={(event) => setSnapshotReceiveShareId(event.target.value)} className="h-9 w-full rounded-md border bg-background px-3 text-sm"><option value="">Choose a destination share</option>{(shares ?? []).map((share) => <option key={share.id} value={share.id}>{share.name}</option>)}</select><p className="text-xs text-muted-foreground">The token can receive Btrfs snapshots only into this share. Use a dedicated share for replicated snapshots.</p></div> : null}
            <label className="flex items-start gap-2 rounded-lg border p-3 text-sm"><input type="checkbox" checked={fleetStatus} disabled={backupWrite || replicationReceive || snapshotReplicationReceive || workstationBackup} onChange={(event) => setFleetStatus(event.target.checked)} className="mt-0.5" />Read this NAS's version and health in a paired fleet view only</label>
            <label className="flex items-start gap-2 rounded-lg border p-3 text-sm"><input type="checkbox" checked={workstationBackup} disabled={backupWrite || replicationReceive || snapshotReplicationReceive || fleetStatus} onChange={(event) => setWorkstationBackup(event.target.checked)} className="mt-0.5" />Back up and restore one workstation on a single share</label>
            {workstationBackup ? <div className="space-y-2"><Label htmlFor="workstation-backup-share">Dedicated backup share</Label><select id="workstation-backup-share" value={backupShareId} onChange={(event) => setBackupShareId(event.target.value)} className="h-9 w-full rounded-md border bg-background px-3 text-sm"><option value="">Choose a private share</option>{(shares ?? []).map((share) => <option key={share.id} value={share.id}>{share.name}</option>)}</select><p className="text-xs text-muted-foreground">The token can upload and download files only on this share. Create a private share for workstation backup archives.</p></div> : null}
          </div>}
          <DialogFooter>{createdToken ? <><Button variant="outline" onClick={() => navigator.clipboard.writeText(createdToken)}><Copy />Copy token</Button><Button onClick={() => { setCreatedToken(null); setCreatedScopes(''); setOpen(false) }}>Done</Button></> : <><Button variant="ghost" onClick={() => setOpen(false)}>Cancel</Button><Button disabled={!name.trim() || create.isPending || (workstationBackup && !backupShareId) || (snapshotReplicationReceive && !snapshotReceiveShareId)} onClick={createToken}>Create token</Button></>}</DialogFooter>
        </DialogContent>
      </Dialog>
    </CardContent>
  </Card>
}
