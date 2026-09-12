import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, Cable, CheckCircle2, Network as NetworkIcon, Play } from 'lucide-react'
import { apiGet, apiPost } from '@/api/client'
import { PageHeader } from '@/components/core/page-header'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { NetworkMonitoring } from '@/features/network/network-monitoring'
import { WireGuardSection } from '@/features/network/wireguard-section'
import { TailscaleSection } from '@/features/network/tailscale-section'

type Connection = { id: string; name: string; interface: string; type?: string; enabled: boolean; status: string; ipv4: { method: string }; ipv6: { method: string } }
type Binding = { service: string; address: string; port: number; enabled: boolean; scopes?: string[] }
type Firewall = { enabled: boolean; default: string; services: Record<string, { lan: boolean; tailscale: boolean; iot: boolean }> }
type Job = { id: string; state: string; stage?: string; error?: string }

export function NetworkPage() {
  const queryClient = useQueryClient()
  const connections = useQuery({ queryKey: ['admin', 'network', 'connections'], queryFn: () => apiGet<Connection[]>('/network/connections') })
  const bindings = useQuery({ queryKey: ['admin', 'network', 'bindings'], queryFn: () => apiGet<Binding[]>('/network/bindings') })
  const firewall = useQuery({ queryKey: ['admin', 'network', 'firewall'], queryFn: () => apiGet<Firewall>('/network/firewall/policy') })
  const [name, setName] = useState('')
  const [interfaceName, setInterfaceName] = useState('')
  const [type, setType] = useState('ethernet')
  const [diagnostic, setDiagnostic] = useState('interfaces')
  const [target, setTarget] = useState('')
  const [message, setMessage] = useState('')
  const createConnection = useMutation({
    mutationFn: () => apiPost<Connection>('/network/connections', { id: name, name, interface: interfaceName, type, enabled: true, ipv4: { method: 'auto' }, ipv6: { method: 'disabled' }, reauthenticated: true }),
    onSuccess: () => { setName(''); setInterfaceName(''); setMessage('Connection saved; apply it through a checkpoint before it becomes active.'); void queryClient.invalidateQueries({ queryKey: ['admin', 'network'] }) },
    onError: (error) => setMessage(error instanceof Error ? error.message : 'Unable to create connection.'),
  })
  const runDiagnostic = useMutation({
    mutationFn: () => apiPost<Job>('/network/diagnostics', { kind: diagnostic, target }),
    onSuccess: (job) => setMessage(`Diagnostic queued: ${job.id}`),
    onError: (error) => setMessage(error instanceof Error ? error.message : 'Unable to start diagnostic.'),
  })
  const applyConnection = useMutation({
    mutationFn: (connection: Connection) => apiPost(`/network/connections/${connection.id}/apply`, { reauthenticated: true, timeoutSeconds: 60 }),
    onSuccess: () => { setMessage('Network checkpoint started. Confirm it from the pending operation before timeout.'); void queryClient.invalidateQueries({ queryKey: ['admin', 'network', 'connections'] }) },
    onError: (error) => setMessage(error instanceof Error ? error.message : 'Unable to start network checkpoint.'),
  })

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title="Network" description="Manage NetworkManager connections, service exposure, firewall policy, and diagnostics." />
      <NetworkMonitoring />
      <WireGuardSection />
      <TailscaleSection />
      <div className="grid gap-4 xl:grid-cols-[1.4fr_1fr]">
        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><NetworkIcon className="size-4 text-primary" />Connections</CardTitle><CardDescription>Changes use a checkpoint so an unreachable host can roll back safely.</CardDescription></CardHeader>
          <CardContent>
            {connections.isLoading ? <p className="text-sm text-muted-foreground">Loading connections…</p> : connections.isError ? <p className="text-sm text-critical">Unable to load network connections.</p> : <Table><TableHeader><TableRow><TableHead>Name</TableHead><TableHead>Interface</TableHead><TableHead>Addressing</TableHead><TableHead>Status</TableHead><TableHead /></TableRow></TableHeader><TableBody>{connections.data?.map((connection) => <TableRow key={connection.id}><TableCell className="font-medium">{connection.name}<div className="text-xs capitalize text-muted-foreground">{connection.type || 'ethernet'}</div></TableCell><TableCell className="font-mono">{connection.interface}</TableCell><TableCell>IPv4 {connection.ipv4.method} · IPv6 {connection.ipv6.method}</TableCell><TableCell><Badge variant={connection.status.includes('pending') ? 'warning' : 'success'}>{connection.status}</Badge></TableCell><TableCell>{connection.status.includes('pending') ? <Button size="sm" variant="outline" onClick={() => applyConnection.mutate(connection)}><CheckCircle2 />Apply</Button> : null}</TableCell></TableRow>)}</TableBody></Table>}
          </CardContent>
        </Card>
        <Card>
          <CardHeader><CardTitle className="flex items-center gap-2"><Cable className="size-4 text-primary" />Add connection</CardTitle><CardDescription>Reauthentication is required by the API for network mutations.</CardDescription></CardHeader>
          <CardContent className="space-y-3"><Label htmlFor="connection-name">Connection ID and name</Label><Input id="connection-name" value={name} onChange={(event) => setName(event.target.value)} placeholder="lan" /><Label htmlFor="connection-interface">Interface</Label><Input id="connection-interface" value={interfaceName} onChange={(event) => setInterfaceName(event.target.value)} placeholder="eth0" /><Label>Type</Label><Select value={type} onValueChange={setType}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{['ethernet', 'vlan', 'bond', 'bridge'].map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select><Button className="w-full" disabled={!name || !interfaceName || createConnection.isPending} onClick={() => createConnection.mutate()}><CheckCircle2 />Save checkpointed connection</Button></CardContent>
        </Card>
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card><CardHeader><CardTitle className="flex items-center gap-2"><Activity className="size-4 text-primary" />Diagnostics</CardTitle><CardDescription>Run bounded reachability and local network checks as background jobs.</CardDescription></CardHeader><CardContent className="space-y-3"><Select value={diagnostic} onValueChange={setDiagnostic}><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>{['interfaces', 'route-table', 'neighbor-table', 'ping', 'dns-lookup', 'traceroute', 'gateway-reachability', 'internet-reachability', 'update-endpoint', 'docker-registry', 'port-test'].map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectContent></Select>{!['interfaces', 'route-table', 'neighbor-table'].includes(diagnostic) ? <><Label htmlFor="diagnostic-target">Target</Label><Input id="diagnostic-target" value={target} onChange={(event) => setTarget(event.target.value)} placeholder="1.1.1.1 or example.com" /></> : null}<Button onClick={() => runDiagnostic.mutate()} disabled={runDiagnostic.isPending || (!['interfaces', 'route-table', 'neighbor-table'].includes(diagnostic) && !target)}><Play />Run diagnostic</Button></CardContent></Card>
        <Card><CardHeader><CardTitle>Exposure and firewall</CardTitle><CardDescription>Current service bindings and default policy.</CardDescription></CardHeader><CardContent className="space-y-3"><div className="flex items-center justify-between rounded-md border px-3 py-2"><span>Default firewall policy</span><Badge variant={firewall.data?.default === 'deny' ? 'success' : 'warning'}>{firewall.data?.default ?? 'loading'}</Badge></div><div className="flex flex-wrap gap-2">{bindings.data?.map((binding) => <Badge key={binding.service} variant={binding.enabled ? 'info' : 'offline'}>{binding.service}:{binding.port}</Badge>)}</div><p className="text-xs text-muted-foreground">Bindings: {bindings.data?.length ?? 0} · Firewall: {firewall.data?.enabled ? 'enabled' : 'disabled'}</p></CardContent></Card>
      </div>
      {message ? <p className="text-sm text-muted-foreground" role="status">{message}</p> : null}
    </div>
  )
}
