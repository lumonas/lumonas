package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
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

	"github.com/lumonas/lumonas/internal/collector"
	dockerruntime "github.com/lumonas/lumonas/internal/docker"
	"github.com/lumonas/lumonas/internal/events"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/network"
	"github.com/lumonas/lumonas/internal/notify"
	"github.com/lumonas/lumonas/internal/power"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/recovery"
	"github.com/lumonas/lumonas/internal/services"
	"github.com/lumonas/lumonas/internal/shares"
	"github.com/lumonas/lumonas/internal/storage"
	"github.com/lumonas/lumonas/internal/store"
)

type apiServer struct {
	store         *store.Store
	hub           *events.Hub
	log           *slog.Logger
	jobsMu        sync.Mutex
	version       string
	diskFunc      func() ([]model.Disk, error)
	authRequired  bool
	dockerService dockerruntime.Service
	catalogFile   string
	alertMu       sync.Mutex
	acknowledged  map[string]bool
}

var version = "0.1.0-dev"

func main() {
	listen := flag.String("listen", envOr("MYNASD_LISTEN", "127.0.0.1:8080"), "HTTP listen address")
	dbPath := flag.String("db", envOr("MYNAS_DB_PATH", "/var/lib/mynas/mynas.db"), "SQLite database path")
	versionFlag := flag.String("version", envOr("MYNAS_VERSION", version), "service version")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	db, err := store.Open(*dbPath)
	if err != nil {
		logger.Error("database initialization failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	nasUUID := ensureNASUUID(db)
	if password := os.Getenv("MYNAS_ADMIN_PASSWORD"); password != "" {
		if err := db.EnsureAdmin("admin", password); err != nil {
			logger.Error("admin bootstrap failed", "error", err)
			os.Exit(1)
		}
	}
	server := &apiServer{store: db, hub: events.NewHub(), log: logger, version: *versionFlag, diskFunc: func() ([]model.Disk, error) { return collector.Disks(nil) }, authRequired: os.Getenv("MYNAS_AUTH_REQUIRED") == "true", acknowledged: make(map[string]bool)}
	server.dockerService = dockerruntime.New(envOr("MYNAS_STACK_ROOT", "/srv/mynas/docker/stacks"), nil)
	server.catalogFile = envOr("MYNAS_CATALOG_FILE", "/usr/share/lumonas/catalog/apps.json")
	server.ensureRestartedJobs()
	go server.metricsLoop()

	httpServer := &http.Server{Addr: *listen, Handler: server.routes(), ReadHeaderTimeout: 5 * time.Second}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		logger.Info("mynasd started", "listen", *listen, "db", *dbPath, "nas_uuid", nasUUID)
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

func (s *apiServer) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/readyz", s.readyz)
	mux.HandleFunc("/api/v1/", s.api)
	return requestMiddleware(s.authMiddleware(mux))
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
	case r.Method == http.MethodPost && endpoint == "/auth/logout":
		s.authLogout(w, r)
	case r.Method == http.MethodGet && endpoint == "/server":
		s.serverInfo(w)
	case r.Method == http.MethodGet && endpoint == "/disks":
		s.disks(w)
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/disks/"):
		s.disk(w, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/pools":
		s.pools(w, r)
	case r.Method == http.MethodGet && endpoint == "/storage/protection":
		s.protection(w)
	case r.Method == http.MethodGet && endpoint == "/storage/safety":
		writeJSON(w, http.StatusOK, map[string]any{"state": "locked", "unlockedUntil": nil})
	case r.Method == http.MethodPost && endpoint == "/storage/operations/plan":
		s.planStorageOperation(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/storage/operations/") && strings.HasSuffix(endpoint, "/confirm"):
		s.confirmStorageOperation(w, r, path.Base(path.Dir(endpoint)))
	case r.Method == http.MethodGet && endpoint == "/recovery/status":
		s.recoveryStatus(w)
	case r.Method == http.MethodGet && endpoint == "/recovery/plan":
		s.recoveryPlan(w)
	case r.Method == http.MethodPost && endpoint == "/recovery/export":
		s.recoveryExport(w)
	case r.Method == http.MethodGet && endpoint == "/jobs":
		s.listJobs(w)
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/jobs/"):
		s.job(w, path.Base(endpoint))
	case r.Method == http.MethodPost && endpoint == "/jobs":
		s.createJob(w, r)
	case r.Method == http.MethodGet && endpoint == "/alerts":
		s.alerts(w)
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/alerts/"):
		s.ackAlert(w, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/activity":
		s.activity(w)
	case r.Method == http.MethodGet && endpoint == "/audit":
		s.audit(w, r)
	case r.Method == http.MethodPost && endpoint == "/notifications/test":
		s.notificationTest(w, r)
	case r.Method == http.MethodGet && endpoint == "/system/metrics":
		s.metrics(w)
	case r.Method == http.MethodGet && endpoint == "/network/interfaces":
		s.networkInterfaces(w)
	case r.Method == http.MethodPost && endpoint == "/network/checkpoints":
		s.networkCheckpoint(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/network/checkpoints/"):
		s.networkCheckpointAction(w, r, endpoint)
	case r.Method == http.MethodGet && endpoint == "/services":
		s.services(w, r)
	case r.Method == http.MethodGet && endpoint == "/power/ups":
		s.ups(w, r)
	case r.Method == http.MethodPost && endpoint == "/power/action":
		s.powerAction(w, r)
	case r.Method == http.MethodGet && endpoint == "/shares":
		s.listShares(w)
	case r.Method == http.MethodPost && endpoint == "/shares":
		s.createShare(w, r)
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
	case r.Method == http.MethodPost && strings.HasPrefix(endpoint, "/docker/images/"):
		s.dockerImageAction(w, r, endpoint)
	case r.Method == http.MethodGet && endpoint == "/docker/volumes":
		s.dockerVolumes(w, r)
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/docker/logs/"):
		s.dockerLogs(w, r, path.Base(endpoint))
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "endpoint not found"})
	}
}

func (s *apiServer) authStatus(w http.ResponseWriter, r *http.Request) {
	authenticated := false
	if cookie, err := r.Cookie("mynas_session"); err == nil {
		_, authenticated = s.store.SessionUser(cookie.Value)
	}
	writeJSON(w, http.StatusOK, map[string]any{"required": s.authRequired, "configured": s.store.HasUsers(), "authenticated": authenticated})
}

func (s *apiServer) authLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	token, expires, err := s.store.CreateSession(input.Username, input.Password, 12*time.Hour)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "mynas_session", Value: token, Path: "/", Expires: expires, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: os.Getenv("MYNAS_COOKIE_SECURE") == "true"})
	writeJSON(w, http.StatusOK, map[string]any{"username": input.Username, "expiresAt": expires})
}

