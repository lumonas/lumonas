import { useMemo, useState } from 'react'
import { Cable, Lock, LockOpen, Pencil, Plus, RefreshCw, Settings2, Wifi } from 'lucide-react'
import {
  useApplyNetworkConnection,
  useCreateNetworkConnection,
  type ConnectionPayload,
  useNetworkInterfaces,
  useUpdateNetworkConnection,
  useWiFiScan,
} from '@/api/queries'
import type { ConnectionType, IPMethod, NetworkConnection } from '@/api/types'
import { AlertBanner } from '@/components/core/alert-banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
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
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { cn } from '@/lib/utils'

type SetupMode = 'lan' | 'wifi' | 'manual'

const INTERFACE_PATTERN = /^[A-Za-z0-9_.:-]{1,64}$/
const CIDR_PATTERN = /^[0-9a-fA-F.]+\/\d{1,3}$/

interface WizardState {
  name: string
  interfaceName: string
  type: ConnectionType
  ssid: string
  wifiOpen: boolean
  password: string
  hiddenSSID: boolean
  ipv4Method: IPMethod
  ipv4Address: string
  ipv4Gateway: string
  ipv4DNS: string
  ipv4Metric: string
  ipv6Method: IPMethod
  mtu: string
  parent: string
  vlanId: string
  members: string[]
  connectNow: boolean
  enabled: boolean
}

const initialState: WizardState = {
  name: '',
  interfaceName: '',
  type: 'ethernet',
  ssid: '',
  wifiOpen: false,
  password: '',
  hiddenSSID: false,
  ipv4Method: 'auto',
  ipv4Address: '',
  ipv4Gateway: '',
  ipv4DNS: '',
  ipv4Metric: '',
  ipv6Method: 'disabled',
  mtu: '',
  parent: '',
  vlanId: '',
  members: [],
  connectNow: true,
  enabled: true,
}

function stateFromConnection(connection: NetworkConnection): WizardState {
  return {
    ...initialState,
    name: connection.name,
    interfaceName: connection.interface,
    type: connection.type ?? 'ethernet',
    ssid: connection.ssid ?? '',
    wifiOpen: connection.wifiOpen ?? false,
    ipv4Method: connection.ipv4.method,
    ipv4Address: connection.ipv4.addresses?.[0] ?? '',
    ipv4Gateway: connection.ipv4.gateway ?? '',
    ipv4DNS: connection.ipv4.dns?.join(', ') ?? '',
    ipv4Metric: connection.ipv4.metric ? String(connection.ipv4.metric) : '',
    ipv6Method: connection.ipv6.method,
    mtu: connection.mtu ? String(connection.mtu) : '',
    parent: connection.parent ?? '',
    vlanId: connection.vlanId ? String(connection.vlanId) : '',
    members: connection.members ?? [],
    enabled: connection.enabled,
  }
}

function splitList(value: string): string[] {
  return value
    .split(',')
    .map((item) => item.trim())
    .filter(Boolean)
}

function validate(state: WizardState, mode: SetupMode): string | null {
  if (!state.name.trim() || state.name.length > 128) return 'Enter a connection name (1–128 characters).'
  if (state.interfaceName && !INTERFACE_PATTERN.test(state.interfaceName)) {
    return 'Interface name contains invalid characters.'
  }
  if (mode === 'wifi') {
    if (state.ssid.length < 1 || state.ssid.length > 32) return 'Wi-Fi networks need an SSID of 1–32 characters.'
    if (!state.wifiOpen && state.password && (state.password.length < 8 || state.password.length > 63)) {
      return 'Wi-Fi passwords must be 8–63 characters.'
    }
  } else if (!state.interfaceName) {
    return 'Select an interface.'
  }
  if (mode === 'manual') {
    if (state.type === 'vlan') {
      if (!state.parent || !INTERFACE_PATTERN.test(state.parent)) return 'VLAN connections need a parent interface.'
      const id = Number(state.vlanId)
      if (!Number.isInteger(id) || id < 1 || id > 4094) return 'VLAN ID must be between 1 and 4094.'
    }
    if ((state.type === 'bond' || state.type === 'bridge') && state.members.length === 0) {
      return `${state.type === 'bond' ? 'Bond' : 'Bridge'} connections need at least one member.`
    }
    if (state.mtu) {
      const mtu = Number(state.mtu)
      if (!Number.isInteger(mtu) || mtu < 576 || mtu > 9000) return 'MTU must be between 576 and 9000.'
    }
  }
  if (state.ipv4Method === 'manual') {
    if (!state.ipv4Address || !CIDR_PATTERN.test(state.ipv4Address)) return 'Static IPv4 needs an address in CIDR form, e.g. 192.168.1.10/24.'
    if (state.ipv4Gateway && Number.isNaN(Number(state.ipv4Gateway.replaceAll('.', '')))) {
      return 'Gateway must be an IP address.'
    }
  }
  if (state.ipv4Method !== 'manual' && (state.ipv4Address || state.ipv4Gateway)) {
    return 'Addresses and gateway require the manual IPv4 method.'
  }
  if (state.ipv6Method === 'manual' && (!state.ipv4Address || !state.ipv4Address.includes(':'))) {
    return 'Static IPv6 needs an address in CIDR form, e.g. fd00::10/64.'
  }
  return null
}

