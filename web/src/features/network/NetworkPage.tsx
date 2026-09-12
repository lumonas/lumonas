import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Activity, Cable, CheckCircle2, KeyRound, Lock, Pencil, Play, Plus, Wifi } from 'lucide-react'
import { apiGet, apiPost } from '@/api/client'
import {
  queryKeys,
  useApplyNetworkConnection,
  useNetworkConnections,
} from '@/api/queries'
import type { NetworkConnection } from '@/api/types'
import { AlertBanner } from '@/components/core/alert-banner'
import { PageHeader } from '@/components/core/page-header'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { ConnectionWizard } from '@/features/network/connection-wizard'

type Binding = { service: string; address: string; port: number; enabled: boolean; scopes?: string[] }
type Firewall = { enabled: boolean; default: string; services: Record<string, { lan: boolean; tailscale: boolean; iot: boolean }> }
type Job = { id: string; state: string; stage?: string; error?: string }

function statusVariant(status: string): 'success' | 'attention' | 'warning' | 'info' {
  if (status.includes('activated') || status === 'configured') return 'success'
  if (status.includes('pending') || status.includes('checkpoint')) return 'warning'
  if (status.includes('activating')) return 'attention'
  return 'info'
}

function needsApply(connection: NetworkConnection): boolean {
  return connection.status.includes('pending') || connection.status.includes('checkpoint') || connection.status === 'activating'
}

