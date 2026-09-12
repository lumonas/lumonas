package store

import (
	"fmt"
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
)

func TestPruneJobsKeepsActiveAndNewestTerminalHistory(t *testing.T) {
	database, err := Open(t.TempDir() + "/lumonas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for index := 0; index < 125; index++ {
		job := model.Job{ID: fmt.Sprintf("completed-%03d", index), Type: "test", Title: "completed", State: "completed", CreatedAt: time.Now().UTC().Add(time.Duration(index) * time.Second)}
		if _, err := database.db.Exec(`INSERT INTO jobs(id,type,title,state,created_at) VALUES(?,?,?,?,?)`, job.ID, job.Type, job.Title, job.State, job.CreatedAt.Format(timeFormat)); err != nil {
			t.Fatal(err)
		}
	}
	active := model.Job{ID: "running-job", Type: "test", Title: "active", State: "running", CreatedAt: time.Now().UTC().Add(time.Hour)}
	if _, err := database.db.Exec(`INSERT INTO jobs(id,type,title,state,created_at) VALUES(?,?,?,?,?)`, active.ID, active.Type, active.Title, active.State, active.CreatedAt.Format(timeFormat)); err != nil {
		t.Fatal(err)
	}
	if err := database.PruneJobs(100); err != nil {
		t.Fatal(err)
	}
	var terminal, activeCount int
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE state IN ('completed','failed','canceled')`).Scan(&terminal); err != nil {
		t.Fatal(err)
	}
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE state='running'`).Scan(&activeCount); err != nil {
		t.Fatal(err)
	}
	if terminal != 100 || activeCount != 1 {
		t.Fatalf("retention removed the wrong jobs: terminal=%d active=%d", terminal, activeCount)
	}
	var newest string
	if err := database.db.QueryRow(`SELECT id FROM jobs WHERE state='completed' ORDER BY created_at DESC LIMIT 1`).Scan(&newest); err != nil {
		t.Fatal(err)
	}
	if newest != "completed-124" {
		t.Fatalf("newest terminal job was not retained: %s", newest)
	}
}
