package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	"github.com/lumonas/lumonas/internal/store"
)

const replicationMaxBundleBytes = int64(4 << 30)

var replicationProbeClient = func() *http.Client {
	return &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
}

func validateReplicationURL(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("replication peer must use an HTTPS URL without embedded credentials")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (s *apiServer) replicationPeers(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, false); !ok {
		return
	}
	peers, err := s.store.ReplicationPeers()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	var wait sync.WaitGroup
	semaphore := make(chan struct{}, 4)
	statusTokens := make([]string, len(peers))
	for index, peer := range peers {
		_, encrypted, credentialErr := s.store.ReplicationPeerCredentials(peer.ID)
		if credentialErr != nil {
			continue
		}
		credentials, decryptErr := backup.DecryptCredentials(encrypted, []byte(s.recoveryKeyString()))
		if decryptErr == nil {
			statusTokens[index] = credentials.HealthToken
		}
	}
	for index := range peers {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			status, version, health, checkedAt := probeReplicationPeerDetails(ctx, peers[index].URL, statusTokens[index])
			peers[index].RemoteStatus, peers[index].RemoteVersion, peers[index].RemoteHealth, peers[index].RemoteCheckedAt = status, version, health, checkedAt
		}(index)
	}
	wait.Wait()
	writeJSON(w, http.StatusOK, peers)
}

func probeReplicationPeer(ctx context.Context, peerURL string) bool {
	status, _, _, _ := probeReplicationPeerDetails(ctx, peerURL, "")
	return status == "online"
}

func probeReplicationPeerDetails(ctx context.Context, peerURL, statusToken string) (string, string, string, *time.Time) {
	base, err := validateReplicationURL(peerURL)
	if err != nil {
		return "offline", "", "", nil
	}
	endpoint := base + "/healthz"
	if statusToken != "" {
		endpoint = base + "/api/v1/fleet/status"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "offline", "", "", nil
	}
	if statusToken != "" {
		request.Header.Set("Authorization", "Bearer "+statusToken)
	}
	response, err := replicationProbeClient().Do(request)
	if err != nil {
		return "offline", "", "", nil
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "offline", "", "", nil
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, 2049))
	if err != nil || len(payload) > 2048 {
		return "offline", "", "", nil
	}
	if statusToken == "" {
		if !strings.Contains(string(payload), `"status":"ok"`) {
			return "offline", "", "", nil
		}
		checkedAt := time.Now().UTC()
		return "online", "", "", &checkedAt
	}
	var status struct {
		Status  string `json:"status"`
		Version string `json:"version"`
		Health  string `json:"health"`
	}
	if json.Unmarshal(payload, &status) != nil || status.Status != "ok" || status.Version == "" {
		return "offline", "", "", nil
	}
	checkedAt := time.Now().UTC()
	return "online", status.Version, status.Health, &checkedAt
}
func (s *apiServer) saveReplicationPeer(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	key := s.recoveryKeyString()
	if key == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "recovery key is required before pairing a replication peer"})
		return
	}
	var input struct {
		Name        string `json:"name"`
		URL         string `json:"url"`
		Token       string `json:"token"`
		StatusToken string `json:"statusToken"`
	}
	if json.NewDecoder(r.Body).Decode(&input) != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}
	peerURL, err := validateReplicationURL(input.URL)
	if err != nil || strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Token) == "" || len(input.StatusToken) > 4096 {
		if err == nil {
			err = fmt.Errorf("peer name and receive token are required")
		}
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	peers, listErr := s.store.ReplicationPeers()
	if listErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": listErr.Error()})
		return
	}
	if len(peers) >= 16 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "up to 16 replication peers can be paired"})
		return
	}
	encrypted, err := backup.EncryptCredentials(backup.Credentials{Password: strings.TrimSpace(input.Token), HealthToken: strings.TrimSpace(input.StatusToken)}, []byte(key))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	peer := store.ReplicationPeer{ID: newID("peer"), Name: strings.TrimSpace(input.Name), URL: peerURL}
	if err = s.store.SaveReplicationPeer(peer, encrypted); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	s.recordRequestAudit(r, actor, "replication.peer.save", peer.ID, map[string]any{"url": peerURL})
	writeJSON(w, http.StatusCreated, peer)
}
func (s *apiServer) runReplication(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	key := s.recoveryKeyString()
	peer, encrypted, err := s.store.ReplicationPeerCredentials(id)
	if err != nil || key == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "replication peer or recovery key not found"})
		return
	}
	credentials, err := backup.DecryptCredentials(encrypted, []byte(key))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "stored replication credential cannot be decrypted"})
		return
	}
	source := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "latest.mrb")
	file, err := os.Open(source)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "create a verified recovery bundle before replication"})
		return
	}
	defer file.Close()
	info, _ := file.Stat()
	if info == nil || info.Size() <= 0 || info.Size() > replicationMaxBundleBytes {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "recovery bundle is missing, empty, or exceeds the replication size limit"})
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPut, peer.URL+"/api/v1/replication/receive", io.LimitReader(file, replicationMaxBundleBytes))
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	request.Header.Set("Authorization", "Bearer "+credentials.Password)
	request.Header.Set("Content-Type", "application/octet-stream")
	request.Header.Set("X-LumoNAS-Source", "local")
	request.ContentLength = info.Size()
	response, err := (&http.Client{Timeout: 10 * time.Minute}).Do(request)
	if err != nil || response.StatusCode/100 != 2 {
		detail := "peer request failed"
		if err != nil {
			detail = err.Error()
		} else {
			detail = response.Status
			response.Body.Close()
		}
		_ = s.store.RecordReplicationResult(id, detail)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "replication failed: " + detail})
		return
	}
	response.Body.Close()
	_ = s.store.RecordReplicationResult(id, "")
	s.recordRequestAudit(r, actor, "replication.run", id, map[string]any{"bytes": info.Size()})
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "peerId": id, "bytes": info.Size()})
}
func (s *apiServer) deleteReplicationPeer(w http.ResponseWriter, r *http.Request, id string) {
	actor, ok := s.identityActor(w, r, true)
	if !ok {
		return
	}
	tasks, err := s.store.SnapshotReplicationTasks(id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "replication tasks could not be checked"})
		return
	}
	if len(tasks) > 0 {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "remove snapshot replication tasks before removing this peer"})
		return
	}
	if err := s.store.DeleteReplicationPeer(id); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "replication peer not found"})
		return
	}
	s.recordRequestAudit(r, actor, "replication.peer.delete", id, nil)
	w.WriteHeader(http.StatusNoContent)
}
func (s *apiServer) receiveReplication(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.identityActor(w, r, true); !ok {
		return
	}
	root := filepath.Join(envOr("LUMONAS_RECOVERY_DIR", "/var/lib/lumonas/recovery"), "replicas")
	if err := os.MkdirAll(root, 0o700); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	temporary, err := os.CreateTemp(root, ".incoming-*.mrb")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer os.Remove(temporary.Name())
	written, err := io.Copy(temporary, http.MaxBytesReader(w, r.Body, replicationMaxBundleBytes))
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil || written == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid or oversized replication bundle"})
		return
	}
	target := filepath.Join(root, time.Now().UTC().Format("20060102T150405Z")+".mrb")
	if err = os.Rename(temporary.Name(), target); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"stored": path.Base(target), "bytes": written})
}
