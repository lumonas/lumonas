package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lumonas/lumonas/internal/auth"
	"github.com/lumonas/lumonas/internal/collector"
	dockerruntime "github.com/lumonas/lumonas/internal/docker"
	"github.com/lumonas/lumonas/internal/events"
	"github.com/lumonas/lumonas/internal/httpobs"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/monitoring"
	"github.com/lumonas/lumonas/internal/network"
	"github.com/lumonas/lumonas/internal/notify"
	"github.com/lumonas/lumonas/internal/power"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/services"
	"github.com/lumonas/lumonas/internal/shares"
	"github.com/lumonas/lumonas/internal/storage"
	"github.com/lumonas/lumonas/internal/store"
	"github.com/lumonas/lumonas/internal/trace"
	"github.com/lumonas/lumonas/internal/updates"
)

type apiServer struct {
	store                       *store.Store
	hub                         *events.Hub
	log                         *slog.Logger
	jobsMu                      sync.Mutex
	version                     string
	diskFunc                    func() ([]model.Disk, error)
	authRequired                bool
	dynamicAuth                 bool
	dockerService               dockerruntime.Service
	catalogFile                 string
	notificationMu              sync.Mutex
	notificationFailures        map[string]notificationFailureState
	notificationClient          *http.Client
	updateHTTPClient            *http.Client
	totpMu                      sync.Mutex
	totpChallenges              map[string]totpChallenge
	safetyMu                    sync.Mutex
	safetyUntil                 time.Time
	brokerExec                  func(ctx context.Context, request privileged.Request) error
	recordNetworkCheckpointFn   func(operationID, connectionID, state string) error
	completeNetworkCheckpointFn func(operationID, state string) (string, error)
	persistExecutedPlanFn       func(storage.Plan) error
	corsOrigins                 []string
	csrfTokens                  map[string]csrfBinding
	csrfMu                      sync.Mutex
	fixStages                   map[string]string
	fixStageMu                  sync.Mutex
	runtimeStateFunc            func() map[string]any
	rateMu                      sync.Mutex
	rateAttempts                map[string][]time.Time
	clock                       func() time.Time
}

var version = "0.1.0-dev"

// authEnabled reports whether API requests must be authenticated. When the
// daemon runs in dynamic mode (the production default), an explicit
// LUMONAS_AUTH_REQUIRED wins; otherwise the API stays open only until the
// first management user exists, so a freshly installed appliance is
// reachable for onboarding but never left wide open on the LAN.
// fixStage carries the SnapRAID data slot a queued fix job should recover.
// The map lives in memory: a daemon restart fails every unfinished job, so
// the stage cannot outlive the process that queued it.
func (s *apiServer) setFixStage(jobID, dataName string) {
	s.fixStageMu.Lock()
	defer s.fixStageMu.Unlock()
	if s.fixStages == nil {
		s.fixStages = make(map[string]string)
	}
	s.fixStages[jobID] = dataName
}

func (s *apiServer) takeFixStage(jobID string) string {
	s.fixStageMu.Lock()
	defer s.fixStageMu.Unlock()
	name := s.fixStages[jobID]
	delete(s.fixStages, jobID)
	return name
}

func (s *apiServer) authEnabled() bool {
	if s.authRequired || !s.dynamicAuth {
		return s.authRequired
	}
	if value := os.Getenv("LUMONAS_AUTH_REQUIRED"); value != "" {
		return value == "true"
	}
	return s.store.HasUsers()
}

// csrfBinding ties a CSRF token to the session digest it was issued for and
// an expiry; tokens only verify for the matching session.
type csrfBinding struct {
	sessionDigest string
	expires       int64
}

// clientIP returns the best-effort client address for rate limiting. The
// X-Forwarded-For header is only trusted for connections from loopback —
// i.e. requests proxied by lumonas-web on the same host, which appends the
// real client address. For direct connections the socket address wins and
// client-supplied forwarding headers are ignored so the limiter cannot be
// bypassed by rotating spoofed XFF values.
func clientIP(r *http.Request) string {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		host = h
	}
	if remote := net.ParseIP(host); remote != nil && remote.IsLoopback() {
		if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
			parts := strings.Split(forwarded, ",")
			// The proxy appends the real client address, so the last entry
			// is the only one the client cannot forge ahead of.
			if candidate := strings.TrimSpace(parts[len(parts)-1]); candidate != "" {
				return candidate
			}
		}
	}
	return host
}

func main() {
	listen := flag.String("listen", envOr("LUMONASD_LISTEN", "127.0.0.1:8080"), "HTTP listen address")
	dbPath := flag.String("db", envOr("LUMONAS_DB_PATH", "/var/lib/lumonas/lumonas.db"), "SQLite database path")
	versionFlag := flag.String("version", envOr("LUMONAS_VERSION", version), "service version")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	db, err := store.Open(*dbPath)
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	nasUUID := ensureNASUUID(db)
	if password := os.Getenv("LUMONAS_ADMIN_PASSWORD"); password != "" {
		if err := db.EnsureAdmin("admin", password); err != nil {
			logger.Error("admin bootstrap failed", "error", err)
			os.Exit(1)
		}
	}
	server := &apiServer{store: db, hub: events.NewHub(), log: logger, version: *versionFlag, diskFunc: func() ([]model.Disk, error) { return collector.Disks(nil) }, authRequired: os.Getenv("LUMONAS_AUTH_REQUIRED") == "true", dynamicAuth: true, corsOrigins: parseCORSOrigins(), csrfTokens: make(map[string]csrfBinding), rateAttempts: make(map[string][]time.Time)}
	server.dockerService = dockerruntime.New(envOr("LUMONAS_STACK_ROOT", "/srv/lumonas/docker/stacks"), nil)
	server.catalogFile = envOr("LUMONAS_CATALOG_FILE", "/usr/share/lumonas/catalog/apps.json")
	server.reconcileUpdateBoot()
	server.ensureRestartedJobs()
	go server.persistMountState("startup")
	go server.metricsLoop()
	go server.capacityLoop()
	go server.backupLoop()
	go server.upsMonitorLoop()
	go server.scheduleLoop()
	go server.retentionLoop()

	httpServer := &http.Server{Addr: *listen, Handler: server.routes(), ReadHeaderTimeout: 5 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		logger.Info("lumonasd started", "listen", *listen, "db", *dbPath, "nas_uuid", nasUUID)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server stopped", "error", err)
			os.Exit(1)
		}
	}()
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}

func (s *apiServer) reconcileUpdateBoot() {
	state, rolledBack, err := s.updateManager().RecordBoot(s.version, updates.DefaultMaxBootAttempts)
	if err != nil {
		s.log.Warn("update boot state could not be recorded", "error", err)
		return
	}
	if state.PendingSlot == "" && rolledBack {
		s.publish("update.health_failed", "critical", nil, map[string]any{"reason": state.LastError})
		s.log.Error("pending update failed boot health checks", "reason", state.LastError)
		return
	}
	if state.PendingSlot != "" && state.BootAttempts > 0 {
		s.log.Info("pending update boot recorded", "slot", state.PendingSlot, "attempt", state.BootAttempts)
	}
}

func (s *apiServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/readyz", s.readyz)
	mux.HandleFunc("/api/v1/", s.api)
	return s.requestMiddleware(s.authMiddleware(mux))
}

