import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { CheckCircle2, Download, HardDrive, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { useCompleteOnboarding, useCreateRecoveryKey, useOnboardingState } from '@/api/queries'
import { Logo } from '@/components/layout/Logo'
import { AlertBanner } from '@/components/core/alert-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
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
import { formatBytes } from '@/lib/format'
import { cn } from '@/lib/utils'
import { loadProgress, saveProgress, useOnboardingStore } from '@/stores/onboarding'
import type { DiskRole, OnboardingDisk } from '@/api/types'

const STEPS = ['Welcome', 'Storage', 'Protection', 'Recovery', 'Finish'] as const

const CLASSIFICATION_BADGES: Record<OnboardingDisk['classification'], { label: string; variant: 'secondary' | 'attention' | 'info' | 'critical' }> = {
  system: { label: 'System', variant: 'secondary' },
  blank: { label: 'Blank', variant: 'secondary' },
  existing: { label: 'Data found', variant: 'attention' },
  lumonas: { label: 'LumoNAS disk', variant: 'info' },
  'suspected-parity': { label: 'Suspected parity', variant: 'info' },
  removable: { label: 'Removable', variant: 'secondary' },
}

const ROLE_OPTIONS: Record<string, { value: DiskRole; label: string; danger?: boolean }[]> = {
  system: [{ value: 'system', label: 'System (required)' }],
  blank: [
    { value: 'data', label: 'Data — recommended' },
    { value: 'parity', label: 'Parity' },
    { value: 'apps', label: 'Apps & cache' },
    { value: 'unknown', label: 'Not used now' },
  ],
  existing: [
    { value: 'data', label: 'Import without modifying data — recommended' },
    { value: 'parity', label: 'Use as parity' },
    { value: 'unknown', label: 'Leave alone' },
    { value: 'backup', label: 'Erase and reuse', danger: true },
  ],
  'suspected-parity': [
    { value: 'parity', label: 'Parity — recommended' },
    { value: 'data', label: 'Data (existing contents stay readable)' },
    { value: 'unknown', label: 'Not used now' },
  ],
  removable: [{ value: 'external', label: 'External — managed separately' }],
  lumonas: [
    { value: 'data', label: 'Data — reuse existing LumoNAS disk' },
    { value: 'parity', label: 'Parity' },
  ],
}

interface WizardState {
  step: number
  serverName: string
  roles: Record<string, DiskRole>
  syncTime: string
  scrubDay: string
  autoConfigBackup: boolean
  dailySnapshot: boolean
  twoCopies: boolean
  destination: string
  keyAcknowledged: boolean
}

export function OnboardingPage() {
  const navigate = useNavigate()
  const { data: state } = useOnboardingState()
  const createRecoveryKey = useCreateRecoveryKey()
  const completeOnboarding = useCompleteOnboarding()
  const completeSetup = useOnboardingStore((s) => s.complete)

  const [wizard, setWizard] = useState<WizardState>(
    () =>
      loadProgress<WizardState>() ?? {
        step: 0,
        serverName: '',
        roles: {},
        syncTime: '02:00',
        scrubDay: 'sunday',
        autoConfigBackup: true,
        dailySnapshot: true,
        twoCopies: true,
        destination: 'usb',
        keyAcknowledged: false,
      },
  )

  const resolved: WizardState = useMemo(() => {
    if (!state) return wizard
    const merged = { ...wizard }
    if (!merged.serverName) merged.serverName = state.server.name
    for (const disk of state.disks) {
      if (!merged.roles[disk.id]) merged.roles[disk.id] = disk.recommendedRole
    }
    return merged
  }, [state, wizard])

  function update(partial: Partial<WizardState>) {
    const next = { ...resolved, ...partial }
    setWizard(next)
    saveProgress(next)
  }

  if (!state) {
    return (
      <div className="flex min-h-dvh items-center justify-center bg-background">
        <Loader2 className="size-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  const dataDisks = state.disks.filter((d) => resolved.roles[d.id] === 'data')
  const parityDisks = state.disks.filter((d) => resolved.roles[d.id] === 'parity')
  const usableBytes = dataDisks.reduce((sum, d) => sum + d.sizeBytes, 0)
  const largestData = dataDisks.reduce((max, d) => Math.max(max, d.sizeBytes), 0)
  const paritySize = parityDisks.reduce((sum, d) => sum + d.sizeBytes, 0)
  const parityTooSmall = parityDisks.length > 0 && paritySize < largestData
  const noParity = dataDisks.length > 0 && parityDisks.length === 0

  function finish() {
    completeOnboarding.mutate(
      {
        serverName: resolved.serverName,
        roles: resolved.roles,
        protection: { syncTime: resolved.syncTime, scrubDay: resolved.scrubDay },
        recovery: {
          autoConfigBackup: resolved.autoConfigBackup,
          destination: resolved.destination,
          keyAcknowledged: resolved.keyAcknowledged,
        },
      },
      {
        onSuccess: (result) => {
          completeSetup()
          toast.success('Setup complete', {
            description: result.initialSyncStarted
              ? 'Initial parity sync is running as a background job.'
              : undefined,
          })
          navigate('/')
        },
      },
    )
  }

  function downloadRecoveryKey() {
    createRecoveryKey.mutate(undefined, {
      onSuccess: ({ key }) => {
        const blob = new Blob([`LumoNAS recovery key\n${key}\n`], { type: 'text/plain;charset=utf-8' })
        const url = URL.createObjectURL(blob)
        const anchor = document.createElement('a')
        anchor.href = url
        anchor.download = 'lumonas-recovery-key.txt'
        anchor.click()
        URL.revokeObjectURL(url)
        toast.success('Recovery key downloaded')
      },
    })
  }

  return (
    <div className="flex min-h-dvh items-start justify-center bg-background px-4 py-10">
      <div className="w-full max-w-3xl">
        <div className="mb-6 flex items-center gap-3">
          <Logo className="size-9" />
          <div>
            <h1 className="text-lg font-semibold tracking-tight">LumoNAS — first-time setup</h1>
            <p className="text-sm text-muted-foreground">
              Everything here can be changed later. No internet connection required.
            </p>
          </div>
        </div>

        <ol className="mb-5 flex items-center gap-2" aria-label="Steps">
          {STEPS.map((label, index) => (
            <li key={label} className="flex flex-1 items-center gap-2">
              <span
                className={cn(
                  'flex size-5 shrink-0 items-center justify-center rounded-full border text-[11px] font-semibold',
                  index < resolved.step && 'border-success bg-success/15 text-success',
                  index === resolved.step && 'border-primary bg-primary/15 text-primary',
                  index > resolved.step && 'border-border text-muted-foreground',
                )}
              >
                {index < resolved.step ? <CheckCircle2 className="size-3.5" /> : index + 1}
              </span>
              <span
                className={cn(
                  'hidden text-xs sm:inline',
                  index === resolved.step ? 'font-medium text-foreground' : 'text-muted-foreground',
                )}
              >
                {label}
              </span>
              {index < STEPS.length - 1 && <span className="h-px flex-1 bg-border" />}
            </li>
          ))}
        </ol>

        <Card>
          <CardContent className="flex flex-col gap-4 p-5">
            {resolved.step === 0 && (
              <>
                <div>
                  <h2 className="text-base font-semibold">Welcome to your NAS</h2>
                  <p className="mt-1 text-sm text-muted-foreground">
                    This wizard configures storage, protection and recovery. Detected hardware:
                  </p>
                </div>
                <div className="tnum grid grid-cols-1 gap-3 rounded-lg border p-3 text-sm sm:grid-cols-3">
                  <div>
                    <p className="text-xs text-muted-foreground">CPU</p>
                    <p className="font-medium">{state.hardware.cpu}</p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Memory</p>
                    <p className="font-medium">{formatBytes(state.hardware.ramBytes)}</p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Disks detected</p>
                    <p className="font-medium">{state.hardware.diskCount}</p>
                  </div>
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label htmlFor="ob-name">Server name</Label>
                    <Input
                      id="ob-name"
                      value={resolved.serverName}
                      onChange={(e) => update({ serverName: e.target.value })}
                      className="font-mono"
                    />
                    <p className="text-xs text-muted-foreground">
                      Reachable as <span className="font-mono">{resolved.serverName || 'lumo-one'}.local</span> and{' '}
                      <span className="font-mono">{state.server.ip}</span>
                    </p>
                  </div>
                  <div className="grid content-start gap-2 text-sm">
                    <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
                      Already configured
                    </p>
                    <p className="flex items-center justify-between gap-2">
                      Timezone <span className="text-muted-foreground">{state.server.timezone}</span>
                    </p>
                    <p className="flex items-center justify-between gap-2">
                      SSH{' '}
                      <Badge variant={state.server.sshEnabled ? 'success' : 'secondary'}>
                        {state.server.sshEnabled ? 'Key-only, enabled' : 'Disabled'}
                      </Badge>
                    </p>
                    <p className="flex items-center justify-between gap-2">
                      Updates <span className="text-muted-foreground">Stable channel</span>
                    </p>
                  </div>
                </div>
              </>
            )}

            {resolved.step === 1 && (
              <>
                <div>
                  <h2 className="text-base font-semibold">Assign your disks</h2>
                  <p className="mt-1 text-sm text-muted-foreground">
                    Disks with existing data are imported without modifying anything. Wiping a disk
                    is a separate, explicit action — never a side effect.
                  </p>
                </div>
                <div className="flex flex-col gap-2">
                  {state.disks.map((disk) => (
                    <div
                      key={disk.id}
                      className="flex flex-wrap items-center justify-between gap-3 rounded-lg border px-3 py-2.5"
                    >
                      <div className="flex min-w-0 items-center gap-3">
                        <HardDrive className="size-4 shrink-0 text-muted-foreground" />
                        <div className="min-w-0">
                          <p className="flex flex-wrap items-center gap-2 text-sm font-medium">
                            {disk.model}
                            <Badge variant={CLASSIFICATION_BADGES[disk.classification].variant}>
                              {CLASSIFICATION_BADGES[disk.classification].label}
                            </Badge>
                          </p>
                          <p className="tnum text-xs text-muted-foreground">
                            {formatBytes(disk.sizeBytes)} · serial …{disk.serialSuffix}
                            {disk.filesystem ? ` · ${disk.filesystem}` : ''}
                          </p>
                        </div>
                      </div>
                      <Select
                        value={resolved.roles[disk.id]}
                        onValueChange={(role) =>
                          update({ roles: { ...resolved.roles, [disk.id]: role as DiskRole } })
                        }
                      >
                        <SelectTrigger className="h-8 w-full max-w-64 text-xs sm:w-64">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          {(ROLE_OPTIONS[disk.classification] ?? ROLE_OPTIONS.blank).map((option) => (
                            <SelectItem
                              key={option.value}
                              value={option.value}
                              className={cn(option.danger && 'text-critical')}
                            >
                              {option.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  ))}
                </div>

                <div className="tnum grid grid-cols-3 gap-3 rounded-lg border bg-muted/30 p-3 text-sm">
                  <div>
                    <p className="text-xs text-muted-foreground">Usable data</p>
                    <p className="font-semibold">{formatBytes(usableBytes)}</p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Parity</p>
                    <p className="font-semibold">{paritySize > 0 ? formatBytes(paritySize) : 'none'}</p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Tolerance</p>
                    <p className="font-semibold">
                      {paritySize > 0 ? '1 disk after each sync' : 'none'}
                    </p>
                  </div>
                </div>
                {noParity && dataDisks.length > 0 && (
                  <AlertBanner tone="attention" title="No parity disk assigned">
                    Without parity, a failed disk loses everything stored on it only — other files
                    stay readable. You can add parity later.
                  </AlertBanner>
                )}
                {parityTooSmall && (
                  <AlertBanner tone="critical" title="Parity is smaller than a data disk">
                    Parity must be at least as large as the largest protected data disk. Pick a
                    bigger parity or reduce data disks.
                  </AlertBanner>
                )}
                <p className="text-xs text-muted-foreground">
                  SnapRAID parity is scheduled (default nightly), not realtime — changes made after
                  the last successful sync are not yet fully protected.
                </p>
              </>
            )}

            {resolved.step === 2 && (
              <>
                <div>
                  <h2 className="text-base font-semibold">Protection</h2>
                  <p className="mt-1 text-sm text-muted-foreground">
                    Parity covers {parityDisks.length > 0 ? parityDisks.map((d) => d.model).join(', ') : 'no disks yet'} and protects{' '}
                    {dataDisks.length} data disk{dataDisks.length === 1 ? '' : 's'}.
                  </p>
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <div className="grid gap-2">
                    <Label>Parity sync</Label>
                    <Select
                      value={resolved.syncTime}
                      onValueChange={(syncTime) => update({ syncTime })}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="01:00">Nightly at 01:00</SelectItem>
                        <SelectItem value="02:00">Nightly at 02:00</SelectItem>
                        <SelectItem value="03:00">Nightly at 03:00</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="grid gap-2">
                    <Label>Scrub (verify data)</Label>
                    <Select
                      value={resolved.scrubDay}
                      onValueChange={(scrubDay) => update({ scrubDay })}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="saturday">Weekly on Saturday</SelectItem>
                        <SelectItem value="sunday">Weekly on Sunday</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <AlertBanner tone="info" title="How parity works">
                  Changes made after the last successful sync are not yet fully represented in
                  parity. Parity files also replicate to every data disk for crash resilience. The
                  initial sync runs as a background job.
                </AlertBanner>
              </>
            )}

            {resolved.step === 3 && (
              <>
                <div>
                  <h2 className="text-base font-semibold">Disaster recovery</h2>
                  <p className="mt-1 text-sm text-muted-foreground">
                    Strongly recommended: automatic configuration protection so a dead system disk
                    is an inconvenience, not a disaster.
                  </p>
                </div>
                <div className="flex flex-col divide-y rounded-lg border">
                  <SwitchRow
                    label="Automatic config backup"
                    hint="After every meaningful change"
                    checked={resolved.autoConfigBackup}
                    onChange={(next) => update({ autoConfigBackup: next })}
                  />
                  <SwitchRow
                    label="Daily snapshot"
                    hint="Encrypted and verified immediately"
                    checked={resolved.dailySnapshot}
                    onChange={(next) => update({ dailySnapshot: next })}
                  />
                  <SwitchRow
                    label="Keep copies on two data disks"
                    hint="Survives any single disk failure"
                    checked={resolved.twoCopies}
                    onChange={(next) => update({ twoCopies: next })}
                  />
                  <div className="flex items-center justify-between gap-3 px-3 py-2.5">
                    <div>
                      <p className="text-sm">Encrypted archives</p>
                      <p className="text-xs text-muted-foreground">Always on — no plaintext backups.</p>
                    </div>
                    <Switch checked disabled aria-label="Encrypted (always on)" />
                  </div>
                  <div className="flex items-center justify-between gap-3 px-3 py-2.5">
                    <div>
                      <p className="text-sm">Offsite destination</p>
                      <p className="text-xs text-muted-foreground">
                        Optional — cloud is never required.
                      </p>
                    </div>
                    <Select
                      value={resolved.destination}
                      onValueChange={(destination) => update({ destination })}
                    >
                      <SelectTrigger className="h-8 w-44 text-xs">
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="usb">USB disk</SelectItem>
                        <SelectItem value="nas">Another NAS</SelectItem>
                        <SelectItem value="sftp">SFTP server</SelectItem>
                        <SelectItem value="s3">S3 / Backblaze</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                </div>
                <div className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3">
                  <div>
                    <p className="text-sm font-medium">Recovery key</p>
                    <p className="max-w-md text-xs text-muted-foreground">
                      Unlocks your config backups without this NAS. Download it, print it, and
                      store at least one copy away from the server.
                    </p>
                  </div>
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={downloadRecoveryKey}
                    disabled={createRecoveryKey.isPending}
                  >
                    {createRecoveryKey.isPending ? <Loader2 className="animate-spin" /> : <Download />}
                    {createRecoveryKey.isPending ? 'Generating…' : 'Download key'}
                  </Button>
                </div>
                <label className="flex cursor-pointer items-start gap-3 rounded-lg border p-3">
                  <input
                    type="checkbox"
                    className="mt-0.5 accent-primary"
                    checked={resolved.keyAcknowledged}
                    onChange={(e) => update({ keyAcknowledged: e.target.checked })}
                  />
                  <span className="text-sm">
                    I have stored the recovery key somewhere safe, away from this server.
                  </span>
                </label>
              </>
            )}

            {resolved.step === 4 && (
              <>
                <div>
                  <h2 className="text-base font-semibold">You're all set</h2>
                  <p className="mt-1 text-sm text-muted-foreground">
                    Summary of your setup — every part of it can be changed later.
                  </p>
                </div>
                <div className="tnum grid grid-cols-1 gap-3 rounded-lg border p-3 text-sm sm:grid-cols-3">
                  <div>
                    <p className="text-xs text-muted-foreground">Server</p>
                    <p className="font-medium">{resolved.serverName}</p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Usable data</p>
                    <p className="font-medium">{formatBytes(usableBytes)}</p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Protection</p>
                    <p className="font-medium">
                      {paritySize > 0 ? `Parity ${formatBytes(paritySize)} · nightly sync` : 'None yet'}
                    </p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Config backup</p>
                    <p className="font-medium">
                      {resolved.autoConfigBackup ? 'Automatic' : 'Skipped'}
                    </p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Recovery key</p>
                    <p className="font-medium">
                      {resolved.keyAcknowledged ? 'Stored safely' : 'Not confirmed'}
                    </p>
                  </div>
                  <div>
                    <p className="text-xs text-muted-foreground">Readiness</p>
                    <p className="font-medium">
                      {resolved.autoConfigBackup && resolved.keyAcknowledged ? '100%' : 'Check Backups'}
                    </p>
                  </div>
                </div>
                <p className="text-sm font-medium">Suggested next steps</p>
                <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                  {[
                    { label: 'Create your first share', to: '/shares?create=1' },
                    { label: 'Install a Docker app', to: '/docker' },
                    { label: 'Set up notifications', to: '/monitoring?tab=alerts' },
                    { label: 'Configure UPS', to: '/settings?tab=power' },
                  ].map((task) => (
                    <Button
                      key={task.to}
                      variant="outline"
                      className="justify-start"
                      onClick={() => {
                        completeSetup()
                        navigate(task.to)
                      }}
                    >
                      {task.label}
                    </Button>
                  ))}
                </div>
                {completeOnboarding.isPending && (
                  <p className="flex items-center gap-2 text-sm text-muted-foreground">
                    <Loader2 className="size-4 animate-spin" />
                    Committing configuration…
                  </p>
                )}
              </>
            )}
          </CardContent>
        </Card>

        <div className="mt-4 flex items-center justify-between gap-2">
          <Button
            variant="ghost"
            onClick={() => {
              completeSetup()
              navigate('/')
            }}
          >
            Skip setup for now
          </Button>
          <div className="flex gap-2">
            {resolved.step > 0 && (
              <Button variant="outline" onClick={() => update({ step: resolved.step - 1 })}>
                Back
              </Button>
            )}
            {resolved.step < 4 && (
              <Button
                onClick={() => update({ step: resolved.step + 1 })}
                disabled={resolved.step === 3 && !resolved.keyAcknowledged}
              >
                Continue
              </Button>
            )}
            {resolved.step === 4 && (
              <Button onClick={finish} disabled={completeOnboarding.isPending}>
                {completeOnboarding.isPending && <Loader2 className="animate-spin" />}
                Finish setup
              </Button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}

function SwitchRow({
  label,
  hint,
  checked,
  onChange,
}: {
  label: string
  hint: string
  checked: boolean
  onChange: (next: boolean) => void
}) {
  return (
    <div className="flex items-center justify-between gap-3 px-3 py-2.5">
      <div>
        <p className="text-sm">{label}</p>
        <p className="text-xs text-muted-foreground">{hint}</p>
      </div>
      <Switch checked={checked} onCheckedChange={onChange} aria-label={label} />
    </div>
  )
}
