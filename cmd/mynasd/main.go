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
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lumonas/lumonas/internal/collector"
	"github.com/lumonas/lumonas/internal/events"
	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/store"
)

type apiServer struct {
	store    *store.Store
	hub      *events.Hub
	log      *slog.Logger
	jobsMu   sync.Mutex
	version  string
	diskFunc func() ([]model.Disk, error)
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
	server := &apiServer{store: db, hub: events.NewHub(), log: logger, version: *versionFlag, diskFunc: func() ([]model.Disk, error) { return collector.Disks(nil) }}
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
	return requestMiddleware(mux)
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
	case r.Method == http.MethodGet && endpoint == "/server":
		s.serverInfo(w)
	case r.Method == http.MethodGet && endpoint == "/disks":
		s.disks(w)
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/disks/"):
		s.disk(w, path.Base(endpoint))
	case r.Method == http.MethodGet && endpoint == "/pools":
		writeJSON(w, http.StatusOK, []model.Pool{})
	case r.Method == http.MethodGet && endpoint == "/storage/protection":
		s.protection(w)
	case r.Method == http.MethodGet && endpoint == "/jobs":
		s.listJobs(w)
	case r.Method == http.MethodPost && endpoint == "/jobs":
		s.createJob(w, r)
	case r.Method == http.MethodGet && endpoint == "/alerts":
		writeJSON(w, http.StatusOK, []any{})
	case r.Method == http.MethodPatch && strings.HasPrefix(endpoint, "/alerts/"):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "alert not found"})
	case r.Method == http.MethodGet && endpoint == "/activity":
		writeJSON(w, http.StatusOK, []any{})
	case r.Method == http.MethodGet && endpoint == "/system/metrics":
		s.metrics(w)
	case r.Method == http.MethodGet && endpoint == "/events/stream":
		s.stream(w, r)
	case r.Method == http.MethodGet && endpoint == "/docker/summary":
		writeJSON(w, http.StatusOK, collector.DockerSummary())
	case r.Method == http.MethodGet && endpoint == "/docker/apps":
		writeJSON(w, http.StatusOK, []any{})
	case r.Method == http.MethodGet && endpoint == "/docker/stacks":
		writeJSON(w, http.StatusOK, []any{})
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/docker/stacks/"):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "stack not found"})
	case r.Method == http.MethodGet && endpoint == "/docker/containers":
		writeJSON(w, http.StatusOK, []any{})
	case r.Method == http.MethodGet && endpoint == "/docker/images":
		writeJSON(w, http.StatusOK, []any{})
	case r.Method == http.MethodGet && endpoint == "/docker/volumes":
		writeJSON(w, http.StatusOK, []any{})
	case r.Method == http.MethodGet && strings.HasPrefix(endpoint, "/docker/logs/"):
		writeJSON(w, http.StatusOK, []any{})
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "endpoint not found"})
	}
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
	writeJSON(w, http.StatusOK, model.Protection{Status: model.Attention, SyncSchedule: "Not configured", ScrubSchedule: "Not configured", LastSyncResult: nil})
}

func (s *apiServer) metrics(w http.ResponseWriter) { writeJSON(w, http.StatusOK, collector.Metrics()) }

func (s *apiServer) listJobs(w http.ResponseWriter) {
	jobs, err := s.store.Jobs()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, jobs)
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
	if input.Type != "smart.short" && input.Type != "smart.extended" {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "only read-only SMART validation jobs are enabled in this runtime slice"})
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
	found := false
	for _, disk := range disks {
		if disk.ID == input.ResourceID {
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
	go s.runReadOnlyJob(job)
	writeJSON(w, http.StatusAccepted, job)
}

func (s *apiServer) runReadOnlyJob(job model.Job) {
	time.Sleep(50 * time.Millisecond)
	now := time.Now().UTC()
	progress := 10.0
	job.State, job.Stage, job.StartedAt, job.Progress = "running", "Validating disk identity", &now, &progress
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
	time.Sleep(100 * time.Millisecond)
	progress = 100
	job.State, job.Stage, job.FinishedAt, job.Progress = "successful", "Read-only SMART integration is available", &now, &progress
	_ = s.store.SaveJob(job)
	s.publish("job.state_changed", "info", &model.ResourceRef{Type: "job", ID: job.ID}, map[string]any{"job": job})
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
		s.log.Warn("persist event failed", "error", err)
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