func (s *apiServer) healthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (s *apiServer) readyz(w http.ResponseWriter, _ *http.Request) {
	if _, ok := s.store.Meta("nas_uuid"); !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *apiServer) api(w http.ResponseWriter, r *http.Request) {
	endpoint := strings.TrimPrefix(r.URL.Path, "/api/v1")
	switch {
	case r.Method == http.MethodGet && endpoint == "/auth/status":
		s.authStatus(w, r)
	case r.Method == http.MethodPost && endpoint == "/auth/login":
		s.authLogin(w, r)
	case r.Method == http.MethodPost && endpoint == "/auth/login/2fa":
		s.loginTwoFactor(w, r)
	case r.Method == http.MethodPost && endpoint == "/auth/logout":
		s.authLogout(w, r)
	case r.Method == http.MethodGet && endpoint == "/auth/csrf":
		s.csrfForSession(w, r)
	case r.Method == http.MethodGet && endpoint == "/users":
		s.listUsers(w, r)
	case r.Method == http.MethodGet && endpoint == "/principals":
		s.listPrincipals(w, r)
	case r.Method == http.MethodPost && endpoint == "/users":
		s.createUser(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/users/") && strings.HasSuffix(endpoint, "/2fa/setup"):
		s.setupTwoFactor(w, r, twoFactorUserID(endpoint))
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/users/") && strings.HasSuffix(endpoint, "/2fa/enable"):
		s.enableTwoFactor(w, r, twoFactorUserID(endpoint))
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/users/") && strings.HasSuffix(endpoint, "/2fa/disable"):
		s.disableTwoFactor(w, r, twoFactorUserID(endpoint))
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/users/") && strings.HasSuffix(endpoint, "/password"):
		s.setUserPassword(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/users/"):
		s.updateUser(w, r, path.Base(endpoint))
	case r.Method == http.MethodDelete && strings.HasPrefix(endpoint, "/users/"):
		s.deletePrincipal(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/groups":
		s.listGroups(w, r)
	case r.Method == http.MethodPost && endpoint == "/groups":
		s.createGroup(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(endpoint, "/groups/") && strings.HasSuffix(endpoint, "/members"):
		s.setGroupMembers(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/groups/"):
		s.updateGroup(w, r, path.Base(endpoint))
	case r.Method == http.MethodDelete && strings.HasPrefix(endpoint, "/groups/"):
		s.deletePrincipal(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/server":
		s.serverInfo(w)
	case r.Method == http.MethodGet && endpoint == "/onboarding/state":
		s.onboardingState(w, r)
	case r.Method == http.MethodPost && endpoint == "/onboarding/complete":
		s.completeOnboarding(w, r)
	case r.Method == http.MethodGet && endpoint == "/disks":
		s.disks(w)
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/disks/"):
		s.disk(w, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/pools":
		s.pools(w, r)
	case r.Method == http.MethodPost && endpoint == "/storage/pools/plan":
		s.planStoragePool(w, r)
	case r.Method == http.MethodPost && endpoint == "/storage/pools/setup/plan":
		s.planStoragePoolSetup(w, r)
	case r.Method == http.MethodPost && endpoint == "/storage/pools/setup/confirm":
		s.confirmStoragePoolSetup(w, r)
	case r.Method == http.MethodPost && endpoint == "/storage/protection/replacement/plan":
		s.planDiskReplacement(w, r)
	case r.Method == http.MethodPost && endpoint == "/storage/protection/replacement/confirm":
		s.confirmDiskReplacement(w, r)
	case r.Method == http.MethodPost && endpoint == "/storage/pools/unmount/plan":
		s.planStoragePoolUnmount(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/storage/pools/unmount/") && strings.HasSuffix(endpoint, "/confirm"):
		s.confirmStoragePoolUnmount(w, r, poolUnmountOperationID(endpoint))
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/storage/pools/") && strings.HasSuffix(endpoint, "/confirm"):
		s.confirmStoragePool(w, r, poolOperationID(endpoint))
	case r.Method == http.MethodGet && endpoint == "/storage/protection":
		s.protection(w)
	case r.Method == http.MethodGet && endpoint == "/storage/protection/config":
		s.protectionConfig(w, r)
	case r.Method == http.MethodPut && endpoint == "/storage/protection/config":
		s.updateProtectionConfig(w, r)
	case r.Method == http.MethodGet && endpoint == "/storage/mounts":
		s.storageMounts(w)
	case r.Method == http.MethodGet && endpoint == "/storage/safety":
		s.storageSafety(w)
	case r.Method == http.MethodPost && endpoint == "/storage/safety/unlock":
		s.unlockStorageSafety(w, r)
	case r.Method == http.MethodPost && endpoint == "/storage/safety/lock":
		s.lockStorageSafety(w)
	case r.Method == http.MethodPost && endpoint == "/storage/operations/plan":
		s.planStorageOperation(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/storage/disks/") && strings.HasSuffix(endpoint, "/retire"):
		s.retireDisk(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/storage/operations/") && strings.HasSuffix(endpoint, "/confirm"):
		s.confirmStorageOperation(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodGet && endpoint == "/recovery/status":
		s.recoveryStatus(w)
	case r.Method == http.MethodPost && endpoint == "/recovery/key":
		s.createRecoveryKey(w, r)
	case r.Method == http.MethodGet && endpoint == "/recovery/plan":
		s.recoveryPlan(w)
	case r.Method == http.MethodPost && endpoint == "/recovery/restore/stage":
		s.recoveryStage(w, r)
	case r.Method == http.MethodPost && endpoint == "/recovery/export":
		s.recoveryExport(w, r.Context())
	case r.Method == http.MethodGet && endpoint == "/backups/status":
		s.backupStatus(w, r)
	case r.Method == http.MethodGet && endpoint == "/backups/schedule":
		s.backupSchedule(w, r)
	case r.Method == http.MethodPatch && endpoint == "/backups/schedule":
		s.updateBackupSchedule(w, r)
	case r.Method == http.MethodGet && endpoint == "/backups/destinations":
		s.listBackupDestinations(w, r)
	case r.Method == http.MethodPost && endpoint == "/backups/destinations":
		s.saveBackupDestination(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(endpoint, "/backups/destinations/"):
		s.deleteBackupDestination(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/backups/runs":
		s.listBackupRuns(w, r)
	case r.Method == http.MethodPost && endpoint == "/backups/run":
		s.runBackupNow(w, r)
	case r.Method == http.MethodPost && endpoint == "/backups/verify":
		s.verifyBackupNow(w, r)
	case r.Method == http.MethodGet && endpoint == "/backup/readiness":
		s.backupReadiness(w, r)
	case r.Method == http.MethodGet && endpoint == "/backup/jobs":
		s.backupJobs(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/backup/jobs/") && strings.HasSuffix(endpoint, "/run"):
		s.runBackupJob(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodGet && endpoint == "/backup/destinations":
		s.backupDestinationSummaries(w, r)
	case r.Method == http.MethodGet && endpoint == "/backup/generations":
		s.backupGenerations(w, r)
	case r.Method == http.MethodGet && endpoint == "/backup/restore/plan":
		s.backupRestorePlan(w, r)
	case r.Method == http.MethodGet && endpoint == "/updates/status":
		s.updatesStatus(w, r)
	case r.Method == http.MethodPost && endpoint == "/updates/apply":
		s.applyUpdate(w, r)
	case r.Method == http.MethodPost && endpoint == "/updates/rollback":
		s.rollbackUpdate(w, r)
	case r.Method == http.MethodPost && endpoint == "/updates/health":
		s.updateHealth(w, r)
	case r.Method == http.MethodGet && endpoint == "/settings":
		s.settings(w, r)
	case r.Method == http.MethodPatch && endpoint == "/settings":
		s.updateSettings(w, r)
	case r.Method == http.MethodDelete && strings.HasPrefix(endpoint, "/settings/sessions/"):
		s.revokeSession(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/ssh/keys":
		s.listSSHKeys(w)
	case r.Method == http.MethodPost && endpoint == "/ssh/keys":
		s.addSSHKey(w, r)
	case r.Method == http.MethodPost && endpoint == "/ssh/keys/remove":
		s.removeSSHKey(w, r)
	case r.Method == http.MethodPost && endpoint == "/updates/check":
		s.checkUpdates(w, r)
	case r.Method == http.MethodGet && endpoint == "/jobs":
		s.listJobs(w)
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/jobs/"):
		s.job(w, path.Base(endpoint))
	case r.Method == http.MethodPost && endpoint == "/jobs":
		s.createJob(w, r)
	case r.Method == http.MethodPost && endpoint == "/acl/jobs":
		s.createACLJob(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/acl/jobs/") && strings.HasSuffix(endpoint, "/cancel"):
		s.cancelACLJob(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodGet && endpoint == "/alerts":
		s.alerts(w)
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/alerts/"):
		// Accept both /alerts/{id}/ack (web client) and /alerts/{id}.
		s.ackAlert(w, strings.TrimSuffix(strings.TrimPrefix(endpoint, "/alerts/"), "/ack"))
	case r.Method == http.MethodGet && endpoint == "/alert-rules":
		s.alertRules(w)
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/alert-rules/"):
		s.updateAlertRule(w, r, alertRuleID(endpoint))
	case r.Method == http.MethodGet && endpoint == "/notification-channels":
		s.listNotificationChannels(w, r)
	case r.Method == http.MethodPost && endpoint == "/notification-channels":
		s.saveNotificationChannel(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/notification-channels/") && strings.HasSuffix(endpoint, "/test"):
		s.testNotificationChannel(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/notification-channels/"):
		s.updateNotificationChannel(w, r, path.Base(endpoint))
	case r.Method == http.MethodDelete && strings.HasPrefix(endpoint, "/notification-channels/"):
		s.deleteNotificationChannel(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/notification-rules":
		s.alertRules(w)
	case r.Method == http.MethodPost && endpoint == "/notification-rules":
		s.saveNotificationRule(w, r)
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/notification-rules/"):
		s.updateAlertRule(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/notification-deliveries":
		s.listNotificationDeliveries(w, r)
	case r.Method == http.MethodGet && endpoint == "/schedules":
		s.schedules(w, r)
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/schedules/"):
		s.updateSchedule(w, r, scheduleID(endpoint))
	case r.Method == http.MethodGet && endpoint == "/activity":
		s.activity(w)
	case r.Method == http.MethodGet && endpoint == "/audit":
		s.audit(w, r)
	case r.Method == http.MethodPost && endpoint == "/notifications/test":
		s.notificationTest(w, r)
	case r.Method == http.MethodGet && endpoint == "/system/metrics":
		s.metrics(w)
	case r.Method == http.MethodGet && endpoint == "/capacity/forecast":
		s.capacityForecast(w, r)
	case r.Method == http.MethodGet && endpoint == "/diagnostics/support-bundle":
		s.supportBundle(w, r)
	case r.Method == http.MethodGet && endpoint == "/network/interfaces":
		s.networkInterfaces(w)
	case r.Method == http.MethodGet && endpoint == "/network/interfaces/metrics":
		s.networkInterfaceMetrics(w)
	case r.Method == http.MethodGet && endpoint == "/network/wifi/scan":
		s.networkWiFiScan(w, r)
	case r.Method == http.MethodGet && endpoint == "/network/connections":
		s.listNetworkConnections(w, r)
	case r.Method == http.MethodPost && endpoint == "/network/connections":
		s.createNetworkConnection(w, r)
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/network/connections/"):
		s.updateNetworkConnection(w, r, path.Base(endpoint))
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/network/connections/") && strings.HasSuffix(endpoint, "/apply"):
		s.applyNetworkConnection(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodGet && endpoint == "/network/bindings":
		s.listNetworkBindings(w, r)
	case r.Method == http.MethodPatch && endpoint == "/network/bindings":
		s.updateNetworkBindings(w, r)
	case r.Method == http.MethodGet && endpoint == "/network/firewall/policy":
		s.getNetworkFirewall(w, r)
	case r.Method == http.MethodPatch && endpoint == "/network/firewall/policy":
		s.updateNetworkFirewall(w, r)
	case r.Method == http.MethodPost && endpoint == "/network/diagnostics":
		s.networkDiagnostic(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/network/diagnostics/"):
		s.networkDiagnosticJob(w, r, path.Base(endpoint))
	case r.Method == http.MethodPost && endpoint == "/network/checkpoints":
		s.networkCheckpoint(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/network/checkpoints/"):
		s.networkCheckpointAction(w, r, endpoint)
	case r.Method == http.MethodGet && endpoint == "/network/wireguard/status":
		s.wireguardStatus(w)
	case r.Method == http.MethodPost && endpoint == "/network/wireguard/apply":
		s.applyWireGuard(w, r)
	case r.Method == http.MethodPost && endpoint == "/network/wireguard/keygen":
		s.wireguardKeygen(w)
	case r.Method == http.MethodGet && endpoint == "/network/tailscale/status":
		s.tailscaleStatus(w)
	case r.Method == http.MethodPost && endpoint == "/network/tailscale/up":
		s.tailscaleUp(w, r)
	case r.Method == http.MethodPost && endpoint == "/network/tailscale/down":
		s.tailscaleDown(w, r)
	case r.Method == http.MethodPost && endpoint == "/network/tailscale/exit-node":
		s.tailscaleExitNode(w, r)
	case r.Method == http.MethodGet && endpoint == "/services":
		s.services(w, r)
	case r.Method == http.MethodGet && endpoint == "/power/ups":
		s.ups(w, r)
	case r.Method == http.MethodGet && endpoint == "/ups/status":
		s.upsStatus(w, r)
	case r.Method == http.MethodGet && endpoint == "/ups/policy":
		s.upsPolicy(w, r)
	case r.Method == http.MethodPatch && endpoint == "/ups/policy":
		s.updateUPSPolicy(w, r)
	case r.Method == http.MethodPost && endpoint == "/power/action":
		s.powerAction(w, r)
	case r.Method == http.MethodGet && endpoint == "/power/shutdown/plan":
		s.shutdownPlan(w, r)
	case r.Method == http.MethodPost && endpoint == "/power/shutdown":
		s.shutdownPower(w, r)
	case r.Method == http.MethodGet && endpoint == "/shares":
		s.listManagedShares(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/shares/"):
		s.getManagedShare(w, r, path.Base(endpoint))
	case r.Method == http.MethodPost && endpoint == "/shares":
		s.createManagedShare(w, r)
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/shares/") && strings.HasSuffix(endpoint, "/access"):
		s.updateShareAccess(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/shares/") && strings.Contains(strings.TrimPrefix(endpoint, "/shares/"), "/protocols/"):
		s.updateShareProtocol(w, r, endpoint)
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/shares/"):
		s.updateManagedShare(w, r, path.Base(endpoint))
	case r.Method == http.MethodDelete && strings.HasPrefix(endpoint, "/shares/"):
		s.deleteManagedShare(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/files":
		s.listFiles(w, r)
	case r.Method == http.MethodGet && endpoint == "/files/search":
		s.searchFiles(w, r)
	case r.Method == http.MethodGet && endpoint == "/files/properties":
		s.fileProperties(w, r)
	case r.Method == http.MethodGet && endpoint == "/files/download":
		s.downloadFile(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/mkdir":
		s.makeDirectory(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/rename":
		s.renameFile(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/delete":
		s.deleteFiles(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/transfer":
		s.transferFiles(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/upload":
		s.uploadFile(w, r)
	case r.Method == http.MethodGet && endpoint == "/files/recycle":
		s.listRecycleBin(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/recycle/restore":
		s.restoreRecycleBin(w, r)
	case r.Method == http.MethodPost && endpoint == "/files/recycle/purge":
		s.purgeRecycleBin(w, r)
	case r.Method == http.MethodGet && endpoint == "/events":
		s.stream(w, r)
	case r.Method == http.MethodGet && endpoint == "/events/stream":
		s.stream(w, r)
	case r.Method == http.MethodGet && endpoint == "/docker/summary":
		s.dockerSummary(w, r)
	case r.Method == http.MethodGet && endpoint == "/docker/apps":
		s.dockerApps(w)
	case r.Method == http.MethodGet && endpoint == "/docker/stacks":
		s.dockerStacks(w, r)
	case r.Method == http.MethodPost && endpoint == "/docker/stacks":
		s.createDockerStack(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/docker/stacks/"):
		s.dockerStackAction(w, r, endpoint)
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/docker/stacks/"):
		s.dockerStack(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/docker/containers":
		s.dockerContainers(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/docker/containers/"):
		s.dockerContainerAction(w, r, endpoint)
	case r.Method == http.MethodGet && endpoint == "/docker/images":
		s.dockerImages(w, r)
	case r.Method == http.MethodPost && endpoint == "/docker/images/check-updates":
		s.dockerImagesCheckUpdates(w, r)
	case r.Method == http.MethodPost && endpoint == "/docker/images/import":
		s.dockerImageImport(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/docker/images/"):
		s.dockerImageAction(w, r, endpoint)
	case r.Method == http.MethodGet && endpoint == "/docker/volumes":
		s.dockerVolumes(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/docker/logs/"):
		s.dockerLogs(w, r, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/health/components":
		s.healthComponents(w)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "endpoint not found"})
	}
}

func (s *apiServer) authStatus(w http.ResponseWriter, r *http.Request) {
	authenticated := false
	if cookie, err := r.Cookie("lumonas_session"); err == nil {
		_, authenticated = s.store.SessionUser(cookie.Value)
	}
	writeJSON(w, http.StatusOK, map[string]any{"required": s.authEnabled(), "configured": s.store.HasUsers(), "authenticated": authenticated})
}

func (s *apiServer) checkRateLimit(ip string) bool {
	s.rateMu.Lock()
	defer s.rateMu.Unlock()
	now := time.Now()
	window := 5 * time.Minute
	maxAttempts := 5
	attempts := s.rateAttempts[ip]
	var valid []time.Time
	for _, t := range attempts {
		if now.Sub(t) < window {
			valid = append(valid, t)
		}
	}
	if len(valid) >= maxAttempts {
		return false
	}
	s.rateAttempts[ip] = append(valid, now)
	// Clean up stale IPs with no valid attempts
	for key, attempts := range s.rateAttempts {
		if len(attempts) == 0 {
			delete(s.rateAttempts, key)
		}
	}
	return true
}

func (s *apiServer) cleanupExpiredCSRFTokens() {
	s.csrfMu.Lock()
	defer s.csrfMu.Unlock()
	now := time.Now().Unix()
	for token, binding := range s.csrfTokens {
		if now > binding.expires {
			delete(s.csrfTokens, token)
		}
	}
}

// issueCSRFToken mints a CSRF token bound to the given session so a token
// leaked from one session cannot authorise mutations on another.
func (s *apiServer) issueCSRFToken(sessionToken string, expires time.Time) string {
	csrfToken := newID("csrf")
	s.csrfMu.Lock()
	s.csrfTokens[csrfToken] = csrfBinding{sessionDigest: auth.TokenDigest(sessionToken), expires: expires.Unix()}
	s.csrfMu.Unlock()
	return csrfToken
}

// deleteCSRFTokensForSession removes every CSRF token bound to a session,
// used at logout so the tokens cannot be replayed.
func (s *apiServer) deleteCSRFTokensForSession(sessionToken string) {
	digest := auth.TokenDigest(sessionToken)
	s.csrfMu.Lock()
	defer s.csrfMu.Unlock()
	for token, binding := range s.csrfTokens {
		if binding.sessionDigest == digest {
			delete(s.csrfTokens, token)
		}
	}
}

// csrfForSession issues a fresh CSRF token for the caller's existing session;
// it exists because tokens live in memory and are lost on daemon restart
// while sessions persist in SQLite.
func (s *apiServer) csrfForSession(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("lumonas_session")
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return
	}
	user, ok := s.store.SessionUser(cookie.Value)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
		return
	}
	expires := time.Now().Add(12 * time.Hour)
	writeJSON(w, http.StatusOK, map[string]any{"username": user, "csrfToken": s.issueCSRFToken(cookie.Value, expires)})
}

func (s *apiServer) authLogin(w http.ResponseWriter, r *http.Request) {
	if !s.checkRateLimit(clientIP(r)) {
		w.Header().Set("Retry-After", "300")
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts, try again later"})
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	userID, err := s.store.VerifyCredentials(input.Username, input.Password)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	twoFactor, factorErr := s.store.TOTPEnabled(userID)
	if factorErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "two-factor state unavailable"})
		return
	}
	if twoFactor {
		challengeID := s.createTOTPChallenge(userID, input.Username)
		if challengeID == "" {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "challenge creation failed"})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"twoFactorRequired": true, "challengeId": challengeID})
		return
	}
	token, expires, err := s.store.CreateSessionForUser(userID, 12*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	csrfToken := s.issueCSRFToken(token, expires)
	http.SetCookie(w, &http.Cookie{Name: "lumonas_session", Value: token, Path: "/", Expires: expires, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: os.Getenv("LUMONAS_COOKIE_SECURE") == "true"})
	writeJSON(w, http.StatusOK, map[string]any{"username": input.Username, "expiresAt": expires, "csrfToken": csrfToken})
}

func (s *apiServer) authLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("lumonas_session"); err == nil {
		_ = s.store.DeleteSession(cookie.Value)
		s.deleteCSRFTokensForSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "lumonas_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *apiServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authEnabled() || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/api/v1/auth/status" || r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/login/2fa" || r.URL.Path == "/api/v1/auth/logout" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie("lumonas_session")
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		if _, ok := s.store.SessionUser(cookie.Value); !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
			return
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch || r.Method == http.MethodDelete {
			csrfToken := r.Header.Get("X-CSRF-Token")
			if csrfToken == "" {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "CSRF token required"})
				return
			}
			s.csrfMu.Lock()
			binding, exists := s.csrfTokens[csrfToken]
			s.csrfMu.Unlock()
			if !exists || time.Now().Unix() > binding.expires {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "invalid or expired CSRF token"})
				return
			}
			// A token is only valid for the session that requested it.
			if binding.sessionDigest != auth.TokenDigest(cookie.Value) {
				writeJSON(w, http.StatusForbidden, map[string]string{"error": "CSRF token does not match this session"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *apiServer) serverInfo(w http.ResponseWriter) {
	uuid, _ := s.store.Meta("nas_uuid")
	name, ok := s.store.Meta("server_name")
	if !ok || strings.TrimSpace(name) == "" {
		name = collector.Hostname()
	}
	writeJSON(w, http.StatusOK, model.ServerInfo{ID: "server-1", Name: name, Hostname: collector.Hostname(), Version: s.version, NASUUID: uuid, Timezone: time.Now().Location().String(), Health: s.serverHealth(), IP: primaryIP()})
}

func (s *apiServer) serverHealth() model.HealthState {
	health := model.Healthy
	disks, err := s.diskFunc()
	if err != nil {
		return model.Attention
	}
	for _, disk := range disks {
		if disk.Health == model.Critical {
			return model.Critical
		}
		if disk.Health == model.Warning || disk.Health == model.Attention {
			health = model.Attention
		}
	}
	current := make(map[string]bool, len(disks))
	for _, disk := range disks {
		current[disk.ID] = true
	}
	known, _ := s.store.KnownDisks()
	for _, disk := range known {
		if !current[disk.ID] {
			return model.Critical
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	protection := storage.DiscoverProtection(ctx, disks, nil, envOr("LUMONAS_SNAPRAID_CONFIG", "/etc/lumonas/snapraid.conf"))
	s.enrichProtection(&protection)
	if protection.Status == model.Critical {
		return model.Critical
	}
	if protection.Status == model.Attention {
		health = model.Attention
	}
	return health
}

func (s *apiServer) healthComponents(w http.ResponseWriter) {
	var components []model.HealthComponent
	disks, err := s.diskFunc()
	if err != nil {
		components = append(components, model.HealthComponent{
			ID: "disk-discovery", Label: "Disk discovery", Status: model.Attention,
			Message:     "Disk discovery is unavailable",
			Recommended: "Check that lsblk is installed and accessible",
		})
	} else {
		warningCount, criticalCount, offlineCount := 0, 0, 0
		var warnings, criticals, offlines []string
		for _, disk := range disks {
			switch disk.Health {
			case model.Warning:
				warningCount++
				warnings = append(warnings, disk.Name)
			case model.Critical:
				criticalCount++
				criticals = append(criticals, disk.Name)
			case model.Offline:
				offlineCount++
				offlines = append(offlines, disk.Name)
			}
		}
		diskStatus := model.Healthy
		diskMsg := fmt.Sprintf("%d disk(s) healthy", len(disks)-warningCount-criticalCount-offlineCount)
		if criticalCount > 0 {
			diskStatus = model.Critical
			diskMsg = fmt.Sprintf("%d critical: %s", criticalCount, strings.Join(criticals, ", "))
		} else if warningCount > 0 {
			diskStatus = model.Warning
			diskMsg = fmt.Sprintf("%d warning: %s", warningCount, strings.Join(warnings, ", "))
		} else if offlineCount > 0 {
			diskStatus = model.Attention
			diskMsg = fmt.Sprintf("%d offline: %s", offlineCount, strings.Join(offlines, ", "))
		}
		diskRecommended := ""
		if criticalCount > 0 {
			diskRecommended = "Replace failing disks immediately"
		}
		components = append(components, model.HealthComponent{
			ID: "disks", Label: "Disk health", Status: diskStatus, Message: diskMsg,
			Recommended: diskRecommended,
		})

		known, _ := s.store.KnownDisks()
		currentIDs := make(map[string]bool, len(disks))
		for _, disk := range disks {
			currentIDs[disk.ID] = true
		}
		var missingNames []string
		for _, k := range known {
			if !currentIDs[k.ID] {
				missingNames = append(missingNames, k.ID)
			}
		}
		if len(missingNames) > 0 {
			components = append(components, model.HealthComponent{
				ID: "missing-disks", Label: "Missing disks", Status: model.Critical,
				Message:     fmt.Sprintf("%d known disk(s) missing: %s", len(missingNames), strings.Join(missingNames, ", ")),
				Recommended: "Check cable connections and power. Do not remove protected disks.",
			})
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	protection := storage.DiscoverProtection(ctx, disks, nil, envOr("LUMONAS_SNAPRAID_CONFIG", "/etc/lumonas/snapraid.conf"))
	s.enrichProtection(&protection)
	protMsg := "Parity configured"
	if len(protection.ParityDisks) == 0 {
		protMsg = "No parity disks configured"
	}
	if protection.LastSyncAt != nil {
		protMsg += fmt.Sprintf(", last sync %s", protection.LastSyncAt.Format("2006-01-02 15:04"))
	}
	if protection.ChangesSinceSync > 0 {
		protMsg += fmt.Sprintf(", %d bytes unsynced", protection.ChangesSinceSync)
	}
	protRecommended := ""
	if protection.Status == model.Critical {
		protRecommended = "Sync immediately to restore protection"
	} else if protection.Status == model.Attention && protection.ChangesSinceSync > 0 {
		protRecommended = "Run a sync to protect recent changes"
	}
	components = append(components, model.HealthComponent{
		ID: "protection", Label: "SnapRAID protection", Status: protection.Status, Message: protMsg,
		Recommended: protRecommended,
	})

	score := 100
	for _, c := range components {
		switch c.Status {
		case model.Critical:
			score -= 40
		case model.Warning:
			score -= 20
		case model.Attention:
			score -= 10
		}
	}
	if score < 0 {
		score = 0
	}

	writeJSON(w, http.StatusOK, model.HealthBreakdown{
		Status:     s.serverHealth(),
		Score:      score,
		Components: components,
	})
}

func (s *apiServer) disks(w http.ResponseWriter) {
	disks, err := s.diskFunc()
	if err != nil {
		if s.log != nil {
			s.log.Warn("disk discovery unavailable", "error", err)
		}
		disks = []model.Disk{}
	}
	if err := s.store.SaveDiskInventory(disks); err != nil && s.log != nil {
		s.log.Warn("persist disk inventory failed", "error", err)
	}
	writeJSON(w, http.StatusOK, disks)
}

func (s *apiServer) pools(w http.ResponseWriter, r *http.Request) {
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk discovery unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, storage.DiscoverPools(ctx, disks, nil))
}

func (s *apiServer) disk(w http.ResponseWriter, id string) {
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk discovery unavailable"})
		return
	}
	for _, disk := range disks {
		if disk.ID == id {
			writeJSON(w, http.StatusOK, disk)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "disk not found"})
}

func (s *apiServer) protection(w http.ResponseWriter) {
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk discovery unavailable"})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	configPath := envOr("LUMONAS_SNAPRAID_CONFIG", "/etc/lumonas/snapraid.conf")
	protection := storage.DiscoverProtection(ctx, disks, nil, configPath)
	s.enrichProtection(&protection)
	writeJSON(w, http.StatusOK, protection)
}

func (s *apiServer) enrichProtection(protection *model.Protection) {
	if value, ok := s.store.Meta("snapraid_last_sync_at"); ok {
		if timestamp, err := time.Parse(time.RFC3339Nano, value); err == nil {
			protection.LastSyncAt = &timestamp
			result := "successful"
			protection.LastSyncResult = &result
			if protection.Status == model.Attention {
				protection.Status = model.Healthy
			}
		}
	}
	if value, ok := s.store.Meta("snapraid_last_scrub_at"); ok {
		if timestamp, err := time.Parse(time.RFC3339Nano, value); err == nil {
			protection.LastScrubAt = &timestamp
		}
	}
}

// retireDisk forgets a known disk identity after a physical replacement.
// Without it, the synthesized "disk missing" alert and the server health
// badge stay critical forever. Persisted generated alerts for the disk are
// resolved alongside the inventory row.
func (s *apiServer) retireDisk(w http.ResponseWriter, r *http.Request, diskID string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	known, err := s.store.KnownDisks()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "disk inventory unavailable"})
		return
	}
	var retired *model.Disk
	for index := range known {
		if known[index].ID == diskID {
			retired = &known[index]
			break
		}
	}
	if retired == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "disk is not in the known inventory"})
		return
	}
	if err := s.store.DeleteKnownDisk(diskID); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Retire may be used while the disk is absent (replacement already
	// pulled) or while it is still attached (wiping for reuse).
	if resolved, err := s.store.ResolveGeneratedAlerts("disk-missing", diskID); err == nil && resolved {
		s.publish("alert.resolved", "info", nil, map[string]any{"ruleId": "disk-missing", "resourceId": diskID})
	}
	s.recordRequestAudit(r, actor, "storage.disk.retire", diskID, map[string]any{"model": retired.Model, "serial": retired.Serial})
	s.publish("disk.retired", "info", &model.ResourceRef{Type: "disk", ID: diskID}, map[string]any{"diskId": diskID, "model": retired.Model})
	writeJSON(w, http.StatusOK, map[string]any{"id": diskID, "retired": true})
}

func (s *apiServer) planStorageOperation(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Action         storage.Action `json:"action"`
		DiskID         string         `json:"diskId"`
		RequestedState map[string]any `json:"requestedState"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity discovery unavailable"})
		return
	}
	var target *model.Disk
	for index := range disks {
		if disks[index].ID == input.DiskID {
			target = &disks[index]
			break
		}
	}
	if target == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "diskId must match a currently discovered stable disk identity"})
		return
	}
	plan, err := storage.NewPlan(newID("op"), input.Action, *target, s.currentGeneration(), time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	requestedState := input.RequestedState
	if requestedState == nil {
		requestedState = map[string]any{}
	}
	if err := storage.ValidateRequestedState(input.Action, target.ID, requestedState); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	plan.RequestedState = requestedState
	plan.PlanHash = storage.Hash(plan)
	if err := s.store.SavePlan(plan); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.publish("storage.operation.planned", "warning", &model.ResourceRef{Type: "disk", ID: target.ID}, map[string]any{"operationId": plan.OperationID, "action": plan.Action, "planHash": plan.PlanHash})
	writeJSON(w, http.StatusCreated, plan)
}

func (s *apiServer) confirmStorageOperation(w http.ResponseWriter, r *http.Request, operationID string) {
	plan, err := s.store.Plan(operationID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "operation plan not found"})
		return
	}
	var input struct {
		PlanHash              string `json:"planHash"`
		Reauthenticated       bool   `json:"reauthenticated"`
		StorageSafetyUnlocked bool   `json:"storageSafetyUnlocked"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.PlanHash == "" || input.PlanHash != plan.PlanHash {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "plan hash mismatch"})
		return
	}
	if !input.Reauthenticated || !input.StorageSafetyUnlocked || !s.safetyUnlocked() {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "reauthentication and the storage safety unlock are required"})
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity revalidation unavailable"})
		return
	}
	var actual *model.Disk
	for index := range disks {
		if disks[index].ID == plan.Target.DiskID {
			actual = &disks[index]
			break
		}
	}
	if actual == nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "stable disk identity is no longer present"})
		return
	}
	if err := storage.Validate(plan, *actual, time.Now().UTC(), s.currentGeneration()); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "storage plan revalidation failed: " + err.Error()})
		return
	}
	expectedIdentity := map[string]string{"id": plan.Target.DiskID, "wwn": plan.Target.WWN, "serial": plan.Target.Serial, "model": plan.Target.Model, "gptDiskGuid": plan.Target.GPTDiskGUID, "partitionUuid": plan.Target.PartitionUUID, "filesystemUuid": plan.Target.FilesystemUUID, "sizeBytes": strconv.FormatUint(plan.Target.SizeBytes, 10)}
	request := privileged.Request{Operation: string(plan.Action), OperationID: plan.OperationID, CorrelationID: requestCorrelationID(r), PlanHash: plan.PlanHash, TargetDiskID: plan.Target.DiskID, ExpectedIdentity: expectedIdentity, ExpectedState: map[string]string{"currentPath": plan.ExpectedState.CurrentPath, "mounted": strconv.FormatBool(plan.ExpectedState.Mounted), "role": plan.ExpectedState.Role, "poolId": plan.ExpectedState.PoolID}, RequestedState: plan.RequestedState, ExpiresAt: plan.ExpiresAt, Confirmed: true}
	result, err := s.executePrivileged(r.Context(), request)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	plan.Status = "executed"
	persistPlan := s.persistExecutedPlanFn
	if persistPlan == nil {
		persistPlan = s.store.SavePlan
	}
	if err := persistPlan(plan); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "storage operation completed but plan state persistence failed: " + err.Error()})
		return
	}
	s.advanceGeneration("storage." + string(plan.Action))
	s.publish("storage.operation.completed", "warning", &model.ResourceRef{Type: "disk", ID: plan.Target.DiskID}, map[string]any{"operationId": plan.OperationID, "action": plan.Action, "planHash": plan.PlanHash})
	s.persistMountState("storage." + string(plan.Action))
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) safetyUnlocked() bool {
	s.safetyMu.Lock()
	defer s.safetyMu.Unlock()
	return time.Now().UTC().Before(s.safetyUntil)
}

func (s *apiServer) storageSafety(w http.ResponseWriter) {
	s.safetyMu.Lock()
	defer s.safetyMu.Unlock()
	if time.Now().UTC().Before(s.safetyUntil) {
		writeJSON(w, http.StatusOK, map[string]any{"state": "unlocked", "unlockedUntil": s.safetyUntil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "locked", "unlockedUntil": nil})
}

func (s *apiServer) unlockStorageSafety(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Reauthenticated bool `json:"reauthenticated"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !input.Reauthenticated {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "reauthentication is required"})
		return
	}
	s.safetyMu.Lock()
	s.safetyUntil = time.Now().UTC().Add(15 * time.Minute)
	unlockedUntil := s.safetyUntil
	s.safetyMu.Unlock()
	s.publish("storage.safety.unlocked", "warning", nil, map[string]any{"unlockedUntil": unlockedUntil})
	writeJSON(w, http.StatusOK, map[string]any{"state": "unlocked", "unlockedUntil": unlockedUntil})
}

func (s *apiServer) lockStorageSafety(w http.ResponseWriter) {
	s.safetyMu.Lock()
	s.safetyUntil = time.Time{}
	s.safetyMu.Unlock()
	s.publish("storage.safety.locked", "info", nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"state": "locked", "unlockedUntil": nil})
}

func (s *apiServer) currentGeneration() int64 {
	return s.store.CurrentGeneration()
}

func (s *apiServer) advanceGeneration(action string) {
	generation, err := s.store.BeginGeneration(action)
	if err == nil {
		err = s.store.CommitGeneration(generation)
	}
	if err != nil {
		if s.log != nil {
			s.log.Warn("configuration generation commit failed", "action", action, "error", err)
		}
	} else {
		s.requestAutomaticBackup("config-change")
	}
}

func (s *apiServer) recoveryStatus(w http.ResponseWriter) {
	key := s.recoveryKeyString()
	directory := envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery")
	bundlePath := filepath.Join(directory, "latest.mrb")
	status := map[string]any{"configured": key != "", "latestPath": bundlePath, "verified": false}
	if key != "" {
		if bundle, err := os.ReadFile(bundlePath); err == nil {
			if plan, planErr := recovery.Plan(bundle, []byte(key)); planErr == nil {
				status["verified"] = plan.Verified && plan.DatabaseValid && plan.DesiredStateValid && plan.ComposeValid
				status["manifest"] = plan.Manifest
				if len(plan.Warnings) > 0 {
					status["warnings"] = plan.Warnings
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *apiServer) recoveryExport(w http.ResponseWriter, ctx context.Context) {
	key := s.recoveryKeyString()
	if key == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LUMONAS_RECOVERY_KEY is not configured"})
		return
	}
	database, err := s.store.BackupBytes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database backup failed: " + err.Error()})
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity export failed: " + err.Error()})
		return
	}
	diskIDs := make([]string, 0, len(disks))
	for _, disk := range disks {
		diskIDs = append(diskIDs, disk.ID)
	}
	nasUUID, hasNASUUID := s.store.Meta("nas_uuid")
	if !hasNASUUID || strings.TrimSpace(nasUUID) == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "NAS identity is not configured"})
		return
	}
	desired, _ := json.Marshal(map[string]any{"nasUuid": nasUUID, "configGeneration": s.currentGeneration(), "createdAt": time.Now().UTC()})
	compose := map[string][]byte{}
	stacks, stackErr := s.decoratedDockerStacks(ctx)
	if stackErr != nil {
		stacks, stackErr = s.dockerService.Stacks(ctx)
		if stackErr != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "Docker stack export failed: " + stackErr.Error()})
			return
		}
	}
	for _, stack := range stacks {
		compose[stack.Name+"/compose.yaml"] = []byte(stack.ComposeYAML)
	}
	appdata := s.collectDockerAppdata(ctx, stacks)
	var encrypted []byte
	if secretPath := os.Getenv("LUMONAS_RECOVERY_SECRETS_FILE"); secretPath != "" {
		encrypted, err = os.ReadFile(secretPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recovery secret input failed: " + err.Error()})
			return
		}
	}
	recoveryFiles, err := s.recoveryFiles()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recovery state export failed: " + err.Error()})
		return
	}
	bundle, err := recovery.Create(recovery.Input{Manifest: recovery.Manifest{ConfigSchema: 1, LumoNASVersion: s.version, NASUUID: nasUUID, Generation: s.currentGeneration(), DiskIDs: diskIDs}, DesiredState: desired, Database: database, Compose: compose, Files: recoveryFiles, Appdata: appdata.Payloads, EncryptedData: encrypted}, []byte(key))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "bundle creation failed: " + err.Error()})
		return
	}
	directory := envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery")
	persisted, err := recovery.PersistVerified(directory, bundle, []byte(key), time.Now().UTC())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.pruneRecoveryBundles(directory, 20)
	s.publish("recovery.bundle.created", "info", nil, map[string]any{"generation": persisted.Manifest.Generation})
	writeJSON(w, http.StatusCreated, map[string]any{"path": persisted.LatestPath, "manifest": persisted.Manifest, "verified": true, "appdataArchives": len(appdata.Payloads), "warnings": appdata.Warnings})
}

func (s *apiServer) pruneRecoveryBundles(directory string, keep int) {
	paths, err := filepath.Glob(filepath.Join(directory, "generation-*.mrb"))
	if err != nil || len(paths) <= keep {
		return
	}
	sort.Strings(paths)
	for _, item := range paths[:len(paths)-keep] {
		if err := os.Remove(item); err != nil && s.log != nil {
			s.log.Warn("old recovery bundle removal failed", "path", item, "error", err)
		}
	}
}

func (s *apiServer) recoveryFiles() (map[string][]byte, error) {
	files := make(map[string][]byte)
	// Managed shares are stored in SQLite, while the legacy JSON file remains
	// an import/compatibility surface. Always export the current store view so
	// a new installation can restore shares even when that legacy file never
	// existed.
	managed, err := s.store.ListManagedShares()
	if err != nil {
		return nil, fmt.Errorf("managed shares: %w", err)
	}
	if len(managed) > 0 {
		legacy := make([]shares.Share, 0, len(managed))
		for _, value := range managed {
			legacy = append(legacy, value.Legacy())
		}
		data, marshalErr := json.Marshal(legacy)
		if marshalErr != nil {
			return nil, fmt.Errorf("managed shares: %w", marshalErr)
		}
		files["config/shares.json"] = data
	} else if data, readErr := os.ReadFile(envOr("LUMONAS_SHARES_FILE", "/var/lib/lumonas/shares.json")); readErr == nil {
		files["config/shares.json"] = data
	}
	if data, readErr := os.ReadFile(envOr("LUMONAS_SNAPRAID_CONFIG", "/etc/lumonas/snapraid.conf")); readErr == nil {
		files["storage/snapraid.conf"] = data
	}

	connections, err := s.store.ListNetworkConnections()
	if err != nil {
		return nil, fmt.Errorf("network connections: %w", err)
	}
	if files["config/network-connections.json"], err = json.Marshal(connections); err != nil {
		return nil, fmt.Errorf("network connections: %w", err)
	}
	bindings, err := s.store.ListNetworkBindings()
	if err != nil {
		return nil, fmt.Errorf("network bindings: %w", err)
	}
	if files["config/network-bindings.json"], err = json.Marshal(bindings); err != nil {
		return nil, fmt.Errorf("network bindings: %w", err)
	}
	firewall, err := s.store.NetworkFirewallPolicy()
	if err != nil {
		return nil, fmt.Errorf("firewall policy: %w", err)
	}
	if files["config/firewall-policy.json"], err = json.Marshal(firewall); err != nil {
		return nil, fmt.Errorf("firewall policy: %w", err)
	}
	mounts, err := s.store.MountEntries()
	if err != nil {
		return nil, fmt.Errorf("mount entries: %w", err)
	}
	if files["storage/mounts.json"], err = json.Marshal(mounts); err != nil {
		return nil, fmt.Errorf("mount entries: %w", err)
	}
	return files, nil
}

func (s *apiServer) recoveryPlan(w http.ResponseWriter) {
	key := s.recoveryKeyString()
	if key == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LUMONAS_RECOVERY_KEY is not configured"})
		return
	}
	bundlePath := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
	bundle, err := os.ReadFile(bundlePath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recovery bundle not found"})
		return
	}
	plan, err := recovery.Plan(bundle, []byte(key))
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "recovery verification failed: " + err.Error()})
		return
	}
	if currentUUID, ok := s.store.Meta("nas_uuid"); ok && plan.Manifest.NASUUID != "" && currentUUID != plan.Manifest.NASUUID {
		plan.Warnings = append(plan.Warnings, "bundle belongs to a different NAS identity")
	}
	if disks, diskErr := s.diskFunc(); diskErr == nil {
		present := make(map[string]bool, len(disks))
		for _, disk := range disks {
			present[disk.ID] = true
		}
		for _, diskID := range plan.Manifest.DiskIDs {
			if !present[diskID] {
				plan.Warnings = append(plan.Warnings, "protected disk is missing: "+diskID)
			}
		}
	} else {
		plan.Warnings = append(plan.Warnings, "live disk inventory is unavailable; hardware identity is not verified")
	}
	writeJSON(w, http.StatusOK, plan)
}

func (s *apiServer) recoveryStage(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Confirmed       bool `json:"confirmed"`
		Reauthenticated bool `json:"reauthenticated"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !input.Confirmed || !input.Reauthenticated {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "explicit confirmation and reauthentication are required"})
		return
	}
	key := s.recoveryKeyString()
	if key == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "LUMONAS_RECOVERY_KEY is not configured"})
		return
	}
	bundlePath := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
	bundle, err := os.ReadFile(bundlePath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "recovery bundle not found"})
		return
	}
	stagingDirectory := envOr("LUMONAS_RECOVERY_STAGING_DIR", "/var/lib/lumonas/recovery/staged")
	result, err := recovery.Stage(bundle, []byte(key), stagingDirectory)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "recovery staging failed: " + err.Error()})
		return
	}
	s.pruneRecoveryStages(stagingDirectory, 3, result.Directory)
	s.publish("recovery.restore.staged", "warning", nil, map[string]any{"directory": result.Directory, "files": len(result.Files), "generation": result.Manifest.Generation})
	writeJSON(w, http.StatusAccepted, result)
}

func (s *apiServer) pruneRecoveryStages(directory string, keep int, current string) {
	paths, err := filepath.Glob(filepath.Join(directory, ".restore-*"))
	if err != nil || len(paths) <= keep {
		return
	}
	type stagedPath struct {
		path string
		when time.Time
	}
	items := make([]stagedPath, 0, len(paths))
	for _, path := range paths {
		if path == current {
			continue
		}
		info, statErr := os.Stat(path)
		if statErr == nil && info.IsDir() {
			items = append(items, stagedPath{path: path, when: info.ModTime()})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].when.Before(items[j].when) })
	remove := len(paths) - keep
	if remove > len(items) {
		remove = len(items)
	}
	for _, item := range items[:remove] {
		if err := os.RemoveAll(item.path); err != nil && s.log != nil {
			s.log.Warn("old recovery staging removal failed", "path", item.path, "error", err)
		}
	}
}

func (s *apiServer) metrics(w http.ResponseWriter) { writeJSON(w, http.StatusOK, collector.Metrics()) }

func (s *apiServer) networkInterfaces(w http.ResponseWriter) {
	interfaces, err := network.Interfaces()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, interfaces)
}

func (s *apiServer) networkInterfaceMetrics(w http.ResponseWriter) {
	m := collector.Metrics()
	writeJSON(w, http.StatusOK, m.NetInterfaces)
}

func (s *apiServer) networkWiFiScan(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	networks, available, err := network.ScanWiFi(nil)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"available": available, "networks": networks})
}

func (s *apiServer) networkCheckpoint(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ConnectionUUID  string            `json:"connectionUuid"`
		ConnectionID    string            `json:"connectionId"`
		Devices         []string          `json:"devices"`
		Changes         map[string]string `json:"changes"`
		TimeoutSeconds  int               `json:"timeoutSeconds"`
		Reauthenticated bool              `json:"reauthenticated"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !input.Reauthenticated {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "reauthentication is required for network changes"})
		return
	}
	if input.TimeoutSeconds == 0 {
		input.TimeoutSeconds = 60
	}
	changes := make(map[string]any, len(input.Changes))
	for key, value := range input.Changes {
		changes[key] = value
	}
	requested := map[string]any{"connectionUuid": input.ConnectionUUID, "devices": input.Devices, "changes": changes, "timeoutSeconds": input.TimeoutSeconds}
	operationID := newID("net")
	result, err := s.executePrivileged(r.Context(), privileged.Request{Operation: "network.checkpoint.begin", OperationID: operationID, PlanHash: operationID, RequestedState: requested, ExpiresAt: time.Now().UTC().Add(time.Duration(input.TimeoutSeconds+60) * time.Second), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": result.Error})
		return
	}
	if input.ConnectionID != "" {
		record := s.recordNetworkCheckpointFn
		if record == nil {
			record = s.store.RecordNetworkCheckpoint
		}
		if recordErr := record(operationID, input.ConnectionID, "pending"); recordErr != nil {
			rollbackErr := s.rollbackNetworkCheckpoint(operationID)
			if rollbackErr != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "network checkpoint persistence failed and rollback failed: " + rollbackErr.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "network checkpoint persistence failed; checkpoint rolled back: " + recordErr.Error()})
			return
		}
	}
	s.publish("network.checkpoint.created", "warning", &model.ResourceRef{Type: "network-checkpoint", ID: operationID}, map[string]any{"operationId": operationID, "timeoutSeconds": input.TimeoutSeconds})
	writeJSON(w, http.StatusAccepted, result)
}

func (s *apiServer) networkCheckpointAction(w http.ResponseWriter, r *http.Request, endpoint string) {
	parts := strings.Split(strings.Trim(endpoint, "/"), "/")
	if len(parts) != 4 || (parts[3] != "commit" && parts[3] != "rollback") {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "network checkpoint action not found"})
		return
	}
	var input struct {
		Confirmed bool `json:"confirmed"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
	}
	if !input.Confirmed {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "explicit checkpoint confirmation is required"})
		return
	}
	operationID := parts[2]
	result, err := s.executePrivileged(r.Context(), privileged.Request{Operation: "network.checkpoint." + parts[3], OperationID: operationID, PlanHash: operationID, Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	complete := s.completeNetworkCheckpointFn
	if complete == nil {
		complete = s.store.CompleteNetworkCheckpoint
	}
	connectionID, err := complete(operationID, parts[3])
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "network checkpoint action completed but persistence failed: " + err.Error()})
		return
	}
	connection, err := s.store.NetworkConnection(connectionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "network checkpoint action completed but connection state could not be loaded: " + err.Error()})
		return
	}
	if parts[3] == "commit" {
		connection.Status = "committed"
	} else {
		connection.Status = "rolled-back"
	}
	if _, err := s.store.UpsertNetworkConnection(connection); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "network checkpoint action completed but connection state persistence failed: " + err.Error()})
		return
	}
	if parts[3] == "commit" {
		s.advanceGeneration("network.checkpoint.commit")
	}
	s.publish("network.checkpoint."+parts[3], "info", &model.ResourceRef{Type: "network-checkpoint", ID: operationID}, nil)
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) wireguardStatus(w http.ResponseWriter) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	iface := envOr("LUMONAS_WG_INTERFACE", "wg0")
	status, err := network.WireGuardShow(ctx, iface)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"interface": iface, "connected": false, "peers": 0})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *apiServer) applyWireGuard(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input network.WireGuardConfig
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := network.ValidateWireGuardConfig(input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	iface := envOr("LUMONAS_WG_INTERFACE", "wg0")
	operationID := newID("wireguard")
	result, err := s.executePrivileged(r.Context(), privileged.Request{
		Operation: "network.wireguard.apply", OperationID: operationID, PlanHash: operationID,
		RequestedState: map[string]any{"interface": iface, "config": input},
		ExpiresAt:      time.Now().UTC().Add(2 * time.Minute), Confirmed: true,
	})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	s.recordRequestAudit(r, actor, "network.wireguard.apply", iface, map[string]any{"operationId": operationID})
	s.advanceGeneration("network.wireguard.apply")
	s.publish("network.wireguard.applied", "info", &model.ResourceRef{Type: "wireguard", ID: iface}, map[string]any{"operationId": operationID})
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) wireguardKeygen(w http.ResponseWriter) {
	privKey, pubKey, err := network.GenerateWireGuardKeyPair()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"privateKey": privKey, "publicKey": pubKey})
}

