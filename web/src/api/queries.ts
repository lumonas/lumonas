import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { ApiError, apiDelete, apiGet, apiMultipart, apiPatch, apiPut, apiPost } from '@/api/client'
import { useLogsStore } from '@/stores/logs'
import type {
  ActivityEvent,
  Alert,
  AlertRule,
  AuditEntry,
  BackupDestination,
  BackupJob,
  CapacityForecast,
  CatalogApp,
  ConfigGeneration,
  DockerContainer,
  DockerImage,
  DockerStack,
  DockerSummary,
  DockerVolume,
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
  RestorePlan,
  ServerInfo,
  ServiceStatus,
  Share,
  StorageMount,
  StorageOperationPlan,
  StorageSafety,
  SystemMetrics,
  UPSStatus,
  UPSPolicy,
  UserGroup,
  WireGuardConfig,
  WireGuardStatus,
  TailscaleStatus,
  SSHKey,
  NetworkConnection,
  NetworkInterface,
  WiFiScanResult,
} from '@/api/types'

export const queryKeys = {
  server: ['server'] as const,
  healthComponents: ['health', 'components'] as const,
  disks: ['disks'] as const,
  disk: (id: string) => ['disks', id] as const,
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
  notificationDeliveries: ['notifications', 'deliveries'] as const,
  notificationRules: ['notification-rules'] as const,
  audit: ['audit'] as const,
  capacityForecast: ['capacity', 'forecast'] as const,
  groups: ['groups'] as const,
  ups: ['power', 'ups'] as const,
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

export function useUPS() {
  return useQuery({ queryKey: queryKeys.ups, queryFn: () => apiGet<UPSStatus[]>('/power/ups') })
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
    mutationFn: (input: { type: string; resourceId?: string }) =>
      apiPost<Job>('/jobs', input),
    onSuccess: () => {
      toast.success('Job queued', {
        description: 'Track progress from the jobs menu in the top bar.',
      })
      void qc.invalidateQueries({ queryKey: queryKeys.jobs })
    },
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

export interface NotificationChannelInput {
  type: Exclude<NotificationChannel['type'], 'web'>
  label: string
  target: string
  enabled: boolean
  credentials?: { token?: string; username?: string; password?: string; address?: string }
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
      body: Partial<{ enabled: boolean; hosts: string; readOnly: boolean; quotaBytes: number }>
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
    const body = new FormData()
    body.set('shareId', input.shareId)
    body.set('path', input.path)
    body.set('file', input.file, input.file.name)
    return apiMultipart<{ jobId: string; name: string }>('/files/upload', body)
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

export interface BackupDestinationInput {
  name: string
  type: 'local' | 's3' | 'sftp'
  target: string
  enabled: boolean
  retention: { generations: number; daily: number; monthly: number }
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

export function useStorageSafety() {
  return useQuery({
    queryKey: queryKeys.storageSafety,
    queryFn: () => apiGet<StorageSafety>('/storage/safety'),
  })
}

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
    mutationFn: (input: { operationId: string; planHash: string }) =>
      apiPost<{ ok: boolean }>(`/storage/operations/${input.operationId}/confirm`, {
        planHash: input.planHash,
        reauthenticated: true,
        storageSafetyUnlocked: true,
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

export function useStageRestore() {
  return useMutation({
    mutationFn: () =>
      apiPost<{ directory: string; files: string[]; verified: boolean }>(
        '/recovery/restore/stage',
        { confirmed: true, reauthenticated: true },
      ),
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
      credentials?: { token?: string; username?: string; password?: string; address?: string }
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

export function useAudit() {
  return useQuery({
    queryKey: queryKeys.audit,
    queryFn: () => apiGet<AuditEntry[]>('/audit?limit=100'),
  })
}

export function useCapacityForecast(days = 30) {
  return useQuery({
    queryKey: [...queryKeys.capacityForecast, days],
    queryFn: () => apiGet<CapacityForecast[]>(`/capacity/forecast?days=${days}`),
    throwOnError: false,
  })
}

export function useGroups() {
  return useQuery({
    queryKey: queryKeys.groups,
    queryFn: () => apiGet<Principal[]>('/groups'),
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

export function useFileSearch(query: string) {
  return useQuery({
    queryKey: [...queryKeys.files(null, ''), 'search', query],
    queryFn: () => apiGet<FileEntry[]>(`/files/search?q=${encodeURIComponent(query)}`),
    enabled: query.trim().length >= 2,
    throwOnError: false,
  })
}

export interface Checkpoint {
  operationId: string
  expiresAt?: string
  status?: string
}

export function useBeginCheckpoint() {
  return useMutation({
    mutationFn: () => apiPost<Checkpoint>('/network/checkpoints', { timeoutSeconds: 120 }),
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
