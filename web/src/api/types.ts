export type HealthState = 'healthy' | 'attention' | 'warning' | 'critical' | 'offline'

export type DiskRole = 'system' | 'apps' | 'data' | 'parity' | 'backup' | 'external' | 'unknown'

export interface SmartSummary {
  overall: HealthState
  reallocatedSectors: number
  pendingSectors: number
  uncorrectableSectors: number
  crcErrors: number
  powerOnHours: number
  wearPercent?: number
  lastTest?: { type: 'short' | 'extended'; result: 'passed' | 'failed'; at: string }
}

export interface Disk {
  id: string
  name: string
  currentPath?: string
  model: string
  serial: string
  wwn?: string
  gptDiskGuid?: string
  partitionUuid?: string
  filesystemUuid?: string
  sizeBytes: number
  usedBytes?: number
  role: DiskRole
  rotational: boolean
  interface: 'sata' | 'nvme' | 'usb'
  health: HealthState
  temperatureC: number | null
  filesystem?: string
  mounted?: boolean
  poolId?: string
  standby?: boolean
  lastSeen: string
  smart: SmartSummary
}

export interface SMARTSample {
  diskId: string
  capturedAt: string
  summary: SmartSummary
  temperatureC?: number
}

export interface SMARTTrend {
  diskId: string
  sampleCount: number
  since?: string
  temperatureSlope: number
  reallocatedSlope: number
  pendingSlope: number
  uncorrectableSlope: number
  crcSlope: number
  status: HealthState
  summary: string
}

export interface SMARTHistory {
  diskId: string
  samples: SMARTSample[]
  trend: SMARTTrend
}

export interface PoolMember {
  diskId: string
  enabled: boolean
  branchPath?: string
}

export interface Pool {
  id: string
  name: string
  type: 'mergerfs'
  mountPath: string
  status: HealthState
  sizeBytes: number
  usedBytes: number
  members: PoolMember[]
}

export interface Protection {
  status: HealthState
  parityDisks: { diskId: string; sizeBytes: number; usedBytes: number }[]
  protectedDiskIds: string[]
  lastSyncAt: string | null
  lastSyncResult: 'successful' | 'failed' | null
  lastScrubAt: string | null
  changesSinceSyncBytes: number
  syncSchedule: string
  scrubSchedule: string
  syncRunning: boolean
}

export type JobState =
  | 'queued'
  | 'preparing'
  | 'running'
  | 'waiting-confirmation'
  | 'successful'
  | 'failed'
  | 'cancelled'

export interface Job {
  id: string
  correlationId?: string
  operationId?: string
  planHash?: string
  actor?: string
  generation?: number
  type: string
  title: string
  resourceId?: string
  state: JobState
  progress: number | null
  stage?: string
  createdAt: string
  startedAt?: string
  finishedAt?: string
  error?: string
}

export type ScheduleKind = 'daily' | 'weekly' | 'event'

export interface JobSchedule {
  id: string
  name: string
  jobType: string
  kind: ScheduleKind
  timeOfDay: string
  weekday?: string
  enabled: boolean
  lastStartedAt?: string
  nextDueAt?: string
  schedule: string
  next: string
  snapshotKind?: 'btrfs' | 'zfs'
  snapshotSource?: string
  snapshotLabel?: string
  snapshotKeep?: number
  snapshotLockDays?: number
  filesystemKind?: 'btrfs' | 'zfs'
  filesystemSource?: string
  relocationShareId?: string
  relocationResourceId?: string
  relocationRelativePath?: string
}

export type AlertSeverity = 'info' | 'attention' | 'warning' | 'critical'

export interface Alert {
  id: string
  severity: AlertSeverity
  title: string
  description: string
  resource?: { type: string; id: string; label: string }
  state: 'firing' | 'acknowledged' | 'resolved'
  startedAt: string
  resolvedAt?: string
  snoozedUntil?: string
}

export type ActivityCategory =
  | 'config'
  | 'storage'
  | 'docker'
  | 'backup'
  | 'security'
  | 'update'
  | 'network'