func (s *apiServer) tailscaleStatus(w http.ResponseWriter) {
	if !network.TailscaleIsInstalled() {
		writeJSON(w, http.StatusOK, map[string]any{"installed": false, "running": false})
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	status, err := network.TailscaleGetStatus(ctx)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"installed": true, "running": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *apiServer) tailscaleUp(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Hostname string `json:"hostname"`
		AuthKey  string `json:"authKey,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if err := network.ValidateTailscaleConfig(input.Hostname); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	operationID := newID("tailscale")
	result, err := s.executePrivileged(r.Context(), privileged.Request{
		Operation: "network.tailscale.up", OperationID: operationID, PlanHash: operationID,
		RequestedState: map[string]any{"hostname": input.Hostname, "authKey": input.AuthKey},
		ExpiresAt:      time.Now().UTC().Add(2 * time.Minute), Confirmed: true,
	})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	s.recordRequestAudit(r, actor, "network.tailscale.up", input.Hostname, map[string]any{"operationId": operationID})
	s.advanceGeneration("network.tailscale.up")
	s.publish("network.tailscale.connected", "info", &model.ResourceRef{Type: "tailscale", ID: input.Hostname}, map[string]any{"operationId": operationID})
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) tailscaleDown(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	operationID := newID("tailscale")
	result, err := s.executePrivileged(r.Context(), privileged.Request{Operation: "network.tailscale.down", OperationID: operationID, PlanHash: operationID, ExpiresAt: time.Now().UTC().Add(2 * time.Minute), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	s.recordRequestAudit(r, actor, "network.tailscale.down", "tailscale", map[string]any{"operationId": operationID})
	s.advanceGeneration("network.tailscale.down")
	s.publish("network.tailscale.disconnected", "info", nil, map[string]any{"operationId": operationID})
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) tailscaleExitNode(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		PeerIP string `json:"peerIp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.PeerIP != "" && net.ParseIP(input.PeerIP) == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "peerIp must be a valid IP address"})
		return
	}
	operationID := newID("tailscale")
	result, err := s.executePrivileged(r.Context(), privileged.Request{Operation: "network.tailscale.exit-node", OperationID: operationID, PlanHash: operationID, RequestedState: map[string]any{"peerIp": input.PeerIP}, ExpiresAt: time.Now().UTC().Add(2 * time.Minute), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	s.recordRequestAudit(r, actor, "network.tailscale.exit-node", input.PeerIP, map[string]any{"operationId": operationID})
	s.advanceGeneration("network.tailscale.exit-node")
	s.publish("network.tailscale.exit-node.updated", "info", &model.ResourceRef{Type: "tailscale", ID: "exit-node"}, map[string]any{"operationId": operationID})
	writeJSON(w, http.StatusOK, result)
}

func (s *apiServer) services(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, services.Collect(ctx, services.DefaultNames))
}

func (s *apiServer) ups(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var names []string
	for _, name := range strings.Split(os.Getenv("LUMONAS_UPS_NAMES"), ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	writeJSON(w, http.StatusOK, power.Discover(ctx, names, nil))
}

func (s *apiServer) powerAction(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Action          string `json:"action"`
		Reauthenticated bool   `json:"reauthenticated"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if !input.Reauthenticated {
		writeJSON(w, http.StatusLocked, map[string]string{"error": "reauthentication is required for power actions"})
		return
	}
	if input.Action != "poweroff" && input.Action != "reboot" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "power action is not allow-listed"})
		return
	}
	operationID := newID("power")
	result, err := s.executePrivileged(r.Context(), privileged.Request{Operation: "power.shutdown", OperationID: operationID, PlanHash: operationID, RequestedState: map[string]any{"action": input.Action}, ExpiresAt: time.Now().UTC().Add(2 * time.Minute), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	s.recordRequestAudit(r, actor, "power.action", operationID, map[string]any{"operationId": operationID, "action": input.Action})
	s.publish("power.action", "critical", nil, map[string]any{"operationId": operationID, "action": input.Action})
	writeJSON(w, http.StatusAccepted, result)
}

func (s *apiServer) shareStore() shares.Store {
	return shares.Store{Path: envOr("LUMONAS_SHARES_FILE", "/var/lib/lumonas/shares.json")}
}

func (s *apiServer) listShares(w http.ResponseWriter) {
	values, err := s.shareStore().Load()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, values)
}

func (s *apiServer) createShare(w http.ResponseWriter, r *http.Request) {
	var share shares.Share
	if err := json.NewDecoder(r.Body).Decode(&share); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if share.ID == "" {
		share.ID = newID("share")
	}
	if err := shares.Validate(share); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	store := s.shareStore()
	existing, err := store.Load()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	for _, current := range existing {
		if current.Name == share.Name {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "share name already exists"})
			return
		}
	}
	existing = append(existing, share)
	config, err := shares.RenderSamba(existing)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	configPath := envOr("LUMONAS_SAMBA_CONFIG", "/var/lib/lumonas/generated/smb.conf")
	if err := shares.WriteGenerated(configPath, config); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := shares.ValidateSamba(configPath); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := store.Save(existing); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.advanceGeneration("share.create")
	s.publish("share.created", "info", &model.ResourceRef{Type: "share", ID: share.ID}, map[string]any{"name": share.Name})
	writeJSON(w, http.StatusCreated, share)
}

func (s *apiServer) updateShare(w http.ResponseWriter, r *http.Request, id string) {
	var replacement shares.Share
	if err := json.NewDecoder(r.Body).Decode(&replacement); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	replacement.ID = id
	if err := shares.Validate(replacement); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	shareStore := s.shareStore()
	existing, err := shareStore.Load()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	found := false
	for index := range existing {
		if existing[index].ID == id {
			existing[index] = replacement
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "share not found"})
		return
	}
	config, err := shares.RenderSamba(existing)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	configPath := envOr("LUMONAS_SAMBA_CONFIG", "/var/lib/lumonas/generated/smb.conf")
	if err := shares.WriteGenerated(configPath, config); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := shares.ValidateSamba(configPath); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	if err := shareStore.Save(existing); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.advanceGeneration("share.update")
	s.publish("share.updated", "info", &model.ResourceRef{Type: "share", ID: id}, map[string]any{"name": replacement.Name})
	writeJSON(w, http.StatusOK, replacement)
}

func (s *apiServer) alerts(w http.ResponseWriter) {
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusOK, []model.Alert{})
		return
	}
	_ = s.store.SaveDiskInventory(disks)
	alerts := make([]model.Alert, 0)
	now := time.Now().UTC()
	acknowledged, _ := s.store.AcknowledgedAlerts()
	for _, disk := range disks {
		if disk.Health != model.Warning && disk.Health != model.Critical {
			continue
		}
		severity := "warning"
		if disk.Health == model.Critical {
			severity = "critical"
		}
		alert := model.Alert{ID: "disk-health-" + disk.ID, Severity: severity, Title: "Disk health requires attention", Description: fmt.Sprintf("%s (%s) reported %s health", disk.Name, disk.Model, disk.Health), Resource: &model.ResourceRef{Type: "disk", ID: disk.ID}, State: "firing", StartedAt: now}
		if acknowledged[alert.ID] {
			alert.State = "acknowledged"
		}
		alerts = append(alerts, alert)
	}
	known, _ := s.store.KnownDisks()
	current := make(map[string]bool, len(disks))
	for _, disk := range disks {
		current[disk.ID] = true
	}
	for _, disk := range known {
		if current[disk.ID] {
			continue
		}
		alert := model.Alert{ID: "disk-missing-" + disk.ID, Severity: "critical", Title: "Disk is missing", Description: fmt.Sprintf("%s (%s) with stable identity %s was not discovered", disk.Model, disk.Serial, disk.ID), Resource: &model.ResourceRef{Type: "disk", ID: disk.ID}, State: "firing", StartedAt: disk.LastSeen}
		if acknowledged[alert.ID] {
			alert.State = "acknowledged"
		}
		alerts = append(alerts, alert)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, unit := range power.Discover(ctx, nil, nil) {
		if !unit.OnBattery {
			continue
		}
		alert := model.Alert{ID: "ups-on-battery-" + unit.Name, Severity: "critical", Title: "UPS is on battery", Description: fmt.Sprintf("UPS %s reports status %s", unit.Name, unit.Status), Resource: &model.ResourceRef{Type: "ups", ID: unit.Name}, State: "firing", StartedAt: now}
		if acknowledged[alert.ID] {
			alert.State = "acknowledged"
		}
		alerts = append(alerts, alert)
	}
	generated, err := s.store.GeneratedAlerts()
	if err == nil {
		alerts = append(alerts, generated...)
	}
	writeJSON(w, http.StatusOK, alerts)
}

func (s *apiServer) ackAlert(w http.ResponseWriter, id string) {
	// Persist the acknowledgement for every alert kind first.
	if err := s.store.AckAlert(id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// Generated (rule-fired) alerts are persisted wholesale: return them.
	if generated, err := s.store.GeneratedAlerts(); err == nil {
		for _, alert := range generated {
			if alert.ID == id {
				alert.State = "acknowledged"
				writeJSON(w, http.StatusOK, alert)
				return
			}
		}
	}
	disks, _ := s.diskFunc()
	for _, disk := range disks {
		if "disk-health-"+disk.ID != id {
			continue
		}
		if disk.Health != model.Warning && disk.Health != model.Critical {
			break
		}
		severity := "warning"
		if disk.Health == model.Critical {
			severity = "critical"
		}
		writeJSON(w, http.StatusOK, model.Alert{ID: id, Severity: severity, Title: "Disk health requires attention", Description: fmt.Sprintf("%s (%s) reported %s health", disk.Name, disk.Model, disk.Health), Resource: &model.ResourceRef{Type: "disk", ID: disk.ID}, State: "acknowledged", StartedAt: time.Now().UTC()})
		return
	}
	if strings.HasPrefix(id, "disk-missing-") {
		known, _ := s.store.KnownDisks()
		for _, disk := range known {
			if id != "disk-missing-"+disk.ID {
				continue
			}
			writeJSON(w, http.StatusOK, model.Alert{ID: id, Severity: "critical", Title: "Disk is missing", Description: fmt.Sprintf("%s (%s) with stable identity %s was not discovered", disk.Model, disk.Serial, disk.ID), Resource: &model.ResourceRef{Type: "disk", ID: disk.ID}, State: "acknowledged", StartedAt: disk.LastSeen})
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found"})
}

func (s *apiServer) activity(w http.ResponseWriter) {
	events, err := s.store.Events(100)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	activity := make([]model.ActivityEvent, 0, len(events))
	for _, event := range events {
		activity = append(activity, model.ActivityEvent{ID: event.ID, Timestamp: event.Timestamp, Category: activityCategory(event.Type), Title: activityTitle(event.Type), Description: event.Severity, Resource: event.Resource})
	}
	writeJSON(w, http.StatusOK, activity)
}

func (s *apiServer) audit(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if value := r.URL.Query().Get("limit"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			limit = parsed
		}
	}
	entries, err := s.store.Audit(limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *apiServer) notificationTest(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input notify.Message
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	sender := notify.Sender{Config: notify.Config{WebhookURL: os.Getenv("LUMONAS_NOTIFY_WEBHOOK_URL"), NtfyURL: os.Getenv("LUMONAS_NOTIFY_NTFY_URL"), MinSeverity: envOr("LUMONAS_NOTIFY_MIN_SEVERITY", "warning")}, UserAgent: "LumoNAS/" + s.version}
	if err := sender.Send(r.Context(), input); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	_ = s.store.SaveAudit(store.AuditEntry{Actor: actor, Action: "notification.test", Outcome: "sent", CorrelationID: requestCorrelationID(r), Generation: s.currentGeneration(), Metadata: map[string]any{"severity": input.Severity}})
	writeJSON(w, http.StatusNoContent, nil)
}

func activityCategory(eventType string) string {
	switch {
	case strings.HasPrefix(eventType, "docker."):
		return "docker"
	case strings.HasPrefix(eventType, "storage."), strings.HasPrefix(eventType, "disk."):
		return "storage"
	case strings.HasPrefix(eventType, "recovery."):
		return "backup"
	case strings.HasPrefix(eventType, "job."):
		return "config"
	default:
		return "config"
	}
}
func activityTitle(eventType string) string { return strings.ReplaceAll(eventType, ".", " ") }

func (s *apiServer) dockerSummary(w http.ResponseWriter, r *http.Request) {
	stacks, running, updates := s.dockerCounts()
	writeJSON(w, http.StatusOK, map[string]int{"stacks": stacks, "appsRunning": running, "updatesAvailable": updates})
}

func (s *apiServer) dockerApps(w http.ResponseWriter) {
	apps, err := dockerruntime.LoadCatalog(s.catalogFile)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

func (s *apiServer) dockerStacks(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	stacks, err := s.decoratedDockerStacks(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	containers, _ := s.dockerService.Containers(ctx)
	for index := range stacks {
		own := make([]string, 0)
		for _, container := range containers {
			if container.StackID == stacks[index].Name || container.StackID == stacks[index].ID {
				own = append(own, container.State)
			}
		}
		if len(own) == 0 {
			continue
		}
		stacks[index].State = "running"
		stacks[index].Status = "healthy"
		allStopped := true
		for _, state := range own {
			switch state {
			case "unhealthy":
				stacks[index].State = "unhealthy"
				stacks[index].Status = "critical"
			case "exited", "created":
			default:
				allStopped = false
			}
		}
		if stacks[index].State == "unhealthy" {
			continue
		}
		if allStopped {
			stacks[index].State = "stopped"
			stacks[index].Status = "offline"
		}
	}
	writeJSON(w, http.StatusOK, stacks)
}

func (s *apiServer) decoratedDockerStacks(ctx context.Context) ([]dockerruntime.Stack, error) {
	stacks, err := s.dockerService.Stacks(ctx)
	if err != nil {
		return nil, err
	}
	catalog, catalogErr := dockerruntime.LoadCatalog(s.catalogFile)
	if catalogErr != nil {
		return nil, catalogErr
	}
	for index := range stacks {
		stacks[index] = dockerruntime.EnrichStack(stacks[index], catalog)
	}
	return stacks, nil
}

func (s *apiServer) createDockerStack(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	var input struct {
		Name        string            `json:"name"`
		CatalogID   string            `json:"catalogId"`
		ComposeYAML string            `json:"composeYaml"`
		Env         map[string]string `json:"env"`
		StorageMap  []struct {
			FieldID       string `json:"fieldId"`
			ContainerPath string `json:"containerPath"`
			ResourceID    string `json:"resourceId"`
		} `json:"storageMap"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.Name == "" {
		input.Name = "imported-stack"
	}
	if input.ComposeYAML == "" && input.CatalogID != "" {
		catalog, err := dockerruntime.LoadCatalog(s.catalogFile)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
		for _, app := range catalog {
			if app.ID != input.CatalogID {
				continue
			}
			mappings := make([]dockerruntime.StorageMapping, 0, len(input.StorageMap))
			for _, item := range input.StorageMap {
				mappings = append(mappings, dockerruntime.StorageMapping{ContainerPath: item.ContainerPath, ResourceID: item.ResourceID, ResourceLabel: item.FieldID})
			}
			input.ComposeYAML, err = dockerruntime.BuildCompose(app, input.Name, input.Env, mappings)
			if err != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
				return
			}
			break
		}
		if input.ComposeYAML == "" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "catalog app not found"})
			return
		}
	}
	stack, err := s.dockerService.CreateStack(input.Name, input.ComposeYAML)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.advanceGeneration("docker.stack.create")
	s.recordRequestAudit(r, actor, "docker.stack.create", stack.ID, map[string]any{"name": stack.Name})
	s.publish("docker.stack.created", "info", &model.ResourceRef{Type: "stack", ID: stack.ID}, map[string]any{"stackId": stack.ID})
	writeJSON(w, http.StatusCreated, stack)
}

