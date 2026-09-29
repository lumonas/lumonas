import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ApiError, apiDelete, apiDownload, apiGet, apiMultipart, apiPatch, apiPut, apiPost, apiPutChunk } from '@/api/client'
import { useLogsStore } from '@/stores/logs'
import type {
  ActivityEvent,
  Alert,
  AlertRule,
  AuditPage,
  BackupDestination,
  BackupPolicyTemplate,
  BackupSchedule,
  BackupJob,
  BackupRestoreCheck,
  CapacityForecast,
  CapacityThreshold,
  CatalogApp,
  ConfigGeneration,
  DockerContainer,
  DockerImage,
  DockerImagePackSummary,
  DockerImagePackImportResult,
  DockerStack,
  DockerDeployment,
  DockerSummary,
  DockerVolume,
  CreateVirtualMachineInput,
  VirtualizationMedia,
  VirtualMachine,
  VirtualMachineSnapshot,
  VirtualMachineConsole,
  VirtualMachineDeleteResult,
  RecoverableVirtualMachine,
  VirtualizationStatus,
  Disk,
  FileEntry,
  FileUser,
  HealthBreakdown,
  Job,
  JobSchedule,
  LogLine,
  ManagementUser,
  NotificationChannel,
  NotificationDelivery,
  OnboardingState,
  DiskRole,
  Pool,
  PoolPlan,
  PoolUnmountPlan,
  Principal,
  Protection,
  ProtectionConfig,
  AppSettings,
  RecycleEntry,
  RecoveryPlan,
  RecoveryExportResponse,
  RecoveryReadiness,
  RecoveryStatus,
  WorkloadRecoveryObjective,
  RestorePlan,
  ServerInfo,
  ServiceStatus,
  Share,
  ShareStorageResource,
  ShareRelocationPreview,
  ShareRelocationScheduleResult,
  ShareAccessPreview,
  SharePathAccessCheck,
  ShareClientsSnapshot,
  TLSCertificateStatus,
  StorageMount,
  StorageOperationPlan,
  StorageSafety,
  DiskReplacementPlan,
  DiskReplacementResult,
  APITokenCreated,
  APITokenSummary,
  ReplicationPeer,
  SnapshotReplicationTask,
  SnapshotReplicationRun,
  SnapshotDiff,
  StorageSnapshot,
  StorageSnapshotFiles,
  SystemMetrics,
  UPSStatus,
  UPSConfig,
  UPSPolicy,
  UserGroup,
  WireGuardConfig,
  WireGuardStatus,
  TailscaleStatus,
  SSHKey,
  NetworkConnection,
  LanHost,
  NetworkInterface,
  WiFiScanResult,
  SMARTHistory,
  RestoreDrill,
  RestoreDrillSchedule,
  TroubleshootingReport,
  DependencyGraph,
} from '@/api/types'

export const queryKeys = {
  server: ['server'] as const,
  healthComponents: ['health', 'components'] as const,
  disks: ['disks'] as const,
  disk: (id: string) => ['disks', id] as const,
  smartHistory: (id: string | null) => ['disks', id, 'smart-history'] as const,
  pools: ['pools'] as const,
  protection: ['protection'] as const,
  jobs: ['jobs'] as const,
  alerts: ['alerts'] as const,
  activity: ['activity'] as const,
  metrics: ['metrics'] as const,
  dockerSummary: ['docker-summary'] as const,
  dockerApps: ['docker', 'apps'] as const,
  dockerStacks: ['docker', 'stacks'] as const,
  dockerStack: (id: string | null) => ['docker', 'stacks', id] as const,
  virtualizationStatus: ['virtualization', 'status'] as const,
  virtualMachines: ['virtualization', 'vms'] as const,
  recoverableVirtualMachines: ['virtualization', 'recoverable-vms'] as const,
  virtualizationMedia: ['virtualization', 'media'] as const,
  virtualMachineSnapshots: (name: string | null) => ['virtualization', 'vms', name, 'snapshots'] as const,
  dockerContainers: ['docker', 'containers'] as const,
  dockerImages: ['docker', 'images'] as const,
  dockerVolumes: ['docker', 'volumes'] as const,
  services: ['services'] as const,
  alertRules: ['alert-rules'] as const,
  notificationChannels: ['notification-channels'] as const,
  schedules: ['schedules'] as const,
  shares: ['shares'] as const,
  principals: ['principals'] as const,
  users: ['users'] as const,
  files: (shareId: string | null, path: string) => ['files', shareId, path] as const,
  recycle: (shareId: string | null) => ['recycle', shareId] as const,
  backupReadiness: ['backup', 'readiness'] as const,
  backupSchedule: ['backup', 'schedule'] as const,
  backupJobs: ['backup', 'jobs'] as const,
  backupDestinations: ['backup', 'destinations'] as const,
  generations: ['backup', 'generations'] as const,
  restorePlan: ['backup', 'restore-plan'] as const,
  settings: ['settings'] as const,
  storageMounts: ['storage', 'mounts'] as const,
  storageSafety: ['storage', 'safety'] as const,
  protectionConfig: ['storage', 'protection', 'config'] as const,
  recoveryStatus: ['recovery', 'status'] as const,
  recoveryPlan: ['recovery', 'plan'] as const,
  restoreDrills: ['recovery', 'drills'] as const,
  restoreDrillSchedule: ['recovery', 'drills', 'schedule'] as const,
  troubleshooting: ['troubleshooting'] as const,
  dependencyGraph: ['dependencies', 'graph'] as const,
  notificationDeliveries: ['notifications', 'deliveries'] as const,
  notificationRules: ['notification-rules'] as const,
  audit: ['audit'] as const,
  capacityForecast: ['capacity', 'forecast'] as const,
  groups: ['groups'] as const,
  ups: ['power', 'ups'] as const,
  upsConfig: ['power', 'ups-config'] as const,
  upsPolicy: ['power', 'ups-policy'] as const,
  wireguardStatus: ['network', 'wireguard', 'status'] as const,
  tailscaleStatus: ['network', 'tailscale', 'status'] as const,
  networkInterfaces: ['network', 'interfaces'] as const,
  networkConnections: ['network', 'connections'] as const,
  networkBindings: ['network', 'bindings'] as const,
  networkFirewall: ['network', 'firewall'] as const,
  networkDiagnostic: (id: string) => ['network', 'diagnostics', id] as const,
  wifiScan: ['network', 'wifi-scan'] as const,
}

export function useServer() {
  return useQuery({ queryKey: queryKeys.server, queryFn: () => apiGet<ServerInfo>('/server') })
}

export function useHealthComponents() {
  return useQuery({ queryKey: queryKeys.healthComponents, queryFn: () => apiGet<HealthBreakdown>('/health/components') })
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

export function useSMARTHistory(id: string | null) {
  return useQuery({
    queryKey: queryKeys.smartHistory(id),
    queryFn: () => apiGet<SMARTHistory>(`/storage/disks/${id}/smart-history?days=90`),
    enabled: id != null,
  })
}

export function useTroubleshooting() {
  return useQuery({ queryKey: queryKeys.troubleshooting, queryFn: () => apiGet<TroubleshootingReport>('/troubleshooting') })
}

export function useDependencyGraph() {
  return useQuery({ queryKey: queryKeys.dependencyGraph, queryFn: () => apiGet<DependencyGraph>('/dependencies/graph') })
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

export function useAlertHistory(limit = 50) {
  return useQuery({
    queryKey: [...queryKeys.alerts, 'history', limit],
    queryFn: () => apiGet<Alert[]>(`/alerts/history?limit=${limit}`),
    throwOnError: false,
  })
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

export function useUPS() {
  return useQuery({ queryKey: queryKeys.ups, queryFn: () => apiGet<UPSStatus[]>('/power/ups') })
}

export function useUPSConfig() {
  return useQuery({ queryKey: queryKeys.upsConfig, queryFn: () => apiGet<UPSConfig>('/ups/config') })
}

export function useUpdateUPSConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: UPSConfig) => apiPatch<UPSConfig>('/ups/config', input),
    onSuccess: (config) => {
      qc.setQueryData(queryKeys.upsConfig, config)
      void qc.invalidateQueries({ queryKey: queryKeys.ups })
    },
  })
}

export function useUPSPolicy() {
  return useQuery({ queryKey: queryKeys.upsPolicy, queryFn: () => apiGet<UPSPolicy>('/ups/policy') })
}

export function useUpdateUPSPolicy() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: UPSPolicy) => apiPatch<UPSPolicy>('/ups/policy', input),
    onSuccess: (policy) => {
      qc.setQueryData(queryKeys.upsPolicy, policy)
    },
  })
}

export function usePowerAction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (action: 'poweroff' | 'reboot') =>
      apiPost<{ operationId: string; action: string }>('/power/shutdown', {
        action,
        reauthenticated: true,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
    },
  })
}

export function useDockerSummary() {
  return useQuery({
    queryKey: queryKeys.dockerSummary,
    queryFn: () => apiGet<DockerSummary>('/docker/summary'),
  })
}

export function useVirtualizationStatus() {
  return useQuery({
    queryKey: queryKeys.virtualizationStatus,
    queryFn: () => apiGet<VirtualizationStatus>('/virtualization/status'),
    staleTime: 30_000,
  })
}

export function useVirtualMachines() {
  return useQuery({
    queryKey: queryKeys.virtualMachines,
    queryFn: () => apiGet<VirtualMachine[]>('/virtualization/vms'),
    refetchInterval: 10_000,
  })
}

export function useVirtualizationMedia() {
  return useQuery({
    queryKey: queryKeys.virtualizationMedia,
    queryFn: () => apiGet<VirtualizationMedia[]>('/virtualization/media'),
  })
}