export interface ActivityEvent {
  id: string
  timestamp: string
  category: ActivityCategory
  title: string
  description?: string
  resource?: { type: string; id: string; label: string }
}

export interface ServerInfo {
  id: string
  name: string
  hostname: string
  version: string
  nasUuid: string
  timezone: string
  health: HealthState
  ip: string
}

export interface SystemMetrics {
  cpuPercent: number
  load: [number, number, number]
  ramUsedBytes: number
  ramTotalBytes: number
  cpuTempC: number
  uptimeSeconds: number
  net: { interface: string; upMbps: number; downMbps: number }
  netInterfaces?: NetInterfaceMetrics[]
  filesystems?: FilesystemUsage[]
  disk?: { readMbps: number; writeMbps: number }
}

export interface FilesystemUsage {
  path: string
  totalBytes: number
  usedBytes: number
  availableBytes: number
  usedPercent: number
  state: 'healthy' | 'warning' | 'critical' | 'unknown'
}

export interface NetInterfaceMetrics {
  interface: string
  upMbps: number
  downMbps: number
  errorsIn: number
  errorsOut: number
  droppedIn: number
  droppedOut: number
  up: boolean
}

export interface ServiceStatus {
  id: string
  name: string
  state: 'running' | 'stopped' | 'degraded' | 'unknown'
  detail?: string
}

export interface AlertRule {
  id: string
  name: string
  condition: string
  severity: AlertSeverity
  routes: string[]
  enabled: boolean
  lastTriggeredAt?: string
}

export interface NotificationChannel {
  id: string
  type: 'web' | 'telegram' | 'email' | 'ntfy' | 'discord' | 'webhook' | 'slack' | 'gotify' | 'smtp'
  label: string
  target?: string
  configured: boolean
  enabled: boolean
}

export type AccessLevel = 'none' | 'read' | 'write'

export type ShareProtocolType = 'smb' | 'nfs' | 'sftp' | 'rsync' | 'timemachine'

export interface ShareProtocolConfig {
  protocol: ShareProtocolType
  enabled: boolean
  hosts?: string
  readOnly?: boolean
  quotaBytes?: number
  auditEnabled?: boolean
  auditOperations?: string[]
}

export interface ShareStorageResource {
  id: string
  label: string
  path: string
  kind: 'pool' | 'disk'
  totalBytes?: number
  usedBytes?: number
}

export interface ShareRelocationPreview {
  shareId: string
  shareName: string
  sourcePath: string
  destinationRoot: string
  relativePath: string
  destinationPath: string
  fileCount: number
  bytes: number
  plan: { changes: { path: string; action: string; bytes: number }[]; files: number; bytes: number; deletes: number }
  planHash: string
  retainsSource: boolean
  requiresDowntime: boolean
}

export interface ShareRelocationScheduleResult {
  schedule: JobSchedule
  preview: ShareRelocationPreview
}

export interface Share {
  id: string
  name: string
  path: string
  resourceId: string
  resourceLabel: string
  relativePath: string
  description?: string
  status: HealthState
  recycleBin: boolean
  protocols: ShareProtocolConfig[]
  access: { principalId: string; level: AccessLevel }[]
  usedBytes?: number
}

export interface ShareAccessPreviewEntry {
  principalId: string
  name: string
  kind: 'user' | 'group' | 'service'
  enabled: boolean
  level: AccessLevel
  grantedBy?: string[]
  guest?: boolean
}

export interface ShareAccessPreview {
  shareId: string
  name: string
  enabled: boolean
  guestAccess: boolean
  entries: ShareAccessPreviewEntry[]
  accessRules: Array<{ principalId: string; level: AccessLevel }>
}

export interface SharePathAccessCheck {
  shareId: string
  principalId: string
  path: string
  shareEnabled: boolean
  accountEnabled: boolean
  shareLevel: AccessLevel
  filesystemAccess: 'read' | 'write' | 'none' | 'unknown'
  allowed: boolean
  explanation: string
  approximate: boolean
}