func (s *apiServer) dockerStack(w http.ResponseWriter, r *http.Request, id string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	stacks, err := s.decoratedDockerStacks(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	for _, stack := range stacks {
		if stack.ID == id {
			writeJSON(w, http.StatusOK, stack)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "stack not found"})
}

func (s *apiServer) dockerStackAction(w http.ResponseWriter, r *http.Request, endpoint string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(endpoint, "/"), "/")
	if len(parts) != 4 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "stack action not found"})
		return
	}
	stackID, action := parts[2], parts[3]
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	var input struct {
		ComposeYAML string `json:"composeYaml"`
		Backup      bool   `json:"backup"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
			return
		}
	}
	stacks, err := s.dockerService.Stacks(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	for _, stack := range stacks {
		if stack.ID != stackID {
			continue
		}
		if input.Backup {
			// Pre-update safety net: snapshot the recoverable configuration
			// before touching a running stack.
			s.requestAutomaticBackup("pre-stack-update")
		}
		if input.ComposeYAML != "" {
			updated, updateErr := s.dockerService.UpdateCompose(stack.Name, input.ComposeYAML)
			if updateErr != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": updateErr.Error()})
				return
			}
			stack = updated
			s.advanceGeneration("docker.stack.compose.update")
		}
		if action == "update" {
			result, updateErr := s.dockerService.UpdateStack(ctx, stack, func(probeCtx context.Context) error {
				return s.probeStackHealth(probeCtx, stack.Name)
			}, dockerruntime.UpdateOptions{})
			if updateErr != nil {
				eventKind := "docker.stack.update.failed"
				if result.RolledBack {
					eventKind = "docker.stack.rollback"
				}
				s.publish(eventKind, "warning", &model.ResourceRef{Type: "stack", ID: stack.ID}, map[string]any{"stackId": stack.ID, "reason": result.Reason})
				writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": updateErr.Error(), "rolledBack": result.RolledBack, "reason": result.Reason})
				return
			}
			s.recordRequestAudit(r, actor, "docker.stack.action", stack.ID, map[string]any{"action": action, "healthGated": true})
			s.publish("docker.stack.action", "info", &model.ResourceRef{Type: "stack", ID: stack.ID}, map[string]any{"stackId": stack.ID, "action": action})
			writeJSON(w, http.StatusAccepted, stack)
			return
		}
		if err := s.dockerService.Action(ctx, stack, action); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		s.recordRequestAudit(r, actor, "docker.stack.action", stack.ID, map[string]any{"action": action})
		s.publish("docker.stack.action", "info", &model.ResourceRef{Type: "stack", ID: stack.ID}, map[string]any{"stackId": stack.ID, "action": action})
		writeJSON(w, http.StatusAccepted, stack)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "stack not found"})
}

// probeStackHealth reports whether every container of a stack is currently
// running. It is the health gate for stack updates: restart loops or
// crash-looping services keep failing the probe until the rollback window
// closes.
func (s *apiServer) probeStackHealth(ctx context.Context, stackName string) error {
	containers, err := s.dockerService.Containers(ctx)
	if err != nil {
		return err
	}
	found := 0
	for _, container := range containers {
		if container.StackID != stackName {
			continue
		}
		found++
		if container.State != "running" {
			return fmt.Errorf("container %s is %s", container.Name, container.State)
		}
	}
	if found == 0 {
		return fmt.Errorf("no containers are running for stack %s", stackName)
	}
	return nil
}

func (s *apiServer) dockerContainers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	containers, err := s.dockerService.Containers(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, containers)
}

func (s *apiServer) dockerImages(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	images, err := s.dockerService.Images(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, images)
}

// dockerImagesCheckUpdates compares local images against the registry and
// reports which ones have a newer build upstream. The result also feeds the
// "updates available" counters, which are otherwise permanently zero.
func (s *apiServer) dockerImagesCheckUpdates(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	images, err := s.dockerService.CheckImageUpdates(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	updates := 0
	for _, image := range images {
		if image.UpdateAvailable {
			updates++
		}
	}
	s.recordRequestAudit(r, actor, "docker.images.check_updates", "docker", map[string]any{"updates": updates})
	s.publish("docker.updates.checked", "info", nil, map[string]any{"updates": updates, "images": len(images)})
	writeJSON(w, http.StatusOK, images)
}

const dockerImportMaxBytes int64 = 20 << 30

func (s *apiServer) dockerImageImport(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, dockerImportMaxBytes+1)
	file, _, err := r.FormFile("archive")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "a Docker save archive is required in the archive field"})
		return
	}
	defer file.Close()

	directory := envOr("LUMONAS_DOCKER_IMPORT_DIR", "/var/lib/lumonas/imports")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image import directory could not be created"})
		return
	}
	temporary, err := os.CreateTemp(directory, "image-import-*.tar")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image import staging failed"})
		return
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}
	defer cleanup()
	if err := temporary.Chmod(0o600); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image import staging permissions failed"})
		return
	}
	written, err := io.Copy(temporary, io.LimitReader(file, dockerImportMaxBytes+1))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "image archive upload failed"})
		return
	}
	if written > dockerImportMaxBytes {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "image archive exceeds the 20 GiB limit"})
		return
	}
	if err := temporary.Sync(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image archive could not be flushed"})
		return
	}
	if err := temporary.Close(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "image archive could not be closed"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	if err := s.dockerService.ImportImage(ctx, temporaryPath); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "Docker image import failed"})
		return
	}
	s.recordRequestAudit(r, actor, "docker.image.import", "images", map[string]any{"bytes": written})
	s.publish("docker.image.imported", "info", nil, map[string]any{"bytes": written})
	writeJSON(w, http.StatusOK, map[string]any{"status": "imported", "bytes": written})
}

func (s *apiServer) dockerContainerAction(w http.ResponseWriter, r *http.Request, endpoint string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(endpoint, "/"), "/")
	if len(parts) != 4 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "container action not found"})
		return
	}
	id, action := parts[2], parts[3]
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
	defer cancel()
	if err := s.dockerService.ContainerAction(ctx, id, action); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "docker.container.action", id, map[string]any{"action": action})
	containers, err := s.dockerService.Containers(ctx)
	if err != nil {
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
		return
	}
	for _, container := range containers {
		if container.ID == id || container.Name == id {
			s.publish("docker.container.action", "info", &model.ResourceRef{Type: "container", ID: id}, map[string]any{"action": action})
			writeJSON(w, http.StatusOK, container)
			return
		}
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (s *apiServer) dockerImageAction(w http.ResponseWriter, r *http.Request, endpoint string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	parts := strings.Split(strings.Trim(endpoint, "/"), "/")
	if len(parts) != 4 || parts[3] != "update" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "image action not found"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	images, err := s.dockerService.Images(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	for _, image := range images {
		if image.ID != parts[2] {
			continue
		}
		if err := s.dockerService.UpdateImage(ctx, image); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		s.publish("docker.image.updated", "info", &model.ResourceRef{Type: "image", ID: image.ID}, map[string]any{"repository": image.Repo, "tag": image.Tag})
		s.recordRequestAudit(r, actor, "docker.image.update", image.ID, map[string]any{"repository": image.Repo, "tag": image.Tag})
		writeJSON(w, http.StatusAccepted, image)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "image not found"})
}

func (s *apiServer) dockerVolumes(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	volumes, err := s.dockerService.Volumes(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, volumes)
}

func (s *apiServer) dockerLogs(w http.ResponseWriter, r *http.Request, container string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	lines, err := s.dockerService.Logs(ctx, container, 200)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, lines)
}

func (s *apiServer) listJobs(w http.ResponseWriter) {
	jobs, err := s.store.Jobs()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (s *apiServer) job(w http.ResponseWriter, id string) {
	job, err := s.store.Job(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *apiServer) createJob(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Type       string `json:"type"`
		ResourceID string `json:"resourceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	if input.Type != "smart.short" && input.Type != "smart.extended" && input.Type != "snapraid.sync" && input.Type != "snapraid.scrub" {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "job type is not enabled in this runtime"})
		return
	}
	if input.Type == "snapraid.sync" || input.Type == "snapraid.scrub" {
		job := model.Job{ID: newID("job"), CorrelationID: requestCorrelationID(r), Type: input.Type, Title: strings.ReplaceAll(input.Type, ".", " "), ResourceID: "protection", State: "queued", CreatedAt: time.Now().UTC()}
		if err := s.admitJob(job); err != nil {
			if isJobResourceBusy(err) {
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
		go s.runProtectionJob(job)
		writeJSON(w, http.StatusAccepted, job)
		return
	}
	if input.ResourceID == "" {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "resourceId is required"})
		return
	}
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "disk identity verification unavailable"})
		return
	}
	var target model.Disk
	found := false
	for _, disk := range disks {
		if disk.ID == input.ResourceID {
			target = disk
			found = true
			break
		}
	}
	if !found {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": "resourceId does not match a currently discovered stable disk identity"})
		return
	}
	job := model.Job{ID: newID("job"), CorrelationID: requestCorrelationID(r), Type: input.Type, Title: "SMART " + strings.TrimPrefix(input.Type, "smart.") + " validation", ResourceID: input.ResourceID, State: "queued", CreatedAt: time.Now().UTC()}
	if err := s.admitJob(job); err != nil {
		if isJobResourceBusy(err) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	go s.runReadOnlyJob(job, target)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *apiServer) runReadOnlyJob(job model.Job, target model.Disk) {
	time.Sleep(50 * time.Millisecond)
	now := time.Now().UTC()
	progress := 10.0
	job.State, job.Stage, job.StartedAt, job.Progress = "running", "Validating disk identity", &now, &progress
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	time.Sleep(100 * time.Millisecond)
	smart, err := collector.SMART(nil, target.CurrentPath)
	if err != nil {
		job.State, job.Error, job.Stage, job.FinishedAt = "failed", err.Error(), "SMART read failed", &now
		_ = s.store.SaveJob(job)
		s.publish("job.state_changed", "warning", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
		return
	}
	progress = 100
	job.State, job.Stage, job.FinishedAt, job.Progress = "successful", "SMART data collected", &now, &progress
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job, "smart": smart})
}

