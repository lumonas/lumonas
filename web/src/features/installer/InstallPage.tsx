import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { HardDrive, ShieldAlert } from 'lucide-react'
import { apiGet, apiPost } from '@/api/client'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { formatBytes } from '@/lib/format'
import type { InstallPlanResponse, InstallStatus, InstallTarget } from '@/api/types'

function reviewTarget(target: InstallTarget) {
  return (
    <div key={target.diskId} className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">
          {target.model} <span className="ml-2 font-mono text-xs text-muted-foreground">{target.name}</span>
        </p>
        <p className="text-xs text-muted-foreground">
          {formatBytes(target.sizeBytes)}
          {target.filesystem ? ` · existing ${target.filesystem}` : ''}
        </p>
      </div>
      {target.eligible ? (
        <Badge variant="success">Eligible</Badge>
      ) : (
        <Badge variant="warning" className="shrink-0">
          <ShieldAlert className="mr-1 size-3" />
          {target.protectedBy?.[0] ?? 'protected'}
        </Badge>
      )}
    </div>
  )
}

export function InstallPage() {
  const { data: targets } = useQuery({
    queryKey: ['install', 'targets'],
    queryFn: () => apiGet<InstallTarget[]>('/install/targets'),
  })
  const [targetDiskId, setTargetDiskId] = useState('')
  const [hostname, setHostname] = useState('lumonas')
  const [adminUsername, setAdminUsername] = useState('admin')
  const [filesystem, setFilesystem] = useState<'ext4' | 'xfs'>('ext4')
  const [password, setPassword] = useState('')
  const [plan, setPlan] = useState<InstallPlanResponse | null>(null)
  const status = useQuery({
    queryKey: ['install', 'status'],
    queryFn: () => apiGet<InstallStatus>('/install/status'),
    refetchInterval: plan ? 5_000 : false,
  })
  const planMutation = useMutation({
    mutationFn: () =>
      apiPost<InstallPlanResponse>('/install/plan', {
        targetDiskId,
        hostname,
        adminUsername,
        filesystem,
        uefi: true,
      }),
    onSuccess: setPlan,
  })
  const applyMutation = useMutation({
    mutationFn: () =>
      apiPost<{ status: string }>('/install/apply', {
        hash: plan?.hash ?? '',
        confirm: true,
        adminPassword: password,
      }),
    onSuccess: () => void status.refetch(),
  })

  const stage = status.data?.stage ?? 'idle'

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4 p-6">
      <div>
        <h1 className="text-xl font-semibold">Install LumoNAS</h1>
        <p className="text-sm text-muted-foreground">
          Installs the appliance onto a blank disk. Existing data disks are listed but protected —
          nothing is written until you confirm the exact plan.
        </p>
      </div>

      <Card>
        <CardHeader className="pb-4">
          <CardTitle className="text-sm font-medium text-muted-foreground">1. Choose a disk</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {(targets ?? []).map((target) => (
            <button
              key={target.diskId}
              type="button"
              disabled={!target.eligible}
              onClick={() => setTargetDiskId(target.diskId)}
              className={`flex items-center gap-3 rounded-lg border px-3 py-2 text-left disabled:opacity-60 ${
                targetDiskId === target.diskId ? 'border-primary bg-primary/5' : ''
              }`}
            >
              <HardDrive className="size-4 shrink-0 text-muted-foreground" />
              <span className="min-w-0 flex-1">{reviewTarget(target)}</span>
            </button>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-4">
          <CardTitle className="text-sm font-medium text-muted-foreground">2. System identity</CardTitle>
        </CardHeader>
        <CardContent className="grid gap-3 sm:grid-cols-2">
          <div className="grid gap-1.5">
            <Label htmlFor="install-hostname">Hostname</Label>
            <Input id="install-hostname" value={hostname} onChange={(event) => setHostname(event.target.value)} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="install-admin">Administrator name</Label>
            <Input id="install-admin" value={adminUsername} onChange={(event) => setAdminUsername(event.target.value)} />
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="install-filesystem">Root filesystem</Label>
            <select
              id="install-filesystem"
              value={filesystem}
              onChange={(event) => setFilesystem(event.target.value as 'ext4' | 'xfs')}
              className="h-9 rounded-md border border-input bg-background px-3 text-sm"
            >
              <option value="ext4">ext4</option>
              <option value="xfs">xfs</option>
            </select>
          </div>
          <div className="grid gap-1.5">
            <Label htmlFor="install-password">Administrator password</Label>
            <Input
              id="install-password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="at least 12 characters"
            />
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-4">
          <CardTitle className="text-sm font-medium text-muted-foreground">3. Review and install</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          {plan ? (
            <div className="rounded-lg border bg-muted/20 px-3 py-2 text-sm">
              Plan <span className="font-mono text-xs">{plan.plan.id}</span> targets{' '}
              <span className="font-mono text-xs">{plan.plan.targetDiskId}</span> · hostname{' '}
              <span className="font-mono text-xs">{plan.plan.hostname}</span>. The plan expires at{' '}
              {new Date(plan.plan.expiresAt).toLocaleTimeString()}.
            </div>
          ) : (
            <Button disabled={!targetDiskId || planMutation.isPending} onClick={() => planMutation.mutate()}>
              {planMutation.isPending ? 'Generating plan…' : 'Generate installation plan'}
            </Button>
          )}
          {plan ? (
            <Button
              variant="destructive"
              disabled={applyMutation.isPending || stage === 'applying'}
              onClick={() => applyMutation.mutate()}
            >
              {stage === 'applying' ? 'Installing… this can take several minutes' : 'Erase target disk and install'}
            </Button>
          ) : null}
          {applyMutation.isError ? (
            <p className="text-sm text-destructive" role="alert">
              {String(applyMutation.error)}
            </p>
          ) : null}
          {stage === 'succeeded' ? (
            <p className="text-sm text-success" role="status">
              Installation succeeded. Reboot and remove the installer medium.
            </p>
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}
