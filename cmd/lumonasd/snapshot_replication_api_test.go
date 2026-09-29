package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/backup"
	"github.com/lumonas/lumonas/internal/identity"
	"github.com/lumonas/lumonas/internal/privileged"
	"github.com/lumonas/lumonas/internal/shares"
	"github.com/lumonas/lumonas/internal/storage"
	"github.com/lumonas/lumonas/internal/store"
)

func TestSnapshotReplicationTaskCreateRunAndPersist(t *testing.T) {
	server := testServer(t)
	t.Setenv("LUMONAS_RECOVERY_KEY", "test-recovery-key")
	stage := t.TempDir()
	t.Setenv("LUMONAS_REPLICATION_DIR", stage)
	source := shares.ManagedShare{ID: "share-source", Name: "Source", Path: "/srv/pools/source", Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}}
	if _, err := server.store.CreateManagedShare(source); err != nil {
		t.Fatal(err)
	}
	if err := server.store.SaveReplicationPeer(store.ReplicationPeer{ID: "peer-remote", Name: "Remote", URL: "https://remote.test"}, []byte("unused-peer-token")); err != nil {
		t.Fatal(err)
	}
	input := `{"peerId":"peer-remote","name":"Source replica","sourceShareId":"share-source","destinationShareId":"share-destination","receiveToken":"receive-token","scheduleKind":"daily","timeOfDay":"02:10"}`
	created := httptest.NewRecorder()
	server.routes().ServeHTTP(created, httptest.NewRequest(http.MethodPost, "/api/v1/replication/snapshot-tasks", strings.NewReader(input)))
	if created.Code != http.StatusCreated {
		t.Fatalf("task create failed: %d %s", created.Code, created.Body.String())
	}
	var task store.SnapshotReplicationTask
	if err := json.NewDecoder(created.Body).Decode(&task); err != nil {
		t.Fatal(err)
	}
	if task.PeerID != "peer-remote" || task.ScheduleKind != "daily" || task.TimeOfDay != "02:10" {
		t.Fatalf("task fields were not persisted: %#v", task)
	}
	stream := []byte("btrfs snapshot stream fixture")
	digest := sha256.Sum256(stream)
	var exported bool
	server.brokerExecWithResponse = func(_ context.Context, request privileged.Request) (privileged.Response, error) {
		switch request.Operation {
		case "snapshot.create":
			return privileged.Response{OK: true, Data: storage.Snapshot{Kind: storage.SnapshotBtrfs, Source: source.Path, Name: "replica-20260929-020000", CreatedAt: time.Now().UTC(), Readonly: true}}, nil
		case "snapshot.export":
			exported = true
			if err := os.WriteFile(filepath.Join(stage, "out-"+request.OperationID+".stream"), stream, 0o640); err != nil {
				return privileged.Response{}, err
			}
			return privileged.Response{OK: true, Data: map[string]any{"streamId": request.OperationID, "bytes": int64(len(stream)), "sha256": hex.EncodeToString(digest[:])}}, nil
		case "snapshot.stream.remove":
			return privileged.Response{OK: true}, nil
		case "snapshot.export.cancel", "snapshot.receive.cancel":
			return privileged.Response{OK: true}, nil
		default:
			return privileged.Response{}, io.ErrUnexpectedEOF
		}
	}
	oldClient := snapshotReplicationHTTPClient
	snapshotReplicationHTTPClient = &http.Client{Timeout: time.Minute, Transport: snapshotReplicationTestTransport{t: t, wantPath: "/api/v1/replication/snapshots/share-destination/receive", wantToken: "receive-token", wantName: "replica-20260929-020000", wantDigest: hex.EncodeToString(digest[:]), wantBody: stream}}
	t.Cleanup(func() { snapshotReplicationHTTPClient = oldClient })
	run := httptest.NewRecorder()
	server.routes().ServeHTTP(run, httptest.NewRequest(http.MethodPost, "/api/v1/replication/snapshot-tasks/"+task.ID+"/run", nil))
	if run.Code != http.StatusAccepted {
		t.Fatalf("replication run failed: %d %s", run.Code, run.Body.String())
	}
	deadline := time.Now().Add(3 * time.Second)
	var completedRuns []store.SnapshotReplicationRun
	for {
		runs, historyErr := server.store.SnapshotReplicationRuns(task.ID, 10)
		if historyErr != nil {
			t.Fatal(historyErr)
		}
		if len(runs) > 0 && runs[0].State != "running" {
			if runs[0].State != "successful" {
				t.Fatalf("run did not succeed: %#v", runs[0])
			}
			completedRuns = runs
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("snapshot replication run did not complete: %#v", runs)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !exported {
		t.Fatal("snapshot export was not requested")
	}
	saved, _, err := server.store.SnapshotReplicationTask(task.ID)
	if err != nil || saved.LastSnapshotName != "replica-20260929-020000" || saved.LastSyncAt == nil || saved.LastError != "" {
		t.Fatalf("successful snapshot transfer was not recorded: %#v %v", saved, err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/replication/snapshot-tasks/"+task.ID+"/runs", nil)
	history := httptest.NewRecorder()
	server.routes().ServeHTTP(history, request)
	if history.Code != http.StatusOK || !strings.Contains(history.Body.String(), "successful") || len(completedRuns) == 0 || !strings.Contains(history.Body.String(), "replica-20260929-020000") {
		t.Fatalf("run history is incomplete: %d %s", history.Code, history.Body.String())
	}
}

type snapshotReplicationTestTransport struct {
	t                                         *testing.T
	wantPath, wantToken, wantName, wantDigest string
	wantBody                                  []byte
}

func (transport snapshotReplicationTestTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	transport.t.Helper()
	if request.Method != http.MethodPut || request.URL.Scheme != "https" || request.URL.Path != transport.wantPath || request.Header.Get("Authorization") != "Bearer "+transport.wantToken || request.Header.Get("X-LumoNAS-Snapshot-Name") != transport.wantName || request.Header.Get("X-LumoNAS-Snapshot-SHA256") != transport.wantDigest {
		transport.t.Errorf("unexpected remote transfer request: %s %s headers=%v", request.Method, request.URL, request.Header)
		return nil, io.ErrUnexpectedEOF
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	if string(body) != string(transport.wantBody) || request.ContentLength != int64(len(transport.wantBody)) {
		transport.t.Errorf("stream contents did not match: bytes=%d length=%d", len(body), request.ContentLength)
		return nil, io.ErrUnexpectedEOF
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"ok":true}`)), Request: request}, nil
}

func TestSnapshotReplicationReceiverAcceptsOnlyBoundShareAndChecksDigest(t *testing.T) {
	server := testServer(t)
	server.authRequired = true
	server.dynamicAuth = false
	owner, err := server.store.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "replication-owner", Password: "owner-password", ManagementRole: identity.RoleOwner})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, err := server.store.CreateManagedShare(shares.ManagedShare{ID: "share-destination", Name: "Destination", Path: root, Enabled: true, Protocols: []shares.Protocol{{Name: "smb"}}}); err != nil {
		t.Fatal(err)
	}
	_, token, err := server.store.CreateAPIToken(store.APITokenCreate{ID: "snapshot-receiver", OwnerID: owner.ID, Name: "receive snapshots", Scopes: []string{"replication:snapshot:receive:share-destination"}})
	if err != nil {
		t.Fatal(err)
	}
	incoming := filepath.Join(t.TempDir(), "incoming")
	if err := os.Mkdir(incoming, 0o700); err != nil {
		t.Fatal(err)
	}
	canonicalIncoming, err := filepath.EvalSymlinks(incoming)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("LUMONAS_REPLICATION_INCOMING_DIR", canonicalIncoming)
	var receives int
	server.brokerExecWithResponse = func(_ context.Context, request privileged.Request) (privileged.Response, error) {
		if request.Operation != "snapshot.receive" {
			t.Errorf("unexpected privileged op %q", request.Operation)
			return privileged.Response{}, io.ErrUnexpectedEOF
		}
		receives++
		return privileged.Response{OK: true}, nil
	}
	body := []byte("incoming Btrfs stream")
	sum := sha256.Sum256(body)
	makeRequest := func(shareID, checksum string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/replication/snapshots/"+shareID+"/receive", strings.NewReader(string(body)))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-LumoNAS-Snapshot-Name", "replica-20260929-020000")
		req.Header.Set("X-LumoNAS-Snapshot-SHA256", checksum)
		req.Header.Set("X-LumoNAS-Snapshot-Stream", "stream-1")
		req.Header.Set("X-LumoNAS-Snapshot-Operation", "receive-operation-1")
		resp := httptest.NewRecorder()
		server.routes().ServeHTTP(resp, req)
		return resp
	}
	if response := makeRequest("share-other", hex.EncodeToString(sum[:])); response.Code != http.StatusForbidden {
		t.Fatalf("token wrote outside its share: %d %s", response.Code, response.Body.String())
	}
	if response := makeRequest("share-destination", strings.Repeat("0", 64)); response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid checksum was accepted: %d %s", response.Code, response.Body.String())
	}
	if receives != 0 {
		t.Fatal("invalid transfer reached privileged receiver")
	}
	if response := makeRequest("share-destination", hex.EncodeToString(sum[:])); response.Code != http.StatusOK {
		t.Fatalf("valid receive failed: %d %s", response.Code, response.Body.String())
	}
	if receives != 1 {
		t.Fatalf("expected one privileged receive, got %d", receives)
	}
}

func TestSnapshotReplicationScheduleValidation(t *testing.T) {
	if !validWeekday("Monday") || validWeekday("mon") || validWeekday("funday") {
		t.Fatal("weekday validation accepted an invalid schedule")
	}
	if !snapshotReplicationIDPattern.MatchString("share_7-archive") || snapshotReplicationIDPattern.MatchString("../outside") {
		t.Fatal("share identifier validation allowed path traversal")
	}
	if _, err := backup.EncryptCredentials(backup.Credentials{Password: "receive"}, []byte("test-key")); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotReplicationReceiveCancellationIsShareBound(t *testing.T) {
	server := testServer(t)
	server.authRequired = true
	server.dynamicAuth = false
	owner, err := server.store.CreatePrincipal(identity.CreateInput{Kind: identity.KindUser, Name: "cancel-owner", Password: "owner-password", ManagementRole: identity.RoleOwner})
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := server.store.CreateAPIToken(store.APITokenCreate{ID: "receive-cancel-token", OwnerID: owner.ID, Name: "receive", Scopes: []string{"replication:snapshot:receive:share-target"}})
	if err != nil {
		t.Fatal(err)
	}
	server.snapshotReceiveOwners = map[string]string{"receive-op-7": "share-target"}
	var operation string
	server.brokerExecWithResponse = func(_ context.Context, request privileged.Request) (privileged.Response, error) {
		operation = request.Operation + ":" + request.OperationID
		return privileged.Response{OK: true}, nil
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/replication/snapshots/share-target/cancel", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-LumoNAS-Snapshot-Operation", "receive-op-7")
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || operation != "snapshot.receive.cancel:receive-op-7" {
		t.Fatalf("scoped receive cancellation failed: %d %s op=%q", response.Code, response.Body.String(), operation)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/replication/snapshots/share-target/cancel", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-LumoNAS-Snapshot-Operation", "receive-op-other")
	response = httptest.NewRecorder()
	server.routes().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("cancelled a receive operation not owned by this share: %d %s", response.Code, response.Body.String())
	}
}