func (s *apiServer) authLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("mynas_session"); err == nil {
		_ = s.store.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "mynas_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged_out"})
}

func (s *apiServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.authRequired || r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/api/v1/auth/status" || r.URL.Path == "/api/v1/auth/login" || r.URL.Path == "/api/v1/auth/logout" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie("mynas_session")
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
			return
		}
		if _, ok := s.store.SessionUser(cookie.Value); !ok {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired session"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *apiServer) serverInfo(w http.ResponseWriter) {
	uuid, _ := s.store.Meta("nas_uuid")
	writeJSON(w, http.StatusOK, model.ServerInfo{ID: "server-1", Name: collector.Hostname(), Hostname: collector.Hostname(), Version: s.version, NASUUID: uuid, Timezone: time.Now().Location().String(), Health: model.Healthy, IP: primaryIP()})
}

func (s *apiServer) disks(w http.ResponseWriter) {
	disks, err := s.diskFunc()
	if err != nil {
		s.log.Warn("disk discovery unavailable", "error", err)
		disks = []model.Disk{}
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
	configPath := envOr("MYNAS_SNAPRAID_CONFIG", "/etc/mynas/snapraid.conf")
	writeJSON(w, http.StatusOK, storage.DiscoverProtection(ctx, disks, nil, configPath))
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
	if input.RequestedState != nil {
		plan.RequestedState = input.RequestedState
		plan.PlanHash = storage.Hash(plan)
	}
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
	if !input.Reauthenticated || !input.StorageSafetyUnlocked {
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
	expectedIdentity := map[string]string{"id": plan.Target.DiskID, "wwn": plan.Target.WWN, "serial": plan.Target.Serial, "model": plan.Target.Model, "filesystemUuid": plan.Target.FilesystemUUID, "sizeBytes": strconv.FormatUint(plan.Target.SizeBytes, 10)}
	request := privileged.Request{Operation: string(plan.Action), PlanHash: plan.PlanHash, TargetDiskID: plan.Target.DiskID, ExpectedIdentity: expectedIdentity, ExpectedState: map[string]string{"currentPath": plan.ExpectedState.CurrentPath, "mounted": strconv.FormatBool(plan.ExpectedState.Mounted), "role": plan.ExpectedState.Role, "poolId": plan.ExpectedState.PoolID}, RequestedState: plan.RequestedState, ExpiresAt: plan.ExpiresAt, Confirmed: true}
	broker := privileged.Client{Socket: envOr("MYNAS_PRIVD_SOCKET", "/run/mynas/privd.sock")}
	result, err := broker.Execute(r.Context(), request)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	plan.Status = "executed"
	_ = s.store.SavePlan(plan)
	s.advanceGeneration("storage." + string(plan.Action))
	s.publish("storage.operation.completed", "warning", &model.ResourceRef{Type: "disk", ID: plan.Target.DiskID}, map[string]any{"operationId": plan.OperationID, "action": plan.Action, "planHash": plan.PlanHash})
	writeJSON(w, http.StatusOK, result)
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
	}
}

func (s *apiServer) recoveryStatus(w http.ResponseWriter) {
	key := os.Getenv("MYNAS_RECOVERY_KEY")
	directory := envOr("MYNAS_RECOVERY_DIR", "/var/lib/mynas/recovery")
	bundlePath := filepath.Join(directory, "latest.mrb")
	status := map[string]any{"configured": key != "", "latestPath": bundlePath, "verified": false}
	if key != "" {
		if bundle, err := os.ReadFile(bundlePath); err == nil {
			if manifest, verifyErr := recovery.Verify(bundle, []byte(key)); verifyErr == nil {
				status["verified"] = true
				status["manifest"] = manifest
			}
		}
	}
	writeJSON(w, http.StatusOK, status)
}

func (s *apiServer) recoveryExport(w http.ResponseWriter) {
	key := os.Getenv("MYNAS_RECOVERY_KEY")
	if key == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MYNAS_RECOVERY_KEY is not configured"})
		return
	}
	database, err := s.store.BackupBytes()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "database backup failed: " + err.Error()})
		return
	}
	disks, _ := s.diskFunc()
	diskIDs := make([]string, 0, len(disks))
	for _, disk := range disks {
		diskIDs = append(diskIDs, disk.ID)
	}
	nasUUID, _ := s.store.Meta("nas_uuid")
	desired, _ := json.Marshal(map[string]any{"nasUuid": nasUUID, "configGeneration": s.currentGeneration(), "createdAt": time.Now().UTC()})
	compose := map[string][]byte{}
	stacks, _ := s.dockerService.Stacks(context.Background())
	for _, stack := range stacks {
		compose[stack.Name+"/compose.yaml"] = []byte(stack.ComposeYAML)
	}
	var encrypted []byte
	if secretPath := os.Getenv("MYNAS_RECOVERY_SECRETS_FILE"); secretPath != "" {
		encrypted, err = os.ReadFile(secretPath)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "recovery secret input failed: " + err.Error()})
			return
		}
	}
	bundle, err := recovery.Create(recovery.Input{Manifest: recovery.Manifest{ConfigSchema: 1, MyNASVersion: s.version, NASUUID: nasUUID, Generation: s.currentGeneration(), DiskIDs: diskIDs}, DesiredState: desired, Database: database, Compose: compose, Files: s.recoveryFiles(), EncryptedData: encrypted}, []byte(key))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "bundle creation failed: " + err.Error()})
		return
	}
	directory := envOr("MYNAS_RECOVERY_DIR", "/var/lib/mynas/recovery")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	temporary, err := os.CreateTemp(directory, ".latest-*.mrb")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	temporaryPath := temporary.Name()
	cleanup := func() { temporary.Close(); _ = os.Remove(temporaryPath) }
	if _, err := temporary.Write(bundle); err != nil {
		cleanup()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := temporary.Chmod(0o600); err != nil {
		cleanup()
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryPath)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if err := os.Rename(temporaryPath, filepath.Join(directory, "latest.mrb")); err != nil {
		_ = os.Remove(temporaryPath)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	manifest, _ := recovery.Verify(bundle, []byte(key))
	versioned := filepath.Join(directory, fmt.Sprintf("generation-%d-%s.mrb", manifest.Generation, time.Now().UTC().Format("20060102T150405Z")))
	if err := os.WriteFile(versioned, bundle, 0o600); err != nil {
		if s.log != nil {
			s.log.Warn("versioned recovery copy failed", "error", err)
		}
	}
	s.pruneRecoveryBundles(directory, 20)
	s.publish("recovery.bundle.created", "info", nil, map[string]any{"generation": manifest.Generation})
	writeJSON(w, http.StatusCreated, map[string]any{"path": filepath.Join(directory, "latest.mrb"), "manifest": manifest, "verified": true})
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

func (s *apiServer) recoveryFiles() map[string][]byte {
	files := make(map[string][]byte)
	for name, path := range map[string]string{
		"config/shares.json":    envOr("MYNAS_SHARES_FILE", "/var/lib/mynas/shares.json"),
		"storage/snapraid.conf": envOr("MYNAS_SNAPRAID_CONFIG", "/etc/mynas/snapraid.conf"),
	} {
		if data, err := os.ReadFile(path); err == nil {
			files[name] = data
		}
	}
	return files
}

func (s *apiServer) recoveryPlan(w http.ResponseWriter) {
	key := os.Getenv("MYNAS_RECOVERY_KEY")
	if key == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "MYNAS_RECOVERY_KEY is not configured"})
		return
	}
	bundlePath := filepath.Join(envOr("MYNAS_RECOVERY_DIR", "/var/lib/mynas/recovery"), "latest.mrb")
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
	writeJSON(w, http.StatusOK, plan)
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