export function useUploadVirtualizationMedia() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (file: File) => {
      const form = new FormData()
      form.set('file', file)
      return apiMultipart<VirtualizationMedia>('/virtualization/media', form)
    },
    onSuccess: (media) => {
      toast.success(`${media.name} uploaded`)
      void qc.invalidateQueries({ queryKey: queryKeys.virtualizationMedia })
    },
  })
}

export function useCreateVirtualMachine() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateVirtualMachineInput) => apiPost<VirtualMachine>('/virtualization/vms', input),
    onSuccess: (machine) => {
      toast.success(`${machine.name} created`)
      void qc.invalidateQueries({ queryKey: queryKeys.virtualMachines })
    },
  })
}

export function useVirtualMachineSnapshots(name: string | null, enabled = true) {
  return useQuery({
    queryKey: queryKeys.virtualMachineSnapshots(name),
    queryFn: () => apiGet<VirtualMachineSnapshot[]>(`/virtualization/vms/${encodeURIComponent(name!)}/snapshots`),
    enabled: Boolean(name) && enabled,
  })
}

export function useCreateVirtualMachineSnapshot() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, snapshot }: { name: string; snapshot: string }) =>
      apiPost<VirtualMachineSnapshot>(`/virtualization/vms/${encodeURIComponent(name)}/snapshots`, { name: snapshot }),
    onSuccess: (_snapshot, variables) => {
      toast.success('Virtual machine snapshot created')
      void qc.invalidateQueries({ queryKey: queryKeys.virtualMachineSnapshots(variables.name) })
    },
  })
}

export function useRevertVirtualMachineSnapshot() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, snapshot }: { name: string; snapshot: string }) =>
      apiPost<{ status: string }>(`/virtualization/vms/${encodeURIComponent(name)}/snapshots/${encodeURIComponent(snapshot)}/revert`, {}),
    onSuccess: (_result, variables) => {
      toast.success('Virtual machine snapshot restore requested')
      void qc.invalidateQueries({ queryKey: queryKeys.virtualMachines })
      void qc.invalidateQueries({ queryKey: queryKeys.virtualMachineSnapshots(variables.name) })
    },
  })
}

export function useDeleteVirtualMachineSnapshot() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, snapshot }: { name: string; snapshot: string }) =>
      apiDelete<void>(`/virtualization/vms/${encodeURIComponent(name)}/snapshots/${encodeURIComponent(snapshot)}`),
    onSuccess: (_result, variables) => {
      toast.success('Virtual machine snapshot deleted')
      void qc.invalidateQueries({ queryKey: queryKeys.virtualMachineSnapshots(variables.name) })
    },
  })
}

export function useVirtualMachineAction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, action }: { name: string; action: 'start' | 'shutdown' | 'reboot' | 'suspend' | 'resume' }) =>
      apiPost<VirtualMachine>(`/virtualization/vms/${encodeURIComponent(name)}/action`, { action }),
    onSuccess: () => {
      toast.success('Virtual machine action accepted')
      void qc.invalidateQueries({ queryKey: queryKeys.virtualMachines })
    },
  })
}

export function useDeleteVirtualMachine() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ name, deleteDisk }: { name: string; deleteDisk: boolean }) =>
      apiDelete<VirtualMachineDeleteResult>(`/virtualization/vms/${encodeURIComponent(name)}`, { confirmName: name, deleteDisk }),
    onSuccess: (result) => {
      toast.success(result.diskRemoved ? `${result.name} and its disk were deleted` : `${result.name} removed; recovery files were kept`)
      void qc.invalidateQueries({ queryKey: queryKeys.virtualMachines })
      void qc.invalidateQueries({ queryKey: queryKeys.recoverableVirtualMachines })
    },
  })
}

export function useRecoverableVirtualMachines() {
  return useQuery({
    queryKey: queryKeys.recoverableVirtualMachines,
    queryFn: () => apiGet<RecoverableVirtualMachine[]>('/virtualization/recoverable-vms'),
    retry: false,
  })
}

export function useRestoreVirtualMachineDefinition() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => apiPost<VirtualMachine>(`/virtualization/vms/${encodeURIComponent(name)}/restore-definition`, {}),
    onSuccess: (machine) => {
      toast.success(`${machine.name} definition restored`)
      void qc.invalidateQueries({ queryKey: queryKeys.virtualMachines })
      void qc.invalidateQueries({ queryKey: queryKeys.recoverableVirtualMachines })
    },
  })
}

export function useVirtualMachineConsole(name: string | null) {
  const queryClient = useQueryClient()
  const queryKey = ['virtualization', 'vms', name, 'console'] as const
  return useQuery({
    queryKey,
    queryFn: async () => {
      const previous = queryClient.getQueryData<VirtualMachineConsole>(queryKey)
      const state = await apiGet<VirtualMachineConsole>(`/virtualization/vms/${encodeURIComponent(name!)}/console?cursor=${previous?.cursor ?? 0}`)
      return { ...state, output: `${previous?.output ?? ''}${state.output}`.slice(-128 * 1024) }
    },
    enabled: name != null,
    refetchInterval: 750,
    retry: false,
  })
}

export function useWriteVirtualMachineConsole() {
  return useMutation({
    mutationFn: ({ name, data }: { name: string; data: string }) =>
      apiPost<{ status: string }>(`/virtualization/vms/${encodeURIComponent(name)}/console`, { data }),
  })
}

export function useCloseVirtualMachineConsole() {
  return useMutation({
    mutationFn: (name: string) => apiDelete<void>(`/virtualization/vms/${encodeURIComponent(name)}/console`),
  })
}

export function useDockerApps() {
  return useQuery({
    queryKey: queryKeys.dockerApps,
    queryFn: () => apiGet<CatalogApp[]>('/docker/apps'),
  })
}

export function useDockerStacks() {
  return useQuery({
    queryKey: queryKeys.dockerStacks,
    queryFn: () => apiGet<DockerStack[]>('/docker/stacks'),
  })
}

export function useDockerStack(id: string | null) {
  return useQuery({
    queryKey: queryKeys.dockerStack(id),
    queryFn: () => apiGet<DockerStack>(`/docker/stacks/${id}`),
    enabled: id != null,
  })
}

export function useDockerDeployments(stack?: string) {
  const endpoint = stack ? `/docker/deployments?stack=${encodeURIComponent(stack)}` : '/docker/deployments'
  return useQuery({
    queryKey: ['docker', 'deployments', stack ?? 'all'],
    queryFn: () => apiGet<DockerDeployment[]>(endpoint),
  })
}

export function useDockerContainers() {
  return useQuery({
    queryKey: queryKeys.dockerContainers,
    queryFn: () => apiGet<DockerContainer[]>('/docker/containers'),
  })
}

export function useDockerImages() {
  return useQuery({
    queryKey: queryKeys.dockerImages,
    queryFn: () => apiGet<DockerImage[]>('/docker/images'),
  })
}

export function useDockerImagePacks() {
  return useQuery({
    queryKey: [...queryKeys.dockerImages, 'packs'],
    queryFn: () => apiGet<DockerImagePackSummary[]>('/docker/images/packs'),
    throwOnError: false,
  })
}

export function useImagePackImport() {
  return useDockerMutation((pack: string) =>
    apiPost<DockerImagePackImportResult>('/docker/images/packs/import', { name: pack }),
  )
}

export function useCheckImageUpdates() {
	const qc = useQueryClient()
	return useMutation({
		mutationFn: () => apiPost<DockerImage[]>('/docker/images/check-updates'),
		onSuccess: (images) => {
			qc.setQueryData(queryKeys.dockerImages, images)
			toast.success(`${images.filter((image) => image.updateAvailable).length} image update(s) available with digest details`)
		},
		onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not check image updates'),
	})
}

export function useDockerVolumes() {
  return useQuery({
    queryKey: queryKeys.dockerVolumes,
    queryFn: () => apiGet<DockerVolume[]>('/docker/volumes'),
  })
}

export interface InstallPayload {
  catalogId?: string
  name?: string
  composeYaml?: string
  env?: Record<string, string>
  storageMap?: { fieldId: string; containerPath: string; resourceId: string }[]
  deploy?: boolean
}

function useDockerMutation<TInput, TResult>(fn: (input: TInput) => Promise<TResult>) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['docker'] })
      void qc.invalidateQueries({ queryKey: queryKeys.dockerSummary })
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
    },
  })
}

export function useCreateStack() {
  return useDockerMutation((input: InstallPayload) =>
    apiPost<DockerStack>('/docker/stacks', input),
  )
}

export function useStackAction() {
  return useDockerMutation(
    ({
      id,
      action,
      body,
    }: {
      id: string
      action: 'deploy' | 'update' | 'start' | 'stop' | 'restart'
      body?: Record<string, unknown>
    }) => apiPost<DockerStack | Job>(`/docker/stacks/${id}/${action}`, body),
  )
}

export function useUpdateStackRecoveryProfile() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, appdataPaths }: { id: string; appdataPaths: string[] }) =>
      apiPut<DockerStack>(`/docker/stacks/${encodeURIComponent(id)}/recovery`, { appdataPaths }),
    onSuccess: (stack) => {
      toast.success(`Recovery paths saved for ${stack.name}`)
      void qc.invalidateQueries({ queryKey: queryKeys.dockerStacks })
      void qc.invalidateQueries({ queryKey: queryKeys.dockerStack(stack.id) })
      void qc.invalidateQueries({ queryKey: queryKeys.recoveryPlan })
    },
  })
}

export function useContainerAction() {
  return useDockerMutation(({ id, action }: { id: string; action: 'start' | 'stop' | 'restart' }) =>
    apiPost<DockerContainer>(`/docker/containers/${id}/${action}`),
  )
}

