import { Cable, HardDrive } from 'lucide-react'
import { useDisks } from '@/api/queries'
import { DiskIdentity } from '@/components/core/disk-identity'
import { HealthBadge } from '@/components/core/health-badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { formatBytes } from '@/lib/format'

// This intentionally reports the stable identities and transport topology the
// daemon can verify. Mapping a disk to a physical bay or blinking an LED is
// controller-specific and is not guessed from a potentially stale label.
export function TopologyTab() {
  const { data: disks } = useDisks()
  const groups = ['nvme', 'sata', 'usb'].map((transport) => ({ transport, disks: (disks ?? []).filter((disk) => disk.interface === transport) })).filter((group) => group.disks.length > 0)
  return <div className="flex flex-col gap-4">
    <Card><CardHeader className="pb-3"><CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground"><Cable className="size-4" />Hardware topology</CardTitle></CardHeader><CardContent><p className="text-sm text-muted-foreground">Verified disk identities grouped by transport. Serial, WWN, and filesystem UUID remain the source of truth during replacement; bay LEDs are only shown when the active controller exposes a safe locator API.</p></CardContent></Card>
    {groups.map((group) => <Card key={group.transport}><CardHeader className="pb-3"><CardTitle className="flex items-center gap-2 text-sm font-medium capitalize"><HardDrive className="size-4 text-muted-foreground" />{group.transport} transport <span className="text-xs font-normal text-muted-foreground">{group.disks.length} disk{group.disks.length === 1 ? '' : 's'}</span></CardTitle></CardHeader><CardContent className="divide-y rounded-lg border">{group.disks.map((disk) => <div key={disk.id} className="flex flex-wrap items-center justify-between gap-3 p-3"><div className="min-w-0"><p className="text-sm font-medium">{disk.name} · {disk.model}</p><p className="mt-1 text-xs text-muted-foreground">{formatBytes(disk.sizeBytes)} · {disk.role} · {disk.currentPath ?? 'path unavailable'}</p><div className="mt-2"><DiskIdentity disk={disk} /></div></div><HealthBadge state={disk.health} /></div>)}</CardContent></Card>)}
  </div>
}