func (s *apiServer) runProtectionJob(job model.Job) {
	now := time.Now().UTC()
	progress := 5.0
	job.State, job.Stage, job.StartedAt, job.Progress = "running", "Validating SnapRAID configuration", &now, &progress
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	requested := map[string]any{"configPath": envOr("LUMONAS_SNAPRAID_CONFIG", "/etc/lumonas/snapraid.conf")}
	if job.Type == "snapraid.scrub" {
		requested["scrubPercent"] = envOr("LUMONAS_SNAPRAID_SCRUB_PERCENT", "5")
	}
	if job.Type == "snapraid.fix" {
		dataName := s.takeFixStage(job.ID)
		if dataName == "" {
			job.State, job.Stage, job.Error, job.FinishedAt = "failed", "Missing recovery slot", "no SnapRAID data slot was recorded for this fix job", &now
			_ = s.store.SaveJob(job)
			s.publish("job.state_changed", "warning", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
			return
		}
		requested["dataName"] = dataName
	}
	currentDisks, discoveryErr := s.diskFunc()
	if discoveryErr != nil {
		job.State, job.Stage, job.Error, job.FinishedAt = "failed", "Disk discovery failed", discoveryErr.Error(), &now
		_ = s.store.SaveJob(job)
		s.publish("job.state_changed", "warning", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
		s.publish(job.Type+".failed", "critical", &model.ResourceRef{Type: "protection", ID: "protection"}, map[string]any{"jobId": job.ID, "error": job.Error})
		return
	}
	protection := storage.DiscoverProtection(context.Background(), currentDisks, nil, envOr("LUMONAS_SNAPRAID_CONFIG", "/etc/lumonas/snapraid.conf"))
	if protection.Status == model.Critical {
		job.State, job.Stage, job.Error, job.FinishedAt = "failed", "Protected disk missing", "SnapRAID operation blocked because a configured parity or data disk is missing", &now
		_ = s.store.SaveJob(job)
		s.publish("job.state_changed", "warning", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
		s.publish(job.Type+".failed", "critical", &model.ResourceRef{Type: "protection", ID: "protection"}, map[string]any{"jobId": job.ID, "error": job.Error})
		return
	}
	request := privileged.Request{Operation: job.Type, OperationID: job.ID, CorrelationID: job.CorrelationID, PlanHash: job.ID, RequestedState: requested, ExpiresAt: now.Add(30 * time.Minute), Confirmed: true}
	result, err := s.executePrivileged(context.Background(), request)
	if err != nil || !result.OK {
		job.State, job.Stage, job.Error, job.FinishedAt = "failed", "SnapRAID operation failed", "", &now
		if err != nil {
			job.Error = err.Error()
		} else {
			job.Error = result.Error
		}
		_ = s.store.SaveJob(job)
		s.publish("job.state_changed", "warning", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
		s.publish(job.Type+".failed", "critical", &model.ResourceRef{Type: "protection", ID: "protection"}, map[string]any{"jobId": job.ID, "error": job.Error})
		return
	}
	progress = 100
	job.State, job.Stage, job.FinishedAt, job.Progress = "successful", "SnapRAID operation completed", &now, &progress
	if job.Type == "snapraid.sync" {
		_ = s.store.SetMeta("snapraid_last_sync_at", now.Format(time.RFC3339Nano))
	}
	if job.Type == "snapraid.scrub" {
		_ = s.store.SetMeta("snapraid_last_scrub_at", now.Format(time.RFC3339Nano))
	}
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	s.publish(job.Type+".completed", "info", &model.ResourceRef{Type: "protection", ID: "protection"}, map[string]any{"jobId": job.ID})
	if job.Type == "snapraid.fix" {
		// Parity recovery restores the retired slot's content; follow it
		// with a sync so the parity reflects the recovered data again.
		syncJob := model.Job{ID: newID("job"), CorrelationID: job.CorrelationID, Type: "snapraid.sync", Title: "snapraid sync", ResourceID: "protection", State: "queued", CreatedAt: time.Now().UTC()}
		if err := s.admitJob(syncJob); err == nil {
			s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: syncJob.ID}, map[string]any{"job": syncJob})
			go s.runProtectionJob(syncJob)
		}
	}
}

func (s *apiServer) ensureRestartedJobs() {
	jobs, err := s.store.Jobs()
	if err != nil {
		return
	}
	for _, job := range jobs {
		if job.State == "queued" || job.State == "preparing" || job.State == "running" {
			job.State, job.Error = "failed", "daemon restarted before the job completed"
			now := time.Now().UTC()
			job.FinishedAt = &now
			_ = s.store.SaveJob(job)
		}
	}
}

func (s *apiServer) metricsLoop() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		s.publish("system.metrics", "info", nil, metricsData(collector.Metrics()))
	}
}