export function useUpdateImage() {
  return useDockerMutation((id: string) => apiPost<DockerImage>(`/docker/images/${id}/update`))
}

export function useImportDockerImage() {
  return useDockerMutation((archive: File) => {
    const form = new FormData()
    form.append('archive', archive)
    return apiMultipart<{ status: string; bytes: number }>('/docker/images/import', form)
  })
}

export function useSeedLogs() {
  return useMutation({
    mutationFn: (container: string) => apiGet<LogLine[]>(`/docker/logs/${container}`),
    onSuccess: (lines, container) => {
      useLogsStore.getState().seed(container, lines)
    },
  })
}

export function useCreateJob() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { type: string; resourceId?: string; correlationId?: string; filesystemKind?: 'btrfs' | 'zfs'; filesystemSource?: string }) =>
      apiPost<Job>('/jobs', input),
    onSuccess: () => {
      toast.success('Job queued', {
        description: 'Track progress from the jobs menu in the top bar.',
      })
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
    },
  })
}

export function useCancelQueuedJob() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiPost<Job>(`/jobs/${id}/cancel`),
    onSuccess: (job) => {
      toast.success(job.state === 'running' ? 'Cancellation requested' : 'Queued job cancelled')
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not cancel job'),
  })
}

export function useServices() {
  return useQuery({
    queryKey: queryKeys.services,
    queryFn: () => apiGet<ServiceStatus[]>('/services'),
  })
}

export function useAlertRules() {
  return useQuery({
    queryKey: queryKeys.alertRules,
    queryFn: () => apiGet<AlertRule[]>('/alert-rules'),
  })
}

export function useToggleAlertRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (rule: AlertRule) =>
      apiPatch<AlertRule>(`/alert-rules/${rule.id}`, { enabled: !rule.enabled }),
    onSuccess: (rule) => {
      toast.success(rule.enabled ? `Rule enabled — ${rule.name}` : `Rule disabled — ${rule.name}`)
      void qc.invalidateQueries({ queryKey: queryKeys.alertRules })
    },
  })
}

export function useNotificationChannels() {
  return useQuery({
    queryKey: queryKeys.notificationChannels,
    queryFn: () => apiGet<NotificationChannel[]>('/notification-channels'),
  })
}

export interface NotificationCredentials {
	 token?: string
	 username?: string
	 password?: string
	 address?: string
}

export interface NotificationChannelInput {
	type: Exclude<NotificationChannel['type'], 'web'>
	label: string
	target: string
	enabled: boolean
	credentials?: NotificationCredentials
}

export function useSaveNotificationChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: NotificationChannelInput) =>
      apiPost<NotificationChannel>('/notification-channels', input),
    onSuccess: (channel) => {
      toast.success(`Notification channel saved — ${channel.label}`)
      void qc.invalidateQueries({ queryKey: queryKeys.notificationChannels })
    },
  })
}

export function useTestNotificationChannel() {
  return useMutation({
    mutationFn: (id: string) =>
      apiPost<{ sent: boolean; channelId: string }>(`/notification-channels/${id}/test`, {}),
    onSuccess: () => toast.success('Test notification sent'),
  })
}

export function useSchedules() {
  return useQuery({
    queryKey: queryKeys.schedules,
    queryFn: () => apiGet<JobSchedule[]>('/schedules'),
  })
}

export interface SchedulePatch {
  id: string
  enabled?: boolean
  timeOfDay?: string
  weekday?: string
  snapshotKind?: 'btrfs' | 'zfs'
  snapshotSource?: string
  snapshotLabel?: string
  snapshotKeep?: number
  snapshotLockDays?: number
  filesystemKind?: 'btrfs' | 'zfs'
  filesystemSource?: string
}

export type SnapshotScheduleInput = {
  name: string
  kind: 'daily' | 'weekly'
  timeOfDay: string
  weekday?: string
  enabled: boolean
  snapshotKind: 'btrfs' | 'zfs'
  snapshotSource: string
  snapshotLabel: string
  snapshotKeep: number
  snapshotLockDays: number
}

export function useCreateSnapshotSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: SnapshotScheduleInput) => apiPost<JobSchedule>('/schedules', { ...input, jobType: 'snapshot.create' }),
    onSuccess: (schedule) => {
      toast.success(`Snapshot schedule created — ${schedule.name}`)
      void qc.invalidateQueries({ queryKey: queryKeys.schedules })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not create snapshot schedule'),
  })
}

export type FilesystemScrubScheduleInput = {
  name: string
  kind: 'daily' | 'weekly'
  timeOfDay: string
  weekday?: string
  enabled: boolean
  filesystemKind: 'btrfs' | 'zfs'
  filesystemSource: string
}

export function useCreateFilesystemScrubSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: FilesystemScrubScheduleInput) => apiPost<JobSchedule>('/schedules', { ...input, jobType: 'filesystem.scrub' }),
    onSuccess: (schedule) => {
      toast.success(`Filesystem scrub scheduled — ${schedule.name}`)
      void qc.invalidateQueries({ queryKey: queryKeys.schedules })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not create filesystem scrub schedule'),
  })
}

export function useDeleteCustomSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiDelete(`/schedules/${encodeURIComponent(id)}`),
    onSuccess: () => {
      toast.success('Snapshot schedule deleted')
      void qc.invalidateQueries({ queryKey: queryKeys.schedules })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not delete schedule'),
  })
}

export function useUpdateSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, ...patch }: Omit<SchedulePatch, 'id'> & { id: string }) =>
      apiPatch<JobSchedule>(`/schedules/${id}`, patch),
    onSuccess: (schedule) => {
      toast.success(
        schedule.enabled ? `Schedule enabled — ${schedule.name}` : `Schedule paused — ${schedule.name}`,
      )
      void qc.invalidateQueries({ queryKey: queryKeys.schedules })
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
    },
  })
}

export function useShares() {
  return useQuery({
    queryKey: queryKeys.shares,
    queryFn: () => apiGet<Share[]>('/shares'),
    throwOnError: false,
  })
}

export function useShareStorageResources() {
  return useQuery({
    queryKey: ['shares', 'storage-resources'],
    queryFn: () => apiGet<ShareStorageResource[]>('/shares/storage-resources'),
  })
}

export type ShareRelocationTarget = { resourceId: string; relativePath: string }

export function usePreviewShareRelocation() {
  return useMutation({
    mutationFn: ({ shareId, target }: { shareId: string; target: ShareRelocationTarget }) =>
      apiPost<ShareRelocationPreview>(`/shares/${encodeURIComponent(shareId)}/relocation/preview`, target),
  })
}

export function useStartShareRelocation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ shareId, target, planHash, scheduleKind, timeOfDay, weekday }: { shareId: string; target: ShareRelocationTarget; planHash: string; scheduleKind?: 'manual' | 'daily' | 'weekly'; timeOfDay?: string; weekday?: string }) =>
      apiPost<{ jobId: string; state: string; preview: ShareRelocationPreview } | ShareRelocationScheduleResult>(`/shares/${encodeURIComponent(shareId)}/relocation`, { target, planHash, confirmed: true, scheduleKind, timeOfDay, weekday }),
    onSuccess: (result) => {
      toast.success('schedule' in result ? 'Share relocation scheduled' : 'Share relocation started')
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
      void qc.invalidateQueries({ queryKey: queryKeys.schedules })
      void qc.invalidateQueries({ queryKey: queryKeys.shares })
      void qc.invalidateQueries({ queryKey: [...queryKeys.shares, result.preview.shareId] })
    },
  })
}

export function useShareAccessPreview(shareId: string | null) {
  return useQuery({
    queryKey: [...queryKeys.shares, shareId, 'access-preview'],
    queryFn: () => apiGet<ShareAccessPreview>(`/shares/${encodeURIComponent(shareId!)}/access-preview`),
    enabled: !!shareId,
  })
}

export function useSharePathAccessCheck(shareId: string | null, principalId: string, path: string, enabled: boolean) {
  return useQuery({
    queryKey: [...queryKeys.shares, shareId, 'access-check', principalId, path],
    queryFn: () => {
      const params = new URLSearchParams({ principal: principalId, path })
      return apiGet<SharePathAccessCheck>(`/shares/${encodeURIComponent(shareId!)}/access-check?${params}`)
    },
    enabled: Boolean(shareId && principalId && enabled),
    retry: false,
  })
}

export function useShareClients() {
  return useQuery({
    queryKey: [...queryKeys.shares, 'clients'],
    queryFn: () => apiGet<ShareClientsSnapshot>('/shares/clients'),
    refetchInterval: 15_000,
    retry: false,
  })
}

export function useDisconnectShareClient() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (address: string) => apiPost<{ address: string; disconnected: boolean }>('/shares/clients/disconnect', { address, confirmed: true }),
    onSuccess: (result) => {
      toast.success(`Disconnected SMB client ${result.address}`)
      void qc.invalidateQueries({ queryKey: [...queryKeys.shares, 'clients'] })
      void qc.invalidateQueries({ queryKey: queryKeys.audit })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not disconnect SMB client'),
  })
}

export function useTLSCertificateStatus() {
  return useQuery({
    queryKey: ['security', 'certificate'],
    queryFn: () => apiGet<TLSCertificateStatus>('/security/certificate'),
    staleTime: 60_000,
    retry: false,
  })
}

export function useShare(id: string | null) {
  return useQuery({
    queryKey: [...queryKeys.shares, id],
    queryFn: () => apiGet<Share>(`/shares/${id}`),
    enabled: id != null,
  })
}

export function usePrincipals() {
  return useQuery({
    queryKey: queryKeys.principals,
    queryFn: () => apiGet<Principal[]>('/principals'),
  })
}