export interface ShareClientsSnapshot {
  sessions: Array<{ sessionId: string; username: string; machine: string; dialect?: string; share?: string }>
  openFiles: Array<{ path: string; sharePath?: string; opens: number }>
  capturedAt: string
  service: string
}

export interface TLSCertificateStatus {
  state: 'disabled' | 'incomplete' | 'missing' | 'invalid' | 'not-yet-valid' | 'expired' | 'expiring' | 'valid'
  configured: boolean
  issuer?: string
  subject?: string
  dnsNames?: string[]
  notBefore?: string
  notAfter?: string
  daysRemaining?: number
  detail?: string
  managedBy?: 'none' | 'manual' | 'letsencrypt'
  autoRenewalConfigured: boolean
  acmeFirewallConfigured: boolean
}

export type UserRole = 'owner' | 'operator' | 'readonly'
export type PrincipalType = 'user' | 'group' | 'service'

export interface Principal {
  id: string
  name: string
  type: PrincipalType
}

export interface ManagementUser {
  id: string
  username: string
  fullName?: string
  role: UserRole
  twoFactor: boolean
  lastLoginAt?: string
  enabled: boolean
}

export interface FileUser {
  id: string
  username: string
  fullName?: string
  type: 'user' | 'service'
  groups: string[]
  enabled: boolean
  uid?: number
}

export interface UserGroup {
  id: string
  name: string
  members: string[]
}

export interface FileEntry {
  id: string
  name: string
  type: 'dir' | 'file'
  sizeBytes: number
  modifiedAt: string
}

export interface FileRequestLink {
  id: string
  shareId: string
  path: string
  createdAt: string
  expiresAt: string
  maxFiles: number
  maxBytes: number
  receivedFiles: number
  receivedBytes: number
  revokedAt?: string
}

export interface FileShareLink {
  id: string
  shareId: string
  path: string
  createdAt: string
  expiresAt: string
  downloads: number
  revokedAt?: string
}

export interface FileContentResult {
  shareId: string
  path: string
  name: string
  snippet: string
  sizeBytes: number
  modifiedAt: string
}

export interface FileContentIndexStatus {
  shareId: string
  documents: number
  indexedAt: string | null
  maxFileBytes: number
  maxDocuments: number
  maxIndexBytes: number
}

export interface FileContentIndexBuildResult extends FileContentIndexStatus {
  skipped: number
}

export interface FileIntegrityReport {
  baselineAt: string
  verifiedAt: string
  unchanged: number
  changedCount: number
  missingCount: number
  addedCount: number
  changed: string[]
  missing: string[]
  added: string[]
}

export interface FileIntegrityStatus {
  shareId: string
  fileCount: number
  baselineAt?: string
  report?: FileIntegrityReport
  running: boolean
}

export interface RecycleEntry {
  id: string
  shareId: string
  name: string
  originalPath: string
  deletedAt: string
  sizeBytes: number
}

export interface RecoveryLayer {
  id: string
  label: string
  status: 'current' | 'stale' | 'missing'
  detail: string
  lastSuccessfulAt?: string | null
}

export interface RecoveryCoverage {
  id: string
  name: string
  kind: 'share' | 'docker-appdata'
  status: 'current' | 'stale' | 'missing'
  detail: string
  lastSuccessfulAt?: string | null
}

export interface RecoveryReadiness {
  score: number
  layers: RecoveryLayer[]
  coverage?: RecoveryCoverage[]
  warnings?: string[]
}

export interface HealthComponent {
  id: string
  label: string
  status: HealthState
  message: string
  recommended: string
}

export interface HealthBreakdown {
  status: HealthState
  score: number
  components: HealthComponent[]
}

export interface RestoreDrill {
  id: string
  trigger: 'manual' | 'scheduled'
  state: 'running' | 'successful' | 'failed'
  bundlePath?: string
  generation: number
  startedAt: string
  finishedAt?: string
  verified: boolean
  databaseValid: boolean
  composeValid: boolean
  secretsRestored: boolean
  databaseRestored: boolean
  appdataRestored?: string[]
  sharesRestored?: string[]
  servicesRehearsed?: string[]
  servicesHealthy: boolean
  appliedFiles: number
  warnings?: string[]
  error?: string
}