func (s *apiServer) capacityLoop() {
	interval := 24 * time.Hour
	if value := envOr("LUMONAS_CAPACITY_INTERVAL", ""); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed >= time.Hour {
			interval = parsed
		}
	}
	// Capture once on startup so a new appliance begins building history
	// immediately; the store coalesces repeated captures on the same UTC day.
	s.recordCapacitySnapshots()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		s.recordCapacitySnapshots()
	}
}

func (s *apiServer) recordCapacitySnapshots() {
	disks, err := s.diskFunc()
	if err != nil {
		if s.log != nil {
			s.log.Warn("capacity disk discovery failed", "error", err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, pool := range storage.DiscoverPools(ctx, disks, nil) {
		if pool.MountPath == "" || pool.SizeBytes == 0 || pool.UsedBytes > pool.SizeBytes {
			continue
		}
		if err := s.store.SaveCapacitySnapshot(model.CapacitySnapshot{ResourceID: pool.MountPath, CapturedAt: time.Now().UTC(), TotalBytes: pool.SizeBytes, UsedBytes: pool.UsedBytes}); err != nil && s.log != nil {
			s.log.Warn("capacity snapshot persistence failed", "resource", pool.MountPath, "error", err)
		}
	}
	if err := s.store.PruneCapacitySnapshots(time.Now().UTC().Add(-180 * 24 * time.Hour)); err != nil && s.log != nil {
		s.log.Warn("capacity snapshot retention failed", "error", err)
	}
}

func metricsData(metrics model.SystemMetrics) map[string]any {
	return map[string]any{
		"cpuPercent": metrics.CPUPercent, "load": metrics.Load, "ramUsedBytes": metrics.RAMUsedBytes,
		"ramTotalBytes": metrics.RAMTotalBytes, "cpuTempC": metrics.CPUTempC, "uptimeSeconds": metrics.UptimeSeconds,
		"net": metrics.Net, "netInterfaces": metrics.NetInterfaces, "filesystems": metrics.Filesystems,
	}
}

func (s *apiServer) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	_, _ = w.Write([]byte("retry: 3000\n\n"))
	flusher.Flush()
	if lastID := strings.TrimSpace(r.Header.Get("Last-Event-ID")); lastID != "" {
		if missed, err := s.store.EventsAfter(lastID, 100); err == nil {
			for _, event := range missed {
				s.writeSSEEvent(w, flusher, event)
			}
		} else if s.log != nil {
			s.log.Warn("event replay failed", "lastEventID", lastID, "error", err)
		}
	}
	ch, unsubscribe := s.hub.Subscribe()
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-ch:
			s.writeSSEEvent(w, flusher, event)
		}
	}
}

