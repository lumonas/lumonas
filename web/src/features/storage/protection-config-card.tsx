import { useState } from 'react'
import { Save } from 'lucide-react'
import { useDisks, useProtectionConfig, useUpdateProtectionConfig } from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ROLE_LABELS } from '@/features/storage/roles'

export function ProtectionConfigCard() {
  const { data: config } = useProtectionConfig()
  const { data: disks } = useDisks()
  const update = useUpdateProtectionConfig()
  const [parity, setParity] = useState('')
  const [dataDisks, setDataDisks] = useState<Set<string>>(new Set())
  const [rendered, setRendered] = useState<string | null>(null)
  const [syncedConfig, setSyncedConfig] = useState(config)

  // Derive editable state from the loaded config during render; comparing
  // the previous synced config avoids cascading effect renders.
  if (config && config !== syncedConfig) {
    setSyncedConfig(config)
    setParity(config.parityDiskIds[0] ?? '')
    setDataDisks(new Set(config.dataDiskIds))
  }

  if (!config) return null

  function toggleData(diskId: string) {
    setDataDisks((previous) => {
      const next = new Set(previous)
      if (next.has(diskId)) next.delete(diskId)
      else next.add(diskId)
      return next
    })
  }

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          SnapRAID configuration
        </CardTitle>
        <CardDescription>
          {config.configured
            ? `Rendered to ${config.configPath}`
            : 'No snapraid.conf has been generated yet — pick parity and data disks below.'}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-4">
        <div className="grid gap-4 sm:grid-cols-2">
          <div className="grid content-start gap-2">
            <Label>Parity disk</Label>
            <Select value={parity} onValueChange={setParity}>
              <SelectTrigger className="h-9">
                <SelectValue placeholder="Choose a disk" />
              </SelectTrigger>
              <SelectContent>
                {(disks ?? []).map((disk) => (
                  <SelectItem key={disk.id} value={disk.id}>
                    {disk.name} · {disk.model} ({ROLE_LABELS[disk.role]})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <p className="text-xs text-muted-foreground">
              Must be as large as the biggest data disk.
            </p>
          </div>
          <div className="grid content-start gap-2">
            <Label>Data disks</Label>
            <div className="flex max-h-44 flex-col gap-1 overflow-auto rounded-md border p-2">
              {(disks ?? []).map((disk) => (
                <label key={disk.id} className="flex items-center gap-2 rounded px-1.5 py-1 text-sm hover:bg-muted/40">
                  <input
                    type="checkbox"
                    className="size-4 accent-primary"
                    checked={dataDisks.has(disk.id)}
                    onChange={() => toggleData(disk.id)}
                  />
                  <span className="font-mono text-xs">{disk.name}</span>
                  <span className="mx-1.5 text-xs text-muted-foreground">·</span>
                  <span className="truncate text-xs">{disk.model}</span>
                </label>
              ))}
            </div>
          </div>
        </div>
        {update.isError ? (
          <AlertBanner tone="critical" title="Configuration rejected">
            {update.error instanceof Error ? update.error.message : null}
          </AlertBanner>
        ) : null}
        {rendered ? (
          <pre className="max-h-56 overflow-auto rounded-md border bg-muted/30 p-3 font-mono text-xs">{rendered}</pre>
        ) : null}
        <div>
          <Button
            size="sm"
            disabled={!parity || dataDisks.size === 0 || update.isPending}
            onClick={() =>
              update.mutate(
                { parityDiskId: parity, dataDiskIds: [...dataDisks] },
                { onSuccess: (value) => setRendered(value.config) },
              )
            }
          >
            <Save />
            {update.isPending ? 'Saving…' : 'Save configuration'}
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
