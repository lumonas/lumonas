package store

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestJobCorrelationIDPersistsAcrossStoreReopen(t *testing.T) {
	path := t.TempDir() + "/lumonas.db"
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	job := model.Job{ID: "job-correlation", CorrelationID: "corr-request", Type: "test", Title: "test", State: "queued", CreatedAt: time.Now().UTC()}
	if err := database.SaveJob(job); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Job(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CorrelationID != job.CorrelationID {
		t.Fatalf("correlation id was not persisted: got %q want %q", got.CorrelationID, job.CorrelationID)
	}
}

func TestJobWithoutCorrelationIDFallsBackToJobID(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	job := model.Job{ID: "job-fallback", Type: "test", Title: "test", State: "queued", CreatedAt: time.Now().UTC()}
	if err := database.SaveJob(job); err != nil {
		t.Fatal(err)
	}
	got, err := database.Job(job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.CorrelationID != job.ID {
		t.Fatalf("expected job id fallback, got %q", got.CorrelationID)
	}
}