func (s *apiServer) writeSSEEvent(w http.ResponseWriter, flusher http.Flusher, event model.Event) {
	data, err := events.Encode(event)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "id: %s\ndata: %s\n\n", event.ID, data)
	flusher.Flush()
}

func (s *apiServer) publish(kind, severity string, resource *model.ResourceRef, data map[string]any) {
	event := model.Event{SchemaVersion: events.SchemaVersion, ID: newID("evt"), Type: kind, Timestamp: time.Now().UTC(), Severity: severity, Actor: "system", Generation: s.currentGeneration(), Resource: resource, Data: data}
	event.CorrelationID = eventDataString(data, "correlationId")
	event.OperationID = eventDataString(data, "operationId")
	if event.OperationID == "" {
		event.OperationID = eventDataString(data, "jobId")
	}
	event.PlanHash = eventDataString(data, "planHash")
	if err := s.store.SaveEvent(event); err != nil {
		if s.log != nil {
			s.log.Warn("persist event failed", "error", err)
		}
	}
	if err := s.store.PruneEvents(10000); err != nil {
		if s.log != nil {
			s.log.Warn("prune events failed", "error", err)
		}
	}
	if auditableEvent(kind) {
		entry := store.AuditEntry{Actor: event.Actor, Action: kind, Outcome: "recorded", CorrelationID: event.CorrelationID, OperationID: event.OperationID, PlanHash: event.PlanHash, Generation: event.Generation, Metadata: data}
		if resource != nil {
			entry.ResourceType, entry.ResourceID = resource.Type, resource.ID
		}
		if err := s.store.SaveAudit(entry); err != nil {
			if s.log != nil {
				s.log.Warn("persist audit entry failed", "error", err)
			}
		}
		if err := s.store.PruneAudit(10000); err != nil && s.log != nil {
			s.log.Warn("prune audit entries failed", "error", err)
		}
	}
	if notify.ShouldSend(envOr("LUMONAS_NOTIFY_MIN_SEVERITY", "warning"), severity) {
		body, _ := json.Marshal(map[string]any{"event": kind, "severity": severity, "resource": resource, "data": data})
		message := notify.Message{Title: "LumoNAS " + kind, Body: string(body), Severity: severity}
		go s.sendConfiguredNotifications(kind, severity, message)
	}
	s.hub.Publish(event)
	s.evaluateEventAlert(kind, data)
}