function buildPayload(state: WizardState, mode: SetupMode): ConnectionPayload {
  const type: ConnectionType = mode === 'wifi' ? 'wifi' : mode === 'lan' ? 'ethernet' : state.type
  const dns = splitList(state.ipv4DNS)
  return {
    name: state.name.trim(),
    interface: type === 'wifi' && !state.interfaceName ? '' : state.interfaceName,
    type,
    enabled: state.enabled,
    ...(type === 'wifi' ? { ssid: state.ssid, wifiOpen: state.wifiOpen } : {}),
    ...(type === 'vlan' ? { parent: state.parent, vlanId: Number(state.vlanId) } : {}),
    ...(type === 'bond' || type === 'bridge' ? { members: state.members } : {}),
    ...(state.mtu ? { mtu: Number(state.mtu) } : {}),
    ipv4: {
      method: state.ipv4Method,
      ...(state.ipv4Method === 'manual'
        ? {
            addresses: [state.ipv4Address],
            ...(state.ipv4Gateway ? { gateway: state.ipv4Gateway } : {}),
            ...(dns.length > 0 ? { dns } : {}),
            ...(state.ipv4Metric ? { metric: Number(state.ipv4Metric) } : {}),
          }
        : {}),
    },
    ipv6: { method: mode === 'manual' ? state.ipv6Method : 'disabled' },
    reauthenticated: true,
  }
}