export function useUsers() {
  return useQuery({
    queryKey: queryKeys.users,
    queryFn: () =>
      apiGet<{ management: ManagementUser[]; file: FileUser[]; groups: UserGroup[] }>('/users'),
  })
}

function useConfigMutation<TInput, TResult>(
  fn: (input: TInput) => Promise<TResult>,
  successMessage?: (input: TInput) => string,
) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: fn,
    onSuccess: (_data, input) => {
      if (successMessage) toast.success(successMessage(input))
      void qc.invalidateQueries({ queryKey: queryKeys.shares })
      void qc.invalidateQueries({ queryKey: queryKeys.users })
      void qc.invalidateQueries({ queryKey: queryKeys.activity })
    },
  })
}

export function useCreateShare() {
  return useConfigMutation(
    (input: {
      name: string
      resourceId: string
      resourceLabel: string
      relativePath: string
      access: { principalId: string; level: Share['access'][number]['level'] }[]
      protocols: Share['protocols']
    }) => apiPost<Share>('/shares', input),
    (input) => `Share created — ${input.name}`,
  )
}

export function useShareAccess() {
  return useConfigMutation(
    (input: { id: string; principalId: string; level: Share['access'][number]['level'] }) =>
      apiPatch<Share>(`/shares/${input.id}/access`, {
        principalId: input.principalId,
        level: input.level,
      }),
  )
}

export function useShareProtocol() {
  return useConfigMutation(
    (input: {
      id: string
      protocol: string
      body: Partial<{ enabled: boolean; hosts: string; readOnly: boolean; quotaBytes: number; auditEnabled: boolean; auditOperations: string[] }>
    }) => apiPatch<Share>(`/shares/${input.id}/protocols/${input.protocol}`, input.body),
  )
}

export function useShareSettings() {
  return useConfigMutation(
    (input: { id: string; description?: string; recycleBin?: boolean }) =>
      apiPatch<Share>(`/shares/${input.id}`, input),
  )
}

export function useDeleteShare() {
  return useConfigMutation(
    (id: string) => apiDelete<void>(`/shares/${id}`),
    () => 'Share removed — data on disk was not deleted',
  )
}

export function useCreateUser() {
  return useConfigMutation(
    (input: {
      type: 'management' | 'file'
      username: string
      fullName?: string
      role?: 'owner' | 'operator' | 'readonly'
      group?: string
      password: string
    }) => apiPost<void>('/users', input),
    (input) => `User created — ${input.username}`,
  )
}

export function useToggleUser() {
  return useConfigMutation((input: { id: string; enabled: boolean }) =>
    apiPatch(`/users/${input.id}`, { enabled: input.enabled }),
  )
}

export interface TwoFactorSetup {
  secret: string
  otpauthUri: string
  recoveryCodes: string[]
}

export function useSetupTwoFactor() {
  return useConfigMutation((id: string) => apiPost<TwoFactorSetup>(`/users/${id}/2fa/setup`, {}))
}

export function useEnableTwoFactor() {
  return useConfigMutation((input: { id: string; code: string }) =>
    apiPost<{ twoFactor: boolean }>(`/users/${input.id}/2fa/enable`, { code: input.code }),
  )
}

export function useDisableTwoFactor() {
  return useConfigMutation((id: string) =>
    apiPost<{ twoFactor: boolean }>(`/users/${id}/2fa/disable`, {}),
  )
}

function useFilesMutation<TInput, TResult>(fn: (input: TInput) => Promise<TResult>) {
  const qc = useQueryClient()
  return useMutation<TResult, Error, TInput>({
    mutationFn: fn,
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['files'] })
      void qc.invalidateQueries({ queryKey: ['recycle'] })
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
      void qc.invalidateQueries({ queryKey: queryKeys.activity })
    },
  })
}

export function useFiles(shareId: string | null, path: string) {
  return useQuery({
    queryKey: queryKeys.files(shareId, path),
    queryFn: () =>
      apiGet<{ shareId: string; path: string; entries: FileEntry[] }>(
        `/files?share=${encodeURIComponent(shareId ?? '')}&path=${encodeURIComponent(path)}`,
      ),
    enabled: shareId != null,
    throwOnError: false,
  })
}

export function useRecycleBin(shareId: string | null, enabled = true) {
  return useQuery({
    queryKey: queryKeys.recycle(shareId),
    queryFn: () =>
      apiGet<RecycleEntry[]>(`/files/recycle?share=${encodeURIComponent(shareId ?? '')}`),
    enabled: enabled && shareId != null,
    throwOnError: false,
  })
}

export function useFileRequestLinks(shareId: string | null) {
  return useQuery({
    queryKey: ['file-requests', shareId],
    queryFn: () => apiGet<import('@/api/types').FileRequestLink[]>(`/file-requests?shareId=${encodeURIComponent(shareId!)}`),
    enabled: Boolean(shareId),
  })
}

export function useCreateFileRequestLink() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { shareId: string; path: string; expiresInHours: number; maxFiles: number; maxBytes: number }) => apiPost<{ request: import('@/api/types').FileRequestLink; url: string }>('/file-requests', input),
    onSuccess: (result) => { void qc.invalidateQueries({ queryKey: ['file-requests', result.request.shareId] }) },
  })
}

export function useRevokeFileRequestLink() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { id: string; shareId: string }) => apiDelete(`/file-requests/${encodeURIComponent(input.id)}`),
    onSuccess: (_result, input) => { void qc.invalidateQueries({ queryKey: ['file-requests', input.shareId] }) },
  })
}

export function useFileShareLinks(shareId: string | null) {
  return useQuery({
    queryKey: ['file-share-links', shareId],
    queryFn: () => apiGet<import('@/api/types').FileShareLink[]>(`/file-share-links?shareId=${encodeURIComponent(shareId!)}`),
    enabled: Boolean(shareId),
  })
}

export function useCreateFileShareLink() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { shareId: string; path: string; expiresInHours: number; password?: string }) => apiPost<{ link: import('@/api/types').FileShareLink; url: string }>('/file-share-links', input),
    onSuccess: (result) => { void qc.invalidateQueries({ queryKey: ['file-share-links', result.link.shareId] }) },
  })
}

export function useRevokeFileShareLink() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { id: string; shareId: string }) => apiDelete(`/file-share-links/${encodeURIComponent(input.id)}`),
    onSuccess: (_result, input) => { void qc.invalidateQueries({ queryKey: ['file-share-links', input.shareId] }) },
  })
}

export function useMkdir() {
  return useFilesMutation((input: { shareId: string; path: string; name: string }) =>
    apiPost('/files/mkdir', input),
  )
}

export function useRenameEntry() {
  return useFilesMutation((input: {
    shareId: string
    path: string
    oldName: string
    newName: string
  }) => apiPost('/files/rename', input))
}

export function useDeleteFiles() {
  return useFilesMutation((input: { shareId: string; path: string; names: string[] }) =>
    apiPost<{ deleted: number }>('/files/delete', input),
  )
}

export interface TransferPayload {
  shareId: string
  sourcePath: string
  names: string[]
  targetShareId: string
  targetPath: string
  op: 'copy' | 'move'
  conflict?: 'overwrite' | 'skip' | 'rename'
}

export function useTransfer() {
  return useFilesMutation((input: TransferPayload) => {
    const { conflict, ...body } = input
    return apiPost<{ jobId: string; transferred: number }>('/files/transfer', {
      ...body,
      conflict,
    })
  })
}

export function extractConflicts(error: unknown): string[] | null {
  if (error instanceof ApiError && error.status === 409) {
    const body = error.body as { conflicts?: string[] } | undefined
    if (body?.conflicts) return body.conflicts
  }
  return null
}

export function useUploadFile() {
  return useFilesMutation((input: { shareId: string; path: string; file: File }) => {
    type UploadSession = { id: string; receivedBytes: number; sizeBytes: number }
    const fingerprint = `lumonas-upload:${JSON.stringify([input.shareId, input.path, input.file.name, input.file.size, input.file.lastModified])}`
    return (async () => {
      let session: UploadSession | undefined
      const remembered = localStorage.getItem(fingerprint)
      if (remembered) {
        try { session = await apiGet<UploadSession>(`/files/uploads/${encodeURIComponent(remembered)}`) } catch { localStorage.removeItem(fingerprint) }
      }
      if (!session) {
        session = await apiPost<UploadSession>('/files/uploads', { shareId: input.shareId, path: input.path, name: input.file.name, sizeBytes: input.file.size })
        localStorage.setItem(fingerprint, session.id)
      }
      let offset = session.receivedBytes
      const chunkSize = 8 * 1024 * 1024
      while (offset < input.file.size) {
        const chunk = input.file.slice(offset, Math.min(input.file.size, offset + chunkSize))
        try {
          await apiPutChunk(`/files/uploads/${encodeURIComponent(session.id)}`, chunk, offset)
          offset += chunk.size
        } catch (error) {
          const recovered = await apiGet<UploadSession>(`/files/uploads/${encodeURIComponent(session.id)}`)
          if (recovered.receivedBytes > offset) {
            offset = recovered.receivedBytes
            continue
          }
          throw error
        }
      }
      const completed = await apiPost<{ jobId: string; name: string }>(`/files/uploads/${encodeURIComponent(session.id)}/complete`)
      localStorage.removeItem(fingerprint)
      return completed
    })()
  })
}

export function useRestoreFile() {
  return useFilesMutation((input: { id: string }) =>
    apiPost('/files/recycle/restore', input),
  )
}

export function usePurgeRecycle() {
  return useFilesMutation((input: { id?: string; shareId: string }) =>
    apiPost<{ purged: number }>('/files/recycle/purge', input),
  )
}

