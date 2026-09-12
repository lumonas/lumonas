import { useNavigate } from 'react-router-dom'
import {
  Activity,
  ArrowUpCircle,
  Container,
  FolderOpen,
  FolderPlus,
  HardDrive,
  RefreshCw,
  RotateCw,
  Search,
  SlidersHorizontal,
  UserPlus,
} from 'lucide-react'
import {
  useCreateJob,
  useDisks,
  useDockerStacks,
  useStackAction,
} from '@/api/queries'
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
  const { data: stacks } = useDockerStacks()
  const stackAction = useStackAction()

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
      {stacks && stacks.length > 0 && (
        <>
          <CommandSeparator />
          <CommandGroup heading="Docker stacks">
            {stacks.slice(0, 6).map((stack) => (
              <CommandItem
                key={stack.id}
                value={`stack ${stack.name}`}
                onSelect={run(() => navigate(`/docker?tab=stacks&stack=${stack.id}`))}
              >
                <Container />
                <span className="font-mono text-xs">{stack.name}</span>
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
        {stacks?.some((s) => s.name === 'jellyfin') && (
          <>
            <CommandItem
              onSelect={run(() => {
                const jellyfin = stacks.find((s) => s.name === 'jellyfin')
                if (jellyfin) stackAction.mutate({ id: jellyfin.id, action: 'restart' })
              })}
            >
              <RotateCw />
              Restart Jellyfin
            </CommandItem>
            <CommandItem
              onSelect={run(() => {
                const jellyfin = stacks.find((s) => s.name === 'jellyfin')
                navigate(`/docker?tab=stacks&stack=${jellyfin?.id ?? ''}`)
              })}
            >
              <Container />
              View Jellyfin logs
            </CommandItem>
          </>
        )}
        {stacks?.some((s) => s.updateAvailable) && (
          <CommandItem
            onSelect={run(() => {
              const stack = stacks.find((s) => s.updateAvailable)
              if (stack) navigate(`/docker?tab=stacks&stack=${stack.id}`)
            })}
          >
            <ArrowUpCircle />
            Review pending stack update
          </CommandItem>
        )}
        <CommandItem onSelect={run(() => navigate('/files'))}>
          <FolderOpen />
          Browse files
        </CommandItem>
        <CommandItem onSelect={run(() => navigate('/shares?create=1'))}>
          <FolderPlus />
          Create share
        </CommandItem>
        <CommandItem onSelect={run(() => navigate('/users?create=1'))}>
          <UserPlus />
          Add user
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
        <CommandItem onSelect={run(() => navigate('/docker'))}>
          <Search />
          Browse app catalog
        </CommandItem>
        <CommandItem onSelect={run(() => navigate('/settings?tab=updates'))}>
          <SlidersHorizontal />
          Check for updates
        </CommandItem>
        <CommandItem onSelect={run(() => navigate('/monitoring?tab=timeline'))}>
          <Search />
          View activity log
        </CommandItem>
      </CommandGroup>
    </CommandDialog>
  )
}