export interface RestoreDrillSchedule {
  enabled: boolean
  intervalSeconds: number
  rpoHours: number
  rtoMinutes: number
  lastStartedAt?: string
  nextDueAt?: string
  updatedAt: string
}

export interface WorkloadRecoveryObjective {
  workloadId: string
  rpoHours: number
  rtoMinutes: number
  updatedAt: string
}

export interface TroubleshootingLink { label: string; path: string }
export interface TroubleshootingIssue {
  id: string
  severity: HealthState
  title: string
  summary: string
  cause?: string
  steps: string[]
  resource?: { type: string; id: string }
  links?: TroubleshootingLink[]
}
export interface TroubleshootingReport { status: 'clear' | 'action_required'; issues: TroubleshootingIssue[] }

export interface GraphNode { id: string; type: string; label: string; status: HealthState }
export interface GraphEdge { from: string; to: string; relationship: string }
export interface DependencyGraph { generatedAt: string; nodes: GraphNode[]; edges: GraphEdge[] }

export interface BackupJob {
  id: string
  name: string
  source: string
  destinationId: string
  schedule: string
  strategy: string
  jobType: string
  lastRun?: { status: HealthState; at: string; detail?: string }
  enabled: boolean
}

export interface BackupDestination {
  id: string
  destinationId?: string
  enabled: boolean
  type: 'usb' | 's3' | 'sftp' | 'nas' | 'cloud'
  label: string
  target?: string
  encrypted: boolean
  status: HealthState
  immutableDays?: number
  providerObjectLock?: boolean
  lastVerifiedAt?: string
  detail?: string
}

export interface BackupSchedule {
  id: string
  enabled: boolean
  intervalSeconds: number
  onUsbAttach: boolean
  lastStartedAt?: string
  nextDueAt?: string
  updatedAt: string
}

export interface BackupPolicyTemplate {
  id: string
  name: string
  description: string
  intervalSeconds: number
  generations: number
  daily: number
  monthly: number
}

export interface BackupRestoreCheck {
  state: 'verified'
  destinationId: string
  runId: string
  checksum: string
  bytes: number
  databaseRestored: boolean
  appdataRestored: string[]
  appliedFiles: number
  serviceStartup: 'not_attempted'
}

export interface ConfigGeneration {
  id: number
  createdAt: string
  actor: string
  status: 'committed' | 'failed'
  summary: string
  config: string
}

export interface RestorePlan {
  generationId: number
  interfaces: { old: string; detail: string; options: string[] }[]
  apps: { name: string; appdataAvailable: boolean }[]
  dataDisksNote: string
}

export interface SettingsUpdatesCore {
  channel: 'stable' | 'beta'
  current: string
  available: string | null
  lastCheckedAt: string
  autoUpdate: boolean
  channelUrl?: string
  releaseNotes?: string
  publishedAt?: string
  lastError?: string
}

export interface AppSettings {
  updates: {
    core: SettingsUpdatesCore
    debian: { release: string; pendingCount: number; securityCount?: number; lastCheckedAt: string; autoUpdate: boolean }
    docker: { availableCount: number; autoUpdate: boolean }
  }
  runtime: {
    writeProfile: 'balanced' | 'normal' | 'maximum'
    zram: {
      enabled: boolean
      sizeBytes: number
      compressedBytes: number
      ratio: number
      pressure: 'low' | 'medium' | 'high'
    }
    tmpfs: {
      enabled: boolean
      sizeBytes: number
      mountPath: string
    }
    dockerLogging: {
      driver: string
      maxSizeMb: number
      maxFiles: number
      topConsumers: { name: string; sizeBytes: number }[]
    }
  }
  power: {
    maintenanceMode: boolean
    wol: { interface: string; mac: string; supported: boolean; enabled: boolean }[]
    schedule: { enabled: boolean; action: 'shutdown' | 'reboot'; time: string; days: string }
  }
  security: {
    https: { enabled: boolean; ca: string; acme: boolean }
    ssh: { rootLogin: boolean; passwordAuth: boolean; keyCount: number }
    sessions: {
      id: string
      device: string
      ip: string
      scope: string
      lastActiveAt: string
      expiresAt?: string
      current: boolean
    }[]
  }
}