export function useBackupReadiness() {
  return useQuery({
    queryKey: queryKeys.backupReadiness,
    queryFn: () => apiGet<RecoveryReadiness>('/backup/readiness'),
  })
}

export function useBackupSchedule() {
  return useQuery({
    queryKey: queryKeys.backupSchedule,
    queryFn: () => apiGet<BackupSchedule>('/backups/schedule'),
  })
}

export function useUpdateBackupSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: Pick<BackupSchedule, 'enabled' | 'intervalSeconds' | 'onUsbAttach'>) =>
      apiPatch<BackupSchedule>('/backups/schedule', input),
    onSuccess: () => {
      toast.success('Backup schedule updated')
      void qc.invalidateQueries({ queryKey: queryKeys.backupSchedule })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Backup schedule update failed'),
  })
}

export function useBackupPolicyTemplates() {
  return useQuery({
    queryKey: ['backup', 'policy-templates'],
    queryFn: () => apiGet<BackupPolicyTemplate[]>('/backups/policy-templates'),
  })
}

export function useApplyBackupPolicyTemplate() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (templateId: string) => apiPost<BackupSchedule>('/backups/policy-template', { templateId }),
    onSuccess: () => {
      toast.success('Backup policy applied', { description: 'Schedule and destination retention were updated. Provider object-lock settings were preserved.' })
      void qc.invalidateQueries({ queryKey: queryKeys.backupSchedule })
      void qc.invalidateQueries({ queryKey: queryKeys.backupDestinations })
      void qc.invalidateQueries({ queryKey: ['backup', 'destinations'] })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not apply backup policy'),
  })
}

export function useBackupJobs() {
  return useQuery({
    queryKey: queryKeys.backupJobs,
    queryFn: () => apiGet<BackupJob[]>('/backup/jobs'),
  })
}

export function useRunBackup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiPost<Job>(`/backup/jobs/${id}/run`),
    onSuccess: (job) => {
      toast.success(`Backup queued — ${job.title}`)
      void qc.invalidateQueries({ queryKey: queryKeys.backupJobs })
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
    },
  })
}

export function useBackupDestinations() {
  return useQuery({
    queryKey: queryKeys.backupDestinations,
    queryFn: () => apiGet<BackupDestination[]>('/backup/destinations'),
  })
}

export function useBackupDestinationRestoreCheck() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiPost<BackupRestoreCheck>(`/backup/destinations/${id}/restore-check`),
    onSuccess: (result) => {
      toast.success(`Restore check passed for ${result.appdataRestored.length} app-data set(s)`)
      void qc.invalidateQueries({ queryKey: queryKeys.backupDestinations })
      void qc.invalidateQueries({ queryKey: queryKeys.backupReadiness })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Restore check failed'),
  })
}

export interface BackupRunCopy {
  id: string
  runId: string
  destinationId: string
  object: string
  state: string
  verified: boolean
}

export interface BackupRunEntry {
  run: { id: string; state: string; generation: number; startedAt: string }
  copies: BackupRunCopy[]
}

export function useBackupRuns() {
  return useQuery({
    queryKey: ['backups', 'runs'],
    queryFn: () => apiGet<BackupRunEntry[]>('/backups/runs'),
  })
}

export function useRestoreBackupVirtualMachine() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { destinationId: string; runId: string; name: string; confirmName: string }) =>
      apiPost(`/backup/destinations/${encodeURIComponent(input.destinationId)}/runs/${encodeURIComponent(input.runId)}/virtual-machines/${encodeURIComponent(input.name)}/restore`, { confirmName: input.confirmName }),
    onSuccess: () => {
      toast.success('VM restored stopped; review its settings before starting it')
      void qc.invalidateQueries({ queryKey: queryKeys.virtualMachines })
      void qc.invalidateQueries({ queryKey: queryKeys.recoverableVirtualMachines })
      void qc.invalidateQueries({ queryKey: ['backups', 'runs'] })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'VM restore failed'),
  })
}

export interface BackupDestinationInput {
  name: string
  type: 'local' | 's3' | 'sftp' | 'rclone'
  target: string
  enabled: boolean
  retention: { generations: number; daily: number; monthly: number; immutableDays?: number; providerObjectLock?: boolean }
  credentials?: Record<string, string>
}

export function useSaveBackupDestination() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: BackupDestinationInput) =>
      apiPost('/backups/destinations', input),
    onSuccess: () => {
      toast.success('Backup destination saved')
      void qc.invalidateQueries({ queryKey: queryKeys.backupDestinations })
      void qc.invalidateQueries({ queryKey: queryKeys.backupReadiness })
    },
  })
}

export function useGenerations() {
  return useQuery({
    queryKey: queryKeys.generations,
    queryFn: () => apiGet<ConfigGeneration[]>('/backup/generations'),
  })
}

export function useRestorePlan() {
  return useQuery({
    queryKey: queryKeys.restorePlan,
    queryFn: () => apiGet<RestorePlan>('/backup/restore/plan'),
  })
}

export function useSettings() {
  return useQuery({ queryKey: queryKeys.settings, queryFn: () => apiGet<AppSettings>('/settings') })
}

export function useRevokeSession() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiDelete<void>(`/settings/sessions/${id}`),
    onSuccess: () => {
      toast.success('Session revoked')
      void qc.invalidateQueries({ queryKey: queryKeys.settings })
    },
  })
}

export function useRevokeOtherSessions() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => apiDelete<{ revoked: number }>('/settings/sessions'),
    onSuccess: (result) => {
      toast.success(`${result.revoked} other session${result.revoked === 1 ? '' : 's'} revoked`)
      void qc.invalidateQueries({ queryKey: queryKeys.settings })
    },
  })
}

export function useUpdateSettings() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { section: keyof AppSettings; patch: Record<string, unknown> }) =>
      apiPatch<AppSettings>('/settings', input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.settings })
    },
  })
}

export function useCheckUpdates() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<Job>('/updates/check'),
    onSuccess: (job) => {
      toast.success(`Checking for updates — ${job.title}`)
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
      void qc.invalidateQueries({ queryKey: queryKeys.settings })
    },
  })
}

export function useOnboardingState() {
  return useQuery({
    queryKey: ['onboarding', 'state'],
    queryFn: () => apiGet<OnboardingState>('/onboarding/state'),
  })
}

export function useCreateRecoveryKey() {
  return useMutation({
    mutationFn: () => apiPost<{ key: string; generated: boolean }>('/recovery/key'),
  })
}

export function useCompleteOnboarding() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: {
      serverName: string
      roles: Record<string, DiskRole>
      protection: { syncTime: string; scrubDay: string }
      recovery: { autoConfigBackup: boolean; destination: string; keyAcknowledged: boolean }
    }) => apiPost<{ ok: boolean; initialSyncStarted: boolean }>('/onboarding/complete', input),
    onSuccess: () => {
      void qc.invalidateQueries()
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

export function useSnoozeAlert() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ id, hours }: { id: string; hours: number }) => apiPost<{ id: string; snoozedUntil: string }>(`/alerts/${id}/snooze`, { hours }),
    onSuccess: () => void qc.invalidateQueries({ queryKey: queryKeys.alerts }),
  })
}

export function useWireGuardStatus() {
  return useQuery({ queryKey: queryKeys.wireguardStatus, queryFn: () => apiGet<WireGuardStatus>('/network/wireguard/status') })
}

export function useApplyWireGuard() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (config: WireGuardConfig) => apiPost('/network/wireguard/apply', config),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.wireguardStatus })
      toast.success('WireGuard configuration applied')
    },
  })
}

export function useWireGuardKeygen() {
  return useMutation({
    mutationFn: () => apiPost<{ privateKey: string; publicKey: string }>('/network/wireguard/keygen'),
  })
}

export function useTailscaleStatus() {
  return useQuery({ queryKey: queryKeys.tailscaleStatus, queryFn: () => apiGet<TailscaleStatus>('/network/tailscale/status') })
}

export function useTailscaleUp() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { hostname: string; authKey?: string }) => apiPost('/network/tailscale/up', input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.tailscaleStatus })
      toast.success('Tailscale connected')
    },
  })
}

export function useTailscaleDown() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost('/network/tailscale/down'),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.tailscaleStatus })
      toast.success('Tailscale disconnected')
    },
  })
}

export function useTailscaleExitNode() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (peerIp: string) => apiPost('/network/tailscale/exit-node', { peerIp }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: queryKeys.tailscaleStatus })
      toast.success('Exit node updated')
    },
  })
}

// --- SSH Keys ---

export function useSSHKeys() {
  return useQuery({ queryKey: ['admin', 'ssh', 'keys'], queryFn: () => apiGet<SSHKey[]>('/ssh/keys') })
}

export function useAddSSHKey() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (publicKey: string) => apiPost('/ssh/keys', { publicKey }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin', 'ssh', 'keys'] })
      toast.success('SSH key added')
    },
  })
}

export function useRemoveSSHKey() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (publicKey: string) => apiPost('/ssh/keys/remove', { publicKey }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['admin', 'ssh', 'keys'] })
      toast.success('SSH key removed')
    },
  })
}

export function useNetworkInterfaces() {
  return useQuery({
    queryKey: queryKeys.networkInterfaces,
    queryFn: () => apiGet<NetworkInterface[]>('/network/interfaces'),
    throwOnError: false,
  })
}

export function useLanHosts() {
  return useQuery({
    queryKey: ['network', 'lan', 'hosts'],
    queryFn: () => apiGet<LanHost[]>('/network/lan/hosts'),
    throwOnError: false,
  })
}

export function useLanScan() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<LanHost[]>('/network/lan/scan'),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['network', 'lan'] })
    },
  })
}

export function useLanWake() {
  return useMutation({
    mutationFn: (input: { mac: string; interface: string }) =>
      apiPost<{ status: string }>('/network/lan/hosts/wake', input),
  })
}

