import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Clipboard, FolderOpen, Laptop, Plus, ShieldCheck } from 'lucide-react'
import { useDisconnectShareClient, useShareAccessPreview, useShareClients, useSharePathAccessCheck, useShares } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { EmptyState } from '@/components/core/empty-state'
import { PageHeader } from '@/components/core/page-header'
import { ResourceTable, type Column } from '@/components/core/resource-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { ShareDrawer } from '@/features/shares/share-drawer'
import { CreateShareWizard } from '@/features/shares/create-wizard'
import { formatBytes } from '@/lib/format'
import type { Share } from '@/api/types'

const PROTOCOL_ORDER = ['smb', 'nfs', 'sftp', 'rsync', 'timemachine'] as const

export function SharesPage() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const { data: shares, isLoading } = useShares()
  const clients = useShareClients()
  const disconnectClient = useDisconnectShareClient()
  const [wizardOpen, setWizardOpen] = useState(searchParams.get('create') === '1')
  const [previewShareId, setPreviewShareId] = useState<string | null>(null)
  const [checkPrincipalId, setCheckPrincipalId] = useState('')
  const [checkPath, setCheckPath] = useState('.')
  const [checkSubmitted, setCheckSubmitted] = useState(false)
  const [backupPlatform, setBackupPlatform] = useState<'windows' | 'macos'>('windows')
  const [copiedTarget, setCopiedTarget] = useState(false)
  const { data: accessPreview, isLoading: previewLoading } = useShareAccessPreview(previewShareId)
  const pathCheck = useSharePathAccessCheck(previewShareId, checkPrincipalId, checkPath, checkSubmitted)

  const shareId = searchParams.get('share')
  const backupShares = (shares ?? []).filter((share) =>
    share.protocols.some((protocol) => protocol.enabled && (
      backupPlatform === 'windows'
        ? protocol.protocol === 'smb'
        : protocol.protocol === 'timemachine' || protocol.protocol === 'smb'
    )),
  )
  const backupShare = backupPlatform === 'macos'
    ? backupShares.find((share) => share.protocols.some((protocol) => protocol.enabled && protocol.protocol === 'timemachine')) ?? backupShares[0]
    : backupShares[0]
  const backupTarget = backupShare
    ? backupPlatform === 'windows'
      ? `\\\\${window.location.hostname}\\${backupShare.name}`
      : `smb://${window.location.hostname}/${encodeURIComponent(backupShare.name)}`
    : ''

  function setParam(key: string, value: string | null) {
    const next = new URLSearchParams(searchParams)
    if (value == null) next.delete(key)
    else next.set(key, value)
    setSearchParams(next, { replace: true })
  }

  const columns: Column<Share>[] = [
    {
      id: 'name',
      header: 'Share',
      sortValue: (s) => s.name,
      searchValue: (s) => `${s.name} ${s.description ?? ''} ${s.resourceLabel} ${s.relativePath}`,
      cell: (s) => (
        <div className="flex flex-col">
          <span className="text-[13px] font-medium">{s.name}</span>
          {s.description ? (
            <span className="truncate text-xs text-muted-foreground">{s.description}</span>
          ) : null}
        </div>
      ),
    },
    {
      id: 'location',
      header: 'Location',
      sortValue: (s) => `${s.resourceLabel}${s.relativePath}`,
      cell: (s) => (
        <div className="flex flex-col">
          <span className="text-[13px]">{s.resourceLabel}</span>
          <span className="font-mono text-xs text-muted-foreground">{s.relativePath}</span>
        </div>
      ),
    },
    {
      id: 'protocols',
      header: 'Protocols',
      cell: (s) => (
        <div className="flex flex-wrap gap-1">
          {[...s.protocols]
            .filter((p) => p.enabled)
            .sort((a, b) => PROTOCOL_ORDER.indexOf(a.protocol) - PROTOCOL_ORDER.indexOf(b.protocol))
            .map((p) => (
              <Badge key={p.protocol} variant="secondary" className="uppercase">
                {p.protocol === 'timemachine' ? 'TM' : p.protocol}
              </Badge>
            ))}
        </div>
      ),
    },
    {
      id: 'access',
      header: 'Access',
      cell: (s) => (
        <Button size="sm" variant="ghost" className="h-7 px-2 text-xs" onClick={(event) => { event.stopPropagation(); setPreviewShareId(s.id) }}>
          <ShieldCheck />{s.access.filter((a) => a.level !== 'none').length} rules · Preview
        </Button>
      ),
    },
    {
      id: 'used',
      header: 'Used',
      advanced: true,
      className: 'tnum',
      sortValue: (s) => s.usedBytes ?? 0,
      cell: (s) => (s.usedBytes != null ? formatBytes(s.usedBytes) : '—'),
    },
    {
      id: 'status',
      header: 'Status',
      sortValue: (s) => s.status,
      cell: (s) => <HealthBadge state={s.status} />,
    },
  ]

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Shares"
        description="One share can expose one location over multiple protocols."
        actions={
          <Button size="sm" onClick={() => setWizardOpen(true)}>
            <Plus />
            Create share
          </Button>
        }
      />
      <Card>
        <CardHeader className="flex-row items-start justify-between gap-3 pb-3">
          <div>
            <CardTitle className="flex items-center gap-2 text-sm"><Laptop className="size-4" />Set up a computer backup</CardTitle>
            <CardDescription className="mt-1">Use a network share as the destination for Windows File History or macOS Time Machine.</CardDescription>
          </div>
          <div className="flex gap-1 rounded-md border p-1">
            <Button size="sm" variant={backupPlatform === 'windows' ? 'secondary' : 'ghost'} onClick={() => { setBackupPlatform('windows'); setCopiedTarget(false) }}>Windows</Button>
            <Button size="sm" variant={backupPlatform === 'macos' ? 'secondary' : 'ghost'} onClick={() => { setBackupPlatform('macos'); setCopiedTarget(false) }}>macOS</Button>
          </div>
        </CardHeader>
        <CardContent className="grid gap-3 md:grid-cols-[minmax(0,1fr)_auto] md:items-center">
          {backupShare ? (
            <div className="min-w-0">
              <p className="text-sm font-medium">NAS destination ready: {backupShare.name}</p>
              <p className="mt-1 truncate font-mono text-xs text-muted-foreground">{backupTarget}</p>
              <p className="mt-2 text-xs text-muted-foreground">
                {backupPlatform === 'windows'
                  ? 'On the PC, open Control Panel → File History → Select drive → Add network location, then enter this path.'
                  : backupShare.protocols.some((protocol) => protocol.enabled && protocol.protocol === 'timemachine')
                    ? 'On the Mac, open System Settings → General → Time Machine → Add Backup Disk and choose this NAS share.'
                    : 'This SMB share can store Mac files, but it is not configured as a Time Machine destination. Enable Time Machine on a share first.'}
              </p>
              <p className="mt-1 text-xs text-muted-foreground">NAS readiness only: LumoNAS cannot confirm a client backup completed until the client reports it.</p>
            </div>
          ) : (
            <div>
              <p className="text-sm font-medium">No compatible backup share is ready</p>
              <p className="mt-1 text-xs text-muted-foreground">Create an SMB share for Windows, or a Time Machine share for macOS.</p>
            </div>
          )}
          <div className="flex flex-wrap gap-2">
            {backupTarget ? <Button size="sm" variant="outline" onClick={() => { void navigator.clipboard?.writeText(backupTarget).then(() => setCopiedTarget(true)).catch(() => setCopiedTarget(false)) }}><Clipboard />{copiedTarget ? 'Copied' : 'Copy destination'}</Button> : null}
            {!backupShare ? <Button size="sm" onClick={() => setWizardOpen(true)}><Plus />Create share</Button> : null}
          </div>
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="pb-3"><CardTitle className="text-sm">Active SMB clients</CardTitle><CardDescription>Live Samba sessions and open files. Refreshes every 15 seconds.</CardDescription></CardHeader>
        <CardContent>
          {clients.isLoading ? <p className="text-sm text-muted-foreground">Reading Samba status…</p> : clients.isError ? <p role="alert" className="text-sm text-destructive">Samba client status is unavailable. Check that the Samba service is running and its status interface is accessible.</p> : clients.data?.sessions.length ? <div className="grid gap-4 lg:grid-cols-2">
            <div><p className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Sessions ({clients.data.sessions.length})</p><ul className="divide-y rounded-md border">{clients.data.sessions.map((session) => <li key={session.sessionId} className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"><div><p className="text-sm font-medium">{session.username}</p><p className="text-xs text-muted-foreground">{session.machine}{session.share ? ` · ${session.share}` : ''}</p></div><div className="flex items-center gap-2"><Badge variant="secondary">{session.dialect || 'SMB'}</Badge><Button size="sm" variant="ghost" className="text-destructive hover:text-destructive" disabled={disconnectClient.isPending} title="Disconnect every SMB session from this IP" onClick={() => { if (window.confirm(`Disconnect every SMB session from ${session.machine}? Open file operations will be interrupted.`)) disconnectClient.mutate(session.machine) }}>Disconnect</Button></div></li>)}</ul></div>
            <div><p className="mb-2 text-xs font-medium uppercase tracking-wide text-muted-foreground">Open files ({clients.data.openFiles.length})</p>{clients.data.openFiles.length ? <ul className="max-h-64 divide-y overflow-y-auto rounded-md border">{clients.data.openFiles.map((file, index) => <li key={`${file.path}-${index}`} className="px-3 py-2"><p className="break-all font-mono text-xs">{file.path}</p><p className="text-xs text-muted-foreground">{file.sharePath || 'Share path unavailable'} · {file.opens} open{file.opens === 1 ? '' : 's'}</p></li>)}</ul> : <p className="rounded-md border px-3 py-4 text-sm text-muted-foreground">No files are open.</p>}</div>
          </div> : <p className="text-sm text-muted-foreground">No SMB clients are connected.</p>}
        </CardContent>
      </Card>
      <ResourceTable
        columns={columns}
        rows={shares ?? []}
        loading={isLoading}
        onRowClick={(share) => navigate(`/shares?share=${share.id}`)}
        emptyState={
          <EmptyState
            icon={<FolderOpen />}
            title="No shares"
            description="Create your first share to make storage available on the network."
            className="border-0"
          />
        }
      />

      <ShareDrawer
        shareId={shareId}
        onOpenChange={(open) => setParam('share', open ? shareId : null)}
      />
      <CreateShareWizard
        open={wizardOpen}
        onOpenChange={setWizardOpen}
      />
      <Dialog open={!!previewShareId} onOpenChange={(open) => { if (!open) { setPreviewShareId(null); setCheckSubmitted(false); setCheckPath('.') } }}>
        <DialogContent className="max-w-xl">
          <DialogHeader>
            <DialogTitle>Effective share access</DialogTitle>
            <DialogDescription>{accessPreview?.name ?? 'Review direct, group, and guest access for this share.'}</DialogDescription>
          </DialogHeader>
          {previewLoading ? <p className="text-sm text-muted-foreground">Loading access rules…</p> : accessPreview?.entries.length ? (
            <div className="max-h-[60vh] space-y-2 overflow-y-auto">
              {accessPreview.entries.map((entry) => (
                <div key={entry.principalId} className="flex items-start justify-between gap-3 rounded-md border px-3 py-2">
                  <div className="min-w-0">
                    <p className="truncate text-sm font-medium">{entry.name}</p>
                    <p className="text-xs text-muted-foreground">{entry.grantedBy?.join(', ') || 'No matching share rule'}{!entry.enabled ? ' · account disabled' : ''}</p>
                  </div>
                  <Badge variant={entry.level === 'write' ? 'warning' : entry.level === 'read' ? 'secondary' : 'outline'}>{entry.level}</Badge>
                </div>
              ))}
            </div>
          ) : <p className="text-sm text-muted-foreground">No principals currently have access.</p>}
          <form className="space-y-3 border-t pt-4" onSubmit={(event) => { event.preventDefault(); setCheckSubmitted(true) }}>
            <div>
              <p className="text-sm font-medium">Check a real path</p>
              <p className="text-xs text-muted-foreground">Combines the share rule with Unix mode bits. SMB/NFS ACL modules may change the final result.</p>
            </div>
            <div className="grid gap-2 sm:grid-cols-[1fr_1fr_auto]">
              <select aria-label="User to check" value={checkPrincipalId} onChange={(event) => { setCheckPrincipalId(event.target.value); setCheckSubmitted(false) }} required className="h-9 min-w-0 rounded-md border bg-background px-2 text-sm text-foreground">
                <option value="">Select user</option>
                {(accessPreview?.entries ?? []).filter((entry) => entry.kind === 'user').map((entry) => <option key={entry.principalId} value={entry.principalId}>{entry.name}</option>)}
              </select>
              <input aria-label="Path inside share" value={checkPath} onChange={(event) => { setCheckPath(event.target.value); setCheckSubmitted(false) }} className="h-9 min-w-0 rounded-md border bg-background px-2 text-sm text-foreground" placeholder="folder/report.pdf" />
              <Button type="submit" size="sm" variant="outline" disabled={!checkPrincipalId || pathCheck.isFetching}>Check access</Button>
            </div>
            {pathCheck.isError ? <p role="alert" className="text-xs text-destructive">{pathCheck.error instanceof Error ? pathCheck.error.message : 'Could not check path access.'}</p> : null}
            {pathCheck.data ? <div role="status" className="rounded-md border p-3">
              <div className="flex items-center justify-between gap-2"><p className="text-sm font-medium">{pathCheck.data.allowed ? 'Access appears allowed' : 'Access appears blocked'}</p><Badge variant={pathCheck.data.allowed ? 'success' : 'destructive'}>{pathCheck.data.shareLevel} share · {pathCheck.data.filesystemAccess} filesystem</Badge></div>
              <p className="mt-1 text-xs text-muted-foreground">{pathCheck.data.explanation}</p>
              <p className="mt-1 text-[11px] text-muted-foreground">Approximate check; verify with an SMB/NFS client for protocol ACLs.</p>
            </div> : null}
          </form>
        </DialogContent>
      </Dialog>
    </div>
  )
}
