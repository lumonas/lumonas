import { useState } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { Activity, Cable, CheckCircle2, Globe, KeyRound, Lock, Pencil, Play, Plus, Shield, Timer, Wifi } from 'lucide-react'
import { apiGet, apiPost } from '@/api/client'
import {
  queryKeys,
  useApplyNetworkConnection,
  useBeginCheckpoint,
  useCheckpointAction,
  useNetworkConnections,
} from '@/api/queries'
import type { NetworkConnection } from '@/api/types'
import { AlertBanner } from '@/components/core/alert-banner'
import { PageHeader } from '@/components/core/page-header'
import { WireGuardSection } from '@/features/network/wireguard-section'
import { TailscaleSection } from '@/features/network/tailscale-section'
import { LanHostsSection } from '@/features/network/lan-section'
import { NetworkMonitoring } from '@/features/network/network-monitoring'
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useSearchParams } from 'react-router-dom'
import { ConnectionWizard } from '@/features/network/connection-wizard'

type Binding = { service: string; address: string; port: number; enabled: boolean; scopes?: string[] }
type Firewall = { enabled: boolean; default: string; services: Record<string, { lan: boolean; tailscale: boolean; iot: boolean }> }
type Job = { id: string; state: string; stage?: string; error?: string }
type DiagnosticJob = Job & { kind: string }
type DiagnosticResponse = { job: Job; result?: unknown }

const DIAGNOSTIC_KINDS = [
  'interfaces',
  'route-table',
  'neighbor-table',
  'ping',
  'dns-lookup',
  'traceroute',
  'gateway-reachability',
  'internet-reachability',
  'update-endpoint',
  'docker-registry',
  'port-test',
]

const TABS = [
  { value: 'connections', label: 'Connections' },
  { value: 'lan', label: 'LAN' },
  { value: 'vpn', label: 'VPN' },
  { value: 'diagnostics', label: 'Diagnostics' },
] as const

function statusVariant(status: string): 'success' | 'attention' | 'warning' | 'info' {
  if (status.includes('activated') || status === 'configured') return 'success'
  if (status.includes('pending') || status.includes('checkpoint')) return 'warning'
  if (status.includes('activating')) return 'attention'
  return 'info'
}

function needsApply(connection: NetworkConnection): boolean {
  return connection.status.includes('pending') || connection.status.includes('checkpoint') || connection.status === 'activating'
}

function DiagnosticResultView({ kind, result }: { kind: string; result: unknown }) {
  if (result == null) return null
  if (kind === 'dns-lookup' && Array.isArray(result)) {
    return (
      <div className="flex flex-wrap gap-1.5">
        {result.map((address) => (
          <Badge key={String(address)} variant="info" className="font-mono">{String(address)}</Badge>
        ))}
      </div>
    )
  }
  if (typeof result === 'object' && 'reachable' in (result as Record<string, unknown>)) {
    const outcome = result as { target?: string; port?: number; reachable?: boolean }
    return (
      <p className="flex items-center gap-2 text-sm">
        {outcome.reachable ? (
          <CheckCircle2 className="size-4 text-success" />
        ) : (
          <Globe className="size-4 text-critical" />
        )}
        <span className="font-mono text-xs">{outcome.target}{outcome.port ? `:${outcome.port}` : ''}</span>
        <span className="text-muted-foreground">{outcome.reachable ? 'reachable' : 'unreachable'}</span>
      </p>
    )
  }
  if (typeof result === 'object' && 'output' in (result as Record<string, unknown>)) {
    return (
      <pre className="max-h-64 overflow-auto rounded-md border bg-muted/30 p-3 font-mono text-xs">
        {String((result as { output?: unknown }).output ?? '')}
      </pre>
    )
  }
  if (typeof result === 'string') {
    return (
      <pre className="max-h-64 overflow-auto rounded-md border bg-muted/30 p-3 font-mono text-xs">{result}</pre>
    )
  }
  if (Array.isArray(result)) {
    return (
      <ul className="flex flex-col gap-1 font-mono text-xs">
        {result.map((entry) => (
          <li key={JSON.stringify(entry)}>{typeof entry === 'object' ? JSON.stringify(entry) : String(entry)}</li>
        ))}
      </ul>
    )
  }
  return (
    <pre className="max-h-64 overflow-auto rounded-md border bg-muted/30 p-3 font-mono text-xs">
      {JSON.stringify(result, null, 2)}
    </pre>
  )
}

function CheckpointsCard() {
  const begin = useBeginCheckpoint()
  const action = useCheckpointAction()
  const [checkpointId, setCheckpointId] = useState<string | null>(null)

  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
          <Timer className="size-4" />
          Network checkpoint
        </CardTitle>
        <CardDescription>
          A checkpoint snapshots the current network state. Commit to keep changes, roll back to
          restore the previous configuration — a safety net for remote IP changes.
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap items-center gap-2">
        {checkpointId ? (
          <>
            <Badge variant="warning" className="font-mono">{checkpointId.slice(0, 18)}…</Badge>
            <Button
              size="sm"
              onClick={() =>
                action.mutate(
                  { id: checkpointId, action: 'commit' },
                  { onSuccess: () => setCheckpointId(null) },
                )
              }
              disabled={action.isPending}
            >
              <CheckCircle2 />
              Commit
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() =>
                action.mutate(
                  { id: checkpointId, action: 'rollback' },
                  { onSuccess: () => setCheckpointId(null) },
                )
              }
              disabled={action.isPending}
            >
              Roll back
            </Button>
          </>
        ) : (
          <>
            <p className="flex-1 text-xs text-muted-foreground">No open checkpoint in this session.</p>
            <Button size="sm" variant="outline" onClick={() => begin.mutate(undefined, { onSuccess: (checkpoint) => setCheckpointId(checkpoint.operationId) })} disabled={begin.isPending}>
              <Plus />
              Open checkpoint
            </Button>
          </>
        )}
      </CardContent>
    </Card>
  )
}