export function NetworkPage() {
  const connections = useNetworkConnections()
  const bindings = useQuery({ queryKey: queryKeys.networkBindings, queryFn: () => apiGet<Binding[]>('/network/bindings'), throwOnError: false })
  const firewall = useQuery({ queryKey: queryKeys.networkFirewall, queryFn: () => apiGet<Firewall>('/network/firewall/policy'), throwOnError: false })
  const apply = useApplyNetworkConnection()

  const [wizardOpen, setWizardOpen] = useState(false)
  const [editTarget, setEditTarget] = useState<NetworkConnection | null>(null)
  const [passwordTarget, setPasswordTarget] = useState<NetworkConnection | null>(null)
  const [password, setPassword] = useState('')
  const [applyError, setApplyError] = useState<string | null>(null)

  const [diagnostic, setDiagnostic] = useState('interfaces')
  const [target, setTarget] = useState('')
  const [message, setMessage] = useState('')
  const runDiagnostic = useMutation({
    mutationFn: () => apiPost<Job>('/network/diagnostics', { kind: diagnostic, target }),
    onSuccess: (job) => setMessage(`Diagnostic queued: ${job.id}`),
    onError: (error) => setMessage(error instanceof Error ? error.message : 'Unable to start diagnostic.'),
  })

  function startApply(connection: NetworkConnection) {
    setApplyError(null)
    if (connection.type === 'wifi' && !connection.wifiOpen) {
      setPassword('')
      setPasswordTarget(connection)
      return
    }
    apply.mutate(
      { id: connection.id, ...(connection.type === 'wifi' ? { wifiPassword: '' } : {}) },
      {
        onError: (error) => setApplyError(error.message),
      },
    )
  }

  function submitPassword() {
    if (!passwordTarget) return
    apply.mutate(
      { id: passwordTarget.id, wifiPassword: password },
      {
        onSuccess: () => {
          setPasswordTarget(null)
          setPassword('')
          setApplyError(null)
        },
        onError: (error) => setApplyError(error.message),
      },
    )
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Network"
        description="Set up wired and Wi-Fi connections, service exposure, firewall policy, and diagnostics."
        actions={
          <Button onClick={() => { setEditTarget(null); setWizardOpen(true) }}>
            <Plus />
            Set up connection
          </Button>
        }
      />
      <div className="grid gap-4 xl:grid-cols-[1.4fr_1fr]">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><Cable className="size-4 text-primary" />Connections</CardTitle>
            <CardDescription>Changes use a checkpoint so an unreachable host can roll back safely.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {connections.isError ? (
              <AlertBanner tone="critical" title="Unable to load network connections">
                {connections.error instanceof Error ? connections.error.message : null}
              </AlertBanner>
            ) : null}
            {applyError ? (
              <AlertBanner tone="warning" title="Connection change needs attention">
                {applyError}
              </AlertBanner>
            ) : null}
            {connections.isLoading ? (
              <p className="text-sm text-muted-foreground">Loading connections…</p>
            ) : (connections.data?.length ?? 0) === 0 ? (
              <p className="rounded-md border bg-muted/30 p-3 text-sm text-muted-foreground">
                No connections yet — use “Set up connection” to add wired LAN or Wi-Fi.
              </p>
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Name</TableHead>
                    <TableHead>Interface</TableHead>
                    <TableHead>Addressing</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {connections.data?.map((connection) => (
                    <TableRow key={connection.id}>
                      <TableCell className="font-medium">
                        <span className="flex items-center gap-1.5">
                          {connection.type === 'wifi' ? <Wifi className="size-3.5 text-muted-foreground" /> : <Cable className="size-3.5 text-muted-foreground" />}
                          {connection.name}
                        </span>
                        <div className="text-xs capitalize text-muted-foreground">
                          {connection.type || 'ethernet'}
                          {connection.type === 'wifi' && connection.ssid ? (
                            <span className="ml-1 inline-flex items-center gap-0.5">
                              <Lock className="size-3" />
                              {connection.wifiOpen ? 'open' : 'secured'} · {connection.ssid}
                            </span>
                          ) : null}
                        </div>
                      </TableCell>
                      <TableCell className="font-mono">{connection.interface || '—'}</TableCell>
                      <TableCell>IPv4 {connection.ipv4.method} · IPv6 {connection.ipv6.method}</TableCell>
                      <TableCell><Badge variant={statusVariant(connection.status)}>{connection.status}</Badge></TableCell>
                      <TableCell>
                        <div className="flex justify-end gap-1">
                          {needsApply(connection) ? (
                            <Button size="sm" variant="outline" onClick={() => startApply(connection)} disabled={apply.isPending}>
                              <CheckCircle2 />
                              Apply
                            </Button>
                          ) : null}
                          <Button size="icon-sm" variant="ghost" aria-label={`Edit ${connection.name}`} onClick={() => { setEditTarget(connection); setWizardOpen(true) }}>
                            <Pencil />
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><Plus className="size-4 text-primary" />Basic setup</CardTitle>
            <CardDescription>Wired LAN in one click, Wi-Fi with network scan, or full manual control.</CardDescription>
          </CardHeader>
          <CardContent className="grid gap-3">
            <Button onClick={() => { setEditTarget(null); setWizardOpen(true) }}>
              <Cable />
              Set up wired or Wi-Fi connection
            </Button>
            <p className="text-xs text-muted-foreground">
              Saved connections stay pending until applied — that checkpoint keeps an unreachable host from locking you out.
            </p>
          </CardContent>
        </Card>
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2"><Activity className="size-4 text-primary" />Diagnostics</CardTitle>
            <CardDescription>Run bounded reachability and local network checks as background jobs.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <Select value={diagnostic} onValueChange={setDiagnostic}>
              <SelectTrigger><SelectValue /></SelectTrigger>
              <SelectContent>
                {['interfaces', 'route-table', 'neighbor-table', 'ping', 'dns-lookup', 'traceroute', 'gateway-reachability', 'internet-reachability', 'update-endpoint', 'docker-registry', 'port-test'].map((value) => (
                  <SelectItem key={value} value={value}>{value}</SelectItem>
                ))}
              </SelectContent>
            </Select>
            {!['interfaces', 'route-table', 'neighbor-table'].includes(diagnostic) ? (
              <div className="grid gap-2">
                <Label htmlFor="diagnostic-target">Target</Label>
                <Input id="diagnostic-target" value={target} onChange={(event) => setTarget(event.target.value)} placeholder="1.1.1.1 or example.com" />
              </div>
            ) : null}
            <Button onClick={() => runDiagnostic.mutate()} disabled={runDiagnostic.isPending || (!['interfaces', 'route-table', 'neighbor-table'].includes(diagnostic) && !target)}>
              <Play />
              Run diagnostic
            </Button>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>Exposure and firewall</CardTitle>
            <CardDescription>Current service bindings and default policy.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex items-center justify-between rounded-md border px-3 py-2">
              <span>Default firewall policy</span>
              <Badge variant={firewall.data?.default === 'deny' ? 'success' : 'warning'}>{firewall.data?.default ?? 'loading'}</Badge>
            </div>
            <div className="flex flex-wrap gap-2">
              {bindings.data?.map((binding) => (
                <Badge key={binding.service} variant={binding.enabled ? 'info' : 'offline'}>{binding.service}:{binding.port}</Badge>
              ))}
            </div>
            <p className="text-xs text-muted-foreground">Bindings: {bindings.data?.length ?? 0} · Firewall: {firewall.data?.enabled ? 'enabled' : 'disabled'}</p>
          </CardContent>
        </Card>
      </div>
      {message ? <p className="text-sm text-muted-foreground" role="status">{message}</p> : null}

      <ConnectionWizard
        key={editTarget?.id ?? 'create'}
        open={wizardOpen}
        onOpenChange={setWizardOpen}
        connection={editTarget}
      />

      <Dialog open={passwordTarget != null} onOpenChange={(open) => { if (!open) { setPasswordTarget(null); setPassword('') } }}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <KeyRound className="size-4 text-primary" />
              Wi-Fi password
            </DialogTitle>
            <DialogDescription>
              Enter the password for “{passwordTarget?.ssid}” to start the connection. It is sent once and never stored.
            </DialogDescription>
          </DialogHeader>
          {applyError ? (
            <AlertBanner tone="warning" title="Could not start the connection">
              {applyError}
            </AlertBanner>
          ) : null}
          <div className="grid gap-2">
            <Label htmlFor="apply-password">Password</Label>
            <Input
              id="apply-password"
              type="password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              placeholder="8–63 characters"
              autoComplete="current-password"
            />
          </div>
          <DialogFooter>
            <Button variant="ghost" onClick={() => { setPasswordTarget(null); setPassword('') }}>Cancel</Button>
            <Button onClick={submitPassword} disabled={password.length < 8 || apply.isPending}>Connect</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