func eventDataString(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	value, _ := data[key].(string)
	return value
}

func (s *apiServer) sendConfiguredNotifications(eventType, severity string, message notify.Message) {
	channels, err := s.store.ListNotificationChannels()
	if err != nil {
		if s.log != nil {
			s.log.Warn("notification channel lookup failed", "error", err)
		}
		return
	}
	key := []byte(s.recoveryKeyString())
	routes := s.notificationRoutes(eventType, severity)
	for _, channel := range channels {
		if !channel.Enabled || channel.Type == "web" || (len(routes) > 0 && !routes[channel.ID] && !routes[channel.Type] && !routes["*"] && !routes["all"]) {
			continue
		}
		deliveryID := newID("notification-delivery")
		if !s.notificationDeliveryAllowed(channel.ID, eventType) {
			s.saveNotificationDelivery(notify.Delivery{ID: deliveryID, ChannelID: channel.ID, EventType: eventType, State: "suppressed", AttemptedAt: time.Now().UTC(), Error: "temporarily suppressed after repeated delivery failures"})
			continue
		}
		_, credentials, credentialErr := s.store.NotificationChannel(channel.ID, key)
		if credentialErr != nil {
			s.saveNotificationDelivery(notify.Delivery{ID: deliveryID, ChannelID: channel.ID, EventType: eventType, State: "failed", AttemptedAt: time.Now().UTC(), Error: "notification credentials unavailable"})
			s.recordNotificationDeliveryFailure(channel.ID, eventType, true)
			if s.log != nil {
				s.log.Warn("notification credentials unavailable", "channel", channel.ID, "error", credentialErr)
			}
			continue
		}
		go func(channel notify.Channel, credentials notify.Credentials, deliveryID string) {
			err := notify.SendWithRetry(context.Background(), 3, func(ctx context.Context) error {
				return notify.SendChannel(ctx, nil, channel, credentials, message)
			})
			state := "sent"
			failure := false
			if err != nil {
				state, failure = "failed", true
			}
			s.saveNotificationDelivery(notify.Delivery{ID: deliveryID, ChannelID: channel.ID, EventType: eventType, State: state, AttemptedAt: time.Now().UTC(), Error: notificationDeliveryError(err)})
			s.recordNotificationDeliveryFailure(channel.ID, eventType, failure)
			if err != nil && s.log != nil {
				s.log.Warn("notification delivery failed", "channel", channel.ID, "event", message.Title, "error", err)
			}
		}(channel, credentials, deliveryID)
	}
	legacy := notify.Sender{Config: notify.Config{WebhookURL: os.Getenv("LUMONAS_NOTIFY_WEBHOOK_URL"), NtfyURL: os.Getenv("LUMONAS_NOTIFY_NTFY_URL")}, UserAgent: "LumoNAS/" + s.version}
	if legacy.Config.WebhookURL != "" || legacy.Config.NtfyURL != "" {
		if err := notify.SendWithRetry(context.Background(), 3, func(ctx context.Context) error { return legacy.Send(ctx, message) }); err != nil && s.log != nil {
			s.log.Warn("legacy notification delivery failed", "event", message.Title, "error", err)
		}
	}
}

func (s *apiServer) saveNotificationDelivery(delivery notify.Delivery) {
	if err := s.store.SaveNotificationDelivery(delivery); err != nil && s.log != nil {
		s.log.Warn("notification delivery record failed", "delivery", delivery.ID, "error", err)
	}
}

func notificationDeliveryError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (s *apiServer) notificationRoutes(eventType, severity string) map[string]bool {
	rules, err := s.store.AlertRules()
	if err != nil {
		return nil
	}
	routes := make(map[string]bool)
	category := notificationCategory(eventType)
	for _, rule := range rules {
		if !rule.Enabled || !notify.ShouldSend(rule.Severity, severity) {
			continue
		}
		if required := ruleCategory(rule); required != "" && required != category {
			continue
		}
		for _, route := range rule.Routes {
			routes[route] = true
		}
	}
	return routes
}

func notificationCategory(eventType string) string {
	switch {
	case strings.HasPrefix(eventType, "docker."):
		return "docker"
	case strings.HasPrefix(eventType, "recovery."), strings.HasPrefix(eventType, "backup."):
		return "backup"
	case strings.HasPrefix(eventType, "disk."), strings.HasPrefix(eventType, "storage."), strings.HasPrefix(eventType, "snapraid."):
		return "storage"
	case strings.HasPrefix(eventType, "network."):
		return "network"
	case strings.HasPrefix(eventType, "auth."), strings.HasPrefix(eventType, "security."):
		return "security"
	case strings.HasPrefix(eventType, "ups."), strings.HasPrefix(eventType, "power."):
		return "power"
	default:
		return "system"
	}
}

func ruleCategory(rule monitoring.AlertRule) string {
	text := strings.ToLower(rule.Name + " " + rule.Condition)
	for _, category := range []string{"docker", "container", "backup", "recovery", "network", "security", "login", "ups", "power", "storage", "disk", "smart", "pool", "snapraid", "temperature"} {
		if strings.Contains(text, category) {
			switch category {
			case "container":
				return "docker"
			case "recovery", "backup":
				return "backup"
			case "login", "security":
				return "security"
			case "ups", "power":
				return "power"
			default:
				return "storage"
			}
		}
	}
	return ""
}

func auditableEvent(kind string) bool {
	switch kind {
	case "system.metrics", "docker.log.line", "docker.container.metrics", "job.progress":
		return false
	default:
		return true
	}
}

func (s *apiServer) requestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observed := httpobs.Wrap(w)
		started := time.Now()
		defer func() {
			if s.log != nil {
				s.log.Info("http request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", observed.Status(),
					"bytes", observed.Bytes(),
					"duration_ms", time.Since(started).Seconds()*1000,
					"correlation_id", trace.CorrelationID(r.Context()),
				)
			}
		}()
		w = observed
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if r.TLS != nil {
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		correlationID := trace.NewCorrelationID()
		w.Header().Set("X-Request-ID", correlationID)
		r = r.WithContext(trace.WithCorrelationID(r.Context(), correlationID))
		origin := r.Header.Get("Origin")
		// Same-origin requests never carry CORS headers; only explicitly
		// allowlisted origins get cross-origin access. Reflecting an
		// arbitrary Origin with credentials enabled would defeat the point
		// of the allowlist.
		if origin != "" && len(s.corsOrigins) > 0 {
			for _, allowed := range s.corsOrigins {
				if origin == allowed {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Set("Vary", "Origin")
					w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
					w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-CSRF-Token, Authorization")
					w.Header().Set("Access-Control-Allow-Credentials", "true")
					break
				}
			}
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodPatch {
			limit := int64(32 << 20)
			if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
				limit = 2 << 30
			}
			if r.ContentLength > limit {
				writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body too large"})
				return
			}
			// Do not parse multipart data before authentication. The endpoint
			// handler chooses its own ParseMultipartForm memory budget while
			// this reader enforces the total on-wire request size.
			r.Body = http.MaxBytesReader(w, r.Body, limit)
		}
		next.ServeHTTP(w, r)
	})
}

func requestCorrelationID(r *http.Request) string {
	if id := trace.CorrelationID(r.Context()); id != "" {
		return id
	}
	return trace.NewCorrelationID()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status == http.StatusNoContent || status == http.StatusNotModified {
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}
func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
func parseCORSOrigins() []string {
	raw := os.Getenv("LUMONAS_CORS_ORIGINS")
	if raw == "" {
		return nil
	}
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}
func newID(prefix string) string {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(buf[:])
}
func ensureNASUUID(db *store.Store) string {
	if value, ok := db.Meta("nas_uuid"); ok {
		return value
	}
	value := newID("nas")
	_ = db.SetMeta("nas_uuid", value)
	return value
}
func primaryIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "127.0.0.1"
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			host, _, err := net.ParseCIDR(addr.String())
			if err == nil && host.To4() != nil {
				return host.String()
			}
		}
	}
	return "127.0.0.1"
}
