import { create } from 'zustand'
import type { SystemMetrics, NetInterfaceMetrics } from '@/api/types'

interface MetricsState {
  metrics: SystemMetrics
  cpuHistory: number[]
  ramHistory: number[]
  netHistory: number[]
  diskHistory: number[]
  ifaceHistories: Record<string, { up: number[]; down: number[] }>
  setMetrics: (metrics: SystemMetrics) => void
}

const CAP = 60

function push(history: number[], value: number): number[] {
  return [...history.slice(-(CAP - 1)), value]
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
    disk: { readMbps: 0, writeMbps: 0 },
  },
  cpuHistory: [],
  ramHistory: [],
  netHistory: [],
  diskHistory: [],
  ifaceHistories: {},
  setMetrics: (metrics) =>
    set((state) => {
      const ifaceHistories = { ...state.ifaceHistories }
      for (const iface of metrics.netInterfaces ?? []) {
        const prev = ifaceHistories[iface.interface] ?? { up: [], down: [] }
        ifaceHistories[iface.interface] = {
          up: push(prev.up, Math.round(iface.upMbps * 10) / 10),
          down: push(prev.down, Math.round(iface.downMbps * 10) / 10),
        }
      }
      return {
        metrics,
        cpuHistory: push(state.cpuHistory, metrics.cpuPercent),
        ramHistory: push(state.ramHistory, Math.round((metrics.ramUsedBytes / 1e9) * 10) / 10),
        netHistory: push(state.netHistory, Math.round(metrics.net.downMbps * 10) / 10),
        diskHistory: push(state.diskHistory, Math.round(metrics.disk?.readMbps ?? 0)),
        ifaceHistories,
      }
    }),
}))