export interface UPSStatus {
  name: string
  status: string
  manufacturer?: string
  model?: string
  serial?: string
  chargePercent?: number
  loadPercent?: number
  runtimeSec?: number
  onBattery: boolean
}

export interface UPSConfig {
  names: string[]
}

export interface StorageMount {
  kind: 'disk' | 'pool'
  targetId: string
  mountPath: string
  fstype: string
  source: string
  options: string
  enabled: boolean
}

export interface StorageSnapshot {
  id: string
  kind: 'btrfs' | 'zfs'
  source: string
  name: string
  label?: string
  createdAt: string
  protectedUntil?: string
}

export interface SnapshotEntry {
  name: string
  sizeBytes: number
  directory: boolean
  modifiedAt: string
}

export interface SnapshotChange {
  path: string
  kind: 'added' | 'deleted' | 'modified'
  sizeBytes: number
  modifiedAt: string
}

export interface SnapshotDiff {
  snapshotFiles: number
  currentFiles: number
  added: number
  deleted: number
  modified: number
  unchanged: number
  reviewRecommended: boolean
  changes: SnapshotChange[]
}

export interface StorageSnapshotFiles {
  path: string
  entries: SnapshotEntry[]
  total: number
}

export interface LanHost {
  mac: string
  interface: string
  ip: string
  hostname?: string
  firstSeen: string
  lastSeen: string
}

export interface InstallTarget {
  diskId: string
  name: string
  model: string
  sizeBytes: number
  role: string
  filesystem?: string
  mounted: boolean
  eligible: boolean
  protectedBy?: string[]
  identity: Record<string, string>
}

export interface InstallPlan {
  id: string
  targetDiskId: string
  expectedIdentity: Record<string, string>
  hostname: string
  adminUsername: string
  filesystem: 'ext4' | 'xfs'
  uefi: boolean
  createdAt: string
  expiresAt: string
}

export interface InstallPlanResponse {
  plan: InstallPlan
  hash: string
  targets: InstallTarget[]
}

export interface InstallStatus {
  stage: 'idle' | 'planned' | 'applying' | 'succeeded' | 'failed'
  detail?: string
  planId?: string
  hostname?: string
}

export interface StorageSafety {
  state: 'locked' | 'unlocked'
  unlockedUntil: string | null
}

export interface APITokenSummary {
  id: string
  name: string
  scopes: ('read' | 'backup:write' | 'replication:receive' | 'fleet:status' | `workstation:backup:${string}` | `replication:snapshot:receive:${string}`)[]
  createdAt: string
  expiresAt?: string
  lastUsedAt?: string
}

export interface ReplicationPeer {
  id: string
  name: string
  url: string
  createdAt: string
  lastSyncAt?: string
  lastError?: string
  remoteStatus?: 'online' | 'offline'
  remoteVersion?: string
  remoteHealth?: string
  remoteCheckedAt?: string
}

export interface SnapshotReplicationTask {
  id: string
  peerId: string
  name: string
  sourceShareId: string
  destinationShareId: string
  scheduleKind: 'manual' | 'daily' | 'weekly'
  timeOfDay: string
  weekday?: string
  lastSnapshotName?: string
  lastAttemptAt?: string
  lastSyncAt?: string
  lastError?: string
  running?: boolean
  createdAt: string
}

export interface SnapshotReplicationRun {
  id: string
  taskId: string
  jobId: string
  state: 'running' | 'successful' | 'failed' | 'cancelled'
  stage: string
  progress: number
  bytes: number
  snapshotName?: string
  error?: string
  createdAt: string
  startedAt?: string
  finishedAt?: string
}

export interface APITokenCreated {
  token: string
  summary: APITokenSummary
}

export interface ProtectionConfig {
  configPath: string
  configured: boolean
  parityDiskIds: string[]
  dataDiskIds: string[] | null
}

