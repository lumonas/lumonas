package store

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestRestoreDrillPersistsShareAndServiceRehearsalResults(t *testing.T) {
	database, err := Open(t.TempDir() + "/restore-drill.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	value := model.RestoreDrill{ID: "drill-1", Trigger: "manual", State: "successful", StartedAt: time.Now().UTC(), SharesRestored: []string{"share-media"}, ServicesRehearsed: []string{"photos", "database"}, ServicesHealthy: true}
	if err := database.SaveRestoreDrill(value); err != nil {
		t.Fatal(err)
	}
	values, err := database.RestoreDrills(10)
	if err != nil || len(values) != 1 || !values[0].ServicesHealthy || len(values[0].SharesRestored) != 1 || len(values[0].ServicesRehearsed) != 2 {
		t.Fatalf("restore drill round trip = %#v err=%v", values, err)
	}
}
