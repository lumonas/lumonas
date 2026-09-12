import { useEffect, useState } from 'react'
import { CheckCircle2, Download, RotateCcw, ShieldCheck } from 'lucide-react'
import { toast } from 'sonner'
import { apiGet, apiPost } from '@/api/client'
import { PageHeader } from '@/components/core/page-header'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

interface UpdateState {
  activeSlot: string
  previousSlot?: string
  pendingSlot?: string
  activeVersion?: string
  pendingVersion?: string
  bootAttempts: number
  lastError?: string
  updatedAt: string
}

interface ManifestForm {
  version: string
  packageSha256: string
  packageSize: string
  packagePath: string
  signature: string
  backupPath: string
}

const initialForm: ManifestForm = {
  version: '',
  packageSha256: '',
  packageSize: '',
  packagePath: '',
  signature: '',
  backupPath: '',
}

export function UpdatesPage() {
  const [state, setState] = useState<UpdateState | null>(null)
  const [form, setForm] = useState(initialForm)
  const [busy, setBusy] = useState(false)

  const refresh = () => apiGet<UpdateState>('/updates/status').then(setState)
  useEffect(() => {
    void refresh()
  }, [])

  const update = (key: keyof ManifestForm, value: string) => setForm((current) => ({ ...current, [key]: value }))

  async function apply() {
    setBusy(true)
    try {
      await apiPost('/updates/apply', {
        manifest: {
          formatVersion: 1,
          version: form.version,
          packageSha256: form.packageSha256,
          packageSize: Number(form.packageSize),
          publishedAt: new Date().toISOString(),
        },
        signature: form.signature,
        packagePath: form.packagePath,
        backupPath: form.backupPath || undefined,
      })
      toast.success('Update staged in the inactive slot')
      await refresh()
    } catch {
      toast.error('Update was not staged')
    } finally {
      setBusy(false)
    }
  }

  async function confirmHealth() {
    setBusy(true)
    try {
      await apiPost('/updates/health', { healthy: true, version: state?.pendingVersion })
      toast.success('Update marked healthy')
      await refresh()
    } catch {
      toast.error('Health confirmation failed')
    } finally {
      setBusy(false)
    }
  }

  async function rollback() {
    setBusy(true)
    try {
      await apiPost('/updates/rollback', { reason: 'manual rollback from administration UI' })
      toast.success('Rolled back to the previous slot')
      await refresh()
    } catch {
      toast.error('Rollback is not available')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title="System updates" description="Install signed packages into an inactive A/B slot and confirm health before making it active." />
      <div className="grid gap-6 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><ShieldCheck className="size-4 text-primary" />Slot status</CardTitle>
            <CardDescription>Only verified packages can be staged. A recovery bundle is required before activation.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3 text-sm">
            <div className="grid grid-cols-2 gap-3 rounded-lg border p-3">
              <div><div className="text-muted-foreground">Active slot</div><div className="font-medium">{state?.activeSlot ?? '—'} {state?.activeVersion ? `· ${state.activeVersion}` : ''}</div></div>
              <div><div className="text-muted-foreground">Pending slot</div><div className="font-medium">{state?.pendingSlot ? `${state.pendingSlot} · ${state.pendingVersion}` : 'None'}</div></div>
            </div>
            {state?.lastError ? <p className="text-destructive">Last rollback: {state.lastError}</p> : null}
            <div className="flex flex-wrap gap-2">
              <Button onClick={() => void refresh()} variant="outline">Refresh</Button>
              {state?.pendingSlot ? <Button onClick={() => void confirmHealth()} disabled={busy}><CheckCircle2 />Confirm healthy</Button> : null}
              {state?.previousSlot ? <Button onClick={() => void rollback()} disabled={busy} variant="destructiveOutline"><RotateCcw />Rollback</Button> : null}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><Download className="size-4 text-primary" />Stage signed package</CardTitle>
            <CardDescription>Package paths are local to the NAS update worker. The public verification key is managed server-side.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {(['version', 'packageSha256', 'packageSize', 'packagePath', 'signature', 'backupPath'] as const).map((key) => (
              <div key={key} className="space-y-1">
                <Label htmlFor={`update-${key}`}>{key === 'packageSha256' ? 'Package SHA-256' : key === 'backupPath' ? 'Recovery bundle path (optional)' : key}</Label>
                <Input id={`update-${key}`} value={form[key]} onChange={(event) => update(key, event.target.value)} placeholder={key === 'signature' ? 'base64 Ed25519 signature' : undefined} />
              </div>
            ))}
            <Button className="w-full" disabled={busy || !form.version || !form.packagePath || !form.signature} onClick={() => void apply()}>Stage update</Button>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