export interface PoolMemberPlan {
  diskId: string
  wwn?: string
  serial?: string
  model?: string
  sizeBytes: number
  filesystemUuid?: string
  branchPath: string
}

export interface PoolPlan {
  operationId: string
  name: string
  mountPath: string
  policy: string
  members: PoolMemberPlan[]
  configGeneration: number
  expiresAt: string
  planHash: string
  status: string
}

export interface PoolUnmountPlan {
  operationId: string
  poolId: string
  name: string
  mountPath: string
  configGeneration: number
  expiresAt: string
  planHash: string
  status: string
}

export interface StorageOperationTarget {
  diskId: string
  wwn?: string
  serial?: string
  model?: string
  sizeBytes: number
  filesystemUuid?: string
}

export interface StorageOperationPlan {
  operationId: string
  action: string
  target: StorageOperationTarget
  requestedState: Record<string, unknown>
  dependencySnapshot: string[]
  configGeneration: number
  expiresAt: string
  planHash: string
  status: string
}

export interface DiskReplacementPlan {
  operationId: string
  retiredDiskId: string
  retiredDataName: string
  replacementDiskId: string
  parityDiskId?: string
  configGeneration: number
  expiresAt: string
  planHash: string
  status: string
}

export interface DiskReplacementResult {
  ok: boolean
  jobId: string
  slot: string
  next: string
}

export interface RecoveryManifest {
  formatVersion: number
  configSchema: number
  lumonasVersion: string
  nasUuid: string
  generation: number
  createdAt: string
  diskIds: string[]
  checksums: Record<string, string>
}

export interface RecoveryStatus {
  configured: boolean
  latestPath: string
  verified: boolean
  manifest?: RecoveryManifest
  warnings?: string[]
}

export interface RecoveryPlan {
  manifest: RecoveryManifest
  files: string[]
  verified: boolean
  databaseValid: boolean
  desiredStateValid: boolean
  composeValid: boolean
  encryptedSecrets: boolean
  appdata?: { stack: string; containerPath: string; hostPath: string; archivePath: string; archiveBytes: number }[]
  databaseDumps?: { stack: string; container: string; archivePath: string; archiveBytes: number }[]
  warnings?: string[]
}

export interface RecoveryExportResponse {
  path: string
  manifest: RecoveryManifest
  verified: boolean
  appdataArchives: number
  databaseDumps: number
  warnings: string[]
}

export interface NotificationDelivery {
  id: string
  channelId: string
  eventType: string
  state: string
  attemptedAt: string
  error?: string
}

export interface AuditEntry {
  id: string
  timestamp: string
  actor: string
  action: string
  outcome: string
  correlationId?: string
  operationId?: string
  planHash?: string
  generation?: number
  resourceType?: string
  resourceId?: string
  metadata?: Record<string, unknown>
}

export interface AuditPage {
  entries: AuditEntry[]
  nextCursor?: string
  hasMore: boolean
}

export interface CapacityForecast {
  resourceId: string
  totalBytes: number
  usedBytes: number
  sampleCount: number
  windowDays: number
  growthBytesPerDay: number
  daysToNinetyPercent?: number | null
  daysToFull?: number | null
  estimatedFullAt?: string
  sampleAgeHours: number
  stale: boolean
  confidence: 'low' | 'medium' | 'high' | ''
  available: boolean
  message?: string
}

export interface CapacityThreshold {
  resourceId: string
  thresholdPercent: number
  updatedAt: string
}

export interface SupportSession {
  id: string
  device: string
  ip: string
  scope: string
  lastActiveAt: string
  expiresAt?: string
  current: boolean
}

export interface UPSPolicy {
  enabled: boolean
  minimumRuntimeSec: number
  minimumCharge: number
}

export type OnboardingClassification =
  | 'system'
  | 'blank'
  | 'existing'
  | 'lumonas'
  | 'suspected-parity'
  | 'removable'

