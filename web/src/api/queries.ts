import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiGet, apiPatch, apiPost } from '@/api/client'
import type {
  ActivityEvent,
  Alert,
  Disk,
  DockerSummary,
  Job,
  Pool,
  Protection,
  ServerInfo,
  SystemMetrics,
} from '@/api/types'

export const queryKeys = {
  server: ['server'] as const,
  disks: ['disks'] as const,
  disk: (id: string) => ['disks', id] as const,
  pools: ['pools'] as const,
  protection: ['protection'] as const,
  jobs: ['jobs'] as const,
  alerts: ['alerts'] as const,
  activity: ['activity'] as const,
  metrics: ['metrics'] as const,
  dockerSummary: ['docker-summary'] as const,
}

export function useServer() {
  return useQuery({ queryKey: queryKeys.server, queryFn: () => apiGet<ServerInfo>('/server') })
}

export function useDisks() {
  return useQuery({ queryKey: queryKeys.disks, queryFn: () => apiGet<Disk[]>('/disks') })
}

export function useDisk(id: string | null) {
  return useQuery({
    queryKey: ['disks', id],
    queryFn: () => apiGet<Disk>(`/disks/${id}`),
    enabled: id != null,
  })
}

export function usePools() {
  return useQuery({ queryKey: queryKeys.pools, queryFn: () => apiGet<Pool[]>('/pools') })
}

export function useProtection() {
  return useQuery({
    queryKey: queryKeys.protection,
    queryFn: () => apiGet<Protection>('/storage/protection'),
  })
}

export function useJobs() {
  return useQuery({ queryKey: queryKeys.jobs, queryFn: () => apiGet<Job[]>('/jobs') })
}

export function useAlerts() {
  return useQuery({ queryKey: queryKeys.alerts, queryFn: () => apiGet<Alert[]>('/alerts') })
}

export function useActivity() {
  return useQuery({
    queryKey: queryKeys.activity,
    queryFn: () => apiGet<ActivityEvent[]>('/activity'),
  })
}

export function useMetrics() {
  return useQuery({
    queryKey: queryKeys.metrics,
    queryFn: () => apiGet<SystemMetrics>('/system/metrics'),
  })
}

export function useDockerSummary() {
  return useQuery({
    queryKey: queryKeys.dockerSummary,
    queryFn: () => apiGet<DockerSummary>('/docker/summary'),
  })
}

export function useCreateJob() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { type: string; resourceId?: string }) =>
      apiPost<Job>('/jobs', input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
    },
  })
}

export function useAcknowledgeAlert() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiPatch<Alert>(`/alerts/${id}/ack`),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.alerts })
    },
  })
}
