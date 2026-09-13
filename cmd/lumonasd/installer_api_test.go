package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/privileged"
)

func TestInstallerEndpointsAreUnavailableOutsideInstallerMode(t *testing.T) {
	server := testServer(t)
	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/install/targets", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected installer endpoint to be hidden, got %d: %s", response.Code, response.Body.String())
	}
}

func TestInstallerPlanRequiresExactHashAtApply(t *testing.T) {
	t.Setenv("LUMONAS_INSTALLER_MODE", "true")
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{
			{ID: "wwn:system", Name: "sda", Role: "system", SizeBytes: 32 << 30, Filesystem: "ext4", Mounted: true, Health: model.Healthy, WWN: "wwn:system", Serial: "SYS1"},
			{ID: "wwn:target", Name: "sdb", Role: "unknown", SizeBytes: 16 << 30, Health: model.Healthy, WWN: "wwn:target", Serial: "DATA1", CurrentPath: "/dev/vdb"},
		}, nil
	}
	var brokerRequests []privileged.Request
	server.brokerExec = func(_ context.Context, request privileged.Request) error {
		brokerRequests = append(brokerRequests, request)
		return nil
	}

	planResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(planResponse, httptest.NewRequest(http.MethodPost, "/api/v1/install/plan", strings.NewReader("{\"targetDiskId\":\"wwn:target\",\"hostname\":\"lumonas\",\"adminUsername\":\"admin\",\"filesystem\":\"ext4\",\"uefi\":true}")))
	if planResponse.Code != http.StatusCreated {
		t.Fatalf("expected plan creation, got %d: %s", planResponse.Code, planResponse.Body.String())
	}
	var planned struct {
		Hash string
	}
	if err := json.NewDecoder(planResponse.Body).Decode(&planned); err != nil {
		t.Fatal(err)
	}
	if planned.Hash == "" {
		t.Fatal("installer plan did not return a hash")
	}

	applyResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(applyResponse, httptest.NewRequest(http.MethodPost, "/api/v1/install/apply", strings.NewReader("{\"hash\":\"wrong\",\"confirm\":true,\"adminPassword\":\"a-very-strong-test-password\"}")))
	if applyResponse.Code != http.StatusUnprocessableEntity || len(brokerRequests) != 0 {
		t.Fatalf("hash mismatch was not rejected before broker call: status=%d body=%s requests=%d", applyResponse.Code, applyResponse.Body.String(), len(brokerRequests))
	}
}

func TestInstallerApplyIsSingleFlight(t *testing.T) {
	t.Setenv("LUMONAS_INSTALLER_MODE", "true")
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{
			{ID: "wwn:target", Name: "sdb", Role: "unknown", SizeBytes: 16 << 30, Health: model.Healthy, WWN: "wwn:target", Serial: "DATA1", CurrentPath: "/dev/vdb"},
		}, nil
	}
	planResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(planResponse, httptest.NewRequest(http.MethodPost, "/api/v1/install/plan", strings.NewReader("{\"targetDiskId\":\"wwn:target\",\"hostname\":\"lumonas\",\"adminUsername\":\"admin\",\"filesystem\":\"ext4\",\"uefi\":true}")))
	if planResponse.Code != http.StatusCreated {
		t.Fatalf("expected plan creation, got %d: %s", planResponse.Code, planResponse.Body.String())
	}
	var planned struct{ Hash string }
	if err := json.NewDecoder(planResponse.Body).Decode(&planned); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	server.brokerExec = func(ctx context.Context, _ privileged.Request) error {
		close(started)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	applyBody := `{"hash":"` + planned.Hash + `","confirm":true,"adminPassword":"a-very-strong-test-password"}`
	firstDone := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response := httptest.NewRecorder()
		server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/install/apply", strings.NewReader(applyBody)))
		firstDone <- response
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first installer apply did not reach the privileged broker")
	}

	second := httptest.NewRecorder()
	server.routes().ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/v1/install/apply", strings.NewReader(applyBody)))
	if second.Code != http.StatusConflict {
		t.Fatalf("expected concurrent installer apply to be rejected, got %d: %s", second.Code, second.Body.String())
	}
	planWhileApplying := httptest.NewRecorder()
	server.routes().ServeHTTP(planWhileApplying, httptest.NewRequest(http.MethodPost, "/api/v1/install/plan", strings.NewReader("{\"targetDiskId\":\"wwn:target\",\"hostname\":\"lumonas\",\"adminUsername\":\"admin\",\"filesystem\":\"ext4\",\"uefi\":true}")))
	if planWhileApplying.Code != http.StatusConflict {
		t.Fatalf("expected plan replacement during installation to be rejected, got %d: %s", planWhileApplying.Code, planWhileApplying.Body.String())
	}
	close(release)
	select {
	case first := <-firstDone:
		if first.Code != http.StatusOK {
			t.Fatalf("expected first installer apply to succeed, got %d: %s", first.Code, first.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("first installer apply did not finish")
	}
}

func TestInstallerApplyRejectsPasswordLineBreakBeforeBroker(t *testing.T) {
	t.Setenv("LUMONAS_INSTALLER_MODE", "true")
	server := testServer(t)
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{{ID: "wwn:target", Name: "sdb", SizeBytes: 16 << 30, WWN: "wwn:target", Serial: "DATA1"}}, nil
	}
	planResponse := httptest.NewRecorder()
	server.routes().ServeHTTP(planResponse, httptest.NewRequest(http.MethodPost, "/api/v1/install/plan", strings.NewReader("{\"targetDiskId\":\"wwn:target\",\"hostname\":\"lumonas\",\"adminUsername\":\"admin\",\"filesystem\":\"ext4\",\"uefi\":true}")))
	var planned struct{ Hash string }
	if err := json.NewDecoder(planResponse.Body).Decode(&planned); err != nil {
		t.Fatal(err)
	}
	brokerCalled := false
	server.brokerExec = func(context.Context, privileged.Request) error {
		brokerCalled = true
		return nil
	}
	response := httptest.NewRecorder()
	body := `{"hash":"` + planned.Hash + `","confirm":true,"adminPassword":"a-very-strong\npassword"}`
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/install/apply", strings.NewReader(body)))
	if response.Code != http.StatusUnprocessableEntity || brokerCalled {
		t.Fatalf("line-break password was not rejected before broker call: status=%d body=%s broker=%t", response.Code, response.Body.String(), brokerCalled)
	}
}
