import { useState } from 'react'
import {
  useConfirmStorageOperation,
  usePlanStorageOperation,
  useStorageSafety,
  useUnlockStorageSafety,
} from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { OperationReview } from '@/components/core/operation-review'
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
import { formatDateTime } from '@/lib/format'
import { diskBranchPath } from '@/features/storage/branch-path'
import type { Disk } from '@/api/types'

export type DiskAction = 'format' | 'format-mount' | 'erase' | 'mount' | 'unmount'

const ACTION_META: Record<DiskAction, { title: string; verb: string; destructive: boolean }> = {
  format: { title: 'Format disk', verb: 'Format', destructive: true },
  'format-mount': { title: 'Format & mount disk', verb: 'Format & mount', destructive: true },
  erase: { title: 'Erase disk', verb: 'Erase', destructive: true },
  mount: { title: 'Mount disk', verb: 'Mount', destructive: false },
  unmount: { title: 'Unmount disk', verb: 'Unmount', destructive: false },
}

function requestedState(action: DiskAction, disk: Disk, filesystem: string, label: string, encrypted: boolean) {
  switch (action) {
    case 'format':
      return { filesystem }
    case 'format-mount': {
      const state: Record<string, unknown> = { filesystem, mountPath: diskBranchPath(disk.id) }
      if (label.trim() !== '') state.label = label.trim()
      if (encrypted) state.encrypted = true
      return state
    }
    case 'mount':
      return { filesystem: disk.filesystem ?? '', mountPath: diskBranchPath(disk.id) }
    case 'unmount':
      return { mountPath: diskBranchPath(disk.id) }
    case 'erase':
      return {}
  }
}

function errorMessage(error: unknown): string | null {
  if (error instanceof Error && 'body' in error) {
    const body = (error as { body?: { error?: string } }).body
    if (body?.error) return body.error
  }
  return error instanceof Error ? error.message : null
}

