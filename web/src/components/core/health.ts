import {
  CircleAlert,
  CircleCheck,
  CircleMinus,
  CircleX,
  TriangleAlert,
  type LucideIcon,
} from 'lucide-react'
import type { HealthState } from '@/api/types'

export type HealthColor = 'success' | 'attention' | 'warning' | 'critical' | 'offline'

export const HEALTH: Record<
  HealthState,
  { label: string; color: HealthColor; Icon: LucideIcon }
> = {
  healthy: { label: 'Healthy', color: 'success', Icon: CircleCheck },
  attention: { label: 'Attention', color: 'attention', Icon: CircleAlert },
  warning: { label: 'Warning', color: 'warning', Icon: TriangleAlert },
  critical: { label: 'Critical', color: 'critical', Icon: CircleX },
  offline: { label: 'Offline', color: 'offline', Icon: CircleMinus },
}

// Fallback for states that are not in HEALTH (e.g. unexpected API values) so
// a missing mapping can never crash rendering.
export const HEALTH_FALLBACK: { label: string; color: HealthColor; Icon: LucideIcon } = {
  label: 'Unknown',
  color: 'offline',
  Icon: CircleMinus,
}
