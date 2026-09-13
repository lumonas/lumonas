import { useState } from 'react'
import { BatteryCharging, Power, RotateCw } from 'lucide-react'
import { usePowerAction, useSettings, useUPS, useUPSConfig, useUPSPolicy, useUpdateUPSConfig, useUpdateUPSPolicy, useUpdateSettings } from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
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

export function PowerTab() {
  const { data: settings } = useSettings()
  const { data: upsUnits } = useUPS()
  const { data: upsConfig } = useUPSConfig()
  const { data: upsPolicy } = useUPSPolicy()
  const updateSettings = useUpdateSettings()
  const updateUPSPolicy = useUpdateUPSPolicy()
  const updateUPSConfig = useUpdateUPSConfig()
  const powerAction = usePowerAction()
  const [confirmAction, setConfirmAction] = useState<'shutdown' | 'reboot' | null>(null)
  const [upsNamesDraft, setUpsNamesDraft] = useState<string | null>(null)
  const minimumRuntimeMinutes = Math.round((upsPolicy?.minimumRuntimeSec ?? 300) / 60)
  const minimumCharge = Math.round(upsPolicy?.minimumCharge ?? 10)

  const upsNames = upsNamesDraft ?? upsConfig?.names.join(', ') ?? ''

  if (!settings) return null
  const { power } = settings
  const ups = upsUnits?.[0]

  return (
    <div className="flex flex-col gap-4">
      {power.maintenanceMode && (
        <AlertBanner tone="warning" title="Maintenance mode is active">
          Configuration changes from other admins are blocked and risky operations are disabled
          until maintenance mode is turned off.
        </AlertBanner>
      )}

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Maintenance mode
          </CardTitle>
        </CardHeader>
        <CardContent className="flex items-center justify-between gap-4">
          <p className="max-w-xl text-sm text-muted-foreground">
            Use before disk upgrades or risky maintenance. Safe automation (SnapRAID, backups)
            keeps running; interactive changes are locked out.
          </p>
          <Switch
            checked={power.maintenanceMode}
            onCheckedChange={(next) =>
              updateSettings.mutate({
                section: 'power',
                patch: { maintenanceMode: next },
              })
            }
            aria-label="Toggle maintenance mode"
          />
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Power actions
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex gap-2">
              <Button
                variant="outline"
                className="flex-1"
                onClick={() => setConfirmAction('reboot')}
              >
                <RotateCw />
                Reboot…
              </Button>
              <Button
                variant="destructiveOutline"
                className="flex-1"
                onClick={() => setConfirmAction('shutdown')}
              >
                <Power />
                Shut down…
              </Button>
            </div>
            <p className="text-xs text-muted-foreground">
              Before shutdown, running jobs finish and storage is synced and unmounted in the safe
              order.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Scheduled power
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-center justify-between gap-3">
              <Label className="text-xs text-muted-foreground">Enabled</Label>
              <Switch
                checked={power.schedule.enabled}
                onCheckedChange={(next) =>
                  updateSettings.mutate({
                    section: 'power',
                    patch: { schedule: { ...power.schedule, enabled: next } },
                  })
                }
                aria-label="Toggle scheduled power"
              />
            </div>
            {power.schedule.enabled && (
              <div className="flex flex-wrap items-center gap-3">
                <Select
                  value={power.schedule.action}
                  onValueChange={(action) =>
                    updateSettings.mutate({
                      section: 'power',
                      patch: {
                        schedule: {
                          ...power.schedule,
                          action: action as 'shutdown' | 'reboot',
                        },
                      },
                    })
                  }
                >
                  <SelectTrigger className="h-8 w-32 text-xs">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="shutdown">Shut down</SelectItem>
                    <SelectItem value="reboot">Reboot</SelectItem>
                  </SelectContent>
                </Select>
                <Input
                  type="time"
                  defaultValue={power.schedule.time}
                  className="h-8 w-32 text-xs"
                  onBlur={(e) =>
                    updateSettings.mutate({
                      section: 'power',
                      patch: { schedule: { ...power.schedule, time: e.target.value } },
                    })
                  }
                />
                <span className="text-xs text-muted-foreground">{power.schedule.days}</span>
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Wake-on-LAN
            </CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="flex flex-col divide-y rounded-lg border">
              {power.wol.map((nic) => (
                <li key={nic.interface} className="flex items-center justify-between gap-3 px-3 py-2.5">
                  <div className="min-w-0">
                    <p className="font-mono text-sm">{nic.interface}</p>
                    <p className="tnum font-mono text-xs text-muted-foreground">{nic.mac}</p>
                  </div>
                  {nic.supported ? (
                    <Switch
                      checked={nic.enabled}
                      onCheckedChange={(next) =>
                        updateSettings.mutate({
                          section: 'power',
                          patch: {
                            wol: power.wol.map((entry) =>
                              entry.interface === nic.interface
                                ? { ...entry, enabled: next }
                                : entry,
                            ),
                          },
                        })
                      }
                      aria-label={`Wake-on-LAN for ${nic.interface}`}
                    />
                  ) : (
                    <Badge variant="outline" className="text-muted-foreground">
                      Not supported
                    </Badge>
                  )}
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
              <BatteryCharging className="size-4" />
              UPS (NUT)
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="tnum flex items-center justify-between rounded-lg border px-3 py-2.5 text-sm">
              <div>
                <p className="font-medium">
                  {ups ? [ups.manufacturer, ups.model].filter(Boolean).join(' ') || ups.name : 'No UPS detected'}
                </p>
                <p className="text-xs text-muted-foreground">
                  {ups ? `NUT device ${ups.name}` : 'Configure NUT to monitor a UPS'}
                </p>
              </div>
              <div className="text-right">
                <p className={ups?.onBattery ? 'font-medium text-critical' : 'font-medium text-success'}>
                  {ups?.status ?? 'Not detected'}
                </p>
                <p className="text-xs text-muted-foreground">
                  {ups?.chargePercent != null ? `${Math.round(ups.chargePercent)}%` : '—'} ·{' '}
                  {ups?.runtimeSec != null ? `${Math.round(ups.runtimeSec / 60)} min runtime` : '—'}
                </p>
              </div>
            </div>
            <p className="text-xs text-muted-foreground">
              On battery: alert → stop jobs → flush writes → stop apps → sync/unmount → safe
              shutdown.
            </p>
            <div className="grid gap-1 rounded-lg border p-3">
              <Label htmlFor="ups-names" className="text-xs text-muted-foreground">NUT devices</Label>
              <div className="flex gap-2">
                <Input
                  id="ups-names"
                  value={upsNames}
                  onChange={(event) => setUpsNamesDraft(event.target.value)}
                  placeholder="Auto-discover, or ups@host:3493"
                  className="h-8 text-xs"
                />
                <Button
                  variant="outline"
                  className="h-8 shrink-0 text-xs"
                  onClick={() => updateUPSConfig.mutate({ names: upsNames.split(',').map((name) => name.trim()).filter(Boolean) })}
                  disabled={updateUPSConfig.isPending}
                >
                  Save
                </Button>
              </div>
              <p className="text-[11px] text-muted-foreground">Leave empty to use NUT auto-discovery.</p>
            </div>
            <div className="grid grid-cols-1 gap-3 rounded-lg border p-3 sm:grid-cols-3">
              <div className="flex items-center justify-between gap-3 sm:col-span-3">
                <Label className="text-xs text-muted-foreground">Automatic safe shutdown</Label>
                <Switch
                  checked={upsPolicy?.enabled ?? false}
                  onCheckedChange={(enabled) =>
                    updateUPSPolicy.mutate({ enabled, minimumRuntimeSec: minimumRuntimeMinutes * 60, minimumCharge })
                  }
                  aria-label="Toggle automatic UPS shutdown"
                />
              </div>
              <div className="grid gap-1">
                <Label htmlFor="ups-runtime" className="text-xs text-muted-foreground">Minimum runtime (minutes)</Label>
                <Input
                  id="ups-runtime"
                  type="number"
                  min={0}
                  max={1440}
                  value={minimumRuntimeMinutes}
                  onChange={(event) => updateUPSPolicy.mutate({ enabled: upsPolicy?.enabled ?? false, minimumRuntimeSec: (Number(event.target.value) || 0) * 60, minimumCharge })}
                  className="h-8 text-xs"
                />
              </div>
              <div className="grid gap-1">
                <Label htmlFor="ups-charge" className="text-xs text-muted-foreground">Minimum charge (%)</Label>
                <Input
                  id="ups-charge"
                  type="number"
                  min={0}
                  max={100}
                  value={minimumCharge}
                  onChange={(event) => updateUPSPolicy.mutate({ enabled: upsPolicy?.enabled ?? false, minimumRuntimeSec: minimumRuntimeMinutes * 60, minimumCharge: Number(event.target.value) || 0 })}
                  className="h-8 text-xs"
                />
              </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <Dialog open={confirmAction != null} onOpenChange={(open) => !open && setConfirmAction(null)}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>
              {confirmAction === 'shutdown' ? 'Shut down lumo-one?' : 'Reboot lumo-one?'}
            </DialogTitle>
            <DialogDescription>
              {confirmAction === 'shutdown'
                ? 'The NAS will power off after running jobs finish and storage is safely unmounted.'
                : 'The NAS will restart after running jobs finish. Expect roughly one minute of downtime.'}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="ghost" onClick={() => setConfirmAction(null)}>
              Cancel
            </Button>
            <Button
              variant={confirmAction === 'shutdown' ? 'destructive' : 'default'}
              onClick={() => {
                if (confirmAction) {
                  powerAction.mutate(confirmAction === 'shutdown' ? 'poweroff' : 'reboot')
                  setConfirmAction(null)
                }
              }}
              disabled={powerAction.isPending}
            >
              {confirmAction === 'shutdown' ? 'Shut down' : 'Reboot'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