export function ConnectionWizard({
  open,
  onOpenChange,
  connection,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  connection?: NetworkConnection | null
}) {
  const editing = connection != null
  const [mode, setMode] = useState<SetupMode | null>(null)
  const [state, setState] = useState<WizardState>(editing ? stateFromConnection(connection) : initialState)
  const [activeMode, setActiveMode] = useState<SetupMode>(
    editing ? (connection?.type === 'wifi' ? 'wifi' : connection?.type === 'ethernet' ? 'lan' : 'manual') : 'lan',
  )
  const [error, setError] = useState<string | null>(null)

  const interfaces = useNetworkInterfaces()
  const scan = useWiFiScan(open && !editing)
  const create = useCreateNetworkConnection()
  const update = useUpdateNetworkConnection()
  const apply = useApplyNetworkConnection()

  const usable = useMemo(
    () => (interfaces.data ?? []).filter((item) => !item.loopback),
    [interfaces.data],
  )
  const wired = usable.filter((item) => !item.wireless)
  const wireless = usable.filter((item) => item.wireless)
  const pickInterface = usable.length > 0
  const networks = scan.data?.networks ?? []
  const pending = create.isPending || update.isPending

  function reset() {
    setMode(null)
    setState(initialState)
    setError(null)
  }

  function close() {
    onOpenChange(false)
    reset()
  }

  function chooseMode(next: SetupMode) {
    setMode(next)
    setActiveMode(next)
    setError(null)
    setState((current) => ({ ...current, type: next === 'wifi' ? 'wifi' : next === 'lan' ? 'ethernet' : current.type }))
  }

  function submit() {
    const problem = validate(state, activeMode)
    if (problem) {
      setError(problem)
      return
    }
    const payload = buildPayload(state, activeMode)
    const finish = (id: string) => {
      const needsPassword =
        payload.type === 'wifi' && !payload.wifiOpen && state.password.length > 0 && state.connectNow
      if (needsPassword) {
        apply.mutate(
          { id, wifiPassword: state.password },
          {
            onSuccess: () => close(),
            onError: (applyError) =>
              setError(
                `${applyError instanceof Error ? applyError.message : 'Could not start the Wi-Fi connection.'} The connection was saved — apply it from the list once the appliance service is reachable.`,
              ),
          },
        )
        return
      }
      close()
    }
    if (editing && connection) {
      update.mutate(
        { id: connection.id, ...payload },
        {
          onSuccess: (updated) => finish(updated.id),
          onError: (mutationError) => setError(mutationError.message),
        },
      )
      return
    }
    create.mutate(payload, {
      onSuccess: (created) => finish(created.id),
      onError: (mutationError) => setError(mutationError.message),
    })
  }

  const title = editing ? `Edit ${connection?.name}` : 'Set up a connection'
  const description = editing
    ? 'Changes are saved as a pending configuration — apply them to activate.'
    : 'Connect this NAS to your network. Wired and Wi-Fi setups are one click; manual covers advanced topologies.'

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!next) close()
      }}
    >
      <DialogContent className="max-h-[85dvh] overflow-y-auto sm:max-w-lg">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            {editing ? <Pencil className="size-4 text-primary" /> : <Plus className="size-4 text-primary" />}
            {title}
          </DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>

        {error ? (
          <AlertBanner tone="critical" title="Could not save the connection">
            {error}
          </AlertBanner>
        ) : null}

        {!editing && mode == null ? (
          <div className="grid gap-3">
            <ModeCard
              icon={<Cable className="size-5" />}
              title="Wired LAN"
              description="Pick a network port and connect with DHCP in one click."
              onClick={() => chooseMode('lan')}
            />
            <ModeCard
              icon={<Wifi className="size-5" />}
              title="Wi-Fi"
              description="Scan nearby networks, pick yours, enter the password."
              onClick={() => chooseMode('wifi')}
            />
            <ModeCard
              icon={<Settings2 className="size-5" />}
              title="Manual (advanced)"
              description="Static addressing, VLAN, bond, or bridge — full control."
              onClick={() => chooseMode('manual')}
            />
          </div>
        ) : (
          <div className="grid gap-4">
            <div className="grid gap-2">
              <Label htmlFor="connection-name">Name</Label>
              <Input
                id="connection-name"
                value={state.name}
                onChange={(event) => setState((current) => ({ ...current, name: event.target.value }))}
                placeholder={activeMode === 'wifi' ? 'Home Wi-Fi' : 'Wired LAN'}
              />
            </div>

            {activeMode === 'wifi' ? (
              <>
                {state.hiddenSSID ? (
                  <div className="grid gap-2">
                    <Label htmlFor="wifi-ssid">Network name (SSID)</Label>
                    <Input
                      id="wifi-ssid"
                      value={state.ssid}
                      onChange={(event) => setState((current) => ({ ...current, ssid: event.target.value }))}
                      placeholder="Hidden network name"
                    />
                  </div>
                ) : (
                  <WifiPicker
                    loading={scan.isLoading || scan.isFetching}
                    available={scan.data?.available ?? false}
                    networks={networks}
                    selected={state.ssid}
                    error={scan.isError ? 'Wi-Fi scanning is unavailable on this host.' : null}
                    onSelect={(ssid, secure) =>
                      setState((current) => ({ ...current, ssid, wifiOpen: !secure, password: '' }))
                    }
                    onRefresh={() => void scan.refetch()}
                    onHidden={() => setState((current) => ({ ...current, hiddenSSID: true, ssid: '' }))}
                  />
                )}
                {state.hiddenSSID ? (
                  <Button variant="ghost" size="sm" className="self-start" onClick={() => setState((current) => ({ ...current, hiddenSSID: false, ssid: '' }))}>
                    Back to scanned networks
                  </Button>
                ) : null}
                {state.ssid ? (
                  <div className="grid gap-3 rounded-lg border bg-muted/30 p-3">
                    <p className="text-sm font-medium">
                      {state.ssid}
                      {state.wifiOpen ? (
                        <span className="ml-2 text-xs font-normal text-muted-foreground">Open network — no password needed</span>
                      ) : null}
                    </p>
                    {!state.wifiOpen ? (
                      <div className="grid gap-2">
                        <Label htmlFor="wifi-password">Password</Label>
                        <Input
                          id="wifi-password"
                          type="password"
                          value={state.password}
                          onChange={(event) => setState((current) => ({ ...current, password: event.target.value }))}
                          placeholder="8–63 characters"
                          autoComplete="new-password"
                        />
                      </div>
                    ) : null}
                    <label className="flex items-center gap-2 text-sm text-muted-foreground">
                      <Switch
                        checked={state.connectNow}
                        onCheckedChange={(checked) => setState((current) => ({ ...current, connectNow: checked }))}
                        aria-label="Connect now"
                      />
                      Connect now using this password
                    </label>
                  </div>
                ) : null}
                {wireless.length > 0 ? (
                  <div className="grid gap-2">
                    <Label>Wi-Fi adapter (optional)</Label>
                    <InterfacePicker
                      interfaces={wireless}
                      value={state.interfaceName}
                      allowAny
                      onChange={(value) => setState((current) => ({ ...current, interfaceName: value }))}
                    />
                  </div>
                ) : null}
              </>
            ) : null}

            {activeMode !== 'wifi' ? (
              <div className="grid gap-2">
                <Label>Interface</Label>
                {pickInterface ? (
                  <InterfacePicker
                    interfaces={activeMode === 'lan' ? wired : usable}
                    value={state.interfaceName}
                    onChange={(value) => setState((current) => ({ ...current, interfaceName: value }))}
                  />
                ) : (
                  <Input
                    value={state.interfaceName}
                    onChange={(event) => setState((current) => ({ ...current, interfaceName: event.target.value }))}
                    placeholder="eth0"
                  />
                )}
              </div>
            ) : null}

            {activeMode === 'manual' ? (
              <>
                <div className="grid gap-2">
                  <Label>Type</Label>
                  <Select value={state.type} onValueChange={(value) => setState((current) => ({ ...current, type: value as ConnectionType }))}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {(['ethernet', 'vlan', 'bond', 'bridge'] as ConnectionType[]).map((value) => (
                        <SelectItem key={value} value={value}>{value}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                {state.type === 'vlan' ? (
                  <div className="grid grid-cols-2 gap-3">
                    <div className="grid gap-2">
                      <Label>Parent interface</Label>
                      <Input value={state.parent} onChange={(event) => setState((current) => ({ ...current, parent: event.target.value }))} placeholder="eth0" />
                    </div>
                    <div className="grid gap-2">
                      <Label>VLAN ID</Label>
                      <Input value={state.vlanId} onChange={(event) => setState((current) => ({ ...current, vlanId: event.target.value }))} placeholder="20" inputMode="numeric" />
                    </div>
                  </div>
                ) : null}
                {state.type === 'bond' || state.type === 'bridge' ? (
                  <div className="grid gap-2">
                    <Label>Members</Label>
                    {usable.length > 0 ? (
                      <div className="grid gap-1.5 rounded-md border p-2">
                        {usable.map((item) => (
                          <label key={item.name} className="flex items-center justify-between rounded px-2 py-1 text-sm hover:bg-accent/50">
                            {item.name}
                            <Switch
                              checked={state.members.includes(item.name)}
                              onCheckedChange={(checked) =>
                                setState((current) => ({
                                  ...current,
                                  members: checked ? [...new Set([...current.members, item.name])] : current.members.filter((name) => name !== item.name),
                                }))
                              }
                              aria-label={`Toggle member ${item.name}`}
                            />
                          </label>
                        ))}
                      </div>
                    ) : (
                      <p className="text-xs text-muted-foreground">No interfaces discovered — members can be added after the appliance service is reachable.</p>
                    )}
                  </div>
                ) : null}
                <div className="grid gap-3">
                  <Label>IPv4 addressing</Label>
                  <Select value={state.ipv4Method} onValueChange={(value) => setState((current) => ({ ...current, ipv4Method: value as IPMethod }))}>
                    <SelectTrigger><SelectValue /></SelectTrigger>
                    <SelectContent>
                      <SelectItem value="auto">Automatic (DHCP)</SelectItem>
                      <SelectItem value="manual">Manual (static)</SelectItem>
                      <SelectItem value="disabled">Disabled</SelectItem>
                    </SelectContent>
                  </Select>
                  {state.ipv4Method === 'manual' ? (
                    <div className="grid gap-2 rounded-md border bg-muted/30 p-3">
                      <div className="grid gap-2">
                        <Label htmlFor="ipv4-address">Address (CIDR)</Label>
                        <Input id="ipv4-address" value={state.ipv4Address} onChange={(event) => setState((current) => ({ ...current, ipv4Address: event.target.value }))} placeholder="192.168.1.10/24" />
                      </div>
                      <div className="grid grid-cols-2 gap-3">
                        <div className="grid gap-2">
                          <Label htmlFor="ipv4-gateway">Gateway</Label>
                          <Input id="ipv4-gateway" value={state.ipv4Gateway} onChange={(event) => setState((current) => ({ ...current, ipv4Gateway: event.target.value }))} placeholder="192.168.1.1" />
                        </div>
                        <div className="grid gap-2">
                          <Label htmlFor="ipv4-metric">Metric (optional)</Label>
                          <Input id="ipv4-metric" value={state.ipv4Metric} onChange={(event) => setState((current) => ({ ...current, ipv4Metric: event.target.value }))} placeholder="100" inputMode="numeric" />
                        </div>
                      </div>
                      <div className="grid gap-2">
                        <Label htmlFor="ipv4-dns">DNS servers (comma separated)</Label>
                        <Input id="ipv4-dns" value={state.ipv4DNS} onChange={(event) => setState((current) => ({ ...current, ipv4DNS: event.target.value }))} placeholder="1.1.1.1, 9.9.9.9" />
                      </div>
                    </div>
                  ) : null}
                  <div className="grid grid-cols-2 gap-3">
                    <div className="grid gap-2">
                      <Label>IPv6</Label>
                      <Select value={state.ipv6Method} onValueChange={(value) => setState((current) => ({ ...current, ipv6Method: value as IPMethod }))}>
                        <SelectTrigger><SelectValue /></SelectTrigger>
                        <SelectContent>
                          <SelectItem value="auto">Automatic</SelectItem>
                          <SelectItem value="manual">Manual</SelectItem>
                          <SelectItem value="disabled">Disabled</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                    <div className="grid gap-2">
                      <Label htmlFor="connection-mtu">MTU (optional)</Label>
                      <Input id="connection-mtu" value={state.mtu} onChange={(event) => setState((current) => ({ ...current, mtu: event.target.value }))} placeholder="1500" inputMode="numeric" />
                    </div>
                  </div>
                </div>
              </>
            ) : null}

            {activeMode === 'lan' ? (
              <details className="rounded-md border bg-muted/30 p-3 text-sm">
                <summary className="cursor-pointer font-medium">Configure manually (static IP)</summary>
                <div className="mt-3 grid gap-2">
                  <div className="grid gap-2">
                    <Label htmlFor="lan-ipv4-address">Address (CIDR)</Label>
                    <Input id="lan-ipv4-address" value={state.ipv4Address} onChange={(event) => setState((current) => ({ ...current, ipv4Address: event.target.value, ipv4Method: event.target.value ? 'manual' : 'auto' }))} placeholder="192.168.1.10/24" />
                  </div>
                  {state.ipv4Method === 'manual' ? (
                    <>
                      <div className="grid gap-2">
                        <Label htmlFor="lan-ipv4-gateway">Gateway</Label>
                        <Input id="lan-ipv4-gateway" value={state.ipv4Gateway} onChange={(event) => setState((current) => ({ ...current, ipv4Gateway: event.target.value }))} placeholder="192.168.1.1" />
                      </div>
                      <div className="grid gap-2">
                        <Label htmlFor="lan-ipv4-dns">DNS servers (comma separated)</Label>
                        <Input id="lan-ipv4-dns" value={state.ipv4DNS} onChange={(event) => setState((current) => ({ ...current, ipv4DNS: event.target.value }))} placeholder="1.1.1.1, 9.9.9.9" />
                      </div>
                    </>
                  ) : null}
                </div>
              </details>
            ) : null}

            <label className="flex items-center justify-between rounded-md border px-3 py-2 text-sm">
              Connect automatically
              <Switch checked={state.enabled} onCheckedChange={(checked) => setState((current) => ({ ...current, enabled: checked }))} aria-label="Connect automatically" />
            </label>
          </div>
        )}

        <DialogFooter>
          {editing ? (
            <Button variant="ghost" onClick={close}>Cancel</Button>
          ) : mode == null ? (
            <Button variant="ghost" onClick={close}>Cancel</Button>
          ) : (
            <Button variant="ghost" onClick={() => { setMode(null); setError(null) }}>Back</Button>
          )}
          {mode != null || editing ? (
            <Button disabled={pending || apply.isPending} onClick={submit}>
              {editing ? 'Save changes' : 'Create connection'}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function ModeCard({ icon, title, description, onClick }: { icon: React.ReactNode; title: string; description: string; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex items-start gap-3 rounded-xl border bg-card p-4 text-left outline-none transition-colors hover:border-primary/50 hover:bg-accent/40 focus-visible:ring-2 focus-visible:ring-ring"
    >
      <span className="mt-0.5 text-primary">{icon}</span>
      <span className="min-w-0">
        <span className="block text-sm font-medium">{title}</span>
        <span className="mt-0.5 block text-xs text-muted-foreground">{description}</span>
      </span>
    </button>
  )
}

function InterfacePicker({
  interfaces,
  value,
  onChange,
  allowAny = false,
}: {
  interfaces: { name: string; up: boolean; addresses: string[] }[]
  value: string
  onChange: (value: string) => void
  allowAny?: boolean
}) {
  return (
    <Select value={value} onValueChange={onChange}>
      <SelectTrigger><SelectValue placeholder="Select an interface" /></SelectTrigger>
      <SelectContent>
        {allowAny ? <SelectItem value="">Any available adapter</SelectItem> : null}
        {interfaces.map((item) => (
          <SelectItem key={item.name} value={item.name}>
            {item.name}
            {item.up ? '' : ' (down)'}
            {item.addresses.length > 0 ? ` — ${item.addresses[0]}` : ''}
          </SelectItem>
        ))}
      </SelectContent>
    </Select>
  )
}

function signalStrength(signal: number): string {
  if (signal >= 75) return 'Excellent'
  if (signal >= 50) return 'Good'
  if (signal >= 25) return 'Fair'
  return 'Weak'
}

function WifiPicker({
  loading,
  available,
  networks,
  selected,
  error,
  onSelect,
  onRefresh,
  onHidden,
}: {
  loading: boolean
  available: boolean
  networks: { ssid: string; signal: number; channel: number; band?: string; secure: boolean }[]
  selected: string
  error: string | null
  onSelect: (ssid: string, secure: boolean) => void
  onRefresh: () => void
  onHidden: () => void
}) {
  return (
    <div className="grid gap-2">
      <div className="flex items-center justify-between">
        <Label>Available networks</Label>
        <Button variant="ghost" size="sm" onClick={onRefresh} disabled={loading}>
          <RefreshCw className={cn(loading && 'animate-spin')} />
          Refresh
        </Button>
      </div>
      {error ? (
        <p className="text-sm text-muted-foreground">{error} Enter the network name manually below.</p>
      ) : !available ? (
        <p className="text-sm text-muted-foreground">No Wi-Fi adapter detected on this host. Enter the network name manually below.</p>
      ) : loading && networks.length === 0 ? (
        <p className="rounded-md border bg-muted/30 p-3 text-sm text-muted-foreground">Scanning for networks…</p>
      ) : networks.length === 0 ? (
        <p className="rounded-md border bg-muted/30 p-3 text-sm text-muted-foreground">No networks found. Try refreshing or enter the name manually.</p>
      ) : (
        <div className="grid max-h-56 gap-1 overflow-y-auto rounded-md border p-1.5">
          {networks.map((network) => (
            <button
              key={network.ssid}
              type="button"
              onClick={() => onSelect(network.ssid, network.secure)}
              className={cn(
                'flex items-center gap-2.5 rounded-md px-2.5 py-2 text-left text-sm outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring',
                selected === network.ssid ? 'bg-accent font-medium text-accent-foreground' : 'hover:bg-secondary',
              )}
            >
              {network.secure ? <Lock className="size-3.5 shrink-0 text-muted-foreground" /> : <LockOpen className="size-3.5 shrink-0 text-muted-foreground" />}
              <span className="min-w-0 flex-1 truncate">{network.ssid}</span>
              <span className="flex shrink-0 items-center gap-1.5 text-xs text-muted-foreground">
                {network.band}
                <Badge variant={network.signal >= 50 ? 'success' : network.signal >= 25 ? 'attention' : 'offline'}>{signalStrength(network.signal)}</Badge>
              </span>
            </button>
          ))}
        </div>
      )}
      <Button variant="ghost" size="sm" className="self-start" onClick={onHidden}>
        Hidden network — enter name manually
      </Button>
    </div>
  )
}
