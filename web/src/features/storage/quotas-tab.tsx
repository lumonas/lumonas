import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiGet, apiPut } from '@/api/client'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { formatBytes } from '@/lib/format'
import type { Principal, Share } from '@/api/types'
import { toast } from 'sonner'

type Policy = { id: string; targetType: 'share' | 'user' | 'group'; targetId: string; limitBytes: number; warningPercent: number }
type Status = { policy: Policy; usedBytes: number; percent: number; state: 'healthy' | 'warning' | 'over'; measuredAt: string }
const bytesPerGiB = 1024 ** 3

export function QuotasTab() {
  const client = useQueryClient()
  const [targetType, setTargetType] = useState<Policy['targetType']>('share')
  const [targetId, setTargetId] = useState('')
  const [limitGiB, setLimitGiB] = useState('100')
  const [warningPercent, setWarningPercent] = useState('85')
  const quotas = useQuery({ queryKey: ['quotas'], queryFn: () => apiGet<Status[]>('/quotas') })
  const shares = useQuery({ queryKey: ['shares'], queryFn: () => apiGet<Share[]>('/shares') })
  const principals = useQuery({ queryKey: ['principals'], queryFn: () => apiGet<Principal[]>('/principals') })
  const save = useMutation({
    mutationFn: async (next: Policy[]) => apiPut<Policy[]>('/quotas', next),
    onSuccess: async () => { toast.success('Quota settings saved'); await client.invalidateQueries({ queryKey: ['quotas'] }) },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not save quota settings'),
  })
  const targets = targetType === 'share' ? (shares.data ?? []).map((item) => ({ id: item.id, name: item.name })) : (principals.data ?? []).filter((item) => item.type === targetType).map((item) => ({ id: item.id, name: item.name }))
  const usedTargets = new Set((quotas.data ?? []).map((item) => `${item.policy.targetType}:${item.policy.targetId}`))
  function addQuota(event: React.FormEvent) {
    event.preventDefault()
    const limit = Number(limitGiB)
    if (!Number.isFinite(limit) || limit <= 0 || !targetId) return
    const existing = (quotas.data ?? []).map((item) => item.policy)
    save.mutate([...existing, { id: '', targetType, targetId, limitBytes: Math.round(limit * bytesPerGiB), warningPercent: Number(warningPercent) }])
    setTargetId('')
  }
  function removeQuota(policy: Policy) {
    save.mutate((quotas.data ?? []).map((item) => item.policy).filter((item) => item.id !== policy.id))
  }
  return <div className="flex flex-col gap-4">
    <Card><CardHeader><CardTitle className="text-sm">Storage quotas and usage alerts</CardTitle><CardDescription>Track space used by a managed share, user, or group. LumoNAS warns at your threshold and when the configured limit is reached.</CardDescription></CardHeader><CardContent>
      <form className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5" onSubmit={addQuota}>
        <label className="text-xs text-muted-foreground">Applies to<select aria-label="Quota target type" value={targetType} onChange={(event) => { setTargetType(event.target.value as Policy['targetType']); setTargetId('') }} className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground"><option value="share">Share</option><option value="user">User</option><option value="group">Group</option></select></label>
        <label className="text-xs text-muted-foreground">Target<select aria-label="Quota target" value={targetId} onChange={(event) => setTargetId(event.target.value)} required className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground"><option value="">Select target</option>{targets.map((item) => <option key={item.id} value={item.id} disabled={usedTargets.has(`${targetType}:${item.id}`)}>{item.name}</option>)}</select></label>
        <label className="text-xs text-muted-foreground">Limit in GiB<input aria-label="Quota limit GiB" type="number" min="0.001" step="0.001" value={limitGiB} onChange={(event) => setLimitGiB(event.target.value)} className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground" /></label>
        <label className="text-xs text-muted-foreground">Warn at %<input aria-label="Quota warning percent" type="number" min="50" max="99" value={warningPercent} onChange={(event) => setWarningPercent(event.target.value)} className="mt-1 h-9 w-full rounded-md border bg-background px-2 text-sm text-foreground" /></label>
        <div className="flex items-end"><Button type="submit" disabled={save.isPending || !targetId}>Add quota</Button></div>
      </form>
    </CardContent></Card>
    <Card><CardHeader><CardTitle className="text-sm">Usage</CardTitle><CardDescription>Usage is measured from files stored in managed shares. Symbolic links are not followed.</CardDescription></CardHeader><CardContent>
      {quotas.isLoading ? <p className="py-4 text-sm text-muted-foreground">Measuring storage usage…</p> : quotas.isError ? <p role="alert" className="py-4 text-sm text-destructive">Could not measure quota usage.</p> : quotas.data?.length ? <div className="space-y-3">{quotas.data.map((item) => { const name = item.policy.targetType === 'share' ? shares.data?.find((share) => share.id === item.policy.targetId)?.name : principals.data?.find((principal) => principal.id === item.policy.targetId)?.name; return <div key={item.policy.id} className="rounded-md border p-3"><div className="flex flex-wrap items-center justify-between gap-2"><div><p className="text-sm font-medium">{name ?? 'Unknown target'} <span className="text-xs capitalize text-muted-foreground">· {item.policy.targetType}</span></p><p className="text-xs text-muted-foreground">{formatBytes(item.usedBytes)} of {formatBytes(item.policy.limitBytes)} · {Math.round(item.percent)}%</p></div><div className="flex items-center gap-2"><Badge variant={item.state === 'over' ? 'destructive' : item.state === 'warning' ? 'warning' : 'secondary'}>{item.state === 'over' ? 'Limit reached' : item.state === 'warning' ? 'Near limit' : 'Healthy'}</Badge><Button size="sm" variant="ghost" onClick={() => removeQuota(item.policy)} disabled={save.isPending}>Remove</Button></div></div><div className="mt-2 h-2 overflow-hidden rounded bg-muted"><div className={`h-full ${item.state === 'over' ? 'bg-destructive' : item.state === 'warning' ? 'bg-amber-500' : 'bg-primary'}`} style={{ width: `${Math.min(100, item.percent)}%` }} /></div></div>})}</div> : <p className="py-4 text-center text-sm text-muted-foreground">No quota alerts configured.</p>}
    </CardContent></Card>
  </div>
}