func (s *apiServer) networkCheckpoint(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ConnectionUUID  string            `json:"connectionUuid"`
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
	result, err := (privileged.Client{Socket: envOr("MYNAS_PRIVD_SOCKET", "/run/mynas/privd.sock")}).Execute(r.Context(), privileged.Request{Operation: "network.checkpoint.begin", OperationID: operationID, PlanHash: operationID, RequestedState: requested, ExpiresAt: time.Now().UTC().Add(time.Duration(input.TimeoutSeconds+60) * time.Second), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": result.Error})
		return
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
	result, err := (privileged.Client{Socket: envOr("MYNAS_PRIVD_SOCKET", "/run/mynas/privd.sock")}).Execute(r.Context(), privileged.Request{Operation: "network.checkpoint." + parts[3], OperationID: operationID, PlanHash: operationID, Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	if parts[3] == "commit" {
		s.advanceGeneration("network.checkpoint.commit")
	}
	s.publish("network.checkpoint."+parts[3], "info", &model.ResourceRef{Type: "network-checkpoint", ID: operationID}, nil)
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
	for _, name := range strings.Split(os.Getenv("MYNAS_UPS_NAMES"), ",") {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			names = append(names, trimmed)
		}
	}
	writeJSON(w, http.StatusOK, power.Discover(ctx, names, nil))
}

func (s *apiServer) powerAction(w http.ResponseWriter, r *http.Request) {
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
	result, err := (privileged.Client{Socket: envOr("MYNAS_PRIVD_SOCKET", "/run/mynas/privd.sock")}).Execute(r.Context(), privileged.Request{Operation: "power.action", OperationID: operationID, PlanHash: operationID, RequestedState: map[string]any{"action": input.Action}, ExpiresAt: time.Now().UTC().Add(2 * time.Minute), Confirmed: true})
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	if !result.OK {
		writeJSON(w, http.StatusConflict, map[string]string{"error": result.Error})
		return
	}
	s.publish("power.action", "critical", nil, map[string]any{"operationId": operationID, "action": input.Action})
	writeJSON(w, http.StatusAccepted, result)
}

func (s *apiServer) shareStore() shares.Store {
	return shares.Store{Path: envOr("MYNAS_SHARES_FILE", "/var/lib/mynas/shares.json")}
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
	configPath := envOr("MYNAS_SAMBA_CONFIG", "/var/lib/mynas/generated/smb.conf")
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

func (s *apiServer) alerts(w http.ResponseWriter) {
	disks, err := s.diskFunc()
	if err != nil {
		writeJSON(w, http.StatusOK, []model.Alert{})
		return
	}
	alerts := make([]model.Alert, 0)
	now := time.Now().UTC()
	for _, disk := range disks {
		if disk.Health != model.Warning && disk.Health != model.Critical {
			continue
		}
		severity := "warning"
		if disk.Health == model.Critical {
			severity = "critical"
		}
		alert := model.Alert{ID: "disk-health-" + disk.ID, Severity: severity, Title: "Disk health requires attention", Description: fmt.Sprintf("%s (%s) reported %s health", disk.Name, disk.Model, disk.Health), Resource: &model.ResourceRef{Type: "disk", ID: disk.ID}, State: "firing", StartedAt: now}
		s.alertMu.Lock()
		if s.acknowledged[alert.ID] {
			alert.State = "acknowledged"
		}
		s.alertMu.Unlock()
		alerts = append(alerts, alert)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, unit := range power.Discover(ctx, nil, nil) {
		if !unit.OnBattery {
			continue
		}
		alert := model.Alert{ID: "ups-on-battery-" + unit.Name, Severity: "critical", Title: "UPS is on battery", Description: fmt.Sprintf("UPS %s reports status %s", unit.Name, unit.Status), Resource: &model.ResourceRef{Type: "ups", ID: unit.Name}, State: "firing", StartedAt: now}
		s.alertMu.Lock()
		if s.acknowledged[alert.ID] {
			alert.State = "acknowledged"
		}
		s.alertMu.Unlock()
		alerts = append(alerts, alert)
	}
	writeJSON(w, http.StatusOK, alerts)
}

func (s *apiServer) ackAlert(w http.ResponseWriter, id string) {
	disks, _ := s.diskFunc()
	for _, disk := range disks {
		if "disk-health-"+disk.ID != id {
			continue
		}
		if disk.Health != model.Warning && disk.Health != model.Critical {
			break
		}
		s.alertMu.Lock()
		s.acknowledged[id] = true
		s.alertMu.Unlock()
		severity := "warning"
		if disk.Health == model.Critical {
			severity = "critical"
		}
		writeJSON(w, http.StatusOK, model.Alert{ID: id, Severity: severity, Title: "Disk health requires attention", Description: fmt.Sprintf("%s (%s) reported %s health", disk.Name, disk.Model, disk.Health), Resource: &model.ResourceRef{Type: "disk", ID: disk.ID}, State: "acknowledged", StartedAt: time.Now().UTC()})
		return
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
	var input notify.Message
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	sender := notify.Sender{Config: notify.Config{WebhookURL: os.Getenv("MYNAS_NOTIFY_WEBHOOK_URL"), NtfyURL: os.Getenv("MYNAS_NOTIFY_NTFY_URL"), MinSeverity: envOr("MYNAS_NOTIFY_MIN_SEVERITY", "warning")}, UserAgent: "LumoNAS/" + s.version}
	if err := sender.Send(r.Context(), input); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	_ = s.store.SaveAudit(store.AuditEntry{Actor: "admin", Action: "notification.test", Outcome: "sent", Metadata: map[string]any{"severity": input.Severity}})
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
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	stacks, _ := s.dockerService.Stacks(ctx)
	containers, _ := s.dockerService.Containers(ctx)
	images, _ := s.dockerService.Images(ctx)
	running := 0
	for _, container := range containers {
		if container.State == "running" || container.State == "restarting" {
			running++
		}
	}
	_ = images
	writeJSON(w, http.StatusOK, map[string]int{"stacks": len(stacks), "appsRunning": running, "updatesAvailable": 0})
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
	stacks, err := s.dockerService.Stacks(ctx)
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stacks)
}

func (s *apiServer) createDockerStack(w http.ResponseWriter, r *http.Request) {
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
	s.publish("docker.stack.created", "info", &model.ResourceRef{Type: "stack", ID: stack.ID}, map[string]any{"stackId": stack.ID})
	writeJSON(w, http.StatusCreated, stack)
}

func (s *apiServer) dockerStack(w http.ResponseWriter, r *http.Request, id string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	stacks, err := s.dockerService.Stacks(ctx)
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
		if input.ComposeYAML != "" {
			updated, updateErr := s.dockerService.UpdateCompose(stack.Name, input.ComposeYAML)
			if updateErr != nil {
				writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": updateErr.Error()})
				return
			}
			stack = updated
			s.advanceGeneration("docker.stack.compose.update")
		}
		if err := s.dockerService.Action(ctx, stack, action); err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
			return
		}
		s.publish("docker.stack.action", "info", &model.ResourceRef{Type: "stack", ID: stack.ID}, map[string]any{"stackId": stack.ID, "action": action})
		writeJSON(w, http.StatusAccepted, stack)
		return
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "stack not found"})
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

func (s *apiServer) dockerContainerAction(w http.ResponseWriter, r *http.Request, endpoint string) {
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
		job := model.Job{ID: newID("job"), Type: input.Type, Title: strings.ReplaceAll(input.Type, ".", " "), ResourceID: "protection", State: "queued", CreatedAt: time.Now().UTC()}
		if err := s.store.SaveJob(job); err != nil {
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
	job := model.Job{ID: newID("job"), Type: input.Type, Title: "SMART " + strings.TrimPrefix(input.Type, "smart.") + " validation", ResourceID: input.ResourceID, State: "queued", CreatedAt: time.Now().UTC()}
	if err := s.store.SaveJob(job); err != nil {
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
	requested := map[string]any{"configPath": envOr("MYNAS_SNAPRAID_CONFIG", "/etc/mynas/snapraid.conf")}
	if job.Type == "snapraid.scrub" {
		requested["scrubPercent"] = envOr("MYNAS_SNAPRAID_SCRUB_PERCENT", "5")
	}
	request := privileged.Request{Operation: job.Type, PlanHash: job.ID, RequestedState: requested, ExpiresAt: now.Add(30 * time.Minute), Confirmed: true}
	result, err := (privileged.Client{Socket: envOr("MYNAS_PRIVD_SOCKET", "/run/mynas/privd.sock")}).Execute(context.Background(), request)
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
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	s.publish(job.Type+".completed", "info", &model.ResourceRef{Type: "protection", ID: "protection"}, map[string]any{"jobId": job.ID})
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

func metricsData(metrics model.SystemMetrics) map[string]any {
	return map[string]any{
		"cpuPercent": metrics.CPUPercent, "load": metrics.Load, "ramUsedBytes": metrics.RAMUsedBytes,
		"ramTotalBytes": metrics.RAMTotalBytes, "cpuTempC": metrics.CPUTempC, "uptimeSeconds": metrics.UptimeSeconds,
		"net": metrics.Net,
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
	ch, unsubscribe := s.hub.Subscribe()
	defer unsubscribe()
	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-ch:
			data, err := events.Encode(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *apiServer) publish(kind, severity string, resource *model.ResourceRef, data map[string]any) {
	event := model.Event{ID: newID("evt"), Type: kind, Timestamp: time.Now().UTC(), Severity: severity, Resource: resource, Data: data}
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
	entry := store.AuditEntry{Actor: "system", Action: kind, Outcome: "recorded", Metadata: data}
	if resource != nil {
		entry.ResourceType, entry.ResourceID = resource.Type, resource.ID
	}
	if err := s.store.SaveAudit(entry); err != nil {
		if s.log != nil {
			s.log.Warn("persist audit entry failed", "error", err)
		}
	}
	if notify.ShouldSend(envOr("MYNAS_NOTIFY_MIN_SEVERITY", "warning"), severity) && (os.Getenv("MYNAS_NOTIFY_WEBHOOK_URL") != "" || os.Getenv("MYNAS_NOTIFY_NTFY_URL") != "") {
		body, _ := json.Marshal(map[string]any{"event": kind, "severity": severity, "resource": resource, "data": data})
		go func() {
			sender := notify.Sender{Config: notify.Config{WebhookURL: os.Getenv("MYNAS_NOTIFY_WEBHOOK_URL"), NtfyURL: os.Getenv("MYNAS_NOTIFY_NTFY_URL")}, UserAgent: "LumoNAS/" + s.version}
			if err := sender.Send(context.Background(), notify.Message{Title: "LumoNAS " + kind, Body: string(body), Severity: severity}); err != nil && s.log != nil {
				s.log.Warn("notification delivery failed", "event", kind, "error", err)
			}
		}()
	}
	s.hub.Publish(event)
}

func requestMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Request-ID", newID("req"))
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
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