export function useLanRename() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { mac: string; interface: string; hostname: string }) =>
      apiPost<{ status: string }>('/network/lan/hosts/rename', input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['network', 'lan'] })
    },
  })
}

export function useNetworkConnections() {
  return useQuery({
    queryKey: queryKeys.networkConnections,
    queryFn: () => apiGet<NetworkConnection[]>('/network/connections'),
    throwOnError: false,
  })
}

export function useWiFiScan(enabled = true) {
  return useQuery({
    queryKey: queryKeys.wifiScan,
    queryFn: () => apiGet<WiFiScanResult>('/network/wifi/scan'),
    enabled,
    staleTime: 10_000,
    throwOnError: false,
  })
}

function invalidateNetwork(qc: ReturnType<typeof useQueryClient>) {
  void qc.invalidateQueries({ queryKey: queryKeys.networkConnections })
  void qc.invalidateQueries({ queryKey: queryKeys.activity })
  void qc.invalidateQueries({ queryKey: queryKeys.jobs })
}

export type ConnectionPayload = Omit<NetworkConnection, 'id' | 'generation' | 'status'> & {
  reauthenticated: boolean
}

export function useCreateNetworkConnection() {
  const qc = useQueryClient()
  return useMutation<NetworkConnection, Error, ConnectionPayload>({
    mutationFn: (input) => apiPost<NetworkConnection>('/network/connections', input),
    onSuccess: (_data, input) => {
      toast.success(`Connection “${input.name}” saved — apply it to activate`)
      invalidateNetwork(qc)
    },
  })
}

export function useUpdateNetworkConnection() {
  const qc = useQueryClient()
  return useMutation<NetworkConnection, Error, ConnectionPayload & { id: string }>({
    mutationFn: ({ id, ...input }) => apiPatch<NetworkConnection>(`/network/connections/${id}`, input),
    onSuccess: (data) => {
      toast.success(`Connection “${data.name}” updated — apply it to activate`)
      invalidateNetwork(qc)
    },
  })
}

export function useApplyNetworkConnection() {
  const qc = useQueryClient()
  return useMutation<{ operationId?: string }, Error, { id: string; wifiPassword?: string }>({
    mutationFn: ({ id, wifiPassword }) =>
      apiPost<{ operationId?: string }>(`/network/connections/${id}/apply`, {
        reauthenticated: true,
        timeoutSeconds: 60,
        ...(wifiPassword != null ? { wifiPassword } : {}),
      }),
    onSuccess: () => invalidateNetwork(qc),
  })
}

// --- Storage write-plane ---

export function useStorageMounts() {
  return useQuery({
    queryKey: queryKeys.storageMounts,
    queryFn: () => apiGet<{ entries: StorageMount[] }>('/storage/mounts'),
    select: (data) => data.entries,
  })
}

export function useStorageSnapshots(source?: string) {
  const query = source ? `?source=${encodeURIComponent(source)}` : ''
  return useQuery({
    queryKey: ['storage', 'snapshots', source ?? 'all'],
    queryFn: () => apiGet<StorageSnapshot[]>(`/storage/snapshots${query}`),
    throwOnError: false,
  })
}

export function useStorageSnapshotFiles(snapshotId: string | null, path = '') {
  const query = path ? `?path=${encodeURIComponent(path)}` : ''
  return useQuery({
    queryKey: ['storage', 'snapshot-files', snapshotId, path],
    queryFn: () => apiGet<StorageSnapshotFiles>(`/storage/snapshots/${snapshotId}/files${query}`),
    enabled: snapshotId != null,
    throwOnError: false,
  })
}

export function useStorageSnapshotDiff(snapshotId: string | null) {
  return useQuery({
    queryKey: ['storage', 'snapshots', snapshotId, 'compare'],
    queryFn: () => apiGet<SnapshotDiff>(`/storage/snapshots/${encodeURIComponent(snapshotId!)}/compare`),
    enabled: snapshotId != null,
    throwOnError: false,
  })
}

export function useRestoreSnapshotEntries() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { snapshotId: string; shareId: string; snapshotPath: string; targetPath: string; names: string[]; createTargetDirectories?: boolean }) =>
      apiPost<{ jobId: string; state: string; restoring: number }>(`/storage/snapshots/${encodeURIComponent(input.snapshotId)}/restore`, {
        shareId: input.shareId,
        snapshotPath: input.snapshotPath,
        targetPath: input.targetPath,
        names: input.names,
        createTargetDirectories: input.createTargetDirectories ?? false,
      }),
    onSuccess: (_result, input) => {
      toast.success('Snapshot restore queued')
      void qc.invalidateQueries({ queryKey: ['files', input.shareId] })
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not restore snapshot entry'),
  })
}

export function useCreateStorageSnapshot() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { kind: 'btrfs' | 'zfs'; source: string; label?: string; retentionLockDays?: number }) =>
      apiPost<StorageSnapshot>('/storage/snapshots', input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['storage', 'snapshots'] })
    },
  })
}

export function useDeleteStorageSnapshot() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) =>
      apiDelete<{ status: string }>(`/storage/snapshots/${id}`, {
        reauthenticated: true,
        storageSafetyUnlocked: true,
      }),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['storage', 'snapshots'] })
    },
  })
}

export function useStorageSafety() {
  return useQuery({
    queryKey: queryKeys.storageSafety,
    queryFn: () => apiGet<StorageSafety>('/storage/safety'),
  })
}

export function usePlanDiskReplacement() {
  return useMutation({
    mutationFn: (input: { retiredDiskId: string; replacementDiskId: string }) =>
      apiPost<DiskReplacementPlan>('/storage/protection/replacement/plan', input),
  })
}

export function useConfirmDiskReplacement() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { operationId: string; planHash: string }) =>
      apiPost<DiskReplacementResult>('/storage/protection/replacement/confirm', {
        ...input,
        reauthenticated: true,
        storageSafetyUnlocked: true,
      }),
    onSuccess: () => {
      toast.success('Disk replacement started — parity recovery is queued')
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
      void qc.invalidateQueries({ queryKey: queryKeys.disks })
      void qc.invalidateQueries({ queryKey: queryKeys.protection })
    },
  })
}

export function useAPITokens() {
  return useQuery({ queryKey: ['api-tokens'], queryFn: () => apiGet<APITokenSummary[]>('/api-tokens') })
}

export function useCreateAPIToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { name: string; scopes: ('read' | 'backup:write' | 'replication:receive' | 'fleet:status' | `workstation:backup:${string}` | `replication:snapshot:receive:${string}`)[]; expiresAt?: string }) => apiPost<APITokenCreated>('/api-tokens', input),
    onSuccess: () => void qc.invalidateQueries({ queryKey: ['api-tokens'] }),
  })
}

export function useDeleteAPIToken() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiDelete(`/api-tokens/${id}`),
    onSuccess: () => { toast.success('API token revoked'); void qc.invalidateQueries({ queryKey: ['api-tokens'] }) },
  })
}

export function useReplicationPeers() { return useQuery({ queryKey: ['replication', 'peers'], queryFn: () => apiGet<ReplicationPeer[]>('/replication/peers'), refetchInterval: 30_000 }) }
export function useSaveReplicationPeer() { const qc = useQueryClient(); return useMutation({ mutationFn: (input: { name: string; url: string; token: string; statusToken?: string }) => apiPost<ReplicationPeer>('/replication/peers', input), onSuccess: () => void qc.invalidateQueries({ queryKey: ['replication', 'peers'] }) }) }
export function useRunReplicationPeer() { const qc = useQueryClient(); return useMutation({ mutationFn: (id: string) => apiPost<{ ok: boolean; peerId: string; bytes: number }>(`/replication/peers/${id}/run`), onSuccess: () => { toast.success('Recovery bundle replicated'); void qc.invalidateQueries({ queryKey: ['replication', 'peers'] }) } }) }
export function useDeleteReplicationPeer() { const qc = useQueryClient(); return useMutation({ mutationFn: (id: string) => apiDelete(`/replication/peers/${id}`), onSuccess: () => void qc.invalidateQueries({ queryKey: ['replication', 'peers'] }) }) }

export function useSnapshotReplicationTasks(peerId?: string) { return useQuery({ queryKey: ['replication', 'snapshot-tasks', peerId ?? 'all'], queryFn: () => apiGet<SnapshotReplicationTask[]>(`/replication/snapshot-tasks?peerId=${encodeURIComponent(peerId ?? '')}`), refetchInterval: 10_000 }) }
export function useCreateSnapshotReplicationTask() { const qc=useQueryClient(); return useMutation({ mutationFn:(input:{peerId:string;name:string;sourceShareId:string;destinationShareId:string;receiveToken:string;scheduleKind:'manual'|'daily'|'weekly';timeOfDay:string;weekday?:string})=>apiPost<SnapshotReplicationTask>('/replication/snapshot-tasks',input),onSuccess:()=>{toast.success('Snapshot replication task created');void qc.invalidateQueries({queryKey:['replication','snapshot-tasks']})} }) }
export function useUpdateSnapshotReplicationTask() { const qc=useQueryClient(); return useMutation({mutationFn:(input:{id:string;name:string;scheduleKind:'manual'|'daily'|'weekly';timeOfDay:string;weekday?:string;receiveToken?:string})=>apiPut<SnapshotReplicationTask>(`/replication/snapshot-tasks/${input.id}`,{name:input.name,scheduleKind:input.scheduleKind,timeOfDay:input.timeOfDay,weekday:input.weekday,receiveToken:input.receiveToken}),onSuccess:()=>{toast.success('Snapshot replication task updated');void qc.invalidateQueries({queryKey:['replication','snapshot-tasks']})}}) }
export function useRunSnapshotReplicationTask() { const qc=useQueryClient(); return useMutation({mutationFn:(id:string)=>apiPost<{ok:boolean;run:SnapshotReplicationRun;jobId:string}>(`/replication/snapshot-tasks/${id}/run`),onSuccess:()=>{toast.success('Snapshot replication started');void qc.invalidateQueries({queryKey:['replication','snapshot-tasks']})} }) }
export function useCancelSnapshotReplicationTask() { const qc=useQueryClient(); return useMutation({mutationFn:(id:string)=>apiPost<{status:string}>(`/replication/snapshot-tasks/${id}/cancel`),onSuccess:()=>{toast.info('Cancellation requested');void qc.invalidateQueries({queryKey:['replication','snapshot-tasks']})} }) }
export function useDeleteSnapshotReplicationTask() { const qc=useQueryClient(); return useMutation({mutationFn:(id:string)=>apiDelete(`/replication/snapshot-tasks/${id}`),onSuccess:()=>{toast.success('Snapshot replication task removed');void qc.invalidateQueries({queryKey:['replication','snapshot-tasks']})} }) }
export function useSnapshotReplicationRuns(taskId:string) { return useQuery({queryKey:['replication','snapshot-runs',taskId],queryFn:()=>apiGet<SnapshotReplicationRun[]>(`/replication/snapshot-tasks/${taskId}/runs`),enabled:Boolean(taskId),refetchInterval:5_000}) }