export interface OnboardingDisk {
  id: string
  model: string
  serialSuffix: string
  sizeBytes: number
  classification: OnboardingClassification
  filesystem?: string
  dataFound: boolean
  recommendedRole: DiskRole
  recommendedLabel: string
  offline?: boolean
}

export interface OnboardingState {
  completed: boolean
  server: {
    name: string
    hostname: string
    timezone: string
    ip: string
    sshEnabled: boolean
  }
  hardware: { cpu: string; ramBytes: number; diskCount: number }
  disks: OnboardingDisk[]
}

export interface OnboardingCompleteInput {
  serverName: string
  roles: Record<string, DiskRole>
  protection: { syncTime: string; scrubDay: string }
  recovery: { autoConfigBackup: boolean; destination: string; keyAcknowledged: boolean }
}

export interface DockerSummary {
  stacks: number
  appsRunning: number
  updatesAvailable: number
  available: boolean
}

export type FormFieldType = 'port' | 'storage_ref' | 'secret' | 'text' | 'timezone'

export interface CatalogFormField {
  id: string
  label: string
  type: FormFieldType
  required?: boolean
  defaultValue?: string
  description?: string
  containerPath?: string
  defaultResource?: string
}

export interface CatalogApp {
  id: string
  name: string
  category: string
  tagline: string
  description: string
  accent: 'primary' | 'info' | 'success' | 'attention' | 'warning' | 'critical'
  upstream: string
  image: string
  ports: number[]
  form: CatalogFormField[]
  popular?: boolean
  recovery?: RecoveryContract
  appdataPaths?: string[]
  dbDumpContainer?: string
  trustStatus?: 'verified' | 'unverified' | 'invalid'
  trustMessage?: string
}

export type RecoveryStrategy = 'stop-backup' | 'snapshot' | 'custom' | 'none'

export interface RecoveryHook {
  container?: string
  command: string
  timeout?: number
}

export interface RecoveryContract {
  strategy: RecoveryStrategy
  appdataPaths?: string[]
  preBackupHook?: RecoveryHook
  postBackupHook?: RecoveryHook
  restoreHook?: RecoveryHook
  dbDump?: RecoveryHook
  dbRestore?: RecoveryHook
  stopServices?: string[]
  startServices?: string[]
}

export type RiskFlag =
  | 'privileged'
  | 'docker_socket'
  | 'host_root_bind'
  | 'host_pid'
  | 'host_network'
  | 'devices'

export interface StackEnvVar {
  name: string
  value: string
  scope: 'builtin' | 'global' | 'stack' | 'secret'
}

export interface StackStorageMapping {
  containerPath: string
  resourceId: string
  resourceLabel: string
}

export interface DockerStack {
  id: string
  name: string
  catalogId?: string
  category: string
  status: HealthState
  state: 'running' | 'stopped' | 'deploying' | 'unhealthy'
  images: string[]
  composeYaml: string
  env: StackEnvVar[]
  storage: StackStorageMapping[]
  ports: { host: number; container: number; label?: string }[]
  risks: RiskFlag[]
  cpuPercent: number
  ramUsedBytes: number
  restarts: number
  lastDeploy: string
  updateAvailable?: { current: string; latest: string }
  backup: {
    strategy: 'stop-backup' | 'crash-consistent'
    lastBackupAt?: string
    appdataSizeBytes: number
  }
  recovery?: RecoveryContract
  recoveryCoverage: number
}

export interface DockerDeployment {
  id: string
  stackName: string
  kind: 'install' | 'update' | 'rollback'
  state: 'pending' | 'committed' | 'rolled_back' | 'failed'
  composeBefore?: string
  composeAfter: string
  error?: string
  createdAt: string
  updatedAt: string
}

export type ContainerState = 'running' | 'exited' | 'restarting' | 'unhealthy' | 'created'

export interface DockerContainer {
  id: string
  name: string
  stackId?: string
  image: string
  state: ContainerState
  cpuPercent: number
  ramUsedBytes: number
  restarts: number
  ports: { host: number; container: number }[]
  startedAt?: string
}

export interface VirtualizationStatus {
  libvirtAvailable: boolean
  kvmAvailable: boolean
  architecture: string
  cpuCount: number
  memoryBytes: number
  reason?: string
}

