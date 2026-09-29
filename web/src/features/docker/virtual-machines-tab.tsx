import { useEffect, useRef, useState } from 'react'
import {
  useCreateVirtualMachine,
  useCreateVirtualMachineSnapshot,
  useDeleteVirtualMachineSnapshot,
  useRevertVirtualMachineSnapshot,
  useUploadVirtualizationMedia,
  useVirtualizationMedia,
  useVirtualizationStatus,
  useVirtualMachines,
  useVirtualMachineAction,
  useVirtualMachineSnapshots,
  useVirtualMachineConsole,
  useWriteVirtualMachineConsole,
  useCloseVirtualMachineConsole,
  useDeleteVirtualMachine,
  useRecoverableVirtualMachines,
  useRestoreVirtualMachineDefinition,
} from '@/api/queries'
import { AlertBanner } from '@/components/core/alert-banner'
import { HealthBadge } from '@/components/core/health-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { formatBytes } from '@/lib/format'
import type { VirtualMachine, VirtualMachineSnapshot } from '@/api/types'

function stateLabel(state: string) {
  if (state === 'shut off') return 'stopped'
  return state
}

export function VirtualMachinesTab() {
  const { data: host, isLoading: hostLoading } = useVirtualizationStatus()
  const { data: machines, isLoading: machinesLoading, isError } = useVirtualMachines()
  const { data: media, isLoading: mediaLoading } = useVirtualizationMedia()
  const action = useVirtualMachineAction()
  const uploadMedia = useUploadVirtualizationMedia()
  const createMachine = useCreateVirtualMachine()
  const closeConsole = useCloseVirtualMachineConsole()
  const deleteMachine = useDeleteVirtualMachine()
  const { data: recoverables, isLoading: recoverablesLoading, isError: recoverablesError } = useRecoverableVirtualMachines()
  const restoreDefinition = useRestoreVirtualMachineDefinition()
  const [createOpen, setCreateOpen] = useState(false)
  const [openSnapshots, setOpenSnapshots] = useState<Record<string, boolean>>({})
  const [consoleVM, setConsoleVM] = useState<string | null>(null)
  const [deleteVM, setDeleteVM] = useState<VirtualMachine | null>(null)
  const [deleteConfirmation, setDeleteConfirmation] = useState('')
  const [deleteDisk, setDeleteDisk] = useState(false)
  const [name, setName] = useState('')
  const [iso, setISO] = useState<string | null>(null)
  const [vcpus, setVCPUs] = useState(2)
  const [memoryMiB, setMemoryMiB] = useState(2048)
  const [diskGiB, setDiskGiB] = useState(32)

  const selectedISO = iso ?? media?.[0]?.name ?? ''

  function controls(machine: VirtualMachine) {
    const buttons: { action: 'start' | 'shutdown' | 'reboot' | 'suspend' | 'resume'; label: string }[] =
      machine.state === 'running'
        ? [{ action: 'shutdown', label: 'Shut down' }, { action: 'reboot', label: 'Reboot' }, { action: 'suspend', label: 'Pause' }]
        : machine.state === 'paused'
          ? [{ action: 'resume', label: 'Resume' }]
          : [{ action: 'start', label: 'Start' }]
    return buttons.map((button) => (
      <Button key={button.action} size="sm" variant="outline" disabled={action.isPending || !host?.libvirtAvailable} onClick={() => action.mutate({ name: machine.name, action: button.action })}>
        {button.label}
      </Button>
    ))
  }

  function toggleConsole(name: string) {
    if (consoleVM === name) {
      closeConsole.mutate(name)
      setConsoleVM(null)
      return
    }
    if (consoleVM) closeConsole.mutate(consoleVM)
    setConsoleVM(name)
  }

  return (
    <div className="flex flex-col gap-4">
      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm">Virtualization host</CardTitle>
          <CardDescription>Create installer-ready virtual machines and manage their lifecycle.</CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3">
          {hostLoading ? <p className="text-sm text-muted-foreground">Checking host capabilities…</p> : host?.libvirtAvailable ? (
            <div className="flex flex-wrap gap-x-6 gap-y-2 text-sm">
              <span>{host.architecture} · {host.cpuCount} logical CPUs</span>
              <span>{formatBytes(host.memoryBytes)} host memory</span>
              <HealthBadge state={host.kvmAvailable ? 'healthy' : 'attention'} />
              <span className="text-xs text-muted-foreground">{host.kvmAvailable ? 'KVM acceleration available' : 'KVM unavailable; guests may use slower software emulation'}</span>
            </div>
          ) : (
            <AlertBanner tone="attention" title="Libvirt is not available">
              {host?.reason ?? 'Install and enable libvirt, then restart the LumoNAS daemon. VM controls will appear when the service is reachable.'}
            </AlertBanner>
          )}
          {host?.libvirtAvailable && host.reason ? <AlertBanner tone="warning" title="Virtualization limitation">{host.reason}</AlertBanner> : null}
          <p className="text-xs text-muted-foreground">Guest definitions use fixed libvirt operations and local image paths. The web UI does not expose arbitrary hypervisor commands.</p>
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm">Installation media</CardTitle>
          <CardDescription>Upload a local OS installer ISO. Files stay on this NAS; LumoNAS does not download images from the internet.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-3">
          <Label htmlFor="vm-iso-upload">Upload ISO (maximum 8 GiB)</Label>
          <Input
            id="vm-iso-upload"
            aria-label="Upload installation ISO"
            type="file"
            accept=".iso,application/x-iso9660-image"
            disabled={uploadMedia.isPending}
            onChange={(event) => {
              const file = event.currentTarget.files?.[0]
              if (file) uploadMedia.mutate(file, { onSuccess: (uploaded) => setISO(uploaded.name) })
              event.currentTarget.value = ''
            }}
          />
          {uploadMedia.isError ? <AlertBanner tone="critical" title="ISO upload failed">{uploadMedia.error.message}</AlertBanner> : null}
          {mediaLoading ? <p className="text-xs text-muted-foreground">Loading installer media…</p> : media?.length ? (
            <ul className="flex flex-col divide-y rounded-md border px-3">
              {media.map((item) => <li key={item.name} className="flex items-center justify-between gap-3 py-2 text-xs"><span className="truncate font-mono">{item.name}</span><span className="shrink-0 text-muted-foreground">{formatBytes(item.sizeBytes)}</span></li>)}
            </ul>
          ) : <p className="text-xs text-muted-foreground">No installation media uploaded yet.</p>}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="flex-row items-center justify-between gap-3 pb-3">
          <CardTitle className="text-sm">Virtual machines</CardTitle>
          <Button size="sm" disabled={!host?.libvirtAvailable || !media?.length} onClick={() => setCreateOpen(true)}>Create VM</Button>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          <p className="text-xs text-muted-foreground">State refreshes every 10 seconds. Shutdown requests are graceful. New VMs use a sparse qcow2 disk and boot from the selected installer.</p>
          {isError ? <AlertBanner tone="critical" title="VM inventory could not be loaded">Check libvirt availability and the LumoNAS service account’s socket access.</AlertBanner> : null}
          {action.isError ? <AlertBanner tone="critical" title="VM action failed">{action.error.message}</AlertBanner> : null}
          {machinesLoading ? <p className="text-sm text-muted-foreground">Loading virtual machines…</p> : null}
          {!machinesLoading && machines?.length === 0 ? <p className="text-sm text-muted-foreground">No libvirt guests are defined on this host.</p> : null}
          {(machines ?? []).map((machine) => (
            <div key={machine.uuid ?? machine.name} data-testid={`vm-${machine.name}`} className="rounded-md border px-3 py-2.5">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div className="min-w-0">
                  <p className="truncate font-mono text-sm font-medium">{machine.name}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {machine.vcpus ? `${machine.vcpus} vCPU` : 'vCPU unknown'}
                    {machine.maximumMemoryKiB ? ` · ${formatBytes(machine.maximumMemoryKiB * 1024)} configured RAM` : ''}
                    {machine.uuid ? ` · ${machine.uuid}` : ''}
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <HealthBadge state={machine.state === 'running' ? 'healthy' : machine.state === 'paused' ? 'attention' : 'offline'} />
                  <span className="mr-1 text-xs capitalize text-muted-foreground">{stateLabel(machine.state)}</span>
                  {controls(machine)}
                  {machine.state === 'running' ? <Button size="sm" variant="outline" disabled={!host?.libvirtAvailable || closeConsole.isPending} onClick={() => toggleConsole(machine.name)}>Serial console</Button> : null}
                  <Button size="sm" variant="ghost" aria-label={`Snapshots for ${machine.name}`} aria-expanded={Boolean(openSnapshots[machine.name])} onClick={() => setOpenSnapshots((current) => ({ ...current, [machine.name]: !current[machine.name] }))}>Snapshots</Button>
                  <Button size="sm" variant="ghost" className="text-destructive hover:text-destructive" disabled={!host?.libvirtAvailable} onClick={() => { setDeleteVM(machine); setDeleteConfirmation(''); setDeleteDisk(false) }}>Delete VM</Button>
                </div>
              </div>
              {consoleVM === machine.name ? <VirtualMachineConsolePanel name={machine.name} onClose={() => setConsoleVM(null)} /> : null}
              {openSnapshots[machine.name] ? <VirtualMachineSnapshotPanel name={machine.name} /> : null}
            </div>
          ))}
        </CardContent>
      </Card>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-sm">Saved VM recovery files</CardTitle>
          <CardDescription>Definitions removed while keeping their disks stay here. Restore verifies the qcow2 image before re-registering the guest.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {recoverablesLoading ? <p className="text-sm text-muted-foreground">Checking saved VM recovery files…</p> : null}
          {recoverablesError ? <AlertBanner tone="critical" title="Saved VM recovery files could not be loaded">Check the managed VM image directory and libvirt availability.</AlertBanner> : null}
          {!recoverablesLoading && !recoverablesError && recoverables?.length === 0 ? <p className="text-sm text-muted-foreground">No saved VM recovery files.</p> : null}
          {restoreDefinition.isError ? <AlertBanner tone="critical" title="VM definition could not be restored">{restoreDefinition.error.message}</AlertBanner> : null}
          {(recoverables ?? []).map((saved) => (
            <div key={saved.name} className="flex flex-wrap items-center justify-between gap-3 rounded-md border px-3 py-2.5">
              <div className="min-w-0"><p className="font-mono text-sm font-medium">{saved.name}</p><p className="mt-1 text-xs text-muted-foreground">{formatBytes(saved.diskBytes)} · qcow2 disk retained</p></div>
              <Button size="sm" variant="outline" disabled={restoreDefinition.isPending} onClick={() => restoreDefinition.mutate(saved.name)}>Restore VM definition</Button>
            </div>
          ))}
        </CardContent>
      </Card>

      <Dialog open={createOpen} onOpenChange={setCreateOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Create virtual machine</DialogTitle>
            <DialogDescription>Choose an uploaded installer and a resource budget. The new guest starts from the ISO after its disk is created.</DialogDescription>
          </DialogHeader>
          <div className="grid gap-3">
            <div className="grid gap-1.5"><Label htmlFor="vm-name">VM name</Label><Input id="vm-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="home-server" autoFocus /></div>
            <div className="grid gap-1.5"><Label htmlFor="vm-iso">Installer ISO</Label><select id="vm-iso" aria-label="Installer ISO" value={selectedISO} onChange={(event) => setISO(event.target.value)} className="h-9 rounded-md border border-input bg-background px-3 text-sm"><option value="">Select an ISO</option>{(media ?? []).map((item) => <option key={item.name} value={item.name}>{item.name}</option>)}</select></div>
            <div className="grid grid-cols-3 gap-3">
              <div className="grid gap-1.5"><Label htmlFor="vm-vcpus">vCPUs</Label><Input id="vm-vcpus" type="number" min={1} max={Math.min(host?.cpuCount ?? 1, 64)} value={vcpus} onChange={(event) => setVCPUs(Number(event.target.value))} /></div>
              <div className="grid gap-1.5"><Label htmlFor="vm-memory">Memory (MiB)</Label><Input id="vm-memory" type="number" min={512} max={Math.max(512, Math.floor((host?.memoryBytes ?? 0) / 1_048_576) - 1024)} step={512} value={memoryMiB} onChange={(event) => setMemoryMiB(Number(event.target.value))} /></div>
              <div className="grid gap-1.5"><Label htmlFor="vm-disk">Disk (GiB)</Label><Input id="vm-disk" type="number" min={8} max={16384} value={diskGiB} onChange={(event) => setDiskGiB(Number(event.target.value))} /></div>
            </div>
            <p className="text-xs text-muted-foreground">Host reserve: at least 1 GiB RAM. Virtual disk capacity is sparse; actual use grows as the guest writes data.</p>
            {createMachine.isError ? <AlertBanner tone="critical" title="VM could not be created">{createMachine.error.message}</AlertBanner> : null}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setCreateOpen(false)}>Cancel</Button>
            <Button
              disabled={!name.trim() || !selectedISO || vcpus < 1 || vcpus > Math.min(host?.cpuCount ?? 0, 64) || memoryMiB < 512 || memoryMiB > (host?.memoryBytes ?? 0) / 1_048_576 - 1024 || diskGiB < 8 || diskGiB > 16384 || createMachine.isPending}
              onClick={() => createMachine.mutate({ name: name.trim(), iso: selectedISO, vcpus, memoryMiB, diskGiB }, { onSuccess: () => setCreateOpen(false) })}
            >{createMachine.isPending ? 'Creating…' : 'Create and boot VM'}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteVM != null} onOpenChange={(open) => { if (!open && !deleteMachine.isPending) { setDeleteVM(null); setDeleteConfirmation(''); setDeleteDisk(false); deleteMachine.reset() } }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete {deleteVM?.name}?</DialogTitle>
            <DialogDescription>
              This removes the guest from libvirt and clears its snapshot metadata. Shut it down first. Keeping the disk also keeps its saved definition so you can restore the VM later.
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="rounded-md border border-destructive/30 bg-destructive/5 p-3 text-sm">
              <p className="font-medium">Virtual disk</p>
              <p className="mt-1 break-all font-mono text-xs text-muted-foreground">{deleteVM ? `${deleteVM.name}.qcow2 in the managed VM image directory` : ''}</p>
              <label className="mt-3 flex items-start gap-2">
                <input type="checkbox" checked={deleteDisk} onChange={(event) => setDeleteDisk(event.target.checked)} />
                <span>Permanently delete this disk and all guest data. This cannot be undone.</span>
              </label>
            </div>
            <div className="space-y-2">
              <Label htmlFor="vm-delete-confirm">Type <span className="font-mono">{deleteVM?.name}</span> to confirm</Label>
              <Input id="vm-delete-confirm" value={deleteConfirmation} onChange={(event) => setDeleteConfirmation(event.target.value)} autoComplete="off" />
            </div>
            {deleteVM?.state !== 'shut off' ? <p className="text-xs text-warning-foreground">This guest is {stateLabel(deleteVM?.state ?? 'unknown')}. Shut it down and wait for it to stop before deleting.</p> : null}
            {deleteMachine.isError ? <AlertBanner tone="critical" title="VM could not be deleted">{deleteMachine.error.message}</AlertBanner> : null}
          </div>
          <DialogFooter>
            <Button variant="outline" disabled={deleteMachine.isPending} onClick={() => setDeleteVM(null)}>Cancel</Button>
            <Button
              variant="destructive"
              disabled={!deleteVM || deleteVM.state !== 'shut off' || deleteConfirmation !== deleteVM.name || deleteMachine.isPending}
              onClick={() => deleteVM && deleteMachine.mutate({ name: deleteVM.name, deleteDisk }, { onSuccess: () => { setDeleteVM(null); setDeleteConfirmation('') } })}
            >{deleteMachine.isPending ? 'Deleting…' : deleteDisk ? 'Delete VM and disk' : 'Delete VM, keep recovery files'}</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

function VirtualMachineConsolePanel({ name, onClose }: { name: string; onClose: () => void }) {
  const { data, isError, error } = useVirtualMachineConsole(name)
  const writeConsole = useWriteVirtualMachineConsole()
  const closeConsole = useCloseVirtualMachineConsole()
  const outputRef = useRef<HTMLDivElement>(null)
  const writeQueue = useRef(Promise.resolve())
  useEffect(() => {
    const pane = outputRef.current
    if (pane) pane.scrollTop = pane.scrollHeight
  }, [data?.cursor, data?.output])

  function keyDown(event: React.KeyboardEvent<HTMLDivElement>) {
    const arrows: Record<string, string> = { ArrowUp: '\u001b[A', ArrowDown: '\u001b[B', ArrowRight: '\u001b[C', ArrowLeft: '\u001b[D' }
    let data = arrows[event.key]
    if (event.ctrlKey && event.key.length === 1) data = String.fromCharCode(event.key.toUpperCase().charCodeAt(0) - 64)
    else if (event.key === 'Enter') data = '\r'
    else if (event.key === 'Backspace') data = '\u007f'
    else if (event.key === 'Tab') data = '\t'
    else if (event.key === 'Escape') data = '\u001b'
    else if (event.key.length === 1 && !event.metaKey && !event.altKey) data = event.key
    if (!data) return
    event.preventDefault()
    writeQueue.current = writeQueue.current.then(() => writeConsole.mutateAsync({ name, data })).then(() => undefined).catch(() => undefined)
  }

  function close() {
    closeConsole.mutate(name, { onSettled: onClose })
  }

  return (
    <div className="mt-3 flex flex-col gap-2 border-t pt-3">
      <div className="flex items-center justify-between gap-3">
        <div><p className="text-xs font-medium">Serial console · {data?.connected ? 'connected' : 'connecting'}</p><p className="text-[11px] text-muted-foreground">Text console only. Graphical installer screens are not shown here.</p></div>
        <Button size="sm" variant="ghost" disabled={closeConsole.isPending} onClick={close}>Close console</Button>
      </div>
      {isError ? <AlertBanner tone="critical" title="Serial console could not be opened">{error instanceof Error ? error.message : 'Check that the guest is running and has a serial console.'}</AlertBanner> : null}
      {writeConsole.isError ? <AlertBanner tone="critical" title="Console input failed">{writeConsole.error.message}</AlertBanner> : null}
      <div ref={outputRef} role="log" aria-label={`${name} serial console output`} className="h-64 overflow-auto rounded-md border bg-black p-3 font-mono text-xs leading-relaxed text-green-200" onClick={(event) => event.currentTarget.focus()}>
        <pre className="whitespace-pre-wrap break-all">{data?.output || (data?.connected ? 'Waiting for serial output…' : 'Connecting to the guest serial port…')}</pre>
      </div>
      <div role="textbox" aria-label={`${name} serial console input`} aria-multiline="false" tabIndex={0} onKeyDown={keyDown} className="rounded-md border border-dashed px-3 py-2 text-xs text-muted-foreground outline-none focus-visible:ring-2 focus-visible:ring-ring">Click here and type to send input to the guest. Press Enter to submit.</div>
    </div>
  )
}

function VirtualMachineSnapshotPanel({ name }: { name: string }) {
  const { data: snapshots, isLoading, isError, error } = useVirtualMachineSnapshots(name)
  const createSnapshot = useCreateVirtualMachineSnapshot()
  const revertSnapshot = useRevertVirtualMachineSnapshot()
  const deleteSnapshot = useDeleteVirtualMachineSnapshot()
  const [confirmation, setConfirmation] = useState<{ action: 'restore' | 'delete'; snapshot: VirtualMachineSnapshot } | null>(null)
  const mutationError = createSnapshot.error ?? revertSnapshot.error ?? deleteSnapshot.error
  const busy = createSnapshot.isPending || revertSnapshot.isPending || deleteSnapshot.isPending

  function create() {
    const timestamp = new Date().toISOString().replace(/[:.]/g, '-')
    createSnapshot.mutate({ name, snapshot: `before-${timestamp}` })
  }

  function confirm() {
    if (!confirmation) return
    const mutation = confirmation.action === 'restore' ? revertSnapshot : deleteSnapshot
    mutation.mutate({ name, snapshot: confirmation.snapshot.name }, { onSuccess: () => setConfirmation(null) })
  }

  return (
    <div className="mt-3 flex flex-col gap-2 border-t pt-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs font-medium">VM snapshots</p>
        <Button size="sm" variant="outline" disabled={busy} onClick={create}>Create snapshot</Button>
      </div>
      {mutationError ? <AlertBanner tone="critical" title="Snapshot operation failed">{mutationError.message}</AlertBanner> : null}
      {isError ? <AlertBanner tone="critical" title="Snapshots could not be loaded">{error instanceof Error ? error.message : null}</AlertBanner> : null}
      {isLoading ? <p className="text-xs text-muted-foreground">Loading snapshots…</p> : null}
      {!isLoading && snapshots?.length === 0 ? <p className="text-xs text-muted-foreground">No snapshots for this VM yet.</p> : null}
      {(snapshots ?? []).map((snapshot) => (
        <div key={snapshot.name} className="flex flex-wrap items-center justify-between gap-2 rounded-md bg-muted/30 px-3 py-2">
          <div className="min-w-0"><p className="truncate font-mono text-xs">{snapshot.name}</p><p className="text-[11px] text-muted-foreground">{snapshot.creationTime ?? snapshot.state ?? 'Snapshot'}</p></div>
          <div className="flex gap-2">
            <Button size="sm" variant="outline" disabled={busy} onClick={() => setConfirmation({ action: 'restore', snapshot })}>Restore snapshot</Button>
            <Button size="sm" variant="ghost" disabled={busy} onClick={() => setConfirmation({ action: 'delete', snapshot })}>Delete snapshot</Button>
          </div>
        </div>
      ))}
      <Dialog open={confirmation != null} onOpenChange={(open) => { if (!open) setConfirmation(null) }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{confirmation?.action === 'restore' ? 'Restore VM snapshot?' : 'Delete VM snapshot?'}</DialogTitle>
            <DialogDescription>{confirmation?.action === 'restore'
              ? `Restoring ${confirmation.snapshot.name} replaces the guest’s current state with that saved point in time.`
              : `Deleting ${confirmation?.snapshot.name} permanently removes that recovery point. The current guest state is kept.`}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setConfirmation(null)}>Cancel</Button>
            <Button variant={confirmation?.action === 'delete' ? 'destructive' : 'default'} disabled={busy} onClick={confirm}>
              {busy ? 'Working…' : confirmation?.action === 'restore' ? 'Restore snapshot' : 'Delete snapshot'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
