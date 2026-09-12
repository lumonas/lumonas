export interface NetworkConnection {
  id: string
  name: string
  interface: string
  type?: string
  enabled: boolean
  status: string
  ipv4: { method: string; addresses?: string[]; gateway?: string; dns?: string[] }
  ipv6: { method: string }
  ssid?: string
  wifiOpen?: boolean
  mtu?: number
  parent?: string
  vlanId?: number
  members?: string[]
}

export interface NetworkInterface {
  name: string
  mac?: string
  mtu: number
  up: boolean
  loopback: boolean
  wireless: boolean
  addresses: string[]
}

export const interfaces: NetworkInterface[] = [
  { name: 'eth0', mac: 'aa:bb:cc:dd:ee:01', mtu: 1500, up: true, loopback: false, wireless: false, addresses: ['192.168.1.50/24'] },
  { name: 'eth1', mac: 'aa:bb:cc:dd:ee:02', mtu: 1500, up: false, loopback: false, wireless: false, addresses: [] },
  { name: 'wlan0', mac: 'aa:bb:cc:dd:ee:03', mtu: 1500, up: true, loopback: false, wireless: true, addresses: [] },
  { name: 'lo', mtu: 65536, up: true, loopback: true, wireless: false, addresses: ['127.0.0.1/8'] },
]

export interface WiFiNetwork {
  ssid: string
  signal: number
  channel: number
  band?: string
  security: string
  secure: boolean
}

export const wifiNetworks: WiFiNetwork[] = [
  { ssid: 'HomeNet', signal: 82, channel: 36, band: '5 GHz', security: 'WPA2', secure: true },
  { ssid: 'HomeNet-IoT', signal: 74, channel: 6, band: '2.4 GHz', security: 'WPA2', secure: true },
  { ssid: 'FreeGuest', signal: 61, channel: 11, band: '2.4 GHz', security: '', secure: false },
  { ssid: 'Neighbour_5G', signal: 33, channel: 44, band: '5 GHz', security: 'WPA3', secure: true },
]

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
  {
    id: 'wifi-home',
    name: 'Home Wi-Fi',
    interface: 'wlan0',
    type: 'wifi',
    enabled: true,
    status: 'activated',
    ssid: 'HomeNet',
    ipv4: { method: 'auto' },
    ipv6: { method: 'disabled' },
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
