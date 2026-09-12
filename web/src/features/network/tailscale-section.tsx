import { useState } from 'react'
import { Globe, Power, PowerOff, Route, Server } from 'lucide-react'
import { useTailscaleStatus, useTailscaleUp, useTailscaleDown, useTailscaleExitNode } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function TailscaleSection() {
  const { data: status, isLoading } = useTailscaleStatus()
  const connect = useTailscaleUp()
  const disconnect = useTailscaleDown()
  const exitNode = useTailscaleExitNode()

  const [hostname, setHostname] = useState('')
  const [authKey, setAuthKey] = useState('')
  const [showConnect, setShowConnect] = useState(false)

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
            <Globe className="size-4" />
            Tailscale
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">Loading Tailscale status...</p>
        </CardContent>
      </Card>
    )
  }

  if (status && !status.installed) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
            <Globe className="size-4" />
            Tailscale
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">Tailscale is not installed on this system.</p>
        </CardContent>
      </Card>
    )
  }

  const connected = status?.connected ?? false

  function handleConnect() {
    connect.mutate(
      { hostname: hostname || 'mynas', authKey: authKey || undefined },
      { onSuccess: () => setShowConnect(false) },
    )
  }

  function handleDisconnect() {
    disconnect.mutate()
  }

  function handleExitNode(peerIp: string) {
    exitNode.mutate(peerIp)
  }

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <div>
          <CardTitle className="flex items-center gap-2">
            <Globe className="size-4 text-primary" />
            Tailscale
          </CardTitle>
          <CardDescription>Mesh VPN for secure remote access</CardDescription>
        </div>
        <HealthBadge state={connected ? 'healthy' : status?.running ? 'attention' : 'offline'} />
      </CardHeader>
      <CardContent className="space-y-4">
        {status && connected && (
          <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
            <div>
              <p className="text-xs text-muted-foreground">Hostname</p>
              <p className="font-medium">{status.hostName}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">IPv4</p>
              <p className="font-mono font-medium">{status.tailscaleIp4 || '---'}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">IPv6</p>
              <p className="font-mono font-medium">{status.tailscaleIp6 || '---'}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Version</p>
              <p className="font-medium">{status.version || '---'}</p>
            </div>
          </div>
        )}

        {status && connected && status.exitNode && (
          <div className="flex items-center gap-2 rounded-md border border-warning/30 bg-warning/5 px-3 py-2 text-sm">
            <Route className="size-4 text-warning" />
            <span>Routing through exit node: <strong>{status.exitNode}</strong></span>
            <Button variant="ghost" size="sm" className="ml-auto" onClick={() => handleExitNode('')}>
              Clear
            </Button>
          </div>
        )}

        {status && connected && status.subnetRoutes.length > 0 && (
          <div className="space-y-1">
            <p className="text-xs font-medium text-muted-foreground">Subnet routes</p>
            <div className="flex flex-wrap gap-1.5">
              {status.subnetRoutes.map((route) => (
                <Badge key={route} variant="info" className="font-mono text-xs">{route}</Badge>
              ))}
            </div>
          </div>
        )}

        {!connected && !showConnect && (
          <Button variant="outline" size="sm" onClick={() => setShowConnect(true)}>
            <Power className="size-3.5" />
            Connect
          </Button>
        )}

        {showConnect && (
          <div className="space-y-3 rounded-lg border p-4">
            <div className="space-y-1.5">
              <Label htmlFor="ts-hostname">Hostname</Label>
              <Input
                id="ts-hostname"
                value={hostname}
                onChange={(e) => setHostname(e.target.value)}
                placeholder="mynas"
                className="font-mono text-xs"
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="ts-authkey">Auth key (optional)</Label>
              <Input
                id="ts-authkey"
                type="password"
                value={authKey}
                onChange={(e) => setAuthKey(e.target.value)}
                placeholder="tskey-auth-..."
                className="font-mono text-xs"
              />
            </div>
            <div className="flex gap-2">
              <Button onClick={handleConnect} disabled={connect.isPending}>
                <Power className="size-3.5" />
                {connect.isPending ? 'Connecting...' : 'Connect'}
              </Button>
              <Button variant="ghost" onClick={() => setShowConnect(false)}>
                Cancel
              </Button>
            </div>
          </div>
        )}

        {connected && (
          <div className="space-y-3">
            <div className="flex gap-2">
              <Button variant="outline" size="sm" onClick={handleDisconnect} disabled={disconnect.isPending}>
                <PowerOff className="size-3.5" />
                {disconnect.isPending ? 'Disconnecting...' : 'Disconnect'}
              </Button>
            </div>

            <div className="space-y-2">
              <Label className="text-xs text-muted-foreground">Set exit node</Label>
              <div className="flex gap-2">
                <Input
                  placeholder="Peer IP (e.g. 100.64.0.1)"
                  className="font-mono text-xs"
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      const target = (e.target as HTMLInputElement).value
                      if (target) handleExitNode(target)
                    }
                  }}
                />
              </div>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