export function NetworkPage() {
  const [searchParams, setSearchParams] = useSearchParams()
  const tabParam = searchParams.get('tab')
  const tab = tabParam != null && TABS.some((t) => t.value === tabParam) ? tabParam : 'connections'
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
  const [diagnosticJob, setDiagnosticJob] = useState<DiagnosticJob | null>(null)
  const runDiagnostic = useMutation({
    mutationFn: () => apiPost<Job>('/network/diagnostics', { kind: diagnostic, target }),
    onSuccess: (job) => setDiagnosticJob({ ...job, kind: diagnostic }),
    onError: () => setDiagnosticJob(null),
  })
  const diagnosticResult = useQuery({
    queryKey: queryKeys.networkDiagnostic(diagnosticJob?.id ?? ''),
    queryFn: () => apiGet<DiagnosticResponse>(`/network/diagnostics/${diagnosticJob!.id}`),
    enabled: diagnosticJob != null,
    refetchInterval: (query) => {
      const state = query.state.data?.job.state
      if (state === 'successful' || state === 'failed' || state === 'cancelled') return false
      return 1500
    },
  })

  function setParam(key: string, value: string | null) {
    const next = new URLSearchParams(searchParams)
    if (value == null) next.delete(key)
    else next.set(key, value)
    setSearchParams(next, { replace: true })
  }

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

  const diagnosticDone = diagnosticResult.data?.job.state != null && ['successful', 'failed', 'cancelled'].includes(diagnosticResult.data.job.state)

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Network"
        description="Connections, VPN, service exposure, firewall policy, and diagnostics."
        actions={
          <Button onClick={() => { setEditTarget(null); setWizardOpen(true) }}>
            <Plus />
            Set up connection
          </Button>
        }
      />
      <Tabs value={tab} onValueChange={(value) => setParam('tab', value)} className="gap-6">
        <TabsList className="w-full justify-start overflow-x-auto sm:w-fit">
          {TABS.map((t) => (
            <TabsTrigger key={t.value} value={t.value}>{t.label}</TabsTrigger>
          ))}
        </TabsList>

        <TabsContent value="connections" className="flex flex-col gap-4">
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
          <Card>
            <CardHeader>
              <CardTitle className="flex items-center gap-2"><Shield className="size-4 text-primary" />Exposure and firewall</CardTitle>
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
        </TabsContent>

        <TabsContent value="lan" className="flex flex-col gap-4">
          <LanHostsSection />
        </TabsContent>

        <TabsContent value="vpn" className="flex flex-col gap-4">
          <WireGuardSection />
          <TailscaleSection />
        </TabsContent>

        <TabsContent value="diagnostics" className="flex flex-col gap-4">
          <div className="grid gap-4 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="flex items-center gap-2"><Activity className="size-4 text-primary" />Run a diagnostic</CardTitle>
                <CardDescription>Bounded reachability and local network checks as background jobs.</CardDescription>
              </CardHeader>
              <CardContent className="space-y-3">
                <Select value={diagnostic} onValueChange={setDiagnostic}>
                  <SelectTrigger><SelectValue /></SelectTrigger>
                  <SelectContent>
                    {DIAGNOSTIC_KINDS.map((value) => (
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
                <CardTitle>Result</CardTitle>
                <CardDescription>
                  {diagnosticJob
                    ? diagnosticDone
                      ? diagnosticResult.data?.job.state === 'successful'
                        ? 'Completed.'
                        : `Job ${diagnosticResult.data?.job.state ?? diagnosticJob.state}.`
                      : 'Running…'
                    : 'Run a diagnostic to see its output here.'}
                </CardDescription>
              </CardHeader>
              <CardContent>
                {diagnosticJob && diagnosticResult.data?.job.state === 'failed' ? (
                  <AlertBanner tone="warning" title="Diagnostic failed">
                    {diagnosticResult.data.job.error ?? 'The diagnostic job did not complete.'}
                  </AlertBanner>
                ) : null}
                {diagnosticJob && diagnosticDone && diagnosticResult.data?.job.state === 'successful' ? (
                  <DiagnosticResultView kind={diagnosticJob.kind} result={diagnosticResult.data?.result} />
                ) : null}
                {!diagnosticJob ? (
                  <p className="text-sm text-muted-foreground">No diagnostic has been run in this session.</p>
                ) : null}
              </CardContent>
            </Card>
          </div>
          <NetworkMonitoring />
          <CheckpointsCard />
        </TabsContent>
      </Tabs>

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
