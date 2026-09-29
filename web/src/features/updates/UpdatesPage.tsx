import { useEffect, useState } from 'react'
import { CheckCircle2, Download, HardDrive, RotateCcw, ShieldCheck } from 'lucide-react'
import { toast } from 'sonner'
import { apiGet, apiPost } from '@/api/client'
import { PageHeader } from '@/components/core/page-header'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Badge } from '@/components/ui/badge'
import { useRecoveryStatus, useRestoreDrills } from '@/api/queries'

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

interface ImageSlotForm {
  imagePath: string
  version: string
  imageSha256: string
  imageSize: string
  signature: string
}

interface UpdatePreflight {
  ready: boolean
  checks: { id: string; label: string; passed: boolean; detail: string; required: boolean }[]
  freeBytes: number
  imageSizeBytes: number
  activeSlot: string
  pendingSlot: string
}

const initialForm: ManifestForm = {
  version: '',
  packageSha256: '',
  packageSize: '',
  packagePath: '',
  signature: '',
  backupPath: '',
}

const initialImageForm: ImageSlotForm = {
  imagePath: '',
  version: '',
  imageSha256: '',
  imageSize: '',
  signature: '',
}

export function UpdatesPage() {
  const [state, setState] = useState<UpdateState | null>(null)
  const [form, setForm] = useState(initialForm)
  const [imageForm, setImageForm] = useState(initialImageForm)
  const [busy, setBusy] = useState(false)
  const [rollbackOpen, setRollbackOpen] = useState(false)
  const [rollbackReason, setRollbackReason] = useState('')
  const [activateOpen, setActivateOpen] = useState(false)
  const [preflight, setPreflight] = useState<UpdatePreflight | null>(null)
  const { data: recoveryStatus } = useRecoveryStatus()
  const { data: restoreDrills } = useRestoreDrills()
  const recoveryCanaryPassed = !!recoveryStatus?.manifest && restoreDrills?.some((drill) => drill.state === 'successful' && drill.generation === recoveryStatus.manifest?.generation)

  const refresh = () => apiGet<UpdateState>('/updates/status').then(setState)
  useEffect(() => {
    void refresh()
  }, [])

  const update = (key: keyof ManifestForm, value: string) => setForm((current) => ({ ...current, [key]: value }))
  const updateImage = (key: keyof ImageSlotForm, value: string) => setImageForm((current) => ({ ...current, [key]: value }))

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
    if (!rollbackReason.trim()) return
    setBusy(true)
    try {
      const next = await apiPost<UpdateState>('/updates/rollback', { reason: rollbackReason.trim() })
      setState(next)
      setRollbackOpen(false)
      setRollbackReason('')
      toast.success(`Rolled back to slot ${next.activeSlot}${next.activeVersion ? ` · ${next.activeVersion}` : ''}`)
      await refresh()
    } catch {
      toast.error('Rollback is not available')
    } finally {
      setBusy(false)
    }
  }

  async function stageImage() {
    setBusy(true)
    try {
      await apiPost('/updates/slot/stage', {
        imagePath: imageForm.imagePath,
        manifest: {
          formatVersion: 1,
          version: imageForm.version,
          packageSha256: imageForm.imageSha256,
          packageSize: Number(imageForm.imageSize),
          publishedAt: new Date().toISOString(),
        },
        signature: imageForm.signature,
      })
      toast.success('OS image staged for the inactive slot')
      await refresh()
    } catch {
      toast.error('OS image was not staged')
    } finally {
      setBusy(false)
    }
  }

  async function activateImage() {
    setBusy(true)
    try {
      await apiPost('/updates/slot/activate', {})
      toast.success('Slot image written — reboot to start the new slot')
      setActivateOpen(false)
      await refresh()
    } catch {
      toast.error('Slot activation failed')
    } finally {
      setBusy(false)
    }
  }

  async function reviewActivation() {
    setBusy(true)
    try {
      const result = await apiGet<UpdatePreflight>('/updates/preflight')
      setPreflight(result)
      setActivateOpen(true)
    } catch {
      toast.error('Update preflight could not be completed')
    } finally {
      setBusy(false)
    }
  }

  async function confirmImage() {
    setBusy(true)
    try {
      await apiPost('/updates/slot/confirm', {})
      toast.success('New slot committed as active')
      await refresh()
    } catch {
      toast.error('Slot confirmation failed')
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
              {state?.previousSlot ? <Button onClick={() => setRollbackOpen(true)} disabled={busy} variant="destructiveOutline"><RotateCcw />Rollback</Button> : null}
            </div>
          </CardContent>
      </Card>

      <Dialog open={rollbackOpen} onOpenChange={setRollbackOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Roll back system update?</DialogTitle>
            <DialogDescription>Switch from slot {state?.activeSlot}{state?.activeVersion ? ` (${state.activeVersion})` : ''} to previous slot {state?.previousSlot}. Record why you are rolling back for the audit trail.</DialogDescription>
          </DialogHeader>
          <div className="space-y-2"><Label htmlFor="rollback-reason">Reason</Label><Input id="rollback-reason" value={rollbackReason} onChange={(event) => setRollbackReason(event.target.value)} placeholder="Describe the issue observed" /></div>
          <DialogFooter><Button variant="ghost" onClick={() => setRollbackOpen(false)}>Cancel</Button><Button variant="destructive" disabled={busy || !rollbackReason.trim()} onClick={() => void rollback()}>Rollback to previous slot</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={activateOpen} onOpenChange={setActivateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Review OS slot activation</DialogTitle>
            <DialogDescription>This writes the staged image for {imageForm.version || state?.pendingVersion || 'the staged version'} to inactive slot {state?.pendingSlot ?? '—'} and arms it for the next reboot.</DialogDescription>
          </DialogHeader>
          <ul className="space-y-2 rounded-lg border p-3 text-sm">
            {(preflight?.checks ?? []).map((check) => <li key={check.id} className={check.passed ? 'text-success' : check.required ? 'text-critical' : 'text-warning'}>{check.passed ? '✓' : '!'} {check.label}: {check.detail}</li>)}
            <li className="text-muted-foreground">Free space: {preflight?.freeBytes.toLocaleString() ?? '—'} bytes · staged image: {preflight?.imageSizeBytes.toLocaleString() ?? '—'} bytes</li>
            <li className="text-warning">Reboot is required before the new slot starts.</li>
          </ul>
          <DialogFooter><Button variant="ghost" onClick={() => setActivateOpen(false)}>Cancel</Button><Button variant="destructive" disabled={busy || !state?.pendingSlot || !preflight?.ready} onClick={() => void activateImage()}>Write image and arm next boot</Button></DialogFooter>
        </DialogContent>
      </Dialog>

      <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><Download className="size-4 text-primary" />Stage signed package</CardTitle>
            <CardDescription>Package paths are local to the NAS update worker. The public verification key is managed server-side.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="rounded-lg border p-3 text-sm">
              <p className="mb-2 font-medium">Preflight</p>
              <div className="flex flex-wrap gap-2">
                <Badge variant={recoveryStatus?.verified ? 'success' : 'warning'}>Recovery bundle {recoveryStatus?.verified ? 'verified' : 'needs verification'}</Badge>
                <Badge variant={recoveryCanaryPassed ? 'success' : 'warning'}>Current generation {recoveryCanaryPassed ? 'restore-tested' : 'not restore-tested'}</Badge>
              </div>
              <p className="mt-2 text-xs text-muted-foreground">The server verifies the selected recovery bundle before staging. A passing canary drill confirms the current generation can be restored.</p>
            </div>
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

      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2"><HardDrive className="size-4 text-primary" />OS image slots (immutable A/B)</CardTitle>
          <CardDescription>
            Writes a full signed root-filesystem image to the inactive slot device and boots it once via BootNext.
            The slot becomes active only after the new image reports healthy. Slot devices are configured server-side.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="grid gap-3 sm:grid-cols-5">
            {(['imagePath', 'version', 'imageSha256', 'imageSize', 'signature'] as const).map((key) => (
              <div key={key} className="space-y-1">
                <Label htmlFor={`slot-${key}`}>
                  {key === 'imageSha256' ? 'Image SHA-256' : key === 'imageSize' ? 'Image size (bytes)' : key}
                </Label>
                <Input
                  id={`slot-${key}`}
                  value={imageForm[key]}
                  onChange={(event) => updateImage(key, event.target.value)}
                  placeholder={key === 'signature' ? 'base64 Ed25519 signature' : undefined}
                />
              </div>
            ))}
          </div>
          <div className="flex flex-wrap gap-2">
            <Button
              disabled={busy || !imageForm.imagePath || !imageForm.version || !imageForm.imageSha256 || !imageForm.signature}
              onClick={() => void stageImage()}
            >
              <Download />
              Stage image
            </Button>
            {state?.pendingSlot ? (
              <Button variant="destructive" disabled={busy} onClick={() => void reviewActivation()}>
                Write to inactive slot and arm BootNext
              </Button>
            ) : null}
            {state?.pendingSlot && state.pendingVersion ? (
              <Button variant="outline" disabled={busy} onClick={() => void confirmImage()}>
                <CheckCircle2 />
                Commit healthy slot
              </Button>
            ) : null}
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
