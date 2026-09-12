import { useNavigate } from 'react-router-dom'
import {
  Activity,
  HardDrive,
  RefreshCw,
  Search,
  SlidersHorizontal,
} from 'lucide-react'
import { useCreateJob, useDisks } from '@/api/queries'
import { NAV_ITEMS } from '@/components/layout/nav'
import {
  CommandDialog,
  CommandEmpty,
  CommandGroup,
  CommandItem,
  CommandSeparator,
  CommandShortcut,
} from '@/components/ui/command'
import { useUiStore } from '@/stores/ui'

export function CommandPalette() {
  const navigate = useNavigate()
  const open = useUiStore((s) => s.paletteOpen)
  const setOpen = useUiStore((s) => s.setPaletteOpen)
  const { data: disks } = useDisks()
  const createJob = useCreateJob()

  function run(fn: () => void) {
    return () => {
      setOpen(false)
      fn()
    }
  }

  return (
    <CommandDialog
      open={open}
      onOpenChange={setOpen}
      title="Command palette"
      description="Search LumoNAS resources and actions"
    >
      <CommandEmpty>No results found.</CommandEmpty>
      <CommandGroup heading="Navigation">
        {NAV_ITEMS.map(({ to, label, Icon }) => (
          <CommandItem key={to} onSelect={run(() => navigate(to))}>
            <Icon />
            {label}
          </CommandItem>
        ))}
      </CommandGroup>
      {disks && disks.length > 0 && (
        <>
          <CommandSeparator />
          <CommandGroup heading="Disks">
            {disks.map((disk) => (
              <CommandItem
                key={disk.id}
                value={`disk ${disk.name} ${disk.model} ${disk.serial}`}
                onSelect={run(() => navigate(`/storage?disk=${disk.id}`))}
              >
                <HardDrive />
                <span className="font-mono text-xs">{disk.name}</span>
                <span className="text-muted-foreground">{disk.model}</span>
              </CommandItem>
            ))}
          </CommandGroup>
        </>
      )}
      <CommandSeparator />
      <CommandGroup heading="Actions">
        <CommandItem
          onSelect={run(() => {
            createJob.mutate({ type: 'snapraid.sync' })
            navigate('/storage?tab=protection')
          })}
        >
          <RefreshCw />
          Start SnapRAID sync
          <CommandShortcut>job</CommandShortcut>
        </CommandItem>
        <CommandItem
          onSelect={run(() => {
            const flagged = disks?.find((d) => d.health === 'warning' || d.health === 'critical')
            if (flagged) createJob.mutate({ type: 'smart.short', resourceId: flagged.id })
            navigate('/storage')
          })}
        >
          <Activity />
          Run SMART test on flagged disk
          <CommandShortcut>job</CommandShortcut>
        </CommandItem>
        <CommandItem onSelect={run(() => navigate('/settings'))}>
          <SlidersHorizontal />
          Check for updates
        </CommandItem>
        <CommandItem onSelect={run(() => navigate('/monitoring'))}>
          <Search />
          View activity log
        </CommandItem>
      </CommandGroup>
    </CommandDialog>
  )
}