export interface VirtualMachine {
  name: string
  uuid?: string
  state: string
  vcpus?: number
  memoryKiB?: number
  maximumMemoryKiB?: number
  diskPath?: string
  iso?: string
}

export interface VirtualizationMedia {
  name: string
  sizeBytes: number
}

export interface CreateVirtualMachineInput {
  name: string
  iso: string
  vcpus: number
  memoryMiB: number
  diskGiB: number
}

export interface VirtualMachineSnapshot {
  name: string
  state?: string
  creationTime?: string
}

export interface VirtualMachineConsole {
  output: string
  cursor: number
  connected: boolean
}

export interface VirtualMachineDeleteResult {
  name: string
  diskPath: string
  diskRemoved: boolean
  definitionRetained: boolean
}

export interface RecoverableVirtualMachine {
  name: string
  diskPath: string
  diskBytes: number
}

export interface DockerImage {
  id: string
  repo: string
  tag: string
  localDigest?: string
  remoteDigest?: string
  sizeBytes: number
  createdDaysAgo: number
  updateAvailable: boolean
  inUse: boolean
}

export interface DockerImagePackSummary {
  name: string
  version?: string
  description?: string
  imageCount: number
  totalSizeBytes: number
}

export interface DockerImagePackImportResult {
  pack: string
  imported: string[]
  failed?: { file: string; reason: string }[]
}

export interface DockerVolume {
  id: string
  name: string
  stackId?: string
  stackName?: string
  usedBytes: number
  bindPath?: string
}

export interface LogLine {
  container: string
  ts: string
  level: 'info' | 'warn' | 'error'
  message: string
}

export interface LumoEvent<T = Record<string, unknown>> {
  schemaVersion: number
  id: string
  type: string
  timestamp: string
  severity: 'info' | 'warning' | 'critical'
  correlationId?: string
  operationId?: string
  planHash?: string
  actor?: string
  generation?: number
  resource?: { type: string; id: string }
  data: T
}

export type IPMethod = 'auto' | 'manual' | 'disabled'

export interface IPConfig {
  method: IPMethod
  addresses?: string[]
  gateway?: string
  dns?: string[]
  metric?: number
}

export type ConnectionType = 'ethernet' | 'wifi' | 'vlan' | 'bond' | 'bridge'

export interface NetworkConnection {
  id: string
  uuid?: string
  name: string
  interface: string
  enabled: boolean
  generation?: number
  status: string
  type?: ConnectionType
  parent?: string
  members?: string[]
  vlanId?: number
  ssid?: string
  wifiOpen?: boolean
  ipv4: IPConfig
  ipv6: IPConfig
  mtu?: number
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

export interface WiFiNetwork {
  ssid: string
  signal: number
  channel: number
  band?: string
  security: string
  secure: boolean
}

export interface WiFiScanResult {
  available: boolean
  networks: WiFiNetwork[]
}

export interface WireGuardPeer {
  publicKey: string
  presharedKey?: string
  endpoint?: string
  allowedIps: string[]
  persistentKeepalive: number
}

export interface WireGuardConfig {
  interface: string
  privateKey?: string
  address: string[]
  listenPort: number
  dns?: string[]
  peers: WireGuardPeer[]
  postUp?: string
  postDown?: string
}

export interface WireGuardStatus {
  interface: string
  ip: string
  listenPort: number
  peers: number
  connected: boolean
}

export interface TailscaleStatus {
  installed?: boolean
  running: boolean
  connected: boolean
  health?: string
  backendState: string
  version?: string
  tailscaleIp4?: string
  tailscaleIp6?: string
  hostName: string
  magicDnsSuffix?: string
  exitNode?: string
  exitNodeAllow: boolean
  subnetRoutes: string[]
}

export interface TailscalePeer {
  hostName: string
  tailscaleIp: string
  publicKey: string
  os?: string
  online: boolean
  exitNode: boolean
}

export interface SSHKey {
  id: string
  publicKey: string
  comment?: string
}
