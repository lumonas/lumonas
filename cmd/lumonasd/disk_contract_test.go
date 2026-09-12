package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestDiskAPIExposesStableIdentityContract(t *testing.T) {
	server := testServer(t)
	usedBytes := uint64(1234)
	temperature := 37.5
	server.diskFunc = func() ([]model.Disk, error) {
		return []model.Disk{{
			ID:             "wwn:test",
			Name:           "sda",
			CurrentPath:    "/dev/sda",
			Model:          "Lumo Test Disk",
			Serial:         "SERIAL-1",
			WWN:            "5000cca-test",
			GPTDiskGUID:    "disk-guid",
			PartitionUUID:  "partition-uuid",
			FilesystemUUID: "filesystem-uuid",
			SizeBytes:      10_000,
			UsedBytes:      &usedBytes,
			Role:           "data",
			Rotational:     true,
			Interface:      "sata",
			Health:         model.Healthy,
			Temperature:    &temperature,
			Filesystem:     "ext4",
			Mounted:        true,
			PoolID:         "media",
			LastSeen:       time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC),
		}}, nil
	}

	response := httptest.NewRecorder()
	server.routes().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/disks", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	var disks []map[string]json.RawMessage
	if err := json.NewDecoder(response.Body).Decode(&disks); err != nil {
		t.Fatal(err)
	}
	if len(disks) != 1 {
		t.Fatalf("expected one disk, got %#v", disks)
	}
	for _, field := range []string{"id", "currentPath", "wwn", "gptDiskGuid", "partitionUuid", "filesystemUuid", "mounted", "lastSeen", "smart"} {
		if _, ok := disks[0][field]; !ok {
			t.Errorf("disk response omitted stable contract field %q", field)
		}
	}
}
