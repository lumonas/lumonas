import { useSettings, useUpdateSettings } from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { formatBytes } from '@/lib/format'

const PROFILES = [
  {
    value: 'balanced',
    label: 'Balanced (recommended)',
    hint: 'Small writes are batched briefly; safe default for most NAS workloads.',
  },
  {
    value: 'normal',
    label: 'Normal',
    hint: 'Writes hit disk almost immediately — lowest latency, more wear.',
  },
  {
    value: 'maximum',
    label: 'Maximum write reduction',
    hint: 'Aggressively batches writes to protect disks. Recent writes (logs, databases) can be lost after a power failure.',
  },
] as const

const TMPFS_SIZES = [
  { value: 536_870_912, label: '512 MiB' },
  { value: 1_073_741_824, label: '1 GiB' },
  { value: 2_147_483_648, label: '2 GiB' },
  { value: 4_294_967_296, label: '4 GiB' },
  { value: 8_589_934_592, label: '8 GiB' },
] as const

export function RuntimeTab() {
  const { data: settings } = useSettings()
  const updateSettings = useUpdateSettings()
  if (!settings) return null
  const { runtime } = settings

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm font-medium text-muted-foreground">
            Write optimization
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <div className="grid gap-2">
            <Label htmlFor="write-profile">Profile</Label>
            <Select
              value={runtime.writeProfile}
              onValueChange={(value) =>
                updateSettings.mutate({
                  section: 'runtime',
                  patch: { writeProfile: value },
                })
              }
            >
              <SelectTrigger id="write-profile" className="max-w-sm">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {PROFILES.map((profile) => (
                  <SelectItem key={profile.value} value={profile.value}>
                    {profile.label}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="max-w-xl text-xs text-muted-foreground">
              {PROFILES.find((p) => p.value === runtime.writeProfile)?.hint}
            </p>
          </div>
          {runtime.writeProfile === 'maximum' && (
            <AlertBanner tone="warning" title="Recent writes may be lost after a power failure">
              With maximum write reduction, logs and databases can lose their last seconds of
              writes when power is cut. An UPS is strongly recommended with this profile.
            </AlertBanner>
          )}
        </CardContent>
      </Card>

      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">zram (compressed RAM)</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="tnum grid grid-cols-3 gap-3 text-sm">
              <div>
                <p className="text-xs text-muted-foreground">Size</p>
                <p className="font-medium">{formatBytes(runtime.zram.sizeBytes)}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Compressed</p>
                <p className="font-medium">{formatBytes(runtime.zram.compressedBytes)}</p>
              </div>
              <div>
                <p className="text-xs text-muted-foreground">Ratio</p>
                <p className="font-medium">{runtime.zram.ratio.toFixed(1)}×</p>
              </div>
            </div>
            <div className="flex items-center justify-between gap-3">
              <span className="text-xs text-muted-foreground">Pressure</span>
              <Badge
                variant={
                  runtime.zram.pressure === 'low'
                    ? 'success'
                    : runtime.zram.pressure === 'medium'
                      ? 'attention'
                      : 'critical'
                }
                className="capitalize"
              >
                {runtime.zram.pressure}
              </Badge>
            </div>
            <p className="text-xs text-muted-foreground">
              zram keeps frequently written temporary data in compressed RAM instead of hammering
              the system SSD.
            </p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              RAM transcode cache
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-sm">Temporary files in tmpfs</p>
                <p className="text-xs text-muted-foreground">
                  Uses volatile RAM for transcoding. Contents are lost after reboot.
                </p>
              </div>
              <Switch
                checked={runtime.tmpfs.enabled}
                onCheckedChange={(enabled) =>
                  updateSettings.mutate({
                    section: 'runtime',
                    patch: {
                      tmpfs: {
                        ...runtime.tmpfs,
                        enabled,
                        sizeBytes: runtime.tmpfs.sizeBytes || TMPFS_SIZES[1].value,
                      },
                    },
                  })
                }
                aria-label="Toggle RAM transcode cache"
              />
            </div>
            <div className="flex items-center justify-between gap-3 border-t pt-3">
              <div>
                <p className="text-xs text-muted-foreground">Capacity</p>
                <p className="font-medium">{formatBytes(runtime.tmpfs.sizeBytes)}</p>
              </div>
              <Select
                value={String(runtime.tmpfs.sizeBytes || TMPFS_SIZES[1].value)}
                onValueChange={(value) =>
                  updateSettings.mutate({
                    section: 'runtime',
                    patch: {
                      tmpfs: { ...runtime.tmpfs, sizeBytes: Number(value) },
                    },
                  })
                }
              >
                <SelectTrigger className="h-8 w-28 text-xs" aria-label="RAM transcode capacity">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TMPFS_SIZES.map((size) => (
                    <SelectItem key={size.value} value={String(size.value)}>
                      {size.label}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <p className="font-mono text-[11px] text-muted-foreground">{runtime.tmpfs.mountPath}</p>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-3">
            <CardTitle className="text-sm font-medium text-muted-foreground">
              Docker logging
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3">
            <div className="flex items-center justify-between gap-3">
              <Label className="text-xs text-muted-foreground">Max size per log file</Label>
              <Select
                value={String(runtime.dockerLogging.maxSizeMb)}
                onValueChange={(value) =>
                  updateSettings.mutate({
                    section: 'runtime',
                    patch: {
                      dockerLogging: { ...runtime.dockerLogging, maxSizeMb: Number(value) },
                    },
                  })
                }
              >
                <SelectTrigger className="h-8 w-28 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="5">5 MB</SelectItem>
                  <SelectItem value="10">10 MB</SelectItem>
                  <SelectItem value="50">50 MB</SelectItem>
                  <SelectItem value="100">100 MB</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div className="flex items-center justify-between gap-3">
              <Label className="text-xs text-muted-foreground">Retained files</Label>
              <Select
                value={String(runtime.dockerLogging.maxFiles)}
                onValueChange={(value) =>
                  updateSettings.mutate({
                    section: 'runtime',
                    patch: {
                      dockerLogging: { ...runtime.dockerLogging, maxFiles: Number(value) },
                    },
                  })
                }
              >
                <SelectTrigger className="h-8 w-28 text-xs">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="1">1</SelectItem>
                  <SelectItem value="3">3</SelectItem>
                  <SelectItem value="5">5</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div>
              <p className="mb-1.5 text-xs text-muted-foreground">Loudest containers</p>
              <ul className="flex flex-col divide-y rounded-lg border">
                {runtime.dockerLogging.topConsumers.map((consumer) => (
                  <li key={consumer.name} className="flex items-center justify-between px-3 py-1.5">
                    <span className="truncate font-mono text-xs">{consumer.name}</span>
                    <span className="tnum text-xs text-muted-foreground">
                      {formatBytes(consumer.sizeBytes)}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
