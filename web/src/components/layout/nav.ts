import {
  Activity,
  Archive,
  Container,
  FolderOpen,
  HardDrive,
  LayoutDashboard,
  Network,
  Settings,
  Share2,
  Users,
  type LucideIcon,
} from 'lucide-react'

export interface NavItem {
  to: string
  label: string
  Icon: LucideIcon
}

export const NAV_ITEMS: NavItem[] = [
  { to: '/', label: 'Overview', Icon: LayoutDashboard },
  { to: '/storage', label: 'Storage', Icon: HardDrive },
  { to: '/shares', label: 'Shares', Icon: Share2 },
  { to: '/docker', label: 'Docker', Icon: Container },
  { to: '/files', label: 'Files', Icon: FolderOpen },
  { to: '/backups', label: 'Backups', Icon: Archive },
  { to: '/network', label: 'Network', Icon: Network },
  { to: '/monitoring', label: 'Monitoring', Icon: Activity },
  { to: '/users', label: 'Users', Icon: Users },
  { to: '/settings', label: 'Settings', Icon: Settings },
]

export const MOBILE_PRIMARY: NavItem[] = [
  NAV_ITEMS[0],
  NAV_ITEMS[1],
  NAV_ITEMS[3],
]

export const MOBILE_MORE: NavItem[] = NAV_ITEMS.filter(
  (item) => !MOBILE_PRIMARY.includes(item),
)
