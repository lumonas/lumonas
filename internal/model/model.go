package model

import "time"

type HealthState string

const (
	Healthy   HealthState = "healthy"
	Attention HealthState = "attention"
	Warning   HealthState = "warning"
	Critical  HealthState = "critical"
	Offline   HealthState = "offline"
)

type SmartSummary struct {
	Overall              HealthState `json:"overall"`
	ReallocatedSectors   int64       `json:"reallocatedSectors"`
	PendingSectors       int64       `json:"pendingSectors"`
	UncorrectableSectors int64       `json:"uncorrectableSectors"`
	CRCErrors            int64       `json:"crcErrors"`
	PowerOnHours         int64       `json:"powerOnHours"`
	WearPercent          *float64    `json:"wearPercent,omitempty"`
	LastTest             *LastTest   `json:"lastTest,omitempty"`
}

type LastTest struct {
	Type   string    `json:"type"`
	Result string    `json:"result"`
	At     time.Time `json:"at"`
}

type Disk struct {
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	CurrentPath    string       `json:"currentPath,omitempty"`
	Model          string       `json:"model"`
	Serial         string       `json:"serial"`
	WWN            string       `json:"wwn,omitempty"`
	GPTDiskGUID    string       `json:"gptDiskGuid,omitempty"`
	PartitionUUID  string       `json:"partitionUuid,omitempty"`
	FilesystemUUID string       `json:"filesystemUuid,omitempty"`
	SizeBytes      uint64       `json:"sizeBytes"`
	UsedBytes      *uint64      `json:"usedBytes,omitempty"`
	Role           string       `json:"role"`
	Rotational     bool         `json:"rotational"`
	Interface      string       `json:"interface"`
	Health         HealthState  `json:"health"`
	Temperature    *float64     `json:"temperatureC"`
	Filesystem     string       `json:"filesystem,omitempty"`
	Mounted        bool         `json:"mounted"`
	PoolID         string       `json:"poolId,omitempty"`
	Standby        bool         `json:"standby,omitempty"`
	LastSeen       time.Time    `json:"lastSeen"`
	SMART          SmartSummary `json:"smart"`
}

type PoolMember struct {
	DiskID     string `json:"diskId"`
	Enabled    bool   `json:"enabled"`
	BranchPath string `json:"branchPath,omitempty"`
}

type Pool struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Type      string       `json:"type"`
	MountPath string       `json:"mountPath"`
	Status    HealthState  `json:"status"`
	SizeBytes uint64       `json:"sizeBytes"`
	UsedBytes uint64       `json:"usedBytes"`
	Members   []PoolMember `json:"members"`
}

type CapacitySnapshot struct {
	ResourceID string    `json:"resourceId"`
	CapturedAt time.Time `json:"capturedAt"`
	TotalBytes uint64    `json:"totalBytes"`
	UsedBytes  uint64    `json:"usedBytes"`
}

type Protection struct {
	Status           HealthState `json:"status"`
	ParityDisks      []DiskRef   `json:"parityDisks"`
	ProtectedDiskIDs []string    `json:"protectedDiskIds"`
	LastSyncAt       *time.Time  `json:"lastSyncAt"`
	LastSyncResult   *string     `json:"lastSyncResult"`
	LastScrubAt      *time.Time  `json:"lastScrubAt"`
	ChangesSinceSync uint64      `json:"changesSinceSyncBytes"`
	SyncSchedule     string      `json:"syncSchedule"`
	ScrubSchedule    string      `json:"scrubSchedule"`
	SyncRunning      bool        `json:"syncRunning"`
}

type DiskRef struct {
	DiskID    string `json:"diskId"`
	SizeBytes uint64 `json:"sizeBytes"`
	UsedBytes uint64 `json:"usedBytes"`
}

type Job struct {
	ID            string     `json:"id"`
	CorrelationID string     `json:"correlationId,omitempty"`
	Type          string     `json:"type"`
	Title         string     `json:"title"`
	ResourceID    string     `json:"resourceId,omitempty"`
	State         string     `json:"state"`
	Progress      *float64   `json:"progress"`
	Stage         string     `json:"stage,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
	Error         string     `json:"error,omitempty"`
}

type Event struct {
	SchemaVersion int            `json:"schemaVersion"`
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Timestamp     time.Time      `json:"timestamp"`
	Severity      string         `json:"severity"`
	CorrelationID string         `json:"correlationId,omitempty"`
	OperationID   string         `json:"operationId,omitempty"`
	PlanHash      string         `json:"planHash,omitempty"`
	Actor         string         `json:"actor,omitempty"`
	Generation    int64          `json:"generation,omitempty"`
	Resource      *ResourceRef   `json:"resource,omitempty"`
	Data          map[string]any `json:"data"`
}

type ResourceRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type ServerInfo struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Hostname string      `json:"hostname"`
	Version  string      `json:"version"`
	NASUUID  string      `json:"nasUuid"`
	Timezone string      `json:"timezone"`
	Health   HealthState `json:"health"`
	IP       string      `json:"ip"`
}

type SystemMetrics struct {
	CPUPercent    float64               `json:"cpuPercent"`
	Load          [3]float64            `json:"load"`
	RAMUsedBytes  uint64                `json:"ramUsedBytes"`
	RAMTotalBytes uint64                `json:"ramTotalBytes"`
	CPUTempC      float64               `json:"cpuTempC"`
	UptimeSeconds uint64                `json:"uptimeSeconds"`
	Net           NetMetrics            `json:"net"`
	NetInterfaces []NetInterfaceMetrics `json:"netInterfaces,omitempty"`
}

type NetMetrics struct {
	Interface string  `json:"interface"`
	UpMbps    float64 `json:"upMbps"`
	DownMbps  float64 `json:"downMbps"`
}

type NetInterfaceMetrics struct {
	Interface  string  `json:"interface"`
	UpMbps     float64 `json:"upMbps"`
	DownMbps   float64 `json:"downMbps"`
	ErrorsIn   uint64  `json:"errorsIn"`
	ErrorsOut  uint64  `json:"errorsOut"`
	DroppedIn  uint64  `json:"droppedIn"`
	DroppedOut uint64  `json:"droppedOut"`
	Up         bool    `json:"up"`
}

type DockerSummary struct {
	Stacks           int `json:"stacks"`
	AppsRunning      int `json:"appsRunning"`
	UpdatesAvailable int `json:"updatesAvailable"`
}

type ActivityEvent struct {
	ID          string       `json:"id"`
	Timestamp   time.Time    `json:"timestamp"`
	Category    string       `json:"category"`
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	Resource    *ResourceRef `json:"resource,omitempty"`
}

type Alert struct {
	ID          string       `json:"id"`
	Severity    string       `json:"severity"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Resource    *ResourceRef `json:"resource,omitempty"`
	State       string       `json:"state"`
	StartedAt   time.Time    `json:"startedAt"`
}

type HealthComponent struct {
	ID          string      `json:"id"`
	Label       string      `json:"label"`
	Status      HealthState `json:"status"`
	Message     string      `json:"message,omitempty"`
	Recommended string      `json:"recommended,omitempty"`
}

type HealthBreakdown struct {
	Status     HealthState       `json:"status"`
	Score      int               `json:"score"`
	Components []HealthComponent `json:"components"`
}
