import { useEffect } from 'react'
import { useQueryClient, type QueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { connectEventStream } from '@/api/events'
import { queryKeys } from '@/api/queries'
import type { Disk, Job, LogLine, LumoEvent, SystemMetrics } from '@/api/types'
import { useLogsStore } from '@/stores/logs'
import { useMetricsStore } from '@/stores/metrics'
import { useLiveConnection } from '@/stores/live-connection'

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
    case 'job.state_changed': {
      const job = event.data as unknown as Job
      if (job.state === 'successful') {
        toast.success(`${job.title} — completed`)
      } else if (job.state === 'failed') {
        toast.error(`${job.title} — failed`, {
          description: 'Nothing was changed. Inspect the job log for details.',
        })
      }
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
      void qc.invalidateQueries({ queryKey: queryKeys.activity })
      break
    }
    case 'snapraid.sync.completed':
      void qc.invalidateQueries({ queryKey: queryKeys.protection })
      void qc.invalidateQueries({ queryKey: queryKeys.activity })
      break
    case 'alert.created':
    case 'alert.resolved':
      void qc.invalidateQueries({ queryKey: queryKeys.alerts })
      break
    case 'docker.log.line':
      useLogsStore.getState().append(event.data as unknown as LogLine)
      break
    case 'docker.stack.deployed': {
      const { kind } = event.data as { kind?: string }
      toast.success(kind === 'docker.update' ? 'Stack updated' : 'Stack deployed', {
        description: 'All containers are healthy.',
      })
      void qc.invalidateQueries({ queryKey: ['docker'] })
      void qc.invalidateQueries({ queryKey: queryKeys.dockerSummary })
      void qc.invalidateQueries({ queryKey: queryKeys.activity })
      break
    }
    case 'docker.container.state_changed':
      void qc.invalidateQueries({ queryKey: ['docker'] })
      void qc.invalidateQueries({ queryKey: queryKeys.dockerSummary })
      break
    case 'docker.image.updated':
      void qc.invalidateQueries({ queryKey: ['docker'] })
      void qc.invalidateQueries({ queryKey: queryKeys.dockerSummary })
      break
    case 'share.created':
      void qc.invalidateQueries({ queryKey: queryKeys.shares })
      break
    case 'docker.container.metrics':
      void qc.invalidateQueries({ queryKey: queryKeys.dockerContainers })
      break
    default:
      break
  }
}

export function useEventStream() {
  const qc = useQueryClient()
  useEffect(() => {
    const source = connectEventStream()
    const setState = useLiveConnection.getState().setState
    setState('connecting')
    const onOpen = () => setState('live')
    const onError = () => setState(source.readyState === 2 ? 'offline' : 'reconnecting')
    const handler = (e: { data: string }) => {
      try {
        applyEvent(qc, JSON.parse(e.data) as LumoEvent)
      } catch (err) {
        console.debug('ignored malformed event', err)
      }
    }
    source.addEventListener('open', onOpen)
    source.addEventListener('error', onError)
    source.addEventListener('message', handler)
    return () => {
      source.removeEventListener('open', onOpen)
      source.removeEventListener('error', onError)
      source.removeEventListener('message', handler)
      source.close()
      setState('offline')
    }
  }, [qc])
}