export function useUnlockStorageSafety() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<StorageSafety>('/storage/safety/unlock', { reauthenticated: true }),
    onSuccess: (safety) => {
      toast.success('Storage safety unlocked for 15 minutes')
      qc.setQueryData(queryKeys.storageSafety, safety)
    },
  })
}

export function useUnlockEncryptedDisk() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { diskId: string; passphrase: string }) =>
      apiPost<{ ok: boolean; mountPath: string }>(`/storage/disks/${input.diskId}/unlock`, { passphrase: input.passphrase }),
    onSuccess: () => {
      toast.success('Encrypted disk unlocked and mounted')
      void qc.invalidateQueries({ queryKey: queryKeys.disks })
      void qc.invalidateQueries({ queryKey: queryKeys.storageMounts })
      void qc.invalidateQueries({ queryKey: queryKeys.pools })
    },
  })
}

export function useLockStorageSafety() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<StorageSafety>('/storage/safety/lock', {}),
    onSuccess: (safety) => {
      toast.success('Storage safety locked')
      qc.setQueryData(queryKeys.storageSafety, safety)
    },
  })
}

export function useProtectionConfig() {
  return useQuery({
    queryKey: queryKeys.protectionConfig,
    queryFn: () => apiGet<ProtectionConfig>('/storage/protection/config'),
  })
}

export function useUpdateProtectionConfig() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { parityDiskId: string; dataDiskIds: string[] }) =>
      apiPut<{ configPath: string; config: string }>('/storage/protection/config', input),
    onSuccess: () => {
      toast.success('SnapRAID configuration saved')
      void qc.invalidateQueries({ queryKey: queryKeys.protectionConfig })
      void qc.invalidateQueries({ queryKey: queryKeys.protection })
    },
  })
}

export interface PoolPlanInput {
  name: string
  diskIds: string[]
  expectedGeneration?: number | null
}

export function usePlanPool() {
  return useMutation({
    mutationFn: (input: PoolPlanInput) => apiPost<PoolPlan>('/storage/pools/plan', input),
  })
}

export function useConfirmPool() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { operationId: string; planHash: string }) =>
      apiPost<{ ok: boolean }>(`/storage/pools/${input.operationId}/confirm`, {
        planHash: input.planHash,
        reauthenticated: true,
        storageSafetyUnlocked: true,
      }),
    onSuccess: () => {
      toast.success('Pool operation accepted — check activity for progress')
      void qc.invalidateQueries({ queryKey: queryKeys.pools })
      void qc.invalidateQueries({ queryKey: queryKeys.storageMounts })
      void qc.invalidateQueries({ queryKey: queryKeys.disks })
    },
  })
}

export function usePlanPoolUnmount() {
  return useMutation({
    mutationFn: (input: { name: string; expectedGeneration?: number | null }) =>
      apiPost<PoolUnmountPlan>('/storage/pools/unmount/plan', input),
  })
}

export function useConfirmPoolUnmount() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { operationId: string; planHash: string }) =>
      apiPost<{ ok: boolean }>(`/storage/pools/unmount/${input.operationId}/confirm`, {
        planHash: input.planHash,
        reauthenticated: true,
        storageSafetyUnlocked: true,
      }),
    onSuccess: () => {
      toast.success('Pool unmount accepted')
      void qc.invalidateQueries({ queryKey: queryKeys.pools })
      void qc.invalidateQueries({ queryKey: queryKeys.storageMounts })
    },
  })
}

export type StorageOperationAction =
  | 'filesystem.format'
  | 'filesystem.create'
  | 'filesystem.mount'
  | 'filesystem.unmount'
  | 'disk.erase'

export interface StorageOperationInput {
  action: StorageOperationAction
  diskId: string
  requestedState?: Record<string, unknown>
}

export function usePlanStorageOperation() {
  return useMutation({
    mutationFn: (input: StorageOperationInput) =>
      apiPost<StorageOperationPlan>('/storage/operations/plan', input),
  })
}

export function useConfirmStorageOperation() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { operationId: string; planHash: string; encryptionPassphrase?: string }) =>
      apiPost<{ ok: boolean }>(`/storage/operations/${input.operationId}/confirm`, {
        planHash: input.planHash,
        reauthenticated: true,
        storageSafetyUnlocked: true,
        ...(input.encryptionPassphrase ? { encryptionPassphrase: input.encryptionPassphrase } : {}),
      }),
    onSuccess: () => {
      toast.success('Storage operation completed')
      void qc.invalidateQueries({ queryKey: queryKeys.disks })
      void qc.invalidateQueries({ queryKey: queryKeys.pools })
      void qc.invalidateQueries({ queryKey: queryKeys.storageMounts })
      void qc.invalidateQueries({ queryKey: queryKeys.protection })
      void qc.invalidateQueries({ queryKey: queryKeys.protectionConfig })
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
    },
  })
}

// --- Recovery center ---

export function useRecoveryStatus() {
  return useQuery({
    queryKey: queryKeys.recoveryStatus,
    queryFn: () => apiGet<RecoveryStatus>('/recovery/status'),
  })
}

export function useRecoveryPlan() {
  return useQuery({
    queryKey: queryKeys.recoveryPlan,
    queryFn: () => apiGet<RecoveryPlan>('/recovery/plan'),
    throwOnError: false,
  })
}

export function useExportRecovery() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<RecoveryExportResponse>('/recovery/export', {}),
    onSuccess: () => {
      toast.success('Recovery bundle exported and verified')
      void qc.invalidateQueries({ queryKey: queryKeys.recoveryStatus })
      void qc.invalidateQueries({ queryKey: queryKeys.recoveryPlan })
      void qc.invalidateQueries({ queryKey: queryKeys.backupReadiness })
    },
  })
}

export function useDownloadRecovery() {
  return useMutation({
    mutationFn: () => apiDownload('/recovery/download'),
    onSuccess: (blob) => {
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = 'lumonas-recovery.mrb'
      link.click()
      URL.revokeObjectURL(url)
    },
    onError: () => toast.error('Recovery bundle could not be downloaded'),
  })
}

export function useStageRestore() {
  return useMutation({
    mutationFn: () =>
      apiPost<{ directory: string; files: string[]; verified: boolean }>(
        '/recovery/restore/stage',
        { confirmed: true, reauthenticated: true },
      ),
  })
}

export function useStageAppdata() {
  return useMutation({
    mutationFn: (stack: string) => apiPost<{ scope: 'appdata'; stack: string; directory: string; files: string[]; verified: boolean; generation: number }>(
      '/recovery/restore/stage-appdata',
      { stack, confirmed: true, reauthenticated: true },
    ),
    onSuccess: (result) => toast.success(`${result.stack} app data staged for offline recovery`),
  })
}

export function useRestoreDrills() {
  return useQuery({ queryKey: queryKeys.restoreDrills, queryFn: () => apiGet<RestoreDrill[]>('/recovery/drills'), refetchInterval: 5000 })
}

export function useRestoreDrillSchedule() {
  return useQuery({ queryKey: queryKeys.restoreDrillSchedule, queryFn: () => apiGet<RestoreDrillSchedule>('/recovery/drills/schedule') })
}

export function useUpdateRestoreDrillSchedule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: Partial<RestoreDrillSchedule>) => apiPatch<RestoreDrillSchedule>('/recovery/drills/schedule', input),
    onSuccess: (value) => qc.setQueryData(queryKeys.restoreDrillSchedule, value),
  })
}

export function useWorkloadRecoveryObjectives() {
  return useQuery({ queryKey: ['recovery', 'workload-objectives'], queryFn: () => apiGet<WorkloadRecoveryObjective[]>('/recovery/workload-objectives') })
}

export function useSetWorkloadRecoveryObjective() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: WorkloadRecoveryObjective) => apiPut<WorkloadRecoveryObjective>('/recovery/workload-objectives', input),
    onSuccess: () => {
      toast.success('Workload recovery objectives saved')
      void qc.invalidateQueries({ queryKey: ['recovery', 'workload-objectives'] })
      void qc.invalidateQueries({ queryKey: queryKeys.alerts })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not save workload objectives'),
  })
}

export function useRunRestoreDrill() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: () => apiPost<RestoreDrill>('/recovery/drills/run', {}),
    onSuccess: () => {
      toast.success('Restore drill started')
      void qc.invalidateQueries({ queryKey: queryKeys.restoreDrills })
    },
  })
}

