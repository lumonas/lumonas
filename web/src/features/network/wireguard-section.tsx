import { useState } from 'react'
import { Key, Plus, Shield, Trash2, Copy } from 'lucide-react'
import { useWireGuardStatus, useApplyWireGuard, useWireGuardKeygen } from '@/api/queries'
import { HealthBadge } from '@/components/core/health-badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from 'sonner'
import type { WireGuardPeer } from '@/api/types'

export function WireGuardSection() {
  const { data: status, isLoading } = useWireGuardStatus()
  const applyConfig = useApplyWireGuard()
  const keygen = useWireGuardKeygen()

  const [showConfig, setShowConfig] = useState(false)
  const [privateKey, setPrivateKey] = useState('')
  const [address, setAddress] = useState('10.0.0.1/24')
  const [listenPort, setListenPort] = useState('51820')
  const [dns, setDns] = useState('')
  const [peers, setPeers] = useState<WireGuardPeer[]>([])
  const [newPeerKey, setNewPeerKey] = useState('')
  const [newPeerEndpoint, setNewPeerEndpoint] = useState('')
  const [newPeerAllowedIPs, setNewPeerAllowedIPs] = useState('10.0.0.2/32')

  if (isLoading) {
    return (
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-sm font-medium text-muted-foreground">
            <Shield className="size-4" />
            WireGuard
          </CardTitle>
        </CardHeader>
        <CardContent>
          <p className="text-sm text-muted-foreground">Loading WireGuard status...</p>
        </CardContent>
      </Card>
    )
  }

  const connected = status?.connected ?? false

  function handleKeygen() {
    keygen.mutate(undefined, {
      onSuccess: (data) => {
        setPrivateKey(data.privateKey)
        toast.success('Keys generated - copy the private key and apply')
      },
    })
  }

  function handleAddPeer() {
    if (!newPeerKey) return
    setPeers([
      ...peers,
      {
        publicKey: newPeerKey,
        endpoint: newPeerEndpoint || undefined,
        allowedIps: newPeerAllowedIPs.split(',').map((s) => s.trim()).filter(Boolean),
        persistentKeepalive: 25,
      },
    ])
    setNewPeerKey('')
    setNewPeerEndpoint('')
    setNewPeerAllowedIPs('10.0.0.2/32')
  }

  function handleRemovePeer(index: number) {
    setPeers(peers.filter((_, i) => i !== index))
  }

  function handleApply() {
    applyConfig.mutate({
      interface: status?.interface || 'wg0',
      privateKey: privateKey || undefined,
      address: address.split(',').map((s) => s.trim()).filter(Boolean),
      listenPort: parseInt(listenPort, 10) || 51820,
      dns: dns ? dns.split(',').map((s) => s.trim()).filter(Boolean) : undefined,
      peers,
    })
  }

  function copyToClipboard(text: string) {
    void navigator.clipboard.writeText(text)
    toast.success('Copied to clipboard')
  }

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between space-y-0">
        <div>
          <CardTitle className="flex items-center gap-2">
            <Shield className="size-4 text-primary" />
            WireGuard
          </CardTitle>
          <CardDescription>Point-to-site VPN tunnel configuration</CardDescription>
        </div>
        <HealthBadge state={connected ? 'healthy' : 'offline'} />
      </CardHeader>
      <CardContent className="space-y-4">
        {status && (
          <div className="grid grid-cols-2 gap-3 text-sm sm:grid-cols-4">
            <div>
              <p className="text-xs text-muted-foreground">Interface</p>
              <p className="font-mono font-medium">{status.interface}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">IP</p>
              <p className="font-mono font-medium">{status.ip || '---'}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Listen port</p>
              <p className="font-mono font-medium">{status.listenPort || '---'}</p>
            </div>
            <div>
              <p className="text-xs text-muted-foreground">Peers</p>
              <p className="font-mono font-medium">{status.peers}</p>
            </div>
          </div>
        )}

        <div className="flex gap-2">
          <Button variant="outline" size="sm" onClick={handleKeygen} disabled={keygen.isPending}>
            <Key className="size-3.5" />
            {keygen.isPending ? 'Generating...' : 'Generate keys'}
          </Button>
          <Button variant="outline" size="sm" onClick={() => setShowConfig(!showConfig)}>
            {showConfig ? 'Hide config' : 'Configure'}
          </Button>
        </div>

        {showConfig && (
          <div className="space-y-4 rounded-lg border p-4">
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label>Private key</Label>
                <div className="flex gap-1.5">
                  <Input
                    type="password"
                    value={privateKey}
                    onChange={(e) => setPrivateKey(e.target.value)}
                    placeholder="Generated or pasted manually"
                    className="font-mono text-xs"
                  />
                  {privateKey && (
                    <Button variant="ghost" size="icon" className="shrink-0" onClick={() => copyToClipboard(privateKey)}>
                      <Copy className="size-3.5" />
                    </Button>
                  )}
                </div>
              </div>
              <div className="space-y-1.5">
                <Label>Address (CIDR)</Label>
                <Input value={address} onChange={(e) => setAddress(e.target.value)} placeholder="10.0.0.1/24" className="font-mono text-xs" />
              </div>
              <div className="space-y-1.5">
                <Label>Listen port</Label>
                <Input value={listenPort} onChange={(e) => setListenPort(e.target.value)} placeholder="51820" className="font-mono text-xs" />
              </div>
              <div className="space-y-1.5">
                <Label>DNS (comma-separated)</Label>
                <Input value={dns} onChange={(e) => setDns(e.target.value)} placeholder="1.1.1.1" className="font-mono text-xs" />
              </div>
            </div>

            <div className="space-y-2">
              <Label>Peers</Label>
              {peers.length > 0 && (
                <div className="space-y-2">
                  {peers.map((peer, i) => (
                    <div key={i} className="flex items-center gap-2 rounded-md border px-3 py-2 text-xs">
                      <span className="min-w-0 flex-1 truncate font-mono">{peer.publicKey}</span>
                      {peer.endpoint && <span className="text-muted-foreground">{peer.endpoint}</span>}
                      <span className="text-muted-foreground">{peer.allowedIps.join(', ')}</span>
                      <Button variant="ghost" size="icon" className="shrink-0" onClick={() => handleRemovePeer(i)}>
                        <Trash2 className="size-3.5 text-muted-foreground" />
                      </Button>
                    </div>
                  ))}
                </div>
              )}

              <div className="grid gap-2 sm:grid-cols-[1fr_1fr_1fr_auto]">
                <Input
                  value={newPeerKey}
                  onChange={(e) => setNewPeerKey(e.target.value)}
                  placeholder="Public key"
                  className="font-mono text-xs"
                />
                <Input
                  value={newPeerEndpoint}
                  onChange={(e) => setNewPeerEndpoint(e.target.value)}
                  placeholder="Endpoint (optional)"
                  className="font-mono text-xs"
                />
                <Input
                  value={newPeerAllowedIPs}
                  onChange={(e) => setNewPeerAllowedIPs(e.target.value)}
                  placeholder="Allowed IPs"
                  className="font-mono text-xs"
                />
                <Button variant="outline" size="icon" onClick={handleAddPeer} disabled={!newPeerKey}>
                  <Plus className="size-3.5" />
                </Button>
              </div>
            </div>

            <Button className="w-full" onClick={handleApply} disabled={applyConfig.isPending}>
              <Shield className="size-3.5" />
              {applyConfig.isPending ? 'Applying...' : 'Apply WireGuard config'}
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  )
}
