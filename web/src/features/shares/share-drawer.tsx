import { useState } from 'react'
import { Archive } from 'lucide-react'
import { useDeleteShare, usePrincipals, useShare, useShareAccess, useShareProtocol, useShareSettings } from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { DangerZone } from '@/components/core/danger-zone'
import { HealthBadge } from '@/components/core/health-badge'
import { ResourceDrawer } from '@/components/core/resource-drawer'
import { Badge } from '@/components/ui/badge'
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
import type { AccessLevel, Share, ShareProtocolType } from '@/api/types'

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
              <details className="mt-3">
                <summary className="cursor-pointer text-xs text-muted-foreground select-none">
                  Advanced SMB options
                </summary>
                <p className="mt-2 text-xs text-muted-foreground">
                  Veto files, socket options and auxiliary parameters are preserved as-is and can
                  be edited by experts.
                </p>
              </details>
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