// --- Notifications management ---

export function useUpdateNotificationChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: {
      id: string
      label?: string
      target?: string
      enabled?: boolean
		credentials?: NotificationCredentials
    }) => apiPatch<NotificationChannel>(`/notification-channels/${input.id}`, input),
    onSuccess: (channel) => {
      toast.success(`Notification channel saved — ${channel.label}`)
      void qc.invalidateQueries({ queryKey: queryKeys.notificationChannels })
    },
  })
}

export function useDeleteNotificationChannel() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiDelete<void>(`/notification-channels/${id}`),
    onSuccess: () => {
      toast.success('Notification channel deleted')
      void qc.invalidateQueries({ queryKey: queryKeys.notificationChannels })
    },
  })
}

export function useNotificationDeliveries() {
  return useQuery({
    queryKey: queryKeys.notificationDeliveries,
    queryFn: () => apiGet<NotificationDelivery[]>('/notification-deliveries?limit=50'),
  })
}

export function useNotificationRules() {
  return useQuery({
    queryKey: queryKeys.notificationRules,
    queryFn: () => apiGet<AlertRule[]>('/notification-rules'),
  })
}

export function useSaveNotificationRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { name: string; condition: string; severity?: AlertRule['severity']; routes?: string[] }) =>
      apiPost<AlertRule>('/notification-rules', input),
    onSuccess: (rule) => {
      toast.success(`Notification rule saved — ${rule.name}`)
      void qc.invalidateQueries({ queryKey: queryKeys.notificationRules })
    },
  })
}

export function useToggleNotificationRule() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (rule: AlertRule) =>
      apiPatch<AlertRule>(`/notification-rules/${rule.id}`, { enabled: !rule.enabled }),
    onSuccess: (rule) => {
      toast.success(rule.enabled ? `Rule enabled — ${rule.name}` : `Rule disabled — ${rule.name}`)
      void qc.invalidateQueries({ queryKey: queryKeys.notificationRules })
    },
  })
}

// --- Admin extras ---

export function useAudit(filters: { actor?: string; action?: string; outcome?: string; q?: string; from?: string; to?: string }, cursor?: string) {
  return useQuery({
    queryKey: [...queryKeys.audit, filters, cursor],
    queryFn: () => {
      const params = new URLSearchParams({ page: '1', limit: '100' })
      for (const [key, value] of Object.entries(filters)) if (value) params.set(key, value)
      if (cursor) params.set('cursor', cursor)
      return apiGet<AuditPage>(`/audit?${params.toString()}`)
    },
  })
}

export function useAuditRetention() {
  return useQuery({ queryKey: [...queryKeys.audit, 'retention'], queryFn: () => apiGet<{ retentionDays: number }>('/audit/retention') })
}

export function useSetAuditRetention() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (retentionDays: number) => apiPut<{ retentionDays: number }>('/audit/retention', { retentionDays }),
    onSuccess: () => {
      toast.success('Audit retention updated')
      void qc.invalidateQueries({ queryKey: [...queryKeys.audit, 'retention'] })
    },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not update audit retention'),
  })
}

export function useCapacityForecast(days = 30) {
  return useQuery({
    queryKey: [...queryKeys.capacityForecast, days],
    queryFn: () => apiGet<CapacityForecast[]>(`/capacity/forecast?days=${days}`),
    throwOnError: false,
  })
}

export function useCapacityThresholds() {
  return useQuery({ queryKey: ['capacity', 'thresholds'], queryFn: () => apiGet<CapacityThreshold[]>('/capacity/thresholds') })
}

export function useSetCapacityThreshold() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { resourceId: string; thresholdPercent: number }) => apiPut<CapacityThreshold>('/capacity/thresholds', input),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['capacity', 'thresholds'] })
      void qc.invalidateQueries({ queryKey: queryKeys.alerts })
    },
  })
}

export function useGroups() {
  return useQuery({
    queryKey: queryKeys.groups,
    // The backend serialises identity.Principal with `kind`; normalise it to
    // the frontend Principal shape.
    queryFn: async () => {
      const raw = await apiGet<{ id: string; name: string; kind: string; enabled: boolean }[]>(
        '/groups',
      )
      return raw.map(
        (group): Principal => ({ id: group.id, name: group.name, type: 'group' }),
      )
    },
    throwOnError: false,
  })
}

export function useCreateGroup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (name: string) => apiPost<Principal>('/groups', { name }),
    onSuccess: (group) => {
      toast.success(`Group created — ${group.name}`)
      void qc.invalidateQueries({ queryKey: queryKeys.groups })
      void qc.invalidateQueries({ queryKey: queryKeys.users })
    },
  })
}

export function useDeleteGroup() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiDelete<void>(`/groups/${id}`),
    onSuccess: () => {
      toast.success('Group deleted')
      void qc.invalidateQueries({ queryKey: queryKeys.groups })
      void qc.invalidateQueries({ queryKey: queryKeys.users })
    },
  })
}

export function useSetGroupMembers() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { id: string; memberIds: string[] }) =>
      apiPut<Principal[]>(`/groups/${input.id}/members`, { memberIds: input.memberIds }),
    onSuccess: () => {
      toast.success('Group members updated')
      void qc.invalidateQueries({ queryKey: queryKeys.groups })
      void qc.invalidateQueries({ queryKey: queryKeys.users })
    },
  })
}

export function useSetUserPassword() {
  return useMutation({
    mutationFn: (input: { id: string; password: string }) =>
      apiPost<void>(`/users/${input.id}/password`, { password: input.password }),
    onSuccess: () => toast.success('Password updated'),
  })
}

export function useDeleteUser() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => apiDelete<void>(`/users/${id}`),
    onSuccess: () => {
      toast.success('User deleted')
      void qc.invalidateQueries({ queryKey: queryKeys.users })
    },
  })
}

export function useFileSearch(shareId: string | null, query: string) {
  return useQuery({
    queryKey: [...queryKeys.files(shareId, ''), 'search', query],
    queryFn: async () => {
      const value = await apiGet<{ shareId: string; entries: FileEntry[] }>(
        `/files/search?share=${encodeURIComponent(shareId ?? '')}&q=${encodeURIComponent(query)}`,
      )
      return value.entries
    },
    enabled: Boolean(shareId) && query.trim().length >= 2,
    throwOnError: false,
  })
}

export function useFileContentSearch(shareId: string | null, query: string) {
  return useQuery({
    queryKey: ['file-content-search', shareId, query],
    queryFn: () => apiGet<{ shareId: string; indexedAt: string | null; results: import('@/api/types').FileContentResult[] }>(`/files/content-search?share=${encodeURIComponent(shareId!)}&q=${encodeURIComponent(query)}`),
    enabled: Boolean(shareId) && query.trim().length >= 3,
    retry: false,
    throwOnError: false,
  })
}

export function useFileContentIndexStatus(shareId: string | null) {
  return useQuery({
    queryKey: ['file-content-index', shareId],
    queryFn: () => apiGet<import('@/api/types').FileContentIndexStatus>(`/files/search-index?shareId=${encodeURIComponent(shareId!)}`),
    enabled: Boolean(shareId),
    retry: false,
  })
}

export function useBuildFileContentIndex() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (shareId: string) => apiPost<import('@/api/types').FileContentIndexBuildResult>('/files/search-index', { shareId }),
    onSuccess: (result) => { toast.success(`Indexed ${result.documents.toLocaleString()} text files`, { description: result.skipped ? `${result.skipped.toLocaleString()} files were skipped by the safety limits or file type.` : undefined }); void qc.invalidateQueries({ queryKey: ['file-content-index', result.shareId] }); void qc.invalidateQueries({ queryKey: ['file-content-search', result.shareId] }) },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not index share contents'),
  })
}

export function useFileIntegrityStatus(shareId: string | null) {
  return useQuery({
    queryKey: ['file-integrity', shareId],
    queryFn: () => apiGet<import('@/api/types').FileIntegrityStatus>(`/files/integrity?shareId=${encodeURIComponent(shareId!)}`),
    enabled: Boolean(shareId),
    refetchInterval: (query) => query.state.data?.running ? 3000 : false,
  })
}

export function useStartFileIntegrityScan(action: 'baseline' | 'verify') {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (shareId: string) => action === 'baseline'
      ? apiPost<Job>('/files/integrity/baseline', { shareId })
      : apiPost<Job>('/files/integrity/verify', { shareId }),
    onSuccess: (_job, shareId) => { toast.success(action === 'baseline' ? 'Integrity baseline scan queued' : 'Integrity verification queued'); void qc.invalidateQueries({ queryKey: ['file-integrity', shareId] }); void qc.invalidateQueries({ queryKey: queryKeys.jobs }) },
    onError: (error) => toast.error(error instanceof Error ? error.message : 'Could not start integrity scan'),
  })
}

export interface Checkpoint {
  operationId: string
  expiresAt?: string
  status?: string
}

export function useBeginCheckpoint() {
  return useMutation({
    mutationFn: () =>
      apiPost<Checkpoint>('/network/checkpoints', { reauthenticated: true, timeoutSeconds: 120 }),
    onSuccess: () => toast.success('Network checkpoint opened'),
  })
}

export function useCheckpointAction() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (input: { id: string; action: 'commit' | 'rollback' }) =>
      apiPost<{ ok: boolean }>(`/network/checkpoints/${input.id}/${input.action}`, {}),
    onSuccess: (_data, input) => {
      toast.success(input.action === 'commit' ? 'Checkpoint committed' : 'Checkpoint rolled back')
      void qc.invalidateQueries({ queryKey: queryKeys.networkConnections })
    },
  })
}