export function DiskOperationsDialog({
  disk,
  action,
  onOpenChange,
}: {
  disk: Disk
  action: DiskAction | null
  onOpenChange: (open: boolean) => void
}) {
  const plan = usePlanStorageOperation()
  const confirm = useConfirmStorageOperation()
  const { data: safety } = useStorageSafety()
  const unlock = useUnlockStorageSafety()
  const [filesystem, setFilesystem] = useState('ext4')
  const [label, setLabel] = useState('')
  const [encrypted, setEncrypted] = useState(false)
  const [encryptionPassphrase, setEncryptionPassphrase] = useState('')
  const [encryptionPassphraseConfirm, setEncryptionPassphraseConfirm] = useState('')

  if (action == null) return null
  const meta = ACTION_META[action]
  const needsFilesystem = action === 'format' || action === 'format-mount' || action === 'mount'
  const planResult = plan.data

  function close() {
    plan.reset()
    confirm.reset()
    setLabel('')
    setEncrypted(false)
    setEncryptionPassphrase('')
    setEncryptionPassphraseConfirm('')
    onOpenChange(false)
  }

  function stateDescription(): string[] {
    if (!planResult) return []
    const steps = [
      `Action: ${planResult.action}`,
      `Target: ${planResult.target.model ?? ''} ${planResult.target.diskId}`.trim(),
    ]
    for (const [key, value] of Object.entries(planResult.requestedState)) {
      steps.push(`${key}: ${String(value)}`)
    }
    steps.push(`Plan hash: ${planResult.planHash.slice(0, 16)}…`)
    steps.push(`Expires: ${formatDateTime(planResult.expiresAt)}`)
    return steps
  }

  function warnings() {
    if (meta.destructive) {
      return [
        {
          label: `All data on ${disk.name} will be permanently destroyed`,
          description: 'Shares, pools, and apps using this disk will stop working.',
        },
      ]
    }
    return undefined
  }

  return (
    <OperationReview
      open
      onOpenChange={(next) => {
        if (!next) close()
      }}
      title={planResult ? `Confirm ${meta.title.toLowerCase()} — ${disk.name}` : meta.title}
      description={
        planResult
          ? 'The plan is pinned to the disk identity and expires. Confirming revalidates the disk and requires an unlocked storage safety window.'
          : meta.destructive
            ? 'This operation is planned first and executed only after an explicit confirmation.'
            : 'The operation is planned against the current disk identity before it runs.'
      }
      steps={stateDescription()}
      warnings={warnings()}
      confirmLabel={planResult ? `Confirm ${meta.verb.toLowerCase()}` : 'Plan operation'}
      loading={plan.isPending || confirm.isPending}
      confirmDisabled={Boolean(planResult?.requestedState.encrypted) && (encryptionPassphrase.length < 12 || encryptionPassphrase !== encryptionPassphraseConfirm)}
      onConfirm={() => {
        if (!planResult) {
          plan.mutate(
            { action: operationAction(action), diskId: disk.id, requestedState: requestedState(action, disk, filesystem, label, encrypted) },
            { onError: () => undefined },
          )
          return
        }
        confirm.mutate(
          { operationId: planResult.operationId, planHash: planResult.planHash, ...(planResult.requestedState.encrypted ? { encryptionPassphrase } : {}) },
          { onSuccess: () => close() },
        )
      }}
    >
      {!planResult ? (
        <div className="grid gap-3">
          {needsFilesystem ? (
            <div className="grid gap-2">
              <Label>Filesystem</Label>
              <Select value={filesystem} onValueChange={setFilesystem}>
                <SelectTrigger className="w-52">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="ext4">ext4</SelectItem>
                  <SelectItem value="xfs">xfs</SelectItem>
                </SelectContent>
              </Select>
            </div>
          ) : null}
          {action === 'format-mount' ? (
            <div className="grid gap-3">
              <label className="flex items-start gap-2 rounded-md border p-3 text-sm"><input type="checkbox" aria-label="Encrypt this disk with LUKS" checked={encrypted} onChange={(event) => setEncrypted(event.target.checked)} className="mt-1" /><span><span className="font-medium">Encrypt this disk with LUKS</span><span className="mt-1 block text-xs text-muted-foreground">The passphrase will not be saved on the NAS. You will enter it to unlock this volume after each reboot. Keep a separate recovery copy; a lost passphrase cannot be reset.</span></span></label>
              <div className="grid gap-2">
              <Label htmlFor="disk-label">
                Volume label <span className="text-muted-foreground">(optional, max 12 characters)</span>
              </Label>
              <Input
                id="disk-label"
                value={label}
                maxLength={12}
                onChange={(event) => setLabel(event.target.value)}
                placeholder="media"
                className="font-mono"
              />
              </div>
            </div>
          ) : null}
          {plan.isError ? (
            <AlertBanner tone="critical" title="Plan rejected">
              {errorMessage(plan.error)}
            </AlertBanner>
          ) : null}
        </div>
      ) : (
        <div className="grid gap-3">
          {planResult.requestedState.encrypted ? <div className="grid gap-2"><Label htmlFor="luks-passphrase">LUKS passphrase</Label><Input id="luks-passphrase" type="password" autoComplete="new-password" minLength={12} maxLength={256} value={encryptionPassphrase} onChange={(event) => setEncryptionPassphrase(event.target.value)} /><Label htmlFor="luks-passphrase-confirm">Confirm passphrase</Label><Input id="luks-passphrase-confirm" type="password" autoComplete="new-password" minLength={12} maxLength={256} value={encryptionPassphraseConfirm} onChange={(event) => setEncryptionPassphraseConfirm(event.target.value)} />{encryptionPassphraseConfirm && encryptionPassphrase !== encryptionPassphraseConfirm ? <p role="alert" className="text-xs text-destructive">Passphrases do not match.</p> : null}<p className="text-xs text-muted-foreground">This passphrase is sent directly to the privileged storage worker for formatting. It is never written into the saved operation plan.</p></div> : null}
          {safety?.state !== 'unlocked' ? (
            <AlertBanner tone="warning" title="Storage safety is locked">
              <span className="flex flex-wrap items-center gap-2">
                Confirming requires an unlocked safety window.
                <Button size="sm" variant="outline" onClick={() => unlock.mutate()} disabled={unlock.isPending}>
                  {unlock.isPending ? 'Unlocking…' : 'Unlock (15 min)'}
                </Button>
              </span>
            </AlertBanner>
          ) : null}
          {confirm.isError ? (
            <AlertBanner tone="critical" title="Operation rejected">
              {errorMessage(confirm.error)}
            </AlertBanner>
          ) : null}
        </div>
      )}
    </OperationReview>
  )
}

function operationAction(action: DiskAction) {
  switch (action) {
    case 'format':
      return 'filesystem.format' as const
    case 'format-mount':
      return 'filesystem.create' as const
    case 'mount':
      return 'filesystem.mount' as const
    case 'unmount':
      return 'filesystem.unmount' as const
    case 'erase':
      return 'disk.erase' as const
  }
}
