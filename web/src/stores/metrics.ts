import { create } from 'zustand'
import type { SystemMetrics } from '@/api/types'

interface MetricsState {
  metrics: SystemMetrics
  cpuHistory: number[]
  setMetrics: (metrics: SystemMetrics) => void
}

export const useMetricsStore = create<MetricsState>((set) => ({
  metrics: {
    cpuPercent: 0,
    load: [0, 0, 0],
    ramUsedBytes: 0,
    ramTotalBytes: 0,
    cpuTempC: 0,
    uptimeSeconds: 0,
    net: { interface: 'eth0', upMbps: 0, downMbps: 0 },
  },
  cpuHistory: [],
  setMetrics: (metrics) =>
    set((state) => ({
      metrics,
      cpuHistory: [...state.cpuHistory.slice(-59), metrics.cpuPercent],
    })),
}))
