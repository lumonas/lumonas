export interface NetworkConnection {
  id: string
  name: string
  interface: string
  type?: string
  enabled: boolean
  status: string
  ipv4: { method: string }
  ipv6: { method: string }
}

export interface ServiceBinding {
  service: string
  address: string
  port: number
  enabled: boolean
  scopes?: string[]
}

export interface FirewallPolicy {
  enabled: boolean
  default: string
  services: Record<string, { lan: boolean; tailscale: boolean; iot: boolean }>
}

export const connections: NetworkConnection[] = [
  {
    id: 'wan-lan',
    name: 'Wired LAN',
    interface: 'eth0',
    type: 'ethernet',
    enabled: true,
    status: 'activated',
    ipv4: { method: 'auto' },
    ipv6: { method: 'auto' },
  },
  {
    id: 'vlan-iot',
    name: 'IoT VLAN 20',
    interface: 'eth0.20',
    type: 'vlan',
    enabled: true,
    status: 'activating (checkpoint — auto-rollback in 60s)',
    ipv4: { method: 'manual' },
    ipv6: { method: 'disabled' },
  },
  {
    id: 'tailscale',
    name: 'Tailscale',
    interface: 'tailscale0',
    type: 'tun',
    enabled: true,
    status: 'activated',
    ipv4: { method: 'auto' },
    ipv6: { method: 'auto' },
  },
]

export const bindings: ServiceBinding[] = [
  { service: 'SMB', address: '0.0.0.0', port: 445, enabled: true, scopes: ['lan', 'tailscale'] },
  { service: 'NFS', address: '0.0.0.0', port: 2049, enabled: true, scopes: ['lan'] },
  { service: 'SSH', address: '0.0.0.0', port: 22, enabled: true, scopes: ['lan', 'tailscale'] },
  { service: 'HTTP (UI)', address: '0.0.0.0', port: 80, enabled: true, scopes: ['lan'] },
  { service: 'FTPS', address: '0.0.0.0', port: 990, enabled: false, scopes: [] },
]

export const firewall: FirewallPolicy = {
  enabled: true,
  default: 'deny',
  services: {
    SMB: { lan: true, tailscale: true, iot: false },
    NFS: { lan: true, tailscale: false, iot: false },
    SSH: { lan: true, tailscale: true, iot: false },
    HTTP: { lan: true, tailscale: true, iot: false },
  },
}

export function applyConnection(id: string): NetworkConnection | undefined {
  const connection = connections.find((c) => c.id === id)
  if (!connection) return undefined
  connection.status = 'activating (checkpoint — auto-rollback in 60s)'
  setTimeout(() => {
    connection.status = 'activated'
    connection.ipv4 = { method: connection.ipv4.method }
  }, 3000)
  return connection
}

export interface DiagnosticJobStub {
  id: string
  state: string
  stage?: string
  error?: string
}

export function createDiagnosticJob(kind: string, _target?: string): DiagnosticJobStub {
  return {
    id: `job-diag-${Date.now()}`,
    state: 'running',
    stage: kind,
  }
}
