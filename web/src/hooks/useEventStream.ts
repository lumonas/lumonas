import { useEffect } from 'react'
import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { connectEventStream } from '@/api/events'
import { queryKeys } from '@/api/queries'
import type { Disk, LumoEvent, SystemMetrics } from '@/api/types'
import { useMetricsStore } from '@/stores/metrics'

function applyEvent(qc: QueryClient, event: LumoEvent) {
  switch (event.type) {
    case 'system.metrics': {
      useMetricsStore.getState().setMetrics(event.data as unknown as SystemMetrics)
      break
    }
    case 'disk.temperature.changed': {
      const { diskId, temperatureC } = event.data as { diskId: string; temperatureC: number }
      qc.setQueryData<Disk[]>(queryKeys.disks, (current) =>
        current?.map((disk) => (disk.id === diskId ? { ...disk, temperatureC } : disk)),
      )
      qc.setQueryData<Disk>(['disks', diskId], (current) =>
        current ? { ...current, temperatureC } : current,
      )
      break
    }
    case 'job.progress':
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
      break
    case 'job.state_changed':
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
      void qc.invalidateQueries({ queryKey: queryKeys.activity })
      break
    case 'snapraid.sync.completed':
      void qc.invalidateQueries({ queryKey: queryKeys.protection })
      void qc.invalidateQueries({ queryKey: queryKeys.activity })
      break
    case 'alert.created':
    case 'alert.resolved':
      void qc.invalidateQueries({ queryKey: queryKeys.alerts })
      break
    default:
      break
  }
}

export function useEventStream() {
  const qc = useQueryClient()
  useEffect(() => {
    const source = connectEventStream()
    const handler = (e: { data: string }) => {
      try {
        applyEvent(qc, JSON.parse(e.data) as LumoEvent)
      } catch (err) {
        console.debug('ignored malformed event', err)
      }
    }
    source.addEventListener('message', handler)
    return () => {
      source.removeEventListener('message', handler)
      source.close()
    }
  }, [qc])
}
