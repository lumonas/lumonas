package store

import (
	"testing"
	"time"

	"github.com/lumonas/lumonas/internal/model"
	"github.com/lumonas/lumonas/internal/storage"
)

func TestPoolPlanRoundTrip(t *testing.T) {
	db, err := Open(t.TempDir() + "/mynas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	disk := model.Disk{ID: "wwn:a", CurrentPath: "/dev/sda", SizeBytes: 100, Filesystem: "xfs", Health: model.Healthy}
	plan, err := storage.NewPoolPlan("pool-1", "media", "/srv/pools/media", []model.Disk{disk}, 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SavePoolPlan(plan); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.PoolPlan(plan.OperationID)
	if err != nil || loaded.PlanHash != plan.PlanHash || loaded.Name != "media" {
		t.Fatalf("unexpected pool plan %#v err=%v", loaded, err)
	}
}

func TestPoolUnmountPlanRoundTrip(t *testing.T) {
	db, err := Open(t.TempDir() + "/mynas.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	plan, err := storage.NewPoolUnmountPlan("unmount-1", model.Pool{ID: "pool-1", Name: "media", MountPath: "/srv/pools/media"}, 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SavePoolUnmountPlan(plan); err != nil {
		t.Fatal(err)
	}
	loaded, err := db.PoolUnmountPlan(plan.OperationID)
	if err != nil || loaded.PlanHash != plan.PlanHash || loaded.MountPath != plan.MountPath {
		t.Fatalf("unexpected unmount plan %#v err=%v", loaded, err)
	}
}
